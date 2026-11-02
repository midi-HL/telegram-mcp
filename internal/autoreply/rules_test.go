package autoreply

import (
	"context"
	"testing"

	"github.com/midi-HL/telegram-mcp/internal/cache"
)

func newTestStore(t *testing.T) *RuleStore {
	t.Helper()
	dir := t.TempDir()
	s, err := cache.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return NewRuleStore(s)
}

func TestSaveAndList(t *testing.T) {
	rs := newTestStore(t)
	ctx := context.Background()

	rule := &Rule{
		Enabled:   true,
		ChatScope: []string{"user:123"},
		Keyword:   "在吗",
		Prompt:    "你好 {sender}",
	}
	id, err := rs.Save(ctx, rule)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if id == 0 {
		t.Fatal("返回的 ID 不应为 0")
	}

	list, err := rs.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("规则数 = %d, want 1", len(list))
	}
	got := list[0]
	if !got.Enabled || got.Keyword != "在吗" || got.Prompt != "你好 {sender}" {
		t.Errorf("字段错误：%+v", got)
	}
	if len(got.ChatScope) != 1 || got.ChatScope[0] != "user:123" {
		t.Errorf("ChatScope = %v", got.ChatScope)
	}
}

func TestUpdateRule(t *testing.T) {
	rs := newTestStore(t)
	ctx := context.Background()

	rule := &Rule{Enabled: true, Prompt: "旧"}
	id, _ := rs.Save(ctx, rule)

	rule.ID = id
	rule.Prompt = "新"
	rule.Enabled = false
	if _, err := rs.Save(ctx, rule); err != nil {
		t.Fatal(err)
	}

	got, err := rs.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Prompt != "新" || got.Enabled {
		t.Errorf("更新失败：%+v", got)
	}
}

func TestDeleteRule(t *testing.T) {
	rs := newTestStore(t)
	ctx := context.Background()

	id, _ := rs.Save(ctx, &Rule{Enabled: true})
	if err := rs.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	got, _ := rs.Get(ctx, id)
	if got != nil {
		t.Error("删除后不应存在")
	}
}

func TestRuleMatchDisabled(t *testing.T) {
	r := &Rule{Enabled: false}
	if r.Match(123, "任何内容") {
		t.Error("停用的规则不应匹配")
	}
}

func TestRuleMatchAll(t *testing.T) {
	r := &Rule{Enabled: true}
	if !r.Match(123, "任何内容") {
		t.Error("无范围无关键词应匹配全部")
	}
}

func TestRuleMatchScope(t *testing.T) {
	r := &Rule{Enabled: true, ChatScope: []string{"user:123"}}

	if !r.Match(123, "hi") {
		t.Error("user:123 应匹配")
	}
	if r.Match(456, "hi") {
		t.Error("user:456 不应匹配")
	}
}

func TestRuleMatchGroupScope(t *testing.T) {
	r := &Rule{Enabled: true, ChatScope: []string{"group:789"}}

	// 群 ID 是负数
	if !r.Match(-789, "hi") {
		t.Error("group:789 应匹配 chatID=-789")
	}
	if r.Match(789, "hi") {
		t.Error("正数 789 不应匹配 group:789")
	}
}

func TestRuleMatchWildcard(t *testing.T) {
	r := &Rule{Enabled: true, ChatScope: []string{"*"}}
	if !r.Match(999, "hi") {
		t.Error("通配符应匹配任何聊天")
	}
}

func TestRuleMatchKeyword(t *testing.T) {
	r := &Rule{Enabled: true, Keyword: "在吗"}

	if !r.Match(1, "你好，在吗？") {
		t.Error("包含关键词应匹配")
	}
	if r.Match(1, "你好") {
		t.Error("不含关键词不应匹配")
	}
	// 大小写不敏感
	r2 := &Rule{Enabled: true, Keyword: "Hello"}
	if !r2.Match(1, "say hello world") {
		t.Error("关键词应大小写不敏感")
	}
}

func TestRuleMatchScopeAndKeyword(t *testing.T) {
	r := &Rule{Enabled: true, ChatScope: []string{"user:1"}, Keyword: "ping"}

	if !r.Match(1, "ping") {
		t.Error("范围和关键词都满足应匹配")
	}
	if r.Match(2, "ping") {
		t.Error("范围不满足不应匹配")
	}
	if r.Match(1, "pong") {
		t.Error("关键词不满足不应匹配")
	}
}

func TestLogWriteAndRead(t *testing.T) {
	rs := newTestStore(t)
	ctx := context.Background()

	if err := rs.Log(ctx, &LogEntry{
		RuleID: 1, ChatID: 123, InMsgID: 456, OutText: "回复内容",
	}); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if err := rs.Log(ctx, &LogEntry{
		RuleID: 2, ChatID: 789, InMsgID: 999, Error: "发送失败",
	}); err != nil {
		t.Fatal(err)
	}

	logs, err := rs.RecentLogs(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 {
		t.Fatalf("日志数 = %d, want 2", len(logs))
	}
	// 最新的在前
	if logs[0].Error != "发送失败" {
		t.Errorf("最新日志应是有错误的那条，got %+v", logs[0])
	}
	if logs[1].OutText != "回复内容" {
		t.Errorf("第二条应是成功回复，got %+v", logs[1])
	}
}

func TestSimpleReply(t *testing.T) {
	ctx := context.Background()

	// 无模板
	out, err := SimpleReply(ctx, 1, "张三", "你好", &Rule{})
	if err != nil {
		t.Fatal(err)
	}
	if out != "已收到你的消息" {
		t.Errorf("默认回复 = %q", out)
	}

	// 带占位符
	rule := &Rule{Prompt: "你好 {sender}，你说的是：{text}"}
	out, err = SimpleReply(ctx, 1, "张三", "在吗", rule)
	if err != nil {
		t.Fatal(err)
	}
	want := "你好 张三，你说的是：在吗"
	if out != want {
		t.Errorf("回复 = %q, want %q", out, want)
	}
}

func TestEngineEnabled(t *testing.T) {
	e := New(Config{})
	if e.Enabled() {
		t.Error("初始应关闭")
	}
	e.SetEnabled(true)
	if !e.Enabled() {
		t.Error("开启后应为 true")
	}
	e.SetEnabled(false)
	if e.Enabled() {
		t.Error("关闭后应为 false")
	}
}

func TestEngineIgnoresWhenDisabled(t *testing.T) {
	rs := newTestStore(t)
	called := false
	e := New(Config{
		Rules: rs,
		Reply: func(ctx context.Context, chatID int64, sender, text string, rule *Rule) (string, error) {
			called = true
			return "回复", nil
		},
	})
	// 未开启
	e.Handle(context.Background(), Event{ChatID: 1, Text: "hi"})
	if called {
		t.Error("未开启时不应调用回复函数")
	}
}

func TestEngineIgnoresOutgoing(t *testing.T) {
	rs := newTestStore(t)
	ctx := context.Background()
	_, _ = rs.Save(ctx, &Rule{Enabled: true})

	called := false
	e := New(Config{
		Rules: rs,
		Reply: func(ctx context.Context, chatID int64, sender, text string, rule *Rule) (string, error) {
			called = true
			return "回复", nil
		},
	})
	e.SetEnabled(true)
	e.Handle(ctx, Event{ChatID: 1, Text: "hi", IsOut: true})
	if called {
		t.Error("自己发的消息不应触发自动回复")
	}
}

func TestEngineIgnoresEmptyText(t *testing.T) {
	rs := newTestStore(t)
	ctx := context.Background()
	_, _ = rs.Save(ctx, &Rule{Enabled: true})

	called := false
	e := New(Config{
		Rules: rs,
		Reply: func(ctx context.Context, chatID int64, sender, text string, rule *Rule) (string, error) {
			called = true
			return "回复", nil
		},
	})
	e.SetEnabled(true)
	e.Handle(ctx, Event{ChatID: 1, Text: ""})
	if called {
		t.Error("空文本不应触发")
	}
}
