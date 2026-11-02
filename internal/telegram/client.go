// Package telegram 封装 gotd/td，提供多账号客户端池。
package telegram

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"go.uber.org/zap"

	"github.com/midi-HL/telegram-mcp/internal/config"
)

// Account 是一个已配置的账号客户端。
type Account struct {
	Name    string
	APIID   int
	APIHash string
	Phone   string

	client      *telegram.Client
	dispatcher  *tg.UpdateDispatcher
	sessionPath string

	mu     sync.RWMutex
	self   *tg.User
	runErr error
	api    *tg.Client
}

// API 返回底层 tg.Client（仅在 client.Run 回调内有效）。
func (a *Account) API() *tg.Client {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.api
}

// SetAPI 缓存 tg.Client 引用。
func (a *Account) SetAPI(api *tg.Client) {
	a.mu.Lock()
	a.api = api
	a.mu.Unlock()
}

// SessionPath 返回该账号 session 文件路径。
func (a *Account) SessionPath() string { return a.sessionPath }

// Client 返回底层 gotd 客户端（仅在 Run 内部有效）。
func (a *Account) Client() *telegram.Client { return a.client }

// Dispatcher 返回更新分发器。
func (a *Account) Dispatcher() *tg.UpdateDispatcher { return a.dispatcher }

// Self 返回已登录账号的自身信息（登录完成后可用）。
func (a *Account) Self() *tg.User {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.self
}

// SetSelf 缓存自身信息。
func (a *Account) SetSelf(u *tg.User) {
	a.mu.Lock()
	a.self = u
	a.mu.Unlock()
}

// RunError 返回 Run 过程中的错误（在 Run 结束后可读）。
func (a *Account) RunError() error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.runErr
}

func (a *Account) setRunError(err error) {
	a.mu.Lock()
	a.runErr = err
	a.mu.Unlock()
}

// Pool 管理多个账号客户端。
type Pool struct {
	mu       sync.RWMutex
	accounts map[string]*Account
	order    []string
	logger   *zap.Logger
}

// NewPool 创建账号池。
func NewPool(logger *zap.Logger) *Pool {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Pool{
		accounts: make(map[string]*Account),
		logger:   logger,
	}
}

// NewAccount 构造一个账号客户端（不启动）。
func NewAccount(cfg config.Account) (*Account, error) {
	if cfg.APIID <= 0 {
		return nil, fmt.Errorf("账号 %q: api_id 无效 (%d)", cfg.Name, cfg.APIID)
	}
	if cfg.APIHash == "" {
		return nil, fmt.Errorf("账号 %q: api_hash 为空", cfg.Name)
	}
	sessionDir := cfg.SessionDir
	if sessionDir == "" {
		return nil, fmt.Errorf("账号 %q: session_dir 为空", cfg.Name)
	}
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		return nil, fmt.Errorf("创建 session 目录 %s: %w", sessionDir, err)
	}
	sessionPath := filepath.Join(sessionDir, "session.json")

	dispatcher := tg.NewUpdateDispatcher()

	opts := telegram.Options{
		SessionStorage: &session.FileStorage{Path: sessionPath},
		UpdateHandler:  &dispatcher,
		Device: telegram.DeviceConfig{
			DeviceModel:    "Telegram MCP",
			SystemVersion:  "1.0",
			AppVersion:     "1.0",
			SystemLangCode: "zh",
			LangCode:       "zh",
		},
		// 显式关闭 NoUpdates，确保更新流工作（自动回复与 wait_message 依赖它）
		NoUpdates: false,
	}
	client := telegram.NewClient(cfg.APIID, cfg.APIHash, opts)

	return &Account{
		Name:        cfg.Name,
		APIID:       cfg.APIID,
		APIHash:     cfg.APIHash,
		Phone:       cfg.Phone,
		client:      client,
		dispatcher:  &dispatcher,
		sessionPath: sessionPath,
	}, nil
}

// Add 注册账号到池中。
func (p *Pool) Add(acc *Account) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.accounts[acc.Name]; !exists {
		p.order = append(p.order, acc.Name)
	}
	p.accounts[acc.Name] = acc
}

// Get 按名称取账号。
func (p *Pool) Get(name string) (*Account, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	acc, ok := p.accounts[name]
	return acc, ok
}

// Names 返回所有账号名（保持添加顺序）。
func (p *Pool) Names() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]string, len(p.order))
	copy(out, p.order)
	return out
}

// Len 返回账号数量。
func (p *Pool) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.accounts)
}

// NewFloodWaiter 创建 FLOOD_WAIT 等待器。
// gotd 不会自动重试通用 RPC（只有 uploader/downloader 内部会），
// 必须用 Waiter.Run 包裹 client.Run。
func NewFloodWaiter(logger *zap.Logger) *floodwait.Waiter {
	w := floodwait.NewWaiter()
	if logger != nil {
		w = w.WithCallback(func(ctx context.Context, wait floodwait.FloodWait) {
			logger.Warn("触发 FLOOD_WAIT，自动等待",
				zap.Duration("wait", wait.Duration))
		})
	}
	return w
}

// APIError 从错误中提取 Telegram RPC 错误码与消息。
func APIError(err error) (code int, msg string, ok bool) {
	if err == nil {
		return 0, "", false
	}
	rpcErr, isRPC := tgerr.As(err)
	if !isRPC {
		return 0, "", false
	}
	return rpcErr.Code, rpcErr.Message, true
}

// IsPrivacyRestricted 判断是否为「对方隐私设置不允许发消息」。
func IsPrivacyRestricted(err error) bool {
	return tgerr.Is(err, "USER_PRIVACY_RESTRICTED")
}

// PasswordHashFor 暴露给上层用于两步验证。
var PasswordHashFor = auth.PasswordHashFor
