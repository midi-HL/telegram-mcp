package webui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"go.uber.org/zap"

	"github.com/midi-HL/telegram-mcp/internal/config"
	tgclient "github.com/midi-HL/telegram-mcp/internal/telegram"
)

// fakeLoginAccount 只用于验证 LoginManager 把成功登录交给运行器，
// 不建立真实 Telegram 网络连接。
type fakeLoginAccount struct {
	activated  bool
	activation error
}

func TestFinishLoginCallsActivator(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	pool := tgclient.NewPool(zap.NewNop())

	called := false
	mgr := NewLoginManager(cfg, pool, func(acc *tgclient.Account) error {
		called = true
		if acc == nil {
			t.Fatal("activate 收到 nil account")
		}
		return nil
	}, zap.NewNop())

	sess := mgr.newSession("newacc")
	acc := &tgclient.Account{Name: "newacc"}
	st := &tgclient.Status{Authorized: true, UserID: 123, Username: "tester"}

	if err := mgr.finishLogin(sess, acc, st); err != nil {
		t.Fatalf("finishLogin: %v", err)
	}
	if !called {
		t.Fatal("登录成功必须调用常驻账号激活器，否则新账号无法免重启使用")
	}
	snap := sess.Snap()
	if snap.State != StateSuccess || snap.UserID != 123 || snap.Username != "tester" {
		t.Fatalf("登录状态未正确更新: %+v", snap)
	}
}

func TestFinishLoginRequiresActivator(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	pool := tgclient.NewPool(zap.NewNop())
	mgr := NewLoginManager(cfg, pool, nil, zap.NewNop())
	sess := mgr.newSession("newacc")
	err := mgr.finishLogin(sess, &tgclient.Account{Name: "newacc"}, &tgclient.Status{Authorized: true})
	if err == nil {
		t.Fatal("没有运行器时应明确失败，避免误报登录成功")
	}
}

func TestFinishLoginPropagatesActivationFailure(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	pool := tgclient.NewPool(zap.NewNop())
	mgr := NewLoginManager(cfg, pool, func(*tgclient.Account) error {
		return errors.New("activate failed")
	}, zap.NewNop())
	sess := mgr.newSession("newacc")
	err := mgr.finishLogin(sess, &tgclient.Account{Name: "newacc"}, &tgclient.Status{Authorized: true})
	if err == nil || sess.Snap().State == StateSuccess {
		t.Fatalf("激活失败不应标记为登录成功: err=%v state=%s", err, sess.Snap().State)
	}
}

func TestLoginStatusSuccessSnapshot(t *testing.T) {
	mgr := NewLoginManager(config.Default(), tgclient.NewPool(zap.NewNop()), nil, zap.NewNop())
	sess := mgr.newSession("x")
	sess.setState(StateSuccess)
	data, _ := json.Marshal(sess.Snap())
	if string(data) == "" || !containsJSONState(data) {
		t.Fatalf("快照应序列化 success: %s", data)
	}
}

func containsJSONState(data []byte) bool {
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		return false
	}
	return v["state"] == string(StateSuccess)
}

func TestFirstAccountBecomesDefault(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.UpsertAccount(config.Account{Name: "fresh"})
	if cfg.DefaultAccount != "fresh" {
		t.Fatalf("首个动态登录账号应成为默认账号, got %q", cfg.DefaultAccount)
	}
}

func TestLoginManagerCancelWithoutNetwork(t *testing.T) {
	mgr := NewLoginManager(config.Default(), tgclient.NewPool(zap.NewNop()), func(*tgclient.Account) error {
		return nil
	}, zap.NewNop())
	sess := mgr.newSession("cancel")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sess.cancel = cancel
	if err := mgr.Cancel(sess.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got := sess.Snap().State; got != StateCancelled {
		t.Fatalf("state = %s, want %s", got, StateCancelled)
	}
	_ = ctx
	_ = http.MethodPost
	_ = fakeLoginAccount{}
}
