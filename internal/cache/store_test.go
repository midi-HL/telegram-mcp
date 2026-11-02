package cache

import (
	"context"
	"testing"

	"github.com/midi-HL/telegram-mcp/internal/dto"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpenCreatesSchema(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if v, ok := s.GetMeta(ctx, "schema_version"); !ok || v != "1" {
		t.Errorf("schema_version = %q, ok=%v, want 1", v, ok)
	}
}

func TestUpsertAndListChats(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	chats := []dto.ChatBrief{
		{ID: 1, Title: "群A", Type: dto.ChatTypeSupergroup, Unread: 10, LastAt: 100},
		{ID: 2, Title: "群B", Type: dto.ChatTypeUser, Unread: 0, LastAt: 200},
	}
	for _, c := range chats {
		if err := s.UpsertChat(ctx, c, ChatExtra{}); err != nil {
			t.Fatalf("UpsertChat: %v", err)
		}
	}

	list, err := s.ListChats(ctx, 10)
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
	// 应按 last_at 倒序：群B(200) 在前
	if list[0].ID != 2 {
		t.Errorf("第一条应是 ID=2（last_at 更大），got %d", list[0].ID)
	}
}

func TestUpsertChatUpdates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	c := dto.ChatBrief{ID: 1, Title: "旧名", Type: dto.ChatTypeUser}
	if err := s.UpsertChat(ctx, c, ChatExtra{}); err != nil {
		t.Fatal(err)
	}
	c.Title = "新名"
	c.Unread = 99
	if err := s.UpsertChat(ctx, c, ChatExtra{}); err != nil {
		t.Fatal(err)
	}

	full, err := s.GetChat(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if full.Title != "新名" || full.Unread != 99 {
		t.Errorf("更新失败：title=%q unread=%d", full.Title, full.Unread)
	}
}

func TestGetChatNotFound(t *testing.T) {
	s := newTestStore(t)
	full, err := s.GetChat(context.Background(), 999)
	if err != nil {
		t.Fatalf("不存在的聊天不应报错: %v", err)
	}
	if full != nil {
		t.Error("不存在的聊天应返回 nil")
	}
}

func TestUpsertAndListMessages(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	msgs := []dto.MessageFull{
		{MessageBrief: dto.MessageBrief{ID: 1, Date: 100, Text: "第一条", Out: true}},
		{MessageBrief: dto.MessageBrief{ID: 2, Date: 200, Text: "第二条"}},
	}
	if err := s.UpsertMessages(ctx, 100, msgs); err != nil {
		t.Fatalf("UpsertMessages: %v", err)
	}

	list, err := s.ListMessages(ctx, 100, 10)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
	// 应按 date 倒序
	if list[0].ID != 2 {
		t.Errorf("第一条应是 ID=2，got %d", list[0].ID)
	}
	if !list[1].Out {
		t.Error("ID=1 应该是 out=true")
	}
}

func TestMessageEviction(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// 插入超过上限的消息
	total := MaxMessagesPerChat + 50
	msgs := make([]dto.MessageFull, 0, total)
	for i := 0; i < total; i++ {
		msgs = append(msgs, dto.MessageFull{
			MessageBrief: dto.MessageBrief{ID: i + 1, Date: int64(i)},
		})
	}
	if err := s.UpsertMessages(ctx, 1, msgs); err != nil {
		t.Fatal(err)
	}

	n, err := s.CountMessages(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if n != MaxMessagesPerChat {
		t.Errorf("消息数 = %d, want %d（应淘汰到上限）", n, MaxMessagesPerChat)
	}

	// 应该保留最新的（date 最大的）
	list, _ := s.ListMessages(ctx, 1, 1)
	if len(list) > 0 && list[0].ID != total {
		t.Errorf("保留的最新消息 ID = %d, want %d", list[0].ID, total)
	}
}

func TestUpsertUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p := dto.UserProfile{
		ID:         123,
		FirstName:  "张三",
		LastName:   "李四",
		Username:   "zhangsan",
		Phone:      "+8613800138000",
		Bio:        "简介",
		Status:     "在线",
		Birthday:   "5月21日",
		IsPremium:  true,
		IsContact:  true,
		CommonChats: 3,
	}
	if err := s.UpsertUser(ctx, p); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	got, err := s.GetUser(ctx, 123)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got == nil {
		t.Fatal("用户不存在")
	}
	if got.FirstName != "张三" || got.Username != "zhangsan" {
		t.Errorf("字段错误：%+v", got)
	}
	if !got.IsPremium || !got.IsContact {
		t.Error("布尔字段错误")
	}
	if got.CommonChats != 3 {
		t.Errorf("CommonChats = %d, want 3", got.CommonChats)
	}
}

func TestStats(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_ = s.UpsertChat(ctx, dto.ChatBrief{ID: 1, Title: "a"}, ChatExtra{})
	_ = s.UpsertMessages(ctx, 1, []dto.MessageFull{
		{MessageBrief: dto.MessageBrief{ID: 1, Date: 1}},
	})

	stats, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats["chats"] != 1 {
		t.Errorf("chats = %v, want 1", stats["chats"])
	}
	if stats["messages"] != 1 {
		t.Errorf("messages = %v, want 1", stats["messages"])
	}
}

func TestAccessHash(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_ = s.UpsertChat(ctx, dto.ChatBrief{ID: 1, Title: "a"}, ChatExtra{AccessHash: 12345})
	h, ok := s.AccessHash(ctx, 1)
	if !ok || h != 12345 {
		t.Errorf("AccessHash = %d, ok=%v, want 12345", h, ok)
	}

	_, ok = s.AccessHash(ctx, 999)
	if ok {
		t.Error("不存在的聊天应返回 ok=false")
	}
}
