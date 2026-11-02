package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/midi-HL/telegram-mcp/internal/config"
)

// runDoctor 执行环境自检。
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

	fmt.Fprintf(a.Stdout, "tgmcp 环境自检\n")
	fmt.Fprintf(a.Stdout, "  版本      : %s\n", a.Version)
	fmt.Fprintf(a.Stdout, "  Go 版本   : %s\n", runtime.Version())
	fmt.Fprintf(a.Stdout, "  平台      : %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(a.Stdout, "  cgo       : %v\n", isCgo())
	fmt.Fprintf(a.Stdout, "  数据目录  : %s\n", cfg.DataDir)

	// 检查数据目录可写
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

	// API 凭据
	if id, hash, ok := config.EnvCredentials(); ok {
		fmt.Fprintf(a.Stdout, "  API 凭据  : ✅ 环境变量 (id=%d, hash=%s...)\n", id, mask(hash))
	} else {
		fmt.Fprintf(a.Stdout, "  API 凭据  : ⚠ 环境变量未设置（可用 --api-id/--api-hash 或 TELEGRAM_API_ID/HASH）\n")
	}

	// 账号
	fmt.Fprintf(a.Stdout, "\n账号 (%d 个):\n", len(cfg.Accounts))
	if len(cfg.Accounts) == 0 {
		fmt.Fprintf(a.Stdout, "  （无，请运行 tgmcp auth）\n")
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
		fmt.Fprintf(a.Stdout, "  %s%s  %s  (api_id=%d)\n", mark, acc.Name, exists, acc.APIID)
	}

	// Web UI / ASR 配置摘要
	fmt.Fprintf(a.Stdout, "\n配置:\n")
	fmt.Fprintf(a.Stdout, "  Web UI    : enabled=%v, %s:%d\n", cfg.WebUI.Enabled, cfg.WebUI.Host, cfg.WebUI.Port)
	fmt.Fprintf(a.Stdout, "  ASR       : backend=%s\n", cfg.ASR.Backend)
	fmt.Fprintf(a.Stdout, "\n自检完成\n")
	return nil
}

func mask(s string) string {
	if len(s) <= 6 {
		return "***"
	}
	return s[:6] + "..."
}
