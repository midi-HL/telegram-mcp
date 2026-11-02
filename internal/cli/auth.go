package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/midi-HL/telegram-mcp/internal/config"
	tgclient "github.com/midi-HL/telegram-mcp/internal/telegram"
)

// filePrompter 从文件读取一行答案。用于外部注入验证码：
// 流程为「后台启动登录 → 验证码写入文件 → 登录进程读取」。
type filePrompter struct {
	path string
	out  io.Writer
}

func (p *filePrompter) Ask(ctx context.Context, prompt string) (string, error) {
	if p.out != nil {
		fmt.Fprint(p.out, prompt)
	}
	// 轮询等待文件出现且非空，最多等 30 分钟
	for i := 0; i < 3600; i++ {
		if data, err := os.ReadFile(p.path); err == nil {
			s := strings.TrimSpace(string(data))
			if s != "" {
				_ = os.Remove(p.path) // 用完即删，避免重复使用
				return s, nil
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timeAfter(500):
		}
	}
	return "", fmt.Errorf("等待验证码超时（30 分钟）")
}

// runAuth 执行登录子命令。
func (a *App) runAuth(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("auth", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	a.commonFlags(fs)

	var (
		accountName = fs.String("account", "main", "账号名")
		useQR       = fs.Bool("qr", false, "使用扫码登录")
		phone       = fs.String("phone", "", "手机号（含国际区号，如 +8613800138000）")
		password    = fs.String("password", "", "两步验证密码（可选）")
		apiID       = fs.Int("api-id", 0, "Telegram api_id")
		apiHash     = fs.String("api-hash", "", "Telegram api_hash")
		codeFile    = fs.String("code-file", "", "从该文件读取验证码（非交互模式，配合后台登录）")
		passwordFile = fs.String("password-file", "", "从该文件读取两步验证密码")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := a.resolveConfig()
	if err != nil {
		return err
	}

	id, hash := *apiID, *apiHash
	if acc := cfg.Account(*accountName); acc != nil {
		if id == 0 {
			id = acc.APIID
		}
		if hash == "" {
			hash = acc.APIHash
		}
	}
	if envID, envHash, ok := config.EnvCredentials(); ok {
		if id == 0 {
			id = envID
		}
		if hash == "" {
			hash = envHash
		}
	}
	if id == 0 || hash == "" {
		return fmt.Errorf("缺少 API 凭据：请用 --api-id/--api-hash 或环境变量 TELEGRAM_API_ID / TELEGRAM_API_HASH")
	}

	accCfg := config.Account{Name: *accountName, Phone: *phone, APIID: id, APIHash: hash}
	cfg.UpsertAccount(accCfg)
	if err := cfg.Save(); err != nil {
		return err
	}
	saved := cfg.Account(*accountName)
	if saved == nil {
		return fmt.Errorf("账号 %q 保存后无法读回", *accountName)
	}

	acc, err := tgclient.NewAccount(*saved)
	if err != nil {
		return err
	}

	mode := tgclient.AuthModePhone
	if *useQR {
		mode = tgclient.AuthModeQR
	}

	// 选择 Prompter
	var prompter tgclient.Prompter
	switch {
	case *codeFile != "":
		prompter = &filePrompter{path: *codeFile, out: a.Stderr}
	default:
		prompter = &tgclient.StdinPrompter{In: os.Stdin, Out: a.Stderr}
	}

	// 两步验证密码：flag > 文件 > 交互
	pwd := *password
	if pwd == "" && *passwordFile != "" {
		if data, err := os.ReadFile(*passwordFile); err == nil {
			pwd = strings.TrimSpace(string(data))
		}
	}

	opts := tgclient.AuthOptions{
		Mode:     mode,
		Phone:    *phone,
		Password: pwd,
		Prompt:   prompter,
		QRWriter: a.Stderr,
	}

	a.Logger.Info("开始登录", zap.String("account", *accountName), zap.String("mode", string(mode)))

	waiter := tgclient.NewFloodWaiter(a.Logger)
	err = waiter.Run(ctx, func(ctx context.Context) error {
		return acc.Client().Run(ctx, func(ctx context.Context) error {
			if err := tgclient.Authenticate(ctx, acc, opts, a.Logger); err != nil {
				return err
			}
			st, err := tgclient.CheckStatus(ctx, acc)
			if err != nil {
				return err
			}
			if !st.Authorized {
				return fmt.Errorf("登录后状态仍为未授权")
			}
			fmt.Fprintf(a.Stdout, "\n登录成功\n")
			fmt.Fprintf(a.Stdout, "  账号名   : %s\n", *accountName)
			fmt.Fprintf(a.Stdout, "  User ID  : %d\n", st.UserID)
			if st.Username != "" {
				fmt.Fprintf(a.Stdout, "  用户名   : @%s\n", st.Username)
			}
			if st.Phone != "" {
				fmt.Fprintf(a.Stdout, "  手机号   : %s\n", st.Phone)
			}
			fmt.Fprintf(a.Stdout, "  Session  : %s\n", acc.SessionPath())
			return nil
		})
	})
	if err != nil {
		return fmt.Errorf("登录失败: %w", err)
	}
	return nil
}

// timeAfter 返回一个在 ms 毫秒后关闭的 channel。
func timeAfter(ms int) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		time.Sleep(time.Duration(ms) * time.Millisecond)
		close(ch)
	}()
	return ch
}
