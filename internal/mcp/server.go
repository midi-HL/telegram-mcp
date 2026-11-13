// Package mcp 提供 MCP 服务层。
package mcp

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

// Version 是服务版本，由 ldflags 注入。
var Version = "dev"

// currentServer 保存本次进程创建的 MCP server，供唤醒引擎主动推送通知。
var currentServer atomic.Pointer[mcp.Server]

// CurrentServer 返回当前 MCP server（stdio 或 HTTP 传输共用）。
func CurrentServer() *mcp.Server { return currentServer.Load() }

// NewServer 构造 MCP 服务并注册所有工具。
func NewServer(deps *Deps) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "telegram-mcp",
		Version: Version,
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{
			Tools:   &mcp.ToolCapabilities{ListChanged: true},
			Logging: &mcp.LoggingCapabilities{},
		},
		// 2026-07-28 协议废弃 logging/notifications/message 且不订阅它。
		// 限制到 2025-11-25 可让新版 SDK 客户端自动降级，保留主动唤醒通知；
		// 更老/更新的第三方客户端仍按 negotiatedVersion 兼容协商。
		SupportedProtocolVersions: []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"},
	})
	registerSystemTools(s, deps)
	registerChatTools(s, deps)
	registerMessageTools(s, deps)
	registerGroupTools(s, deps)
	registerContactTools(s, deps)
	registerMediaTools(s, deps)
	registerStickerTools(s, deps)
	registerAccountTools(s, deps)
	registerBotTools(s, deps)
	registerBotUseTools(s, deps)
	registerASRTools(s, deps)
	registerStatsTools(s, deps)
	registerWakeTools(s, deps)
	currentServer.Store(s)
	return s
}

// RunStdio 以 stdio 传输启动服务。
func RunStdio(ctx context.Context, deps *Deps) error {
	s := NewServer(deps)
	deps.Logger.Info("MCP 服务启动（stdio）", zap.String("version", Version))
	// stdio 会话没有 logging/setLevel 时 SDK 默认静默 Log；显式以 info 级别初始化。
	ss, err := s.Connect(ctx, &mcp.StdioTransport{}, &mcp.ServerSessionOptions{
		State: &mcp.ServerSessionState{LogLevel: "info"},
	})
	if err != nil {
		return fmt.Errorf("stdio 服务: %w", err)
	}
	go func() {
		<-ctx.Done()
		_ = ss.Close()
	}()
	if err := ss.Wait(); err != nil {
		return fmt.Errorf("stdio 服务: %w", err)
	}
	return nil
}

// fail 构造一个 MCP 工具错误结果。
func fail(msg string, args ...any) (*mcp.CallToolResult, error) {
	text := fmt.Sprintf(msg, args...)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: true,
	}, nil
}

// warn 向 stderr 打印警告（stdio 模式下绝不能写 stdout）。
func warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[tgmcp] "+format+"\n", args...)
}
