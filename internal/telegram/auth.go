package telegram

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"go.uber.org/zap"
)

// AuthMode 是登录方式。
type AuthMode string

const (
	// AuthModeQR 扫码登录。
	AuthModeQR AuthMode = "qr"
	// AuthModePhone 手机号 + 验证码登录。
	AuthModePhone AuthMode = "phone"
)

// AuthOptions 是登录参数。
type AuthOptions struct {
	Mode AuthMode
	// Phone 手机号（含国际区号，如 +8613800138000）。
	Phone string
	// Password 两步验证密码，可为空。
	Password string
	// Prompt 用于交互式输入验证码/密码。
	Prompt Prompter
	// QRWriter 二维码输出目标，默认 stderr。
	QRWriter io.Writer
}

// Prompter 抽象交互式输入，便于测试时替换。
type Prompter interface {
	// Ask 提示用户输入一行文本。
	Ask(ctx context.Context, prompt string) (string, error)
}

// StdinPrompter 从标准输入读取。
type StdinPrompter struct {
	In  io.Reader
	Out io.Writer
}

// Ask 打印提示并读取一行。
func (p *StdinPrompter) Ask(ctx context.Context, prompt string) (string, error) {
	if p.In == nil {
		p.In = os.Stdin
	}
	if p.Out == nil {
		p.Out = os.Stderr
	}
	fmt.Fprint(p.Out, prompt)
	sc := bufio.NewScanner(p.In)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return "", err
		}
		return "", errors.New("输入已关闭")
	}
	return strings.TrimSpace(sc.Text()), nil
}

// StaticPrompter 是预设答案的 Prompter（用于自动化测试）。
type StaticPrompter struct {
	Answers []string
	idx     int
}

// Ask 依次返回预设答案。
func (p *StaticPrompter) Ask(ctx context.Context, prompt string) (string, error) {
	if p.idx >= len(p.Answers) {
		return "", fmt.Errorf("没有更多预设答案（prompt=%q）", prompt)
	}
	a := p.Answers[p.idx]
	p.idx++
	return a, nil
}

// Authenticate 执行登录流程。必须在 client.Run 的上下文内调用。
func Authenticate(ctx context.Context, acc *Account, opts AuthOptions, logger *zap.Logger) error {
	if logger == nil {
		logger = zap.NewNop()
	}
	switch opts.Mode {
	case AuthModeQR:
		return authQR(ctx, acc, opts, logger)
	case AuthModePhone, "":
		return authPhone(ctx, acc, opts, logger)
	default:
		return fmt.Errorf("未知登录方式 %q（支持 qr / phone）", opts.Mode)
	}
}

// authQR 扫码登录。
func authQR(ctx context.Context, acc *Account, opts AuthOptions, logger *zap.Logger) error {
	qrWriter := opts.QRWriter
	if qrWriter == nil {
		qrWriter = os.Stderr
	}

	// 必须先注册 OnLoginToken，否则不会收到 UpdateLoginToken，登录会卡住
	dispatcher := acc.Dispatcher()
	loggedIn := qrlogin.OnLoginToken(dispatcher)

	_, err := acc.Client().QR().Auth(ctx, loggedIn, func(ctx context.Context, token qrlogin.Token) error {
		fmt.Fprintln(qrWriter, "")
		fmt.Fprintln(qrWriter, "请用 Telegram 手机端扫描以下链接生成的二维码：")
		fmt.Fprintln(qrWriter, token.URL())
		fmt.Fprintln(qrWriter, "")
		return nil
	})
	if err != nil {
		// 需要两步验证
		if tgerr.Is(err, "SESSION_PASSWORD_NEEDED") {
			return handlePassword(ctx, acc, opts)
		}
		return fmt.Errorf("扫码登录失败: %w", err)
	}
	logger.Info("扫码登录成功", zap.String("account", acc.Name))
	return nil
}

// userAuthenticator 同时提供手机号、验证码与两步验证密码。
// 相比 auth.CodeOnly，它能在需要时直接给出 2FA 密码，避免
// "password requested but not provided" 错误。
type userAuthenticator struct {
	phone    string
	password string
	code     auth.CodeAuthenticator
	prompt   Prompter
}

func (a userAuthenticator) Phone(ctx context.Context) (string, error) {
	return a.phone, nil
}

func (a userAuthenticator) Password(ctx context.Context) (string, error) {
	if a.password != "" {
		return a.password, nil
	}
	if a.prompt != nil {
		pwd, err := a.prompt.Ask(ctx, "账号启用了两步验证，请输入密码: ")
		if err != nil {
			return "", err
		}
		if pwd == "" {
			return "", auth.ErrPasswordNotProvided
		}
		return pwd, nil
	}
	return "", auth.ErrPasswordNotProvided
}

func (a userAuthenticator) AcceptTermsOfService(ctx context.Context, tos tg.HelpTermsOfService) error {
	return errors.New("该账号需要先接受服务条款，请先用官方客户端登录一次后再试")
}

func (a userAuthenticator) SignUp(ctx context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, errors.New("该手机号尚未注册 Telegram，请先用官方客户端注册")
}

func (a userAuthenticator) Code(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	return a.code.Code(ctx, sentCode)
}

// authPhone 手机号登录。
func authPhone(ctx context.Context, acc *Account, opts AuthOptions, logger *zap.Logger) error {
	phone := opts.Phone
	if phone == "" {
		phone = acc.Phone
	}
	if phone == "" {
		return errors.New("手机号登录需要 --phone 参数")
	}
	if opts.Prompt == nil {
		opts.Prompt = &StdinPrompter{}
	}

	codeAuth := auth.CodeAuthenticatorFunc(func(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
		// 尽量提示验证码发送到了哪里
		hint := describeSentCode(sentCode)
		if hint != "" {
			fmt.Fprintf(os.Stderr, "验证码已发送：%s\n", hint)
		}
		return opts.Prompt.Ask(ctx, "请输入 Telegram 验证码: ")
	})

	flow := auth.NewFlow(
		userAuthenticator{
			phone:    phone,
			password: opts.Password,
			code:     codeAuth,
			prompt:   opts.Prompt,
		},
		auth.SendCodeOptions{},
	)

	if err := flow.Run(ctx, acc.Client().Auth()); err != nil {
		if tgerr.Is(err, "SESSION_PASSWORD_NEEDED") {
			return handlePassword(ctx, acc, opts)
		}
		return fmt.Errorf("手机号登录失败: %w", err)
	}
	logger.Info("手机号登录成功", zap.String("account", acc.Name))
	return nil
}

// handlePassword 处理两步验证。
func handlePassword(ctx context.Context, acc *Account, opts AuthOptions) error {
	pwd := opts.Password
	if pwd == "" {
		if opts.Prompt == nil {
			opts.Prompt = &StdinPrompter{}
		}
		var err error
		pwd, err = opts.Prompt.Ask(ctx, "账号启用了两步验证，请输入密码: ")
		if err != nil {
			return fmt.Errorf("读取两步验证密码: %w", err)
		}
	}
	if _, err := acc.Client().Auth().Password(ctx, pwd); err != nil {
		if tgerr.Is(err, "PASSWORD_HASH_INVALID") {
			return errors.New("两步验证密码错误")
		}
		return fmt.Errorf("两步验证失败: %w", err)
	}
	return nil
}

// describeSentCode 描述验证码的发送渠道，便于用户查找。
func describeSentCode(sentCode *tg.AuthSentCode) string {
	if sentCode == nil {
		return ""
	}
	switch t := sentCode.Type.(type) {
	case *tg.AuthSentCodeTypeApp:
		return fmt.Sprintf("Telegram App 内（其他已登录设备）")
	case *tg.AuthSentCodeTypeSMS:
		return "短信"
	case *tg.AuthSentCodeTypeCall:
		return "电话语音"
	case *tg.AuthSentCodeTypeFlashCall:
		return "闪信来电"
	case *tg.AuthSentCodeTypeMissedCall:
		return "未接来电"
	case *tg.AuthSentCodeTypeEmailCode:
		return "邮箱 " + t.EmailPattern
	case *tg.AuthSentCodeTypeFragmentSMS:
		return "Fragment 短信"
	default:
		return ""
	}
}

// Status 返回账号登录状态。
type Status struct {
	Authorized bool   `json:"authorized"`
	UserID     int64  `json:"user_id,omitempty"`
	Username   string `json:"username,omitempty"`
	FirstName  string `json:"first_name,omitempty"`
	LastName   string `json:"last_name,omitempty"`
	Phone      string `json:"phone,omitempty"`
}

// CheckStatus 查询当前登录状态（必须在 Run 内调用）。
func CheckStatus(ctx context.Context, acc *Account) (*Status, error) {
	st, err := acc.Client().Auth().Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询登录状态: %w", err)
	}
	out := &Status{Authorized: st.Authorized}
	if !st.Authorized {
		return out, nil
	}
	self, err := acc.Client().Self(ctx)
	if err != nil {
		return out, nil // 已授权但取不到自身信息，仍返回已授权
	}
	acc.SetSelf(self)
	out.UserID = self.ID
	out.Username, _ = self.GetUsername()
	out.FirstName = self.FirstName
	out.LastName = self.LastName
	out.Phone, _ = self.GetPhone()
	return out, nil
}
