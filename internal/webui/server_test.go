package webui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/midi-HL/telegram-mcp/internal/cache"
	"github.com/midi-HL/telegram-mcp/internal/config"
	tgclient "github.com/midi-HL/telegram-mcp/internal/telegram"
)

func init() {
	// 测试用低 cost（4），避免 -race 模式下超时
	cache.SetBcryptCost(4)
}

func newTestServer(t *testing.T) (*Server, *cache.Store) {
	t.Helper()
	dir := t.TempDir()
	store, err := cache.Open(dir)
	if err != nil {
		t.Fatalf("Open cache: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	cfg := config.Default()
	cfg.DataDir = dir
	cfg.WebUI.Host = "127.0.0.1"
	cfg.WebUI.Port = 0

	pool := tgclient.NewPool(zap.NewNop())
	s, err := New(cfg, store, pool, zap.NewNop())
	if err != nil {
		t.Fatalf("New webui: %v", err)
	}
	return s, store
}

// doRequest 发送请求并返回响应。
func doRequest(t *testing.T, h http.Handler, method, path string, body any, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		r = httptest.NewRequest(method, path, strings.NewReader(string(b)))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestLoginPageRenders(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s.Handler(), http.MethodGet, "/ui/login", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Telegram MCP") {
		t.Error("登录页应包含标题")
	}
	if !strings.Contains(body, "password") {
		t.Error("登录页应包含密码输入框")
	}
	// 关键：不应有用户名输入框
	if strings.Contains(body, `name="username"`) {
		t.Error("登录页不应有用户名输入框（纯密码登录）")
	}
}

func TestUnauthorizedRedirect(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s.Handler(), http.MethodGet, "/ui/accounts", nil, nil)
	if w.Code != http.StatusFound {
		t.Fatalf("未登录访问应 302，got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/ui/login" {
		t.Errorf("跳转目标 = %q, want /ui/login", loc)
	}
}

func TestUnauthorizedAPI(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s.Handler(), http.MethodGet, "/api/accounts", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("未登录 API 应 401，got %d", w.Code)
	}
}

func TestLoginSuccess(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s.Handler(), http.MethodPost, "/api/login",
		map[string]string{"password": "Admin"}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("登录状态 = %d, body=%s", w.Code, w.Body.String())
	}

	// 检查 Cookie
	cookies := w.Result().Cookies()
	var session *http.Cookie
	for _, c := range cookies {
		if c.Name == sessionCookie {
			session = c
			break
		}
	}
	if session == nil {
		t.Fatal("登录后应设置 session cookie")
	}
	if !session.HttpOnly {
		t.Error("session cookie 应 HttpOnly")
	}
	if session.Value == "" {
		t.Error("session cookie 值不应为空")
	}
}

func TestLoginWrongPassword(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s.Handler(), http.MethodPost, "/api/login",
		map[string]string{"password": "wrong"}, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("错误密码应 401，got %d", w.Code)
	}
}

func TestLoginThenAccess(t *testing.T) {
	s, _ := newTestServer(t)

	// 登录
	w := doRequest(t, s.Handler(), http.MethodPost, "/api/login",
		map[string]string{"password": "Admin"}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("登录失败: %d", w.Code)
	}
	var session *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie {
			session = c
		}
	}

	// 带 Cookie 访问
	w2 := doRequest(t, s.Handler(), http.MethodGet, "/ui/accounts", nil, []*http.Cookie{session})
	if w2.Code != http.StatusOK {
		t.Fatalf("登录后访问应 200，got %d", w2.Code)
	}
	if !strings.Contains(w2.Body.String(), "账号管理") {
		t.Error("页面应包含「账号管理」")
	}
}

func TestChangePassword(t *testing.T) {
	s, store := newTestServer(t)
	ctx := context.Background()

	// 登录
	w := doRequest(t, s.Handler(), http.MethodPost, "/api/login",
		map[string]string{"password": "Admin"}, nil)
	var session *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie {
			session = c
		}
	}

	// 改密码（太短）
	w2 := doRequest(t, s.Handler(), http.MethodPost, "/api/password", map[string]string{
		"old_password": "Admin", "new_password": "short", "confirm": "short",
	}, []*http.Cookie{session})
	if w2.Code != http.StatusBadRequest {
		t.Errorf("短密码应 400，got %d", w2.Code)
	}

	// 改密码（不一致）
	w3 := doRequest(t, s.Handler(), http.MethodPost, "/api/password", map[string]string{
		"old_password": "Admin", "new_password": "NewPass123", "confirm": "Different1",
	}, []*http.Cookie{session})
	if w3.Code != http.StatusBadRequest {
		t.Errorf("不一致应 400，got %d", w3.Code)
	}

	// 改密码（旧密码错）
	w4 := doRequest(t, s.Handler(), http.MethodPost, "/api/password", map[string]string{
		"old_password": "wrong", "new_password": "NewPass123", "confirm": "NewPass123",
	}, []*http.Cookie{session})
	if w4.Code != http.StatusUnauthorized {
		t.Errorf("旧密码错应 401，got %d", w4.Code)
	}

	// 改密码（正确）
	w5 := doRequest(t, s.Handler(), http.MethodPost, "/api/password", map[string]string{
		"old_password": "Admin", "new_password": "NewPass123", "confirm": "NewPass123",
	}, []*http.Cookie{session})
	if w5.Code != http.StatusOK {
		t.Errorf("正确改密码应 200，got %d: %s", w5.Code, w5.Body.String())
	}

	// 验证新密码可用
	ok, _ := store.VerifyWebUIPassword(ctx, "NewPass123")
	if !ok {
		t.Error("新密码应可用")
	}
	// 旧密码失效
	ok, _ = store.VerifyWebUIPassword(ctx, "Admin")
	if ok {
		t.Error("旧密码应失效")
	}
	// 所有会话应被清除
	ok, _ = store.ValidateSession(ctx, session.Value)
	if ok {
		t.Error("改密码后旧会话应失效")
	}
}

func TestLogout(t *testing.T) {
	s, store := newTestServer(t)
	ctx := context.Background()

	w := doRequest(t, s.Handler(), http.MethodPost, "/api/login",
		map[string]string{"password": "Admin"}, nil)
	var session *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie {
			session = c
		}
	}

	// 登出
	w2 := doRequest(t, s.Handler(), http.MethodPost, "/api/logout", nil, []*http.Cookie{session})
	if w2.Code != http.StatusOK {
		t.Errorf("登出应 200，got %d", w2.Code)
	}

	// 会话应失效
	ok, _ := store.ValidateSession(ctx, session.Value)
	if ok {
		t.Error("登出后会话应失效")
	}
}

func TestAddAdminRequiresUsername(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s.Handler(), http.MethodPost, "/api/login",
		map[string]string{"password": "Admin"}, nil)
	var session *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie {
			session = c
		}
	}

	// 空用户名
	w2 := doRequest(t, s.Handler(), http.MethodPost, "/api/admins",
		map[string]string{"username": ""}, []*http.Cookie{session})
	if w2.Code != http.StatusBadRequest {
		t.Errorf("空用户名应 400，got %d", w2.Code)
	}
}

func TestStatusAPI(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s.Handler(), http.MethodPost, "/api/login",
		map[string]string{"password": "Admin"}, nil)
	var session *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie {
			session = c
		}
	}

	w2 := doRequest(t, s.Handler(), http.MethodGet, "/api/status", nil, []*http.Cookie{session})
	if w2.Code != http.StatusOK {
		t.Fatalf("状态 API = %d", w2.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w2.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析 JSON: %v", err)
	}
	if resp["ok"] != true {
		t.Error("应返回 ok=true")
	}
	if resp["is_default"] != true {
		t.Error("初始应是默认密码")
	}
}

func TestCookieSameSite(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s.Handler(), http.MethodPost, "/api/login",
		map[string]string{"password": "Admin"}, nil)
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie {
			if c.SameSite != http.SameSiteLaxMode {
				t.Errorf("SameSite = %v, want Lax", c.SameSite)
			}
			return
		}
	}
	t.Fatal("未找到 session cookie")
}
