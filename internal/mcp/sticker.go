package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// StickerInput 是 sticker 工具的输入。
type StickerInput struct {
	// Action: search_sets / set_info / send / react / custom_emoji
	Action  string `json:"action" jsonschema:"search_sets|set_info|send|react|custom_emoji"`
	Query   string `json:"query,omitempty" jsonschema:"search query or short name"`
	SetID   int64  `json:"set_id,omitempty" jsonschema:"sticker set id"`
	Chat    string `json:"chat,omitempty" jsonschema:"chat id"`
	Sticker string `json:"sticker,omitempty" jsonschema:"sticker emoji or doc id"`
	MsgID   int    `json:"msg_id,omitempty" jsonschema:"message id (for react)"`
	Emoji   string `json:"emoji,omitempty" jsonschema:"reaction emoji"`
	Limit   int    `json:"limit,omitempty" jsonschema:"default 50"`
	Account string `json:"account,omitempty" jsonschema:"account name"`
}

// StickerSet 是贴纸包摘要。
type StickerSet struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	ShortName string `json:"short_name"`
	Count     int    `json:"count"`
	Installed bool   `json:"installed,omitempty"`
	Archived  bool   `json:"archived,omitempty"`
	Official  bool   `json:"official,omitempty"`
	Animated  bool   `json:"animated,omitempty"`
}

// Sticker 是单个贴纸。
type Sticker struct {
	ID        int64  `json:"id"`
	Emoji     string `json:"emoji,omitempty"`
	MimeType  string `json:"mime,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Width     int    `json:"w,omitempty"`
	Height    int    `json:"h,omitempty"`
	Animated  bool   `json:"animated,omitempty"`
	Video     bool   `json:"video,omitempty"`
}

// StickerOutput 是 sticker 工具的输出。
type StickerOutput struct {
	Action  string       `json:"action"`
	Sets    []StickerSet `json:"sets,omitempty"`
	Set     *StickerSet  `json:"set,omitempty"`
	Stickers []Sticker   `json:"stickers,omitempty"`
	Count   int          `json:"count,omitempty"`
	MsgID   int          `json:"msg_id,omitempty"`
	Note    string       `json:"note,omitempty"`
}

// registerStickerTools 注册 sticker 工具。
func registerStickerTools(s *mcp.Server, deps *Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "sticker",
		Description: "贴纸与表情。action: search_sets 搜索贴纸包 / set_info 包详情 / send 发贴纸 / react 加表情反应 / custom_emoji 自定义 emoji",
		Annotations: &mcp.ToolAnnotations{Title: "贴纸", ReadOnlyHint: false},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in StickerInput) (*mcp.CallToolResult, StickerOutput, error) {
		return handleSticker(ctx, deps, in)
	})
}

func handleSticker(ctx context.Context, deps *Deps, in StickerInput) (*mcp.CallToolResult, StickerOutput, error) {
	out := StickerOutput{Action: in.Action}

	switch in.Action {
	case "search_sets":
		return handleStickerSearch(ctx, deps, in, out)
	case "set_info":
		return handleStickerSetInfo(ctx, deps, in, out)
	case "send":
		return handleStickerSend(ctx, deps, in, out)
	case "react":
		return handleStickerReact(ctx, deps, in, out)
	case "custom_emoji":
		return handleCustomEmoji(ctx, deps, in, out)
	default:
		out.Note = fmt.Sprintf("未知 action %q", in.Action)
		return nil, out, nil
	}
}

// handleStickerSearch 搜索贴纸包。
func handleStickerSearch(ctx context.Context, deps *Deps, in StickerInput, out StickerOutput) (*mcp.CallToolResult, StickerOutput, error) {
	acc, _, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}

	res, err := acc.API().MessagesSearchStickerSets(ctx, &tg.MessagesSearchStickerSetsRequest{
		Q: in.Query,
	})
	if err != nil {
		out.Note = fmt.Sprintf("搜索失败: %v", err)
		return nil, out, nil
	}

	var found []tg.StickerSetCoveredClass
	switch v := res.(type) {
	case *tg.MessagesFoundStickerSets:
		found = v.Sets
	case *tg.MessagesFoundStickerSetsNotModified:
		out.Note = "贴纸包列表未变更（用 set_info 查详情）"
		return nil, out, nil
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	for _, fc := range found {
		if len(out.Sets) >= limit {
			break
		}
		if covered, ok := fc.(*tg.StickerSetCovered); ok {
			out.Sets = append(out.Sets, projectStickerSet(covered.Set))
		}
	}
	out.Count = len(out.Sets)
	out.Note = fmt.Sprintf("找到 %d 个贴纸包", out.Count)
	return nil, out, nil
}

// handleStickerSetInfo 获取贴纸包详情（含所有贴纸）。
func handleStickerSetInfo(ctx context.Context, deps *Deps, in StickerInput, out StickerOutput) (*mcp.CallToolResult, StickerOutput, error) {
	acc, _, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}

	inputSet := &tg.InputStickerSetID{ID: in.SetID}
	if in.SetID == 0 && in.Query != "" {
		inputSet = nil
		// 用 short name 查
		res, err := acc.API().MessagesGetStickerSet(ctx, &tg.MessagesGetStickerSetRequest{
			Stickerset: &tg.InputStickerSetShortName{ShortName: in.Query},
		})
		if err != nil {
			out.Note = fmt.Sprintf("获取贴纸包失败: %v", err)
			return nil, out, nil
		}
		projectStickerSetFull(res, out)
		return nil, out, nil
	}

	res, err := acc.API().MessagesGetStickerSet(ctx, &tg.MessagesGetStickerSetRequest{
		Stickerset: inputSet,
	})
	if err != nil {
		out.Note = fmt.Sprintf("获取贴纸包失败: %v", err)
		return nil, out, nil
	}
	projectStickerSetFull(res, out)
	return nil, out, nil
}

// projectStickerSetFull 填充贴纸包详情。
func projectStickerSetFull(res tg.MessagesStickerSetClass, out StickerOutput) {
	ms, ok := res.(*tg.MessagesStickerSet)
	if !ok {
		out.Note = "贴纸包数据不可用"
		return
	}
	set := projectStickerSet(ms.Set)
	out.Set = &set

	limit := 100
	for _, docClass := range ms.Documents {
		if len(out.Stickers) >= limit {
			break
		}
		doc, ok := docClass.(*tg.Document)
		if !ok {
			continue
		}
		out.Stickers = append(out.Stickers, projectSticker(doc))
	}
	out.Count = len(out.Stickers)
	out.Note = fmt.Sprintf("贴纸包「%s」共 %d 个贴纸", set.Title, set.Count)
}

// handleStickerSend 发送贴纸。
func handleStickerSend(ctx context.Context, deps *Deps, in StickerInput, out StickerOutput) (*mcp.CallToolResult, StickerOutput, error) {
	if in.Chat == "" {
		out.Note = "send 需要 chat 和 sticker（贴纸 ID 或 emoji）"
		return nil, out, nil
	}
	acc, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	peer, err := resolver.Resolve(ctx, in.Chat)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}

	// 从贴纸包找匹配的贴纸
	if in.SetID == 0 {
		out.Note = "send 需要 set_id（先用 search_sets 找贴纸包）"
		return nil, out, nil
	}
	res, err := acc.API().MessagesGetStickerSet(ctx, &tg.MessagesGetStickerSetRequest{
		Stickerset: &tg.InputStickerSetID{ID: in.SetID},
	})
	if err != nil {
		out.Note = fmt.Sprintf("获取贴纸包失败: %v", err)
		return nil, out, nil
	}
	ms, ok := res.(*tg.MessagesStickerSet)
	if !ok {
		out.Note = "贴纸包数据不可用"
		return nil, out, nil
	}

	// 找匹配 emoji 的贴纸
	var target *tg.Document
	for _, dc := range ms.Documents {
		doc, ok := dc.(*tg.Document)
		if !ok {
			continue
		}
		for _, attr := range doc.Attributes {
			if st, ok := attr.(*tg.DocumentAttributeSticker); ok {
				if in.Sticker == "" || st.Alt == in.Sticker {
					target = doc
					break
				}
			}
		}
		if target != nil {
			break
		}
	}
	if target == nil {
		out.Note = "贴纸包里没有匹配的贴纸"
		return nil, out, nil
	}

	media := &tg.InputMediaDocument{
		ID: &tg.InputDocument{
			ID:            target.ID,
			AccessHash:    target.AccessHash,
			FileReference: target.FileReference,
		},
	}
	res2, err := acc.API().MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
		Peer:     peer,
		Media:    media,
		RandomID: randomID(),
	})
	if err != nil {
		out.Note = fmt.Sprintf("发送贴纸失败: %v", err)
		return nil, out, nil
	}
	if id, _ := extractSentMessageID(res2); id != 0 {
		out.MsgID = id
	}
	out.Note = "贴纸已发送"
	return nil, out, nil
}

// handleStickerReact 给消息加表情反应。
func handleStickerReact(ctx context.Context, deps *Deps, in StickerInput, out StickerOutput) (*mcp.CallToolResult, StickerOutput, error) {
	if in.Chat == "" || in.MsgID == 0 || in.Emoji == "" {
		out.Note = "react 需要 chat、msg_id、emoji"
		return nil, out, nil
	}
	acc, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	peer, err := resolver.Resolve(ctx, in.Chat)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	_, err = acc.API().MessagesSendReaction(ctx, &tg.MessagesSendReactionRequest{
		Peer:     peer,
		MsgID:    in.MsgID,
		Reaction: []tg.ReactionClass{&tg.ReactionEmoji{Emoticon: in.Emoji}},
	})
	if err != nil {
		out.Note = fmt.Sprintf("添加反应失败: %v", err)
		return nil, out, nil
	}
	out.MsgID = in.MsgID
	out.Note = "已添加反应 " + in.Emoji
	return nil, out, nil
}

// handleCustomEmoji 获取自定义 emoji。
func handleCustomEmoji(ctx context.Context, deps *Deps, in StickerInput, out StickerOutput) (*mcp.CallToolResult, StickerOutput, error) {
	acc, _, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	// 拉取所有已安装贴纸包，筛出自定义 emoji
	res, err := acc.API().MessagesGetAllStickers(ctx, 0)
	if err != nil {
		out.Note = fmt.Sprintf("获取贴纸失败: %v", err)
		return nil, out, nil
	}
	all, ok := res.(*tg.MessagesAllStickers)
	if !ok {
		out.Note = "没有已安装的贴纸包"
		return nil, out, nil
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	for _, s := range all.Sets {
		if len(out.Sets) >= limit {
			break
		}
		ss := projectStickerSet(s)
		if ss.Animated || ss.Count > 0 {
			out.Sets = append(out.Sets, ss)
		}
	}
	out.Count = len(out.Sets)
	out.Note = fmt.Sprintf("共 %d 个已安装贴纸包", out.Count)
	return nil, out, nil
}

// projectStickerSet 把 StickerSet 投影。
func projectStickerSet(s tg.StickerSet) StickerSet {
	return StickerSet{
		ID:        s.ID,
		Title:     s.Title,
		ShortName: s.ShortName,
		Count:     s.Count,
		Installed: s.InstalledDate != 0,
		Archived:  s.Archived,
		Official:  s.Official,
		Animated:  s.Masks || s.Emojis, // 近似判断
	}
}

// projectSticker 把 Document 投影成 Sticker。
func projectSticker(doc *tg.Document) Sticker {
	st := Sticker{
		ID:       doc.ID,
		MimeType: doc.MimeType,
		Size:     doc.Size,
	}
	for _, attr := range doc.Attributes {
		switch a := attr.(type) {
		case *tg.DocumentAttributeSticker:
			st.Emoji = a.Alt
		case *tg.DocumentAttributeImageSize:
			st.Width = a.W
			st.Height = a.H
		}
	}
	if strings.Contains(doc.MimeType, "tgs") {
		st.Animated = true
	}
	if strings.Contains(doc.MimeType, "webm") {
		st.Video = true
	}
	return st
}
