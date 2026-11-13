package mcp

import (
	"context"
	"testing"

	"github.com/midi-HL/telegram-mcp/internal/config"
	"github.com/midi-HL/telegram-mcp/internal/wake"
)

func TestHandleWakeTool(t *testing.T) {
	deps := newTestDeps(t)
	cfg := config.DefaultWake()
	deps.Wake = wake.New(wake.Options{Config: cfg})

	_, out, err := handleWake(context.Background(), deps, WakeInput{Action: "on"})
	if err != nil || !out.OK || !deps.Wake.Enabled() {
		t.Fatalf("on: out=%+v err=%v enabled=%v", out, err, deps.Wake.Enabled())
	}
	_, out, _ = handleWake(context.Background(), deps, WakeInput{Action: "config", Mode: "webhook"})
	if !out.OK || deps.Wake.Config().Mode != "webhook" {
		t.Fatalf("config: out=%+v mode=%q", out, deps.Wake.Config().Mode)
	}
	_, out, _ = handleWake(context.Background(), deps, WakeInput{Action: "test", Text: "hello"})
	if !out.OK {
		t.Fatalf("test: %+v", out)
	}
	if len(deps.Wake.Pending()) != 1 {
		t.Fatalf("pending = %d, want 1", len(deps.Wake.Pending()))
	}
	_, out, _ = handleWake(context.Background(), deps, WakeInput{Action: "pending"})
	if !out.OK {
		t.Fatalf("pending action: %+v", out)
	}
	_, out, _ = handleWake(context.Background(), deps, WakeInput{Action: "ack"})
	if !out.OK || len(deps.Wake.Pending()) != 0 {
		t.Fatalf("ack: %+v", out)
	}
	_, out, _ = handleWake(context.Background(), deps, WakeInput{Action: "off"})
	if !out.OK || deps.Wake.Enabled() {
		t.Fatalf("off: %+v", out)
	}
}

func TestHandleWakeToolMissingEngine(t *testing.T) {
	deps := newTestDeps(t)
	_, out, err := handleWake(context.Background(), deps, WakeInput{Action: "status"})
	if err != nil {
		t.Fatal(err)
	}
	if out.OK || out.Message == "" {
		t.Errorf("缺少引擎应返回明确消息: %+v", out)
	}
}
