// Package webui 提供内嵌的 Web 管理界面。
package webui

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/midi-HL/telegram-mcp/internal/autoreply"
	"github.com/midi-HL/telegram-mcp/internal/cache"
	"github.com/midi-HL/telegram-mcp/internal/config"
	tgclient "github.com/midi-HL/telegram-mcp/internal/telegram"
)

//go:embed templates/*.html static/*
var content embed.FS

// 会话 Cookie 名
const sessionCookie = "tgmcp_session"

// Server 是 Web UI 服务器。
type Server struct {
	cfg       *config.Config
	store     *cache.Store
	pool      *tgclient.Pool
	logger    *zap.Logger
	autoreply *autoreply.Engine

	tmpl *template.Template
	mux  *http.ServeMux
}

// New 创建 Web UI 服务器。
func New(cfg *config.Config, store *cache.Store, pool *tgclient.Pool, logger *zap.Logger) (*Server, error) {
	return NewWithAutoReply(cfg, store, pool, nil, logger)
}

// NewWithAutoReply 创建带自动回复引擎的 Web UI 服务器。
func NewWithAutoReply(cfg *config.Config, store *cache.Store, pool *tgclient.Pool, ar *autoreply.Engine, logger *zap.Logger) (*Server, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	// 解析模板
	tmpl, err := template.ParseFS(content, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("解析模板: %w", err)
	}

	s := &Server{
		cfg:       cfg,
		store:     store,
		pool:      pool,
		logger:    logger,
		autoreply: ar,
		tmpl:      tmpl,
		mux:       http.NewServeMux(),
	}

	// 确保凭据存在
	ctx := context.Background()
	isDefault, err := store.EnsureWebUICredential(ctx)
	if err != nil {
		return nil, fmt.Errorf("初始化凭据: %w", err)
	}
	if isDefault {
		logger.Warn("Web UI 正在使用默认密码，请登录后在设置页修改",
			zap.String("default_password", cache.DefaultWebUIPassword))
	}

	s.routes()
	return s, nil
}

// routes 注册路由。
func (s *Server) routes() {
	// 静态资源
	staticFS, _ := fs.Sub(content, "static")
	s.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// 公开路由
	s.mux.HandleFunc("/ui/login", s.handleLogin)
	s.mux.HandleFunc("/ui/logout", s.handleLogout)

	// 需要登录的路由
	s.mux.HandleFunc("/ui", s.withAuth(s.handleIndex))
	s.mux.HandleFunc("/ui/", s.withAuth(s.handleIndex))
	s.mux.HandleFunc("/ui/accounts", s.withAuth(s.handleAccounts))
	s.mux.HandleFunc("/ui/admins", s.withAuth(s.handleAdmins))
	s.mux.HandleFunc("/ui/settings", s.withAuth(s.handleSettings))
	s.mux.HandleFunc("/ui/chat", s.withAuth(s.handleChat))
	s.mux.HandleFunc("/ui/autoreply", s.withAuth(s.handleAutoReply))

	// API
	s.mux.HandleFunc("/api/login", s.handleAPILogin)
	s.mux.HandleFunc("/api/logout", s.handleAPILogout)
	s.mux.HandleFunc("/api/password", s.withAuth(s.handleAPIPassword))
	s.mux.HandleFunc("/api/accounts", s.withAuth(s.handleAPIAccounts))
	s.mux.HandleFunc("/api/admins", s.withAuth(s.handleAPIAdmins))
	s.mux.HandleFunc("/api/status", s.withAuth(s.handleAPIStatus))
	s.mux.HandleFunc("/api/chats", s.withAuth(s.handleAPIChats))
	s.mux.HandleFunc("/api/messages", s.withAuth(s.handleAPIMessages))
	s.mux.HandleFunc("/api/send", s.withAuth(s.handleAPISend))
	s.mux.HandleFunc("/api/autoreply", s.withAuth(s.handleAPIAutoReply))
	s.mux.HandleFunc("/api/autoreply/toggle", s.withAuth(s.handleAPIAutoReplyToggle))

	// 根路径重定向
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/ui", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})
}

// Handler 返回 http.Handler。
func (s *Server) Handler() http.Handler { return s.mux }

// Run 启动 HTTP 服务（阻塞）。
func (s *Server) Run(ctx context.Context) error {
	addr := net.JoinHostPort(s.cfg.WebUI.Host, fmt.Sprintf("%d", s.cfg.WebUI.Port))
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// 清理过期会话
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if n, err := s.store.CleanExpiredSessions(ctx); err == nil && n > 0 {
					s.logger.Debug("清理过期会话", zap.Int64("count", n))
				}
			}
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	s.logger.Info("Web UI 启动", zap.String("url", "http://"+addr+"/ui"))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("Web UI 服务: %w", err)
	}
	return nil
}

// withAuth 是登录校验中间件。
func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			s.redirectLogin(w, r)
			return
		}
		ok, err := s.store.ValidateSession(r.Context(), c.Value)
		if err != nil || !ok {
			s.redirectLogin(w, r)
			return
		}
		next(w, r)
	}
}

// redirectLogin 重定向到登录页（API 请求返回 401）。
func (s *Server) redirectLogin(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "未登录"})
		return
	}
	http.Redirect(w, r, "/ui/login", http.StatusFound)
}

// clientIP 提取客户端 IP（兼容代理头）。
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return strings.TrimSpace(xr)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// writeJSON 输出 JSON。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// render 渲染模板。
func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		s.logger.Error("渲染模板失败", zap.String("template", name), zap.Error(err))
		http.Error(w, "模板渲染失败", http.StatusInternalServerError)
	}
}

// pageData 是模板的通用数据。
type pageData struct {
	Title        string
	Active       string
	IsDefaultPwd bool
	Flash        string
	Error        string
	Data         any
}
