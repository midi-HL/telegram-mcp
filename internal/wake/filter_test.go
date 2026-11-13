package wake

import (
	"context"
	"testing"

	"github.com/midi-HL/telegram-mcp/internal/config"
)

func TestWakeChatFilter(t *testing.T) {
	cfg := config.DefaultWake()
	cfg.Enabled = true
	cfg.Mode = "webhook"
	cfg.WebhookURL = "http://127.0.0.1:1/unreachable"
	cfg.ChatIDs = []int64{77}
	e := newTestEngine(t, cfg, nil, nil, nil)
	e.Handle(context.Background(), Event{ChatID: 1, MsgID: 1})
	e.Handle(context.Background(), Event{ChatID: 77, MsgID: 2})
	if len(e.Pending()) != 1 || e.Pending()[0].Event.ChatID != 77 {
		t.Fatalf("只应投递命中 chat_id 的事件: %+v", e.Pending())
	}
}

func TestWakeSenderFilter(t *testing.T) {
	cfg := config.DefaultWake()
	cfg.Enabled = true
	cfg.Mode = "webhook"
	cfg.WebhookURL = "http://127.0.0.1:1/unreachable"
	cfg.SenderIDs = []int64{9}
	e := newTestEngine(t, cfg, nil, nil, nil)
	e.Handle(context.Background(), Event{ChatID: 1, MsgID: 1, SenderID: 1})
	e.Handle(context.Background(), Event{ChatID: 1, MsgID: 2, SenderID: 9})
	if len(e.Pending()) != 1 || e.Pending()[0].Event.SenderID != 9 {
		t.Fatalf("只应投递命中 sender_id 的事件: %+v", e.Pending())
	}
}

func TestWakeFolderFilterDynamicLookup(t *testing.T) {
	cfg := config.DefaultWake()
	cfg.Enabled = true
	cfg.Mode = "webhook"
	cfg.WebhookURL = "http://127.0.0.1:1/unreachable"
	cfg.FolderIDs = []int{2}
	called := 0
	e := New(Options{
		Config: cfg,
		FolderLookup: func(_ context.Context, account string, ids []int) map[int64]bool {
			called++
			if account != "acc" || len(ids) != 1 || ids[0] != 2 {
				t.Fatalf("folder lookup 参数异常: %q %v", account, ids)
			}
			return map[int64]bool{88: true}
		},
	})
	e.Handle(context.Background(), Event{Account: "acc", ChatID: 1, MsgID: 1})
	e.Handle(context.Background(), Event{Account: "acc", ChatID: 88, MsgID: 2})
	if called != 2 || len(e.Pending()) != 1 || e.Pending()[0].Event.ChatID != 88 {
		t.Fatalf("分类成员应动态展开并只投递命中项: called=%d pending=%+v", called, e.Pending())
	}
}

func TestWakeTestBypassesFilters(t *testing.T) {
	cfg := config.DefaultWake()
	cfg.Enabled = true
	cfg.ChatIDs = []int64{999}
	e := newTestEngine(t, cfg, nil, nil, nil)
	note := e.Test(context.Background(), "manual")
	if note.ID == "" {
		t.Fatal("测试唤醒应始终投递，不受过滤影响")
	}
}
