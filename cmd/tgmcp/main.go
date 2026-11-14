// Command tgmcp 是 Telegram MCP 服务入口。
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/midi-HL/telegram-mcp/internal/cli"
	tgmcp "github.com/midi-HL/telegram-mcp/internal/mcp"
)

// version 由 ldflags 注入：-X main.version=v1.0.0
var version = "dev"

func main() {
	// 日志一律写 stderr，stdout 留给 MCP 的 JSON-RPC 帧
	cfg := zap.NewProductionConfig()
	cfg.OutputPaths = []string{"stderr"}
	cfg.ErrorOutputPaths = []string{"stderr"}
	cfg.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	cfg.Encoding = "console"
	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	logger, err := cfg.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志失败: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Release 只注入 main.version；同步给 MCP 层，避免 system 工具仍显示 dev。
	tgmcp.Version = version

	app := cli.New(version, logger)
	if err := app.Run(ctx, os.Args[1:]); err != nil {
		logger.Error("命令执行失败", zap.Error(err))
		os.Exit(1)
	}
}
