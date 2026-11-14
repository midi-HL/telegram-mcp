package mcp

import (
	"encoding/json"
	"testing"

	"github.com/gotd/td/tg"

	"github.com/midi-HL/telegram-mcp/internal/dto"
)

func TestChatBriefIDIsString(t *testing.T) {
	b, _ := projectDialog(dialogItem{ID: 9007199254740993, Title: "x"})
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	if v["id"] != "9007199254740993" {
		t.Fatalf("id = %#v, want exact string", v["id"])
	}
	if v["metadata_missing"] != true {
		t.Fatalf("缺失元数据应显式标记: %s", data)
	}
}

func TestFolderPeerIDSchemaConsistent(t *testing.T) {
	p := dto.FolderPeer{ID: "6834764154"}
	data, _ := json.Marshal(p)
	var v map[string]any
	_ = json.Unmarshal(data, &v)
	if v["id"] != "6834764154" {
		t.Fatalf("folder peer id = %#v, want string", v["id"])
	}
}

func TestResolveStickerSetInput(t *testing.T) {
	if _, err := resolveStickerSetInput(StickerInput{SetID: "5240184525769671681"}); err == nil {
		t.Error("set_id 缺 access_hash 时应明确报错，避免 STICKERSET_INVALID")
	}
	in, err := resolveStickerSetInput(StickerInput{Query: "HotCherry"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := in.(*tg.InputStickerSetShortName); !ok {
		t.Fatalf("short_name 应使用 InputStickerSetShortName, got %T", in)
	}
	in, err = resolveStickerSetInput(StickerInput{SetID: "123", SetHash: "456"})
	if err != nil {
		t.Fatal(err)
	}
	v, ok := in.(*tg.InputStickerSetID)
	if !ok || v.ID != 123 || v.AccessHash != 456 {
		t.Fatalf("set_id+hash 构造错误: %#v", in)
	}
}

func TestFindStickerDocumentByIDAndEmoji(t *testing.T) {
	ms := &tg.MessagesStickerSet{Documents: []tg.DocumentClass{
		&tg.Document{ID: 111, Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeSticker{Alt: "😀"}}},
		&tg.Document{ID: 222, Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeSticker{Alt: "🐱"}}},
	}}
	if d, err := findStickerDocument(ms, "222"); err != nil || d.ID != 222 {
		t.Fatalf("document_id 查找失败: %v %#v", err, d)
	}
	if d, err := findStickerDocument(ms, "🐱"); err != nil || d.ID != 222 {
		t.Fatalf("emoji 查找失败: %v %#v", err, d)
	}
	if d, err := findStickerDocument(ms, ""); err != nil || d.ID != 111 {
		t.Fatalf("空 selector 应取第一张: %v %#v", err, d)
	}
}
