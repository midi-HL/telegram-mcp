package dto

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 5, "hello…"},
		{"中文测试内容", 2, "中文…"},
		{"", 5, ""},
		{"abc", 0, "abc"},
	}
	for _, tt := range tests {
		if got := Truncate(tt.input, tt.max); got != tt.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}

func TestFormatStatus(t *testing.T) {
	text, _ := FormatStatus(&tg.UserStatusOnline{})
	if text != "在线" {
		t.Errorf("UserStatusOnline → %q, want 在线", text)
	}

	text, _ = FormatStatus(&tg.UserStatusRecently{})
	if text != "最近上线" {
		t.Errorf("UserStatusRecently → %q, want 最近上线", text)
	}

	text, _ = FormatStatus(&tg.UserStatusLastWeek{})
	if text != "一周内上线" {
		t.Errorf("UserStatusLastWeek → %q, want 一周内上线", text)
	}

	text, _ = FormatStatus(&tg.UserStatusLastMonth{})
	if text != "一个月内上线" {
		t.Errorf("UserStatusLastMonth → %q, want 一个月内上线", text)
	}

	text, _ = FormatStatus(&tg.UserStatusEmpty{})
	if text != "" {
		t.Errorf("UserStatusEmpty → %q, want 空", text)
	}
}

func TestFormatStatusOffline(t *testing.T) {
	st := &tg.UserStatusOffline{WasOnline: 1730000000}
	text, last := FormatStatus(st)
	if text == "" {
		t.Error("UserStatusOffline 应返回非空文本")
	}
	if last != 1730000000 {
		t.Errorf("lastSeen = %d, want 1730000000", last)
	}
	if !containsSub(text, "最近上线") {
		t.Errorf("文本 %q 应包含「最近上线」", text)
	}
}

func TestFormatBirthday(t *testing.T) {
	// 有年份
	got := FormatBirthday(tg.Birthday{Day: 21, Month: 5, Year: 2001})
	if got != "2001-05-21" {
		t.Errorf("完整生日 → %q, want 2001-05-21", got)
	}

	// 无年份
	got = FormatBirthday(tg.Birthday{Day: 21, Month: 5})
	if got != "5月21日" {
		t.Errorf("无年份生日 → %q, want 5月21日", got)
	}

	// 零值
	got = FormatBirthday(tg.Birthday{})
	if got != "" {
		t.Errorf("零值生日 → %q, want 空", got)
	}
}

func TestChatTypeOf(t *testing.T) {
	if got := ChatTypeOf(nil); got != "" {
		t.Errorf("nil peer → %q, want 空", got)
	}
	if got := ChatTypeOf(&tg.PeerUser{UserID: 1}); got != ChatTypeUser {
		t.Errorf("PeerUser → %q, want user", got)
	}
	if got := ChatTypeOf(&tg.PeerChat{ChatID: 1}); got != ChatTypeBasicGroup {
		t.Errorf("PeerChat → %q, want group", got)
	}
	if got := ChatTypeOf(&tg.PeerChannel{ChannelID: 1}); got != ChatTypeSupergroup {
		t.Errorf("PeerChannel → %q, want supergroup", got)
	}
}

func TestChatBriefJSON(t *testing.T) {
	c := ChatBrief{ID: 1, Title: "测试", Type: ChatTypeUser, Unread: 5, LastText: "hi"}
	if c.ID != 1 || c.Unread != 5 {
		t.Error("ChatBrief 字段赋值失败")
	}
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
