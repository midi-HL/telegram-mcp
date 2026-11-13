package cli

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	"github.com/midi-HL/telegram-mcp/internal/config"
)

// runDoctor 执行完整可行性自检；检查结果不会修改配置或连接 Telegram。
func (a *App) runDoctor(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	a.commonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := a.resolveConfig()
	if err != nil {
		return fmt.Errorf("加载配置: %w", err)
	}

	fmt.Fprintf(a.Stdout, "tgmcp 可行性自检\n")
	fmt.Fprintf(a.Stdout, "  版本      : %s\n", a.Version)
	fmt.Fprintf(a.Stdout, "  Go 版本   : %s\n", runtime.Version())
	fmt.Fprintf(a.Stdout, "  平台      : %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(a.Stdout, "  cgo       : %v\n", isCgo())
	fmt.Fprintf(a.Stdout, "  数据目录  : %s\n", cfg.DataDir)

	// 1. 数据目录可写 + 配置可读
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		fmt.Fprintf(a.Stdout, "  数据目录  : ❌ 无法创建 (%v)\n", err)
	} else {
		probe := filepath.Join(cfg.DataDir, ".write_probe")
		if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
			fmt.Fprintf(a.Stdout, "  可写性    : ❌ %v\n", err)
		} else {
			_ = os.Remove(probe)
			fmt.Fprintf(a.Stdout, "  可写性    : ✅\n")
		}
	}
	if _, err := os.Stat(config.Path(cfg.DataDir)); err == nil {
		fmt.Fprintf(a.Stdout, "  配置文件  : ✅ config.json 可读\n")
	} else {
		fmt.Fprintf(a.Stdout, "  配置文件  : ℹ 不存在，将使用开箱即用默认配置\n")
	}

	// 2. 凭据：内置 + 可替换
	cred := cfg.EffectiveCredentials(cfg.DefaultAccount)
	switch cred.Source {
	case "env":
		fmt.Fprintf(a.Stdout, "  API 凭据  : ✅ 环境变量覆盖 (id=%d, hash=%s...)\n", cred.APIID, mask(cred.APIHash))
	case "builtin":
		fmt.Fprintf(a.Stdout, "  API 凭据  : ✅ 已公开内置默认凭据 (id=%d)，开箱即用\n", cred.APIID)
	default:
		fmt.Fprintf(a.Stdout, "  API 凭据  : ✅ 自定义配置 (id=%d, hash=%s...)\n", cred.APIID, mask(cred.APIHash))
	}

	// 3. 账号与 session
	fmt.Fprintf(a.Stdout, "\n账号 (%d 个):\n", len(cfg.Accounts))
	if len(cfg.Accounts) == 0 {
		fmt.Fprintf(a.Stdout, "  （无；可直接打开 Web UI 登录，无需命令行配置）\n")
	}
	for _, acc := range cfg.Accounts {
		sessionPath := filepath.Join(acc.SessionDir, "session.json")
		exists := "❌ 无 session"
		if _, err := os.Stat(sessionPath); err == nil {
			exists = "✅ 有 session"
		}
		mark := "  "
		if acc.Name == cfg.DefaultAccount {
			mark = "* "
		}
		source := "自定义"
		if config.IsBuiltinCredentials(acc.APIID, acc.APIHash) {
			source = "内置"
		}
		fmt.Fprintf(a.Stdout, "  %s%s  %s  (api_id=%d, %s)\n", mark, acc.Name, exists, acc.APIID, source)
	}

	// 4. Web UI 端口状态
	// 默认只做本地回环的 TCP connect 探测，不调用 net.Listen：
	// Windows Defender 会把每个 go test 生成的临时 exe 当成新程序，
	// 一旦监听 0.0.0.0 就反复弹「是否允许访问网络」，非常烦人。
	fmt.Fprintf(a.Stdout, "\n服务配置:\n")
	addr := net.JoinHostPort(cfg.WebUI.Host, fmt.Sprintf("%d", cfg.WebUI.Port))
	fmt.Fprintf(a.Stdout, "  Web UI    : ℹ 配置监听 %s；启动后从这里访问（doctor 不做任何网络探测）\n", addr)
	fmt.Fprintf(a.Stdout, "  浏览器入口: http://%s:%d/ui\n", displayHost(cfg.WebUI.Host), cfg.WebUI.Port)
	fmt.Fprintf(a.Stdout, "  ASR       : backend=%s\n", cfg.ASR.Backend)
	fmt.Fprintf(a.Stdout, "  Agent 唤醒: enabled=%v, mode=%s", cfg.Wake.Enabled, cfg.Wake.Mode)
	if cfg.Wake.Mode == "webhook" || cfg.Wake.Mode == "all" {
		if err := validateWakeURL(cfg.Wake.WebhookURL); err != nil {
			fmt.Fprintf(a.Stdout, "  ⚠ webhook=%v\n", err)
		} else {
			fmt.Fprintf(a.Stdout, "  webhook=✅\n")
		}
	} else {
		fmt.Fprintf(a.Stdout, "\n")
	}

	// 5. 给小白的下一步
	fmt.Fprintf(a.Stdout, "\n建议:\n")
	switch {
	case len(cfg.Accounts) == 0:
		fmt.Fprintf(a.Stdout, "  1. 运行 tgmcp serve（Web UI 会立即启动）\n")
		fmt.Fprintf(a.Stdout, "  2. 浏览器打开 http://%s:%d/ui，默认密码 Admin\n", displayHost(cfg.WebUI.Host), cfg.WebUI.Port)
		fmt.Fprintf(a.Stdout, "  3. 在「账号」页直接扫码登录，无需申请 api_id/api_hash\n")
		fmt.Fprintf(a.Stdout, "  4. 登录后立即在「设置」页修改密码\n")
	case !hasAnySession(cfg):
		fmt.Fprintf(a.Stdout, "  已有账号配置但没有 session，请打开 Web UI 完成登录\n")
	default:
		fmt.Fprintf(a.Stdout, "  已具备运行条件；用 tgmcp serve --http 启动 MCP HTTP 传输，或接入 stdio\n")
	}
	fmt.Fprintf(a.Stdout, "\n自检完成（只读检查，未连接 Telegram）\n")
	return nil
}

func displayHost(host string) string {
	if host == "" || host == "0.0.0.0" || host == "::" {
		return "127.0.0.1"
	}
	return host
}

func hasAnySession(cfg *config.Config) bool {
	for _, acc := range cfg.Accounts {
		if acc.SessionDir == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(acc.SessionDir, "session.json")); err == nil {
			return true
		}
	}
	return false
}

func validateWakeURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("未填写")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("需要 http:// 或 https:// 地址")
	}
	return nil
}

func mask(s string) string {
	if len(s) <= 6 {
		return "***"
	}
	return s[:6] + "..."
}
