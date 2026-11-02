package webui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/gotd/td/telegram/auth/qrlogin"
	"go.uber.org/zap"

	"github.com/midi-HL/telegram-mcp/internal/config"
	tgclient "github.com/midi-HL/telegram-mcp/internal/telegram"
)

// LoginState 是登录会话的状态。
type LoginState string

const (
	// StateWaitingQR 等待扫码。
	StateWaitingQR LoginState = "waiting_qr"
	// StateWaitingPhone 等待输入手机号。
	StateWaitingPhone LoginState = "waiting_phone"
	// StateWaitingCode 等待输入验证码。
	StateWaitingCode LoginState = "waiting_code"
	// StateWaitingPassword 等待输入两步验证密码。
	StateWaitingPassword LoginState = "waiting_password"
	// StateSuccess 登录成功。
	StateSuccess LoginState = "success"
	// StateFailed 登录失败。
	StateFailed LoginState = "failed"
	// StateCancelled 已取消。
	StateCancelled LoginState = "cancelled"
)

// LoginSession 是一次 Web 端登录流程。
type LoginSession struct {
	ID string

	mu        sync.RWMutex
	state     LoginState
	qrURL     string
	qrExpires time.Time
	phone     string
	hint      string // 验证码发送渠道提示
	errMsg    string
	account   string
	userID    int64
	username  string

	// 交互通道：Web 请求把用户输入投进来，登录 goroutine 读取
	codeCh     chan string
	passwordCh chan string
	phoneCh    chan string

	cancel context.CancelFunc
	done   chan struct{}
}

// Snapshot 是返回给前端的只读快照。
type Snapshot struct {
	ID        string     `json:"id"`
	State     LoginState `json:"state"`
	QRURL     string     `json:"qr_url,omitempty"`
	QRExpires int64      `json:"qr_expires,omitempty"`
	Phone     string     `json:"phone,omitempty"`
	Hint      string     `json:"hint,omitempty"`
	Error     string     `json:"error,omitempty"`
	Account   string     `json:"account,omitempty"`
	UserID    int64      `json:"user_id,omitempty"`
	Username  string     `json:"username,omitempty"`
}

// Snap 返回当前状态快照。
func (s *LoginSession) Snap() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap := Snapshot{
		ID:       s.ID,
		State:    s.state,
		QRURL:    s.qrURL,
		Phone:    s.phone,
		Hint:     s.hint,
		Error:    s.errMsg,
		Account:  s.account,
		UserID:   s.userID,
		Username: s.username,
	}
	if !s.qrExpires.IsZero() {
		snap.QRExpires = s.qrExpires.Unix()
	}
	return snap
}

// setState 更新状态。
func (s *LoginSession) setState(st LoginState) {
	s.mu.Lock()
	s.state = st
	s.mu.Unlock()
}

// setQR 更新二维码。
func (s *LoginSession) setQR(url string, expires time.Time) {
	s.mu.Lock()
	s.qrURL = url
	s.qrExpires = expires
	s.mu.Unlock()
}

// setError 记录错误。
func (s *LoginSession) setError(msg string) {
	s.mu.Lock()
	s.state = StateFailed
	s.errMsg = msg
	s.mu.Unlock()
}

// LoginManager 管理所有 Web 端登录会话。
type LoginManager struct {
	cfg    *config.Config
	pool   *tgclient.Pool
	logger *zap.Logger

	mu       sync.RWMutex
	sessions map[string]*LoginSession
}

// NewLoginManager 创建登录管理器。
func NewLoginManager(cfg *config.Config, pool *tgclient.Pool, logger *zap.Logger) *LoginManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	m := &LoginManager{
		cfg:      cfg,
		pool:     pool,
		logger:   logger,
		sessions: make(map[string]*LoginSession),
	}
	// 定期清理已完成的会话
	go m.cleanupLoop()
	return m
}

// cleanupLoop 每 5 分钟清理超过 30 分钟的会话。
func (m *LoginManager) cleanupLoop() {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for range t.C {
		cutoff := time.Now().Add(-30 * time.Minute)
		m.mu.Lock()
		for id, s := range m.sessions {
			s.mu.RLock()
			finished := s.state == StateSuccess || s.state == StateFailed || s.state == StateCancelled
			s.mu.RUnlock()
			if finished {
				select {
				case <-s.done:
					delete(m.sessions, id)
				default:
				}
			}
		}
		m.mu.Unlock()
		_ = cutoff
	}
}

// Get 按 ID 取会话。
func (m *LoginManager) Get(id string) (*LoginSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

// newSession 创建会话（内部用）。
func (m *LoginManager) newSession(account string) *LoginSession {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	s := &LoginSession{
		ID:         hex.EncodeToString(buf),
		state:      StateWaitingPhone,
		account:    account,
		codeCh:     make(chan string, 1),
		passwordCh: make(chan string, 1),
		phoneCh:    make(chan string, 1),
		done:       make(chan struct{}),
	}
	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()
	return s
}

// StartQR 启动扫码登录。
func (m *LoginManager) StartQR(ctx context.Context, account string, apiID int, apiHash string) (*LoginSession, error) {
	if apiID <= 0 || apiHash == "" {
		return nil, fmt.Errorf("需要先配置 api_id 和 api_hash")
	}
	if account == "" {
		account = "main"
	}
	if acc, ok := m.pool.Get(account); ok && acc != nil && acc.Self() != nil {
		return nil, fmt.Errorf("账号 %q 已登录，请先删除或换一个名字", account)
	}

	// 写入配置
	m.cfg.UpsertAccount(config.Account{
		Name:    account,
		APIID:   apiID,
		APIHash: apiHash,
	})
	if err := m.cfg.Save(); err != nil {
		return nil, fmt.Errorf("保存配置: %w", err)
	}
	saved := m.cfg.Account(account)
	if saved == nil {
		return nil, fmt.Errorf("配置保存后无法读回")
	}

	acc, err := tgclient.NewAccount(*saved)
	if err != nil {
		return nil, err
	}

	sess := m.newSession(account)
	sess.setState(StateWaitingQR)
	runCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	sess.cancel = cancel

	go m.runQR(runCtx, sess, acc)
	return sess, nil
}

// runQR 执行扫码登录流程。
func (m *LoginManager) runQR(ctx context.Context, sess *LoginSession, acc *tgclient.Account) {
	defer close(sess.done)
	defer sess.cancel()

	waiter := tgclient.NewFloodWaiter(m.logger)
	err := waiter.Run(ctx, func(ctx context.Context) error {
		return acc.Client().Run(ctx, func(ctx context.Context) error {
			dispatcher := acc.Dispatcher()
			loggedIn := qrlogin.OnLoginToken(dispatcher)

			_, authErr := acc.Client().QR().Auth(ctx, loggedIn, func(ctx context.Context, token qrlogin.Token) error {
				exp := time.Now().Add(30 * time.Second)
				sess.setQR(token.URL(), exp)
				m.logger.Info("生成二维码", zap.String("account", sess.account))
				return nil
			})
			if authErr != nil {
				sess.setError(fmt.Sprintf("扫码登录失败: %v", authErr))
				return nil
			}

			// 登录成功，取账号信息
			st, err := tgclient.CheckStatus(ctx, acc)
			if err != nil || !st.Authorized {
				sess.setError("登录后状态异常")
				return nil
			}

			// 注册到池
			m.pool.Add(acc)
			// 后台保持连接
			go m.keepAlive(acc)

			sess.mu.Lock()
			sess.state = StateSuccess
			sess.userID = st.UserID
			sess.username = st.Username
			sess.qrURL = ""
			sess.mu.Unlock()
			m.logger.Info("扫码登录成功",
				zap.String("account", sess.account),
				zap.Int64("user_id", st.UserID))
			return nil
		})
	})
	if err != nil && sess.Snap().State != StateSuccess {
		sess.setError(fmt.Sprintf("连接失败: %v", err))
	}
}

// keepAlive 保持账号连接（供已登录账号使用）。
func (m *LoginManager) keepAlive(acc *tgclient.Account) {
	waiter := tgclient.NewFloodWaiter(m.logger)
	_ = waiter.Run(context.Background(), func(ctx context.Context) error {
		return acc.Client().Run(ctx, func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		})
	})
}

// StartPhone 启动手机号登录。
func (m *LoginManager) StartPhone(ctx context.Context, account string, apiID int, apiHash string, phone string) (*LoginSession, error) {
	if apiID <= 0 || apiHash == "" {
		return nil, fmt.Errorf("需要先配置 api_id 和 api_hash")
	}
	if phone == "" {
		return nil, fmt.Errorf("需要手机号（含国际区号，如 +8613800138000）")
	}
	if account == "" {
		account = "main"
	}

	m.cfg.UpsertAccount(config.Account{
		Name:    account,
		Phone:   phone,
		APIID:   apiID,
		APIHash: apiHash,
	})
	if err := m.cfg.Save(); err != nil {
		return nil, fmt.Errorf("保存配置: %w", err)
	}
	saved := m.cfg.Account(account)
	if saved == nil {
		return nil, fmt.Errorf("配置保存后无法读回")
	}

	acc, err := tgclient.NewAccount(*saved)
	if err != nil {
		return nil, err
	}

	sess := m.newSession(account)
	sess.mu.Lock()
	sess.phone = phone
	sess.state = StateWaitingCode
	sess.mu.Unlock()
	runCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	sess.cancel = cancel

	go m.runPhone(runCtx, sess, acc, phone)
	return sess, nil
}

// webPrompter 用 channel 接收 Web 端输入。
type webPrompter struct {
	sess *LoginSession
}

// Ask 等待 Web 端提交。
func (p *webPrompter) Ask(ctx context.Context, prompt string) (string, error) {
	// 根据 prompt 内容判断要等哪个通道
	if containsAny(prompt, "验证码", "code") {
		select {
		case v := <-p.sess.codeCh:
			return v, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	// 默认等密码
	select {
	case v := <-p.sess.passwordCh:
		return v, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) > 0 && len(s) >= len(sub) {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

// runPhone 执行手机号登录流程。
func (m *LoginManager) runPhone(ctx context.Context, sess *LoginSession, acc *tgclient.Account, phone string) {
	defer close(sess.done)
	defer sess.cancel()

	waiter := tgclient.NewFloodWaiter(m.logger)
	err := waiter.Run(ctx, func(ctx context.Context) error {
		return acc.Client().Run(ctx, func(ctx context.Context) error {
			opts := tgclient.AuthOptions{
				Mode:     tgclient.AuthModePhone,
				Phone:    phone,
				Prompt:   &webPrompter{sess: sess},
				Password: "", // 通过 Prompt 提供
			}
			if err := tgclient.Authenticate(ctx, acc, opts, m.logger); err != nil {
				sess.setError(fmt.Sprintf("登录失败: %v", err))
				return nil
			}

			st, err := tgclient.CheckStatus(ctx, acc)
			if err != nil || !st.Authorized {
				sess.setError("登录后状态异常")
				return nil
			}

			m.pool.Add(acc)
			go m.keepAlive(acc)

			sess.mu.Lock()
			sess.state = StateSuccess
			sess.userID = st.UserID
			sess.username = st.Username
			sess.mu.Unlock()
			m.logger.Info("手机号登录成功",
				zap.String("account", sess.account),
				zap.Int64("user_id", st.UserID))
			return nil
		})
	})
	if err != nil && sess.Snap().State != StateSuccess {
		sess.setError(fmt.Sprintf("连接失败: %v", err))
	}
}

// SubmitCode 提交验证码。
func (m *LoginManager) SubmitCode(id, code string) error {
	s, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("会话不存在或已过期")
	}
	select {
	case s.codeCh <- code:
		return nil
	case <-time.After(3 * time.Second):
		return fmt.Errorf("登录流程未在等待验证码")
	}
}

// SubmitPassword 提交两步验证密码。
func (m *LoginManager) SubmitPassword(id, password string) error {
	s, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("会话不存在或已过期")
	}
	select {
	case s.passwordCh <- password:
		return nil
	case <-time.After(3 * time.Second):
		return fmt.Errorf("登录流程未在等待密码")
	}
}

// Cancel 取消登录。
func (m *LoginManager) Cancel(id string) error {
	s, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("会话不存在")
	}
	s.setState(StateCancelled)
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

// SetHint 设置验证码发送渠道提示（供 runPhone 内部调用）。
func (s *LoginSession) SetHint(hint string) {
	s.mu.Lock()
	s.hint = hint
	s.mu.Unlock()
}
