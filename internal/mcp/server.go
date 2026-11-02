// Package mcp 提供 MCP 服务层。
package mcp

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

// Version 是服务版本，由 ldflags 注入。
var Version = "dev"

// NewServer 构造 MCP 服务并注册所有工具。
func NewServer(deps *Deps) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "telegram-mcp",
		Version: Version,
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{ListChanged: true},
		},
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
	return s
}

// RunStdio 以 stdio 传输启动服务。
func RunStdio(ctx context.Context, deps *Deps) error {
	s := NewServer(deps)
	deps.Logger.Info("MCP 服务启动（stdio）", zap.String("version", Version))
	if err := s.Run(ctx, &mcp.StdioTransport{}); err != nil {
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
