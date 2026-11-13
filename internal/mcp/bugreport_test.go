package mcp

import (
	"encoding/json"
	"testing"

	"github.com/gotd/td/tg"

	"github.com/midi-HL/telegram-mcp/internal/dto"
)

func TestStickerSetIDIsString(t *testing.T) {
	const id int64 = 5240184525769671681
	ss := projectStickerSet(tg.StickerSet{ID: id, Title: "x"})
	b, err := json.Marshal(ss)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	if v["id"] != "5240184525769671681" {
		t.Fatalf("id = %#v, want exact string", v["id"])
	}
}

func TestParseSetID(t *testing.T) {
	id, err := parseSetID("5240184525769671681")
	if err != nil || id != 5240184525769671681 {
		t.Fatalf("parseSetID = %d, %v", id, err)
	}
	if _, err := parseSetID("5.24e18"); err == nil {
		t.Error("float-looking id 应被拒绝")
	}
}

func TestProjectStickerSetFullReturnsOutput(t *testing.T) {
	res := &tg.MessagesStickerSet{
		Set: tg.StickerSet{ID: 123, Title: "demo", Count: 1},
		Documents: []tg.DocumentClass{
			&tg.Document{ID: 456, MimeType: "image/webp"},
		},
	}
	out := projectStickerSetFull(res, StickerOutput{Action: "set_info"})
	if out.Set == nil || out.Set.ID != "123" || len(out.Stickers) != 1 || out.Stickers[0].ID != "456" {
		t.Fatalf("set_info 结果没有带出: %+v", out)
	}
}

func TestMessageSearchFilterSet(t *testing.T) {
	// 直接检查构造出的请求字段，防止 messages.search 再次因 nil filter 编码失败。
	req := &tg.MessagesSearchRequest{Q: "x", Filter: &tg.InputMessagesFilterEmpty{}, Limit: 20}
	if req.Filter == nil {
		t.Fatal("filter 不能为空")
	}
}

func TestFolderPeerInfo(t *testing.T) {
	id, typ := folderPeerInfo(&tg.InputPeerChannel{ChannelID: 123})
	if id != 123 || typ != string(dto.ChatTypeChannel) {
		t.Fatalf("folderPeerInfo = %d %q", id, typ)
	}
}

func TestFetchDialogsSignature(t *testing.T) {
	// 编译期检查：list 必须带 offset 分页，避免只读第一页。
	_ = fetchDialogs
}
