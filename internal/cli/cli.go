// Package cli 实现命令行子命令分发。
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"go.uber.org/zap"

	"github.com/midi-HL/telegram-mcp/internal/config"
)

// App 是命令行应用。
type App struct {
	Version string
	Logger  *zap.Logger
	// Stdout / Stderr 可替换，便于测试。
	Stdout io.Writer
	Stderr io.Writer
	// DataDir 覆盖默认数据目录。
	DataDir string
}

// New 创建应用。
func New(version string, logger *zap.Logger) *App {
	return &App{
		Version: version,
		Logger:  logger,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}
}

// usage 打印用法。
func (a *App) usage() {
	fmt.Fprintf(a.Stderr, `tgmcp %s — Telegram MCP 服务

用法:
  tgmcp <command> [flags]

命令:
  auth     登录 Telegram 账号（扫码或手机号）
  serve    启动 MCP 服务
  doctor   环境自检
  asr      语音转写配置（setup 下载模型 / status / config）
  version  打印版本

通用 flags:
  --data-dir <path>   数据目录（默认 %s，可用 TGMCP_DATA_DIR 覆盖）
  --account <name>    账号名（默认 main）

示例:
  tgmcp auth --qr
  tgmcp auth --phone +8613800138000
  tgmcp serve --stdio
  tgmcp doctor
`, a.Version, config.DefaultDataDir())
}

// Run 解析参数并执行子命令。
func (a *App) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		a.usage()
		return nil
	}
	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "auth":
		return a.runAuth(ctx, rest)
	case "serve":
		return a.runServe(ctx, rest)
	case "doctor":
		return a.runDoctor(ctx, rest)
	case "asr":
		return a.runASR(ctx, rest)
	case "version", "-v", "--version":
		fmt.Fprintf(a.Stdout, "tgmcp %s\n", a.Version)
		return nil
	case "help", "-h", "--help":
		a.usage()
		return nil
	default:
		a.usage()
		return fmt.Errorf("未知命令 %q", cmd)
	}
}

// commonFlags 注册所有子命令共用的 flag。
func (a *App) commonFlags(fs *flag.FlagSet) {
	fs.StringVar(&a.DataDir, "data-dir", "", "数据目录")
}

// resolveConfig 加载配置（含 --data-dir 覆盖）。
func (a *App) resolveConfig() (*config.Config, error) {
	dir := a.DataDir
	if dir == "" {
		dir = config.DefaultDataDir()
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}
