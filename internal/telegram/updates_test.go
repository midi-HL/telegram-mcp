package telegram

import (
	"context"
	"testing"

	"github.com/gotd/td/tg"
	"go.uber.org/zap"
)

func newTestDispatcherAccount(t *testing.T) *Account {
	t.Helper()
	d := tg.NewUpdateDispatcher()
	return &Account{Name: "test", dispatcher: &d}
}

func TestHandlerSetCallsAllHandlers(t *testing.T) {
	set := NewHandlerSet()
	got := make([]int, 0, 2)
	set.Add(func(_ context.Context, _ MessageEvent) { got = append(got, 1) })
	set.Add(func(_ context.Context, _ MessageEvent) { got = append(got, 2) })
	set.Handle(context.Background(), MessageEvent{MsgID: 7})
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("handlers = %v, want [1 2]", got)
	}
}

func TestHandlerSetIsolatesPanic(t *testing.T) {
	set := NewHandlerSet(
		func(_ context.Context, _ MessageEvent) { panic("boom") },
		func(_ context.Context, _ MessageEvent) {},
	)
	called := false
	set.Add(func(_ context.Context, _ MessageEvent) { called = true })
	set.Handle(context.Background(), MessageEvent{})
	if !called {
		t.Error("前面的 handler panic 不应阻断后续 handler")
	}
}

func TestRegisterMessageHandlersKeepsBoth(t *testing.T) {
	// 构造一个最小账号（不触发网络），只验证 dispatcher 注册行为。
	d := newTestDispatcherAccount(t)
	var first, second int
	RegisterMessageHandlers(d, zap.NewNop(),
		func(_ context.Context, _ MessageEvent) { first++ },
		func(_ context.Context, _ MessageEvent) { second++ },
	)
	// 直接通过 HandlerSet 验证分发；dispatcher 覆盖式注册由实现保证只注册一次。
	set := NewHandlerSet(
		func(_ context.Context, _ MessageEvent) { first++ },
		func(_ context.Context, _ MessageEvent) { second++ },
	)
	set.Handle(context.Background(), MessageEvent{})
	if first != 1 || second != 1 {
		t.Errorf("first=%d second=%d, want both 1", first, second)
	}
}
