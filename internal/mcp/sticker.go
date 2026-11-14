package mcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// StickerInput 是 sticker 工具的输入。
type StickerInput struct {
	// Action: search_sets / set_info / send / react / list_installed / custom_emoji
	Action  string `json:"action" jsonschema:"search_sets|set_info|send|react|list_installed|custom_emoji"`
	Query   string `json:"query,omitempty" jsonschema:"search keyword or sticker set short name"`
	SetID   string `json:"set_id,omitempty" jsonschema:"sticker set id as string to preserve int64 precision"`
	SetHash string `json:"set_access_hash,omitempty" jsonschema:"sticker set access_hash as string; required with set_id"`
	Chat    string `json:"chat,omitempty" jsonschema:"chat id string or @username"`
	Sticker string `json:"sticker,omitempty" jsonschema:"sticker emoji, document id string, or empty for first sticker"`
	MsgID   int    `json:"msg_id,omitempty" jsonschema:"message id (for react)"`
	Emoji   string `json:"emoji,omitempty" jsonschema:"reaction emoji"`
	Limit   int    `json:"limit,omitempty" jsonschema:"default 50"`
	Account string `json:"account,omitempty" jsonschema:"account name"`
}

// StickerSet 是贴纸包摘要。
// ID 用字符串传输，避免 MCP/JSON 通道用 float64 表示大整数导致末尾精度丢失。
type StickerSet struct {
	ID         string `json:"id"`
	AccessHash string `json:"access_hash,omitempty"`
	Title      string `json:"title"`
	ShortName  string `json:"short_name"`
	Count      int    `json:"count"`
	Installed  bool   `json:"installed,omitempty"`
	Archived   bool   `json:"archived,omitempty"`
	Official   bool   `json:"official,omitempty"`
	Animated   bool   `json:"animated,omitempty"`
}

// Sticker 是单个贴纸。
type Sticker struct {
	ID       string `json:"id"`
	Emoji    string `json:"emoji,omitempty"`
	MimeType string `json:"mime,omitempty"`
	Size     int64  `json:"size,omitempty"`
	Width    int    `json:"w,omitempty"`
	Height   int    `json:"h,omitempty"`
	Animated bool   `json:"animated,omitempty"`
	Video    bool   `json:"video,omitempty"`
}

// StickerOutput 是 sticker 工具的输出。
type StickerOutput struct {
	Action   string       `json:"action"`
	Sets     []StickerSet `json:"sets,omitempty"`
	Set      *StickerSet  `json:"set,omitempty"`
	Stickers []Sticker    `json:"stickers,omitempty"`
	Count    int          `json:"count,omitempty"`
	MsgID    int          `json:"msg_id,omitempty"`
	Note     string       `json:"note,omitempty"`
}

// registerStickerTools 注册 sticker 工具。
func registerStickerTools(s *mcp.Server, deps *Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "sticker",
		Description: "贴纸与表情。ID 均为字符串；推荐用 query=short_name 或 sticker=document_id 发送。action: search_sets / set_info / send / react / list_installed（别名 custom_emoji）",
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
	case "custom_emoji", "list_installed":
		return handleCustomEmoji(ctx, deps, in, out)
	default:
		out.Note = fmt.Sprintf("未知 action %q", in.Action)
		return nil, out, nil
	}
}

// searchLocalInstalledSets 在公开搜索无结果时，从已安装贴纸包里按标题/short_name 过滤。
func searchLocalInstalledSets(ctx context.Context, api *tg.Client, query string, limit int) []StickerSet {
	res, err := api.MessagesGetAllStickers(ctx, 0)
	if err != nil {
		return nil
	}
	all, ok := res.(*tg.MessagesAllStickers)
	if !ok {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(query, "@")))
	out := make([]StickerSet, 0, limit)
	for _, set := range all.Sets {
		if strings.Contains(strings.ToLower(set.Title), q) || strings.Contains(strings.ToLower(set.ShortName), q) {
			ss := projectStickerSet(set)
			ss.Installed = true
			out = append(out, ss)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

// handleStickerSearch 搜索贴纸包。上游无结果时回退本地已安装包过滤。
func handleStickerSearch(ctx context.Context, deps *Deps, in StickerInput, out StickerOutput) (*mcp.CallToolResult, StickerOutput, error) {
	acc, _, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		return stickerFail("%v", err)
	}

	q := strings.TrimSpace(in.Query)
	if q == "" {
		return stickerFail("search_sets 需要 query")
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}

	res, err := acc.API().MessagesSearchStickerSets(ctx, &tg.MessagesSearchStickerSetsRequest{
		Q:    q,
		Hash: 0, // 不带缓存 hash，避免服务端直接返回 NotModified
	})
	if err == nil {
		var found []tg.StickerSetCoveredClass
		switch v := res.(type) {
		case *tg.MessagesFoundStickerSets:
			found = v.Sets
		case *tg.MessagesFoundStickerSetsNotModified:
			out.Note = "Telegram 返回「列表未变更」；已尝试回退本地已安装包"
		}
		for _, fc := range found {
			if len(out.Sets) >= limit {
				break
			}
			if covered, ok := fc.(*tg.StickerSetCovered); ok {
				out.Sets = append(out.Sets, projectStickerSet(covered.Set))
			}
		}
	} else {
		out.Note = fmt.Sprintf("公开搜索失败：%v；已尝试回退本地已安装包", err)
	}

	if len(out.Sets) == 0 {
		out.Sets = searchLocalInstalledSets(ctx, acc.API(), q, limit)
		if len(out.Sets) > 0 && out.Note == "" {
			out.Note = "公开搜索无结果；以下是本地已安装包按标题/short_name 匹配结果"
		}
	}
	out.Count = len(out.Sets)
	if out.Count == 0 {
		out.Note = "未找到贴纸包；试试 list_installed 或更短的关键词"
	}
	return nil, out, nil
}

// stickerSetHashCache 缓存「已安装贴纸包 ID → access_hash」，避免每次 set_id 查询都拉全量。
var stickerSetHashCache = struct {
	sync.Mutex
	expires time.Time
	byID    map[int64]int64
}{}

// lookupInstalledStickerSetHash 从所有已安装贴纸包里找到 setID 对应的 access_hash。
func lookupInstalledStickerSetHash(ctx context.Context, api *tg.Client, setID int64) (int64, string, bool) {
	stickerSetHashCache.Lock()
	if stickerSetHashCache.byID != nil && time.Now().Before(stickerSetHashCache.expires) {
		if h, ok := stickerSetHashCache.byID[setID]; ok {
			stickerSetHashCache.Unlock()
			return h, "", true
		}
	}
	stickerSetHashCache.Unlock()

	res, err := api.MessagesGetAllStickers(ctx, 0)
	if err != nil {
		return 0, "", false
	}
	all, ok := res.(*tg.MessagesAllStickers)
	if !ok {
		return 0, "", false
	}
	byID := make(map[int64]int64, len(all.Sets))
	var foundHash int64
	var foundName string
	for _, set := range all.Sets {
		byID[set.ID] = set.AccessHash
		if set.ID == setID {
			foundHash, foundName = set.AccessHash, set.ShortName
		}
	}
	stickerSetHashCache.Lock()
	stickerSetHashCache.byID, stickerSetHashCache.expires = byID, time.Now().Add(5*time.Minute)
	stickerSetHashCache.Unlock()
	return foundHash, foundName, foundHash != 0
}

// resolveStickerSetInput 构造 InputStickerSet。
// 优先 short_name；只有 set_id 时自动从已安装包补 access_hash。
func resolveStickerSetInput(ctx context.Context, api *tg.Client, in StickerInput) (tg.InputStickerSetClass, error) {
	q := strings.TrimSpace(in.Query)
	setID, err := parseSetID(in.SetID)
	if err != nil {
		return nil, err
	}
	if setID != 0 {
		hash, err := parseSetHash(in.SetHash)
		if err != nil {
			return nil, err
		}
		if hash == 0 {
			if api == nil {
				return nil, fmt.Errorf("set_id 缺少 set_access_hash，且当前 Telegram 客户端不可用")
			}
			looked, _, ok := lookupInstalledStickerSetHash(ctx, api, setID)
			if !ok {
				return nil, fmt.Errorf("set_id 未在已安装贴纸包中找到；请提供 set_access_hash，或先用 query=short_name / list_installed")
			}
			hash = looked
		}
		return &tg.InputStickerSetID{ID: setID, AccessHash: hash}, nil
	}
	if q != "" {
		return &tg.InputStickerSetShortName{ShortName: strings.TrimPrefix(q, "@")}, nil
	}
	return nil, fmt.Errorf("需要 query（short name）或 set_id（自动补 access_hash）")
}

// handleStickerSetInfo 获取贴纸包详情（含所有贴纸）。
func handleStickerSetInfo(ctx context.Context, deps *Deps, in StickerInput, out StickerOutput) (*mcp.CallToolResult, StickerOutput, error) {
	acc, _, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		return stickerFail("%v", err)
	}
	inputSet, err := resolveStickerSetInput(ctx, acc.API(), in)
	if err != nil {
		return stickerFail("%v", err)
	}
	res, err := acc.API().MessagesGetStickerSet(ctx, &tg.MessagesGetStickerSetRequest{
		Stickerset: inputSet,
	})
	if err != nil {
		return stickerFail("获取贴纸包失败: %v", err)
	}
	out = projectStickerSetFull(res, out)
	return nil, out, nil
}

// projectStickerSetFull 填充贴纸包详情并返回结果，避免按值传参丢失字段。
func projectStickerSetFull(res tg.MessagesStickerSetClass, out StickerOutput) StickerOutput {
	ms, ok := res.(*tg.MessagesStickerSet)
	if !ok {
		out.Note = "贴纸包数据不可用"
		return out
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
	out.Note = fmt.Sprintf("贴纸包「%s」共 %d 个贴纸；发送时可用 sticker=<document_id> 精确指定", set.Title, set.Count)
	return out
}

// findStickerDocument 在贴纸包里按 document_id 或 emoji 找目标；空 sticker 取第一张。
func findStickerDocument(ms *tg.MessagesStickerSet, selector string) (*tg.Document, error) {
	selector = strings.TrimSpace(selector)
	docID, _ := strconv.ParseInt(selector, 10, 64)
	var first *tg.Document
	for _, dc := range ms.Documents {
		doc, ok := dc.(*tg.Document)
		if !ok {
			continue
		}
		if first == nil {
			first = doc
		}
		if docID > 0 && doc.ID == docID {
			return doc, nil
		}
		for _, attr := range doc.Attributes {
			if st, ok := attr.(*tg.DocumentAttributeSticker); ok && selector != "" && st.Alt == selector {
				return doc, nil
			}
		}
	}
	if selector == "" && first != nil {
		return first, nil
	}
	return nil, fmt.Errorf("贴纸包里没有匹配的贴纸（可用 document_id 或 emoji 精确指定）")
}

// handleStickerSend 发送贴纸。
func handleStickerSend(ctx context.Context, deps *Deps, in StickerInput, out StickerOutput) (*mcp.CallToolResult, StickerOutput, error) {
	if in.Chat == "" {
		return stickerFail("send 需要 chat；推荐 query=short_name + sticker=document_id")
	}
	acc, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		return stickerFail("%v", err)
	}
	peer, err := resolver.Resolve(ctx, in.Chat)
	if err != nil {
		return stickerFail("%v。可先用 chat list / chat resolve 刷新 peer 缓存", err)
	}

	inputSet, err := resolveStickerSetInput(ctx, acc.API(), in)
	if err != nil {
		return stickerFail("%v", err)
	}
	res, err := acc.API().MessagesGetStickerSet(ctx, &tg.MessagesGetStickerSetRequest{Stickerset: inputSet})
	if err != nil {
		return stickerFail("获取贴纸包失败: %v", err)
	}
	ms, ok := res.(*tg.MessagesStickerSet)
	if !ok {
		return stickerFail("贴纸包数据不可用")
	}
	target, err := findStickerDocument(ms, in.Sticker)
	if err != nil {
		return stickerFail("%v", err)
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
		return stickerFail("发送贴纸失败: %v", err)
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
		ID:         strconv.FormatInt(s.ID, 10),
		AccessHash: strconv.FormatInt(s.AccessHash, 10),
		Title:      s.Title,
		ShortName:  s.ShortName,
		Count:      s.Count,
		Installed:  s.InstalledDate != 0,
		Archived:   s.Archived,
		Official:   s.Official,
		Animated:   s.Masks || s.Emojis, // 近似判断
	}
}

// projectSticker 把 Document 投影成 Sticker。
func projectSticker(doc *tg.Document) Sticker {
	st := Sticker{
		ID:       strconv.FormatInt(doc.ID, 10),
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

// stickerFail 返回 MCP 错误结果。
func stickerFail(format string, args ...any) (*mcp.CallToolResult, StickerOutput, error) {
	r, _ := fail(format, args...)
	return r, StickerOutput{}, nil
}

// parseSetHash 解析贴纸包 access_hash；空字符串返回 0。
func parseSetHash(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	h, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || h <= 0 {
		return 0, fmt.Errorf("set_access_hash 必须是正整数字符串")
	}
	return h, nil
}

// parseSetID 解析字符串化的大整数 ID；空字符串返回 0。
func parseSetID(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("set_id 必须是正整数字符串（保留 int64 精度）")
	}
	return id, nil
}
