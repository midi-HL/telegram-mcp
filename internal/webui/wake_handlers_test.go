package webui

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/midi-HL/telegram-mcp/internal/config"
	"github.com/midi-HL/telegram-mcp/internal/wake"
)

func loginCookieWebUI(t *testing.T, s *Server) *http.Cookie {
	t.Helper()
	w := doRequest(t, s.Handler(), http.MethodPost, "/api/login",
		map[string]string{"password": "Admin"}, nil)
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie {
			return c
		}
	}
	t.Fatal("登录失败")
	return nil
}

func TestWakeAPIRequiresAuth(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s.Handler(), http.MethodGet, "/api/wake", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("未登录应 401, got %d", w.Code)
	}
}

func TestWakeTogglePersists(t *testing.T) {
	s, _ := newTestServer(t)
	e := wake.New(wake.Options{Config: config.DefaultWake()})
	s.SetWakeEngine(e)
	cookie := loginCookieWebUI(t, s)

	w := doRequest(t, s.Handler(), http.MethodPost, "/api/wake/toggle",
		map[string]bool{"enabled": true}, []*http.Cookie{cookie})
	if w.Code != http.StatusOK {
		t.Fatalf("toggle = %d, body=%s", w.Code, w.Body.String())
	}
	if !e.Enabled() {
		t.Error("引擎应已开启")
	}
	loaded, err := config.Load(s.cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Wake.Enabled {
		t.Error("配置应持久化 enabled=true")
	}
}

func TestWakeConfigValidation(t *testing.T) {
	s, _ := newTestServer(t)
	e := wake.New(wake.Options{Config: config.DefaultWake()})
	s.SetWakeEngine(e)
	cookie := loginCookieWebUI(t, s)

	w := doRequest(t, s.Handler(), http.MethodPost, "/api/wake", map[string]any{
		"enabled":     true,
		"mode":        "webhook",
		"webhook_url": "not-a-url",
	}, []*http.Cookie{cookie})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("非法 webhook 应 400, got %d", w.Code)
	}

	w = doRequest(t, s.Handler(), http.MethodPost, "/api/wake", map[string]any{
		"enabled":          true,
		"mode":             "notify",
		"include_text":     true,
		"debounce_seconds": 3,
	}, []*http.Cookie{cookie})
	if w.Code != http.StatusOK {
		t.Fatalf("合法配置 = %d, body=%s", w.Code, w.Body.String())
	}
	if got := e.Config().DebounceSeconds; got != 3 {
		t.Errorf("DebounceSeconds = %d, want 3", got)
	}
}

func TestWakeTestPendingAck(t *testing.T) {
	s, _ := newTestServer(t)
	cfg := config.DefaultWake()
	cfg.Enabled = true
	cfg.Mode = "notify"
	e := wake.New(wake.Options{Config: cfg})
	s.SetWakeEngine(e)
	cookie := loginCookieWebUI(t, s)

	w := doRequest(t, s.Handler(), http.MethodPost, "/api/wake/test",
		map[string]string{"text": "hello"}, []*http.Cookie{cookie})
	if w.Code != http.StatusOK {
		t.Fatalf("test = %d, body=%s", w.Code, w.Body.String())
	}
	var testResp struct {
		OK           bool `json:"ok"`
		Notification struct {
			ID string `json:"id"`
		} `json:"notification"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &testResp); err != nil {
		t.Fatal(err)
	}
	if !testResp.OK || testResp.Notification.ID == "" {
		t.Fatalf("测试响应异常: %s", w.Body.String())
	}

	w = doRequest(t, s.Handler(), http.MethodGet, "/api/wake/pending", nil, []*http.Cookie{cookie})
	if !strings.Contains(w.Body.String(), "hello") {
		t.Errorf("待读队列应含测试正文: %s", w.Body.String())
	}
	w = doRequest(t, s.Handler(), http.MethodPost, "/api/wake/ack", nil, []*http.Cookie{cookie})
	if !strings.Contains(w.Body.String(), `"cleared":1`) {
		t.Errorf("ack 应清空 1 条: %s", w.Body.String())
	}
}

func TestCredentialsAPIBuiltin(t *testing.T) {
	s, _ := newTestServer(t)
	cookie := loginCookieWebUI(t, s)

	w := doRequest(t, s.Handler(), http.MethodGet, "/api/credentials", nil, []*http.Cookie{cookie})
	if w.Code != http.StatusOK {
		t.Fatalf("GET credentials = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"builtin_api_id":34353880`) {
		t.Errorf("应返回内置 api_id: %s", w.Body.String())
	}

	// 留空保存应写入内置凭据
	w = doRequest(t, s.Handler(), http.MethodPost, "/api/credentials", map[string]any{
		"account": "main", "api_id": 0, "api_hash": "",
	}, []*http.Cookie{cookie})
	if w.Code != http.StatusOK {
		t.Fatalf("POST credentials = %d, body=%s", w.Code, w.Body.String())
	}
	acc := s.cfg.Account("main")
	if acc == nil || acc.APIID != config.BuiltinAPIID || acc.APIHash != config.BuiltinAPIHash {
		t.Errorf("空值应写内置凭据: %+v", acc)
	}
}
