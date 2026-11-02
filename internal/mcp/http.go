package mcp

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

// HTTPServer 是 MCP 的 HTTP 传输封装。
type HTTPServer struct {
	server *mcp.Server
	deps   *Deps
	token  string
}

// NewHTTPServer 创建 HTTP 传输服务。token 为空表示不鉴权（仅限本机）。
func NewHTTPServer(deps *Deps, token string) *HTTPServer {
	return &HTTPServer{
		server: NewServer(deps),
		deps:   deps,
		token:  token,
	}
}

// Handler 返回带鉴权的 MCP handler。
func (h *HTTPServer) Handler() http.Handler {
	// SDK 自带 Streamable HTTP handler
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server { return h.server },
		&mcp.StreamableHTTPOptions{
			Stateless:      false, // 保留会话状态以支持 progress/elicitation
			SessionTimeout: 30 * time.Minute,
		},
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 健康检查不需要鉴权
		if r.URL.Path == "/health" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		if h.token != "" && !h.authorized(r) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="tgmcp"`)
			http.Error(w, "未授权", http.StatusUnauthorized)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	})
}

// authorized 校验 Bearer token。
func (h *HTTPServer) authorized(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return false
	}
	got := strings.TrimSpace(auth[len(prefix):])
	// 常数时间比较，防时序攻击
	return subtle.ConstantTimeCompare([]byte(got), []byte(h.token)) == 1
}

// RunHTTP 启动 HTTP 服务（阻塞）。
func (h *HTTPServer) RunHTTP(ctx context.Context, addr string, logger *zap.Logger) error {
	mux := http.NewServeMux()
	mux.Handle("/mcp", h.Handler())
	mux.Handle("/mcp/", h.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       180 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	logger.Info("MCP HTTP 服务启动", zap.String("addr", addr),
		zap.Bool("auth", h.token != ""))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP 服务: %w", err)
	}
	return nil
}
