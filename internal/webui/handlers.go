package webui

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/midi-HL/telegram-mcp/internal/cache"
	"github.com/midi-HL/telegram-mcp/internal/dto"
	tgclient "github.com/midi-HL/telegram-mcp/internal/telegram"
)

// --- 页面 ---

// handleLogin 登录页（GET）。
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	// 已登录则跳转
	if c, err := r.Cookie(sessionCookie); err == nil {
		if ok, _ := s.store.ValidateSession(r.Context(), c.Value); ok {
			http.Redirect(w, r, "/ui", http.StatusFound)
			return
		}
	}
	s.render(w, "login.html", map[string]any{
		"Error":     r.URL.Query().Get("error"),
		"IsDefault": s.store.IsDefaultPassword(r.Context()),
	})
}

// handleLogout 登出（GET）。
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = s.store.DeleteSession(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/ui/login", http.StatusFound)
}

// handleIndex 首页（重定向到账号页）。
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/ui" || r.URL.Path == "/ui/" {
		http.Redirect(w, r, "/ui/accounts", http.StatusFound)
		return
	}
	http.NotFound(w, r)
}

// handleAccounts 账号管理页。
func (s *Server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	accounts := s.accountList(r.Context())
	s.render(w, "accounts.html", pageData{
		Title:        "账号管理",
		Active:       "accounts",
		IsDefaultPwd: s.store.IsDefaultPassword(r.Context()),
		Data:         accounts,
	})
}

// handleAdmins 管理员管理页。
func (s *Server) handleAdmins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	admins, err := s.listAdmins(r.Context())
	if err != nil {
		s.render(w, "admins.html", pageData{
			Title: "管理员", Active: "admins", Error: err.Error(),
		})
		return
	}
	s.render(w, "admins.html", pageData{
		Title:        "管理员",
		Active:       "admins",
		IsDefaultPwd: s.store.IsDefaultPassword(r.Context()),
		Data:         admins,
	})
}

// handleSettings 设置页。
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	s.render(w, "settings.html", pageData{
		Title:        "设置",
		Active:       "settings",
		IsDefaultPwd: s.store.IsDefaultPassword(r.Context()),
		Data: map[string]any{
			"DataDir": s.cfg.DataDir,
			"Host":    s.cfg.WebUI.Host,
			"Port":    s.cfg.WebUI.Port,
			"ASR":     s.cfg.ASR.Backend,
		},
	})
}

// --- API ---

// handleAPILogin 登录 API。
func (s *Server) handleAPILogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不允许"})
		return
	}
	ip := clientIP(r)
	ctx := r.Context()

	// 检查锁定
	locked, remaining, err := s.store.CheckLoginLock(ctx, ip)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "内部错误"})
		return
	}
	if locked {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error": fmt.Sprintf("尝试次数过多，请 %d 分钟后再试", remaining/60+1),
		})
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}

	ok, err := s.store.VerifyWebUIPassword(ctx, req.Password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "内部错误"})
		return
	}
	if !ok {
		lockedNow, err := s.store.RecordLoginFailure(ctx, ip)
		if err != nil {
			s.logger.Error("记录登录失败", zap.Error(err))
		}
		msg := "密码错误"
		if lockedNow {
			msg = fmt.Sprintf("密码错误次数过多，已锁定 %d 分钟", int(cache.LockDuration.Minutes()))
		}
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": msg})
		return
	}

	// 成功
	_ = s.store.ClearLoginFailures(ctx, ip)
	sid, err := s.store.CreateSession(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "创建会话失败"})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    sid,
		Path:     "/",
		MaxAge:   int(cache.SessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	s.logger.Info("Web UI 登录成功", zap.String("ip", ip))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"isDefault": s.store.IsDefaultPassword(ctx),
	})
}

// handleAPILogout 登出 API。
func (s *Server) handleAPILogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = s.store.DeleteSession(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleAPIPassword 修改密码。
func (s *Server) handleAPIPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不允许"})
		return
	}
	ctx := r.Context()
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
		Confirm     string `json:"confirm"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}

	if req.NewPassword != req.Confirm {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "两次输入的新密码不一致"})
		return
	}
	if len(req.NewPassword) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "新密码至少 8 位"})
		return
	}

	ok, err := s.store.VerifyWebUIPassword(ctx, req.OldPassword)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "内部错误"})
		return
	}
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "旧密码错误"})
		return
	}

	if err := s.store.SetWebUIPassword(ctx, req.NewPassword); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "保存失败"})
		return
	}
	// 强制所有会话登出
	_ = s.store.DeleteAllSessions(ctx)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
	})
	s.logger.Info("Web UI 密码已修改，所有会话已登出")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "relogin": true})
}

// AccountView 是账号的展示结构。
type AccountView struct {
	Name      string `json:"name"`
	UserID    int64  `json:"user_id,omitempty"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	Phone     string `json:"phone,omitempty"`
	Ready     bool   `json:"ready"`
	IsDefault bool   `json:"is_default"`
	Session   string `json:"session,omitempty"`
}

// handleAPIAccounts 账号列表 API。
func (s *Server) handleAPIAccounts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"accounts": s.accountList(r.Context())})
}

// accountList 构造账号列表。
func (s *Server) accountList(_ context.Context) []AccountView {
	out := make([]AccountView, 0, s.pool.Len())
	for _, name := range s.pool.Names() {
		acc, ok := s.pool.Get(name)
		if !ok || acc == nil {
			continue
		}
		v := AccountView{
			Name:      name,
			IsDefault: name == s.cfg.DefaultAccount,
			Session:   acc.SessionPath(),
		}
		if self := acc.Self(); self != nil {
			v.UserID = self.ID
			v.Username, _ = self.GetUsername()
			v.FirstName = self.FirstName
			v.Phone, _ = self.GetPhone()
			v.Ready = true
		}
		out = append(out, v)
	}
	return out
}

// AdminView 是管理员展示结构。
type AdminView struct {
	UserID    int64  `json:"user_id"`
	Username  string `json:"username,omitempty"`
	AddedAt   int64  `json:"added_at"`
	AddedBy   int64  `json:"added_by,omitempty"`
}

// handleAPIAdmins 管理员 CRUD。
func (s *Server) handleAPIAdmins(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		admins, err := s.listAdmins(ctx)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"admins": admins})

	case http.MethodPost:
		var req struct {
			Username string `json:"username"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
			return
		}
		if req.Username == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "需要用户名"})
			return
		}

		// ★ 关键：解析用户名 → 数字 ID，只存 ID
		userID, username, err := s.resolveUser(ctx, req.Username)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if err := s.addAdmin(ctx, userID, username); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		s.logger.Info("添加管理员",
			zap.Int64("user_id", userID), zap.String("username", username))
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":       true,
			"user_id":  userID,
			"username": username,
		})

	case http.MethodDelete:
		var req struct {
			UserID int64 `json:"user_id"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
			return
		}
		if err := s.removeAdmin(ctx, req.UserID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不允许"})
	}
}

// resolveUser 把 @username 解析成数字 ID。
func (s *Server) resolveUser(ctx context.Context, username string) (int64, string, error) {
	username = strings.TrimPrefix(username, "@")
	// 找第一个就绪的账号来解析
	for _, name := range s.pool.Names() {
		acc, ok := s.pool.Get(name)
		if !ok || acc.Self() == nil || acc.API() == nil {
			continue
		}
		r := tgclient.NewPeerResolver(acc.API())
		peer, err := r.Resolve(ctx, "@"+username)
		if err != nil {
			continue
		}
		id := tgclient.PeerID(peer)
		if id == 0 {
			continue
		}
		return id, username, nil
	}
	return 0, "", fmt.Errorf("无法解析 @%s（请确认用户名正确且账号已就绪）", username)
}

// handleAPIStatus 状态 API。
func (s *Server) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.Stats(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"is_default":   s.store.IsDefaultPassword(r.Context()),
		"cache":        stats,
		"accounts":     s.pool.Len(),
		"server_time":  time.Now().Format(time.RFC3339),
	})
}

// decodeJSON 解析请求体。
func decodeJSON(r *http.Request, v any) error {
	defer func() { _ = r.Body.Close() }()
	dec := newJSONDecoder(r.Body)
	return dec.Decode(v)
}

// AdminRecord 是管理员记录。
type AdminRecord struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username_snapshot,omitempty"`
	AddedAt  int64  `json:"added_at"`
	AddedBy  int64  `json:"added_by,omitempty"`
}

var _ = dto.ChatBrief{}
