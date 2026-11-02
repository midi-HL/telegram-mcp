package mcp

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/midi-HL/telegram-mcp/internal/config"
	tgclient "github.com/midi-HL/telegram-mcp/internal/telegram"
)

// newTestDeps 构造不依赖网络/账号的测试依赖。
func newTestDeps(t *testing.T) *Deps {
	t.Helper()
	return &Deps{
		Config:         config.Default(),
		Pool:           tgclient.NewPool(zap.NewNop()),
		Logger:         zap.NewNop(),
		DefaultAccount: "main",
	}
}

func TestHandleSystemDoctor(t *testing.T) {
	deps := newTestDeps(t)
	_, out, err := handleSystem(context.Background(), deps, SystemInput{Action: "doctor"})
	if err != nil {
		t.Fatalf("handleSystem(doctor) error: %v", err)
	}
	if out.Action != "doctor" {
		t.Errorf("Action = %q, want doctor", out.Action)
	}
	data, ok := out.Data.(map[string]any)
	if !ok {
		t.Fatalf("Data 类型 = %T, want map[string]any", out.Data)
	}
	if _, exists := data["go_version"]; !exists {
		t.Error("doctor 输出缺少 go_version")
	}
	if data["cgo_enabled"] != false {
		t.Errorf("cgo_enabled = %v, want false（纯 Go 构建）", data["cgo_enabled"])
	}
}

func TestHandleSystemAccountsEmpty(t *testing.T) {
	deps := newTestDeps(t)
	_, out, err := handleSystem(context.Background(), deps, SystemInput{Action: "accounts"})
	if err != nil {
		t.Fatalf("handleSystem(accounts) error: %v", err)
	}
	list, ok := out.Data.([]map[string]any)
	if !ok {
		t.Fatalf("Data 类型 = %T, want []map[string]any", out.Data)
	}
	if len(list) != 0 {
		t.Errorf("空池 accounts len = %d, want 0", len(list))
	}
}

func TestHandleSystemStatusMissingAccount(t *testing.T) {
	deps := newTestDeps(t)
	_, out, err := handleSystem(context.Background(), deps, SystemInput{Action: "status", Account: "nonexistent"})
	if err != nil {
		t.Fatalf("handleSystem(status) 不应返回 error: %v", err)
	}
	if !strings.Contains(out.Message, "不存在") {
		t.Errorf("Message = %q, 应包含「不存在」", out.Message)
	}
}

func TestHandleSystemUnknownAction(t *testing.T) {
	deps := newTestDeps(t)
	_, out, err := handleSystem(context.Background(), deps, SystemInput{Action: "bogus"})
	if err != nil {
		t.Fatalf("未知 action 不应返回 error: %v", err)
	}
	if !strings.Contains(out.Message, "未知 action") {
		t.Errorf("Message = %q, 应包含「未知 action」", out.Message)
	}
}

func TestHandleSystemDefaultAction(t *testing.T) {
	deps := newTestDeps(t)
	_, out, err := handleSystem(context.Background(), deps, SystemInput{})
	if err != nil {
		t.Fatalf("空 action 应默认为 status: %v", err)
	}
	if out.Action != "status" {
		t.Errorf("空 action 时 Action = %q, want status", out.Action)
	}
}

func TestNewServerRegistersTools(t *testing.T) {
	deps := newTestDeps(t)
	s := NewServer(deps)
	if s == nil {
		t.Fatal("NewServer 返回 nil")
	}
}
