package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/midi-HL/telegram-mcp/internal/dto"
	tgclient "github.com/midi-HL/telegram-mcp/internal/telegram"
)

// MessageInput 是 message 工具的输入。
type MessageInput struct {
	// Action: send / edit / delete / forward / search / history / react / pin
	Action string `json:"action" jsonschema:"send|edit|delete|forward|search|history|react|pin"`
	Chat   string `json:"chat,omitempty" jsonschema:"chat id or @username"`
	Text   string `json:"text,omitempty" jsonschema:"text"`
	MsgID  int    `json:"msg_id,omitempty" jsonschema:"msg id"`
	To     string `json:"to,omitempty" jsonschema:"target chat"`
	Query  string `json:"query,omitempty" jsonschema:"query"`
	Limit  int    `json:"limit,omitempty" jsonschema:"default 20, max 100"`
	Before int    `json:"before_id,omitempty" jsonschema:"before msg id"`
	ReplyTo int   `json:"reply_to,omitempty" jsonschema:"reply to"`
	Silent  bool  `json:"silent,omitempty" jsonschema:"silent"`
	Emoji   string `json:"emoji,omitempty" jsonschema:"emoji"`
	Expand  []string `json:"expand,omitempty" jsonschema:"full"`
	Account string `json:"account,omitempty" jsonschema:"account name"`
}

// MessageOutput 是 message 工具的输出。
type MessageOutput struct {
	Action   string              `json:"action"`
	Messages []dto.MessageBrief  `json:"messages,omitempty"`
	Message  *dto.MessageFull    `json:"message,omitempty"`
	MsgID    int                 `json:"msg_id,omitempty"`
	Count    int                 `json:"count,omitempty"`
	Note     string              `json:"note,omitempty"`
	Message2 string              `json:"message_text,omitempty"`
}

// registerMessageTools 注册 message 工具。
func registerMessageTools(s *mcp.Server, deps *Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "message",
		Description: "消息收发。action: send 发送 / edit 编辑 / delete 删除 / forward 转发 / search 搜索 / history 历史 / react 反应 / pin 置顶",
		Annotations: &mcp.ToolAnnotations{Title: "消息", ReadOnlyHint: false},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in MessageInput) (*mcp.CallToolResult, MessageOutput, error) {
		return handleMessage(ctx, deps, in)
	})
}

func handleMessage(ctx context.Context, deps *Deps, in MessageInput) (*mcp.CallToolResult, MessageOutput, error) {
	out := MessageOutput{Action: in.Action}

	switch in.Action {
	case "send":
		return handleSend(ctx, deps, in, out)
	case "edit":
		return handleEdit(ctx, deps, in, out)
	case "delete":
		return handleDelete(ctx, deps, in, out)
	case "forward":
		return handleForward(ctx, deps, in, out)
	case "search":
		return handleSearch(ctx, deps, in, out)
	case "", "history":
		out.Action = "history"
		return handleHistory(ctx, deps, in, out)
	case "react":
		return handleReact(ctx, deps, in, out)
	case "pin":
		return handlePin(ctx, deps, in, out)
	default:
		out.Note = fmt.Sprintf("未知 action %q（支持 send/edit/delete/forward/search/history/react/pin）", in.Action)
		return nil, out, nil
	}
}

// handleSend 发送消息。
func handleSend(ctx context.Context, deps *Deps, in MessageInput, out MessageOutput) (*mcp.CallToolResult, MessageOutput, error) {
	if in.Chat == "" {
		out.Note = "send 需要 chat 参数"
		return nil, out, nil
	}
	if in.Text == "" {
		out.Note = "send 需要 text 参数"
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

	req := &tg.MessagesSendMessageRequest{
		Peer:     peer,
		Message:  in.Text,
		RandomID: randomID(),
		Silent:   in.Silent,
	}
	if in.ReplyTo > 0 {
		req.ReplyTo = &tg.InputReplyToMessage{ReplyToMsgID: in.ReplyTo}
	}

	res, err := acc.API().MessagesSendMessage(ctx, req)
	if err != nil {
		// 隐私限制自动降级
		if tgclient.IsPrivacyRestricted(err) {
			out.Note = "对方隐私设置限制，需要先加联系人。请用 contact add 添加对方后再试。"
			return nil, out, nil
		}
		out.Note = fmt.Sprintf("发送失败: %v", err)
		return nil, out, nil
	}

	// 提取消息 ID
	id, msg := extractSentMessageID(res)
	out.MsgID = id
	if msg != nil {
		out.Message = projectMessage(msg, nil)
	}
	out.Note = "已发送"
	return nil, out, nil
}

// handleEdit 编辑消息。
func handleEdit(ctx context.Context, deps *Deps, in MessageInput, out MessageOutput) (*mcp.CallToolResult, MessageOutput, error) {
	if in.Chat == "" || in.MsgID == 0 {
		out.Note = "edit 需要 chat 和 msg_id"
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

	res, err := acc.API().MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
		Peer:    peer,
		ID:      in.MsgID,
		Message: in.Text,
	})
	if err != nil {
		out.Note = fmt.Sprintf("编辑失败: %v", err)
		return nil, out, nil
	}
	_ = res
	out.MsgID = in.MsgID
	out.Note = "已编辑"
	return nil, out, nil
}

// handleDelete 删除消息。
func handleDelete(ctx context.Context, deps *Deps, in MessageInput, out MessageOutput) (*mcp.CallToolResult, MessageOutput, error) {
	if in.Chat == "" || in.MsgID == 0 {
		out.Note = "delete 需要 chat 和 msg_id"
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

	// 频道/超级群用 channels.deleteMessages，其他用 messages.deleteMessages
	if ch, ok := peer.(*tg.InputPeerChannel); ok {
		_, err = acc.API().ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{
			Channel: &tg.InputChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash},
			ID:      []int{in.MsgID},
		})
	} else {
		_, err = acc.API().MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
			ID:     []int{in.MsgID},
			Revoke: true,
		})
	}
	if err != nil {
		out.Note = fmt.Sprintf("删除失败: %v", err)
		return nil, out, nil
	}
	out.MsgID = in.MsgID
	out.Note = "已删除"
	return nil, out, nil
}

// handleForward 转发消息。
func handleForward(ctx context.Context, deps *Deps, in MessageInput, out MessageOutput) (*mcp.CallToolResult, MessageOutput, error) {
	if in.Chat == "" || in.To == "" || in.MsgID == 0 {
		out.Note = "forward 需要 chat（来源）、to（目标）、msg_id"
		return nil, out, nil
	}
	acc, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	fromPeer, err := resolver.Resolve(ctx, in.Chat)
	if err != nil {
		out.Note = fmt.Sprintf("解析来源: %v", err)
		return nil, out, nil
	}
	toPeer, err := resolver.Resolve(ctx, in.To)
	if err != nil {
		out.Note = fmt.Sprintf("解析目标: %v", err)
		return nil, out, nil
	}

	res, err := acc.API().MessagesForwardMessages(ctx, &tg.MessagesForwardMessagesRequest{
		FromPeer: fromPeer,
		ToPeer:   toPeer,
		ID:       []int{in.MsgID},
		RandomID: []int64{randomID()},
	})
	if err != nil {
		out.Note = fmt.Sprintf("转发失败: %v", err)
		return nil, out, nil
	}
	id, _ := extractSentMessageID(res)
	out.MsgID = id
	out.Note = "已转发"
	return nil, out, nil
}

// handleSearch 搜索消息。
func handleSearch(ctx context.Context, deps *Deps, in MessageInput, out MessageOutput) (*mcp.CallToolResult, MessageOutput, error) {
	acc, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	req := &tg.MessagesSearchRequest{
		Q:     in.Query,
		Limit: limit,
	}
	if in.Chat != "" {
		peer, err := resolver.Resolve(ctx, in.Chat)
		if err != nil {
			out.Note = err.Error()
			return nil, out, nil
		}
		req.Peer = peer
	} else {
		req.Peer = &tg.InputPeerEmpty{}
	}

	res, err := acc.API().MessagesSearch(ctx, req)
	if err != nil {
		out.Note = fmt.Sprintf("搜索失败: %v", err)
		return nil, out, nil
	}
	msgs, users := extractMessages(res)
	out.Messages = projectMessages(msgs, in.Expand)
	out.Messages = wrapMessagesWithTrust(ctx, deps, msgs, out.Messages)
	_ = users
	out.Count = len(out.Messages)
	out.Note = fmt.Sprintf("找到 %d 条", out.Count)
	return nil, out, nil
}

// handleHistory 读取历史消息。
func handleHistory(ctx context.Context, deps *Deps, in MessageInput, out MessageOutput) (*mcp.CallToolResult, MessageOutput, error) {
	if in.Chat == "" {
		out.Note = "history 需要 chat 参数"
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
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	res, err := acc.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:     peer,
		Limit:    limit,
		OffsetID: in.Before,
	})
	if err != nil {
		out.Note = fmt.Sprintf("读取历史失败: %v", err)
		return nil, out, nil
	}
	msgs, users, chats := extractMessagesFull(res)
	out.Messages = projectMessagesWithEntities(msgs, users, chats, in.Expand)
	// ★ 应用信任包装
	out.Messages = wrapMessagesWithTrust(ctx, deps, msgs, out.Messages)
	// ★ 展开引用摘要（自动拉取被引用消息）
	expandReplies(ctx, acc, msgs, out.Messages, users)

	// 写入缓存
	if deps.Cache != nil {
		fulls := projectMessagesFull(msgs, users, chats)
		_ = deps.Cache.UpsertMessages(ctx, PeerIDOf(peer), fulls)
	}

	out.Count = len(out.Messages)
	out.Note = fmt.Sprintf("返回 %d 条", out.Count)
	return nil, out, nil
}

// handleReact 给消息加反应。
func handleReact(ctx context.Context, deps *Deps, in MessageInput, out MessageOutput) (*mcp.CallToolResult, MessageOutput, error) {
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

// handlePin 置顶/取消置顶消息。
func handlePin(ctx context.Context, deps *Deps, in MessageInput, out MessageOutput) (*mcp.CallToolResult, MessageOutput, error) {
	if in.Chat == "" || in.MsgID == 0 {
		out.Note = "pin 需要 chat 和 msg_id"
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

	_, err = acc.API().MessagesUpdatePinnedMessage(ctx, &tg.MessagesUpdatePinnedMessageRequest{
		Peer:  peer,
		ID:    in.MsgID,
		Unpin: in.Text == "unpin",
	})
	if err != nil {
		out.Note = fmt.Sprintf("置顶失败: %v", err)
		return nil, out, nil
	}
	out.MsgID = in.MsgID
	if in.Text == "unpin" {
		out.Note = "已取消置顶"
	} else {
		out.Note = "已置顶"
	}
	return nil, out, nil
}

// randomID 生成消息随机 ID（用时间戳纳秒，满足唯一性要求）。
func randomID() int64 {
	return time.Now().UnixNano()
}

// 辅助：提取消息
func extractMessages(res tg.MessagesMessagesClass) ([]*tg.Message, []*tg.User) {
	switch v := res.(type) {
	case *tg.MessagesMessages:
		return filterMessages(v.Messages), filterUsers(v.Users)
	case *tg.MessagesMessagesSlice:
		return filterMessages(v.Messages), filterUsers(v.Users)
	case *tg.MessagesChannelMessages:
		return filterMessages(v.Messages), filterUsers(v.Users)
	default:
		return nil, nil
	}
}

func extractMessagesFull(res tg.MessagesMessagesClass) ([]*tg.Message, []*tg.User, []tg.ChatClass) {
	switch v := res.(type) {
	case *tg.MessagesMessages:
		return filterMessages(v.Messages), filterUsers(v.Users), v.Chats
	case *tg.MessagesMessagesSlice:
		return filterMessages(v.Messages), filterUsers(v.Users), v.Chats
	case *tg.MessagesChannelMessages:
		return filterMessages(v.Messages), filterUsers(v.Users), v.Chats
	default:
		return nil, nil, nil
	}
}

func filterMessages(ms []tg.MessageClass) []*tg.Message {
	out := make([]*tg.Message, 0, len(ms))
	for _, m := range ms {
		if msg, ok := m.(*tg.Message); ok {
			out = append(out, msg)
		}
	}
	return out
}

func filterUsers(us []tg.UserClass) []*tg.User {
	out := make([]*tg.User, 0, len(us))
	for _, u := range us {
		if user, ok := u.(*tg.User); ok {
			out = append(out, user)
		}
	}
	return out
}

// projectMessages 投影消息列表。
func projectMessages(msgs []*tg.Message, expand []string) []dto.MessageBrief {
	out := make([]dto.MessageBrief, 0, len(msgs))
	for _, m := range msgs {
		full := projectMessage(m, nil)
		out = append(out, full.MessageBrief)
	}
	return out
}

// projectMessagesWithEntities 带实体信息的投影。
func projectMessagesWithEntities(msgs []*tg.Message, users []*tg.User, chats []tg.ChatClass, expand []string) []dto.MessageBrief {
	userMap := make(map[int64]*tg.User, len(users))
	for _, u := range users {
		userMap[u.ID] = u
	}
	out := make([]dto.MessageBrief, 0, len(msgs))
	for _, m := range msgs {
		full := projectMessage(m, userMap)
		out = append(out, full.MessageBrief)
	}
	return out
}

// projectMessagesFull 完整投影（用于缓存）。
func projectMessagesFull(msgs []*tg.Message, users []*tg.User, chats []tg.ChatClass) []dto.MessageFull {
	userMap := make(map[int64]*tg.User, len(users))
	for _, u := range users {
		userMap[u.ID] = u
	}
	out := make([]dto.MessageFull, 0, len(msgs))
	for _, m := range msgs {
		if full := projectMessage(m, userMap); full != nil {
			out = append(out, *full)
		}
	}
	return out
}

// projectMessage 把单条消息投影成 MessageFull。
func projectMessage(m *tg.Message, users map[int64]*tg.User) *dto.MessageFull {
	if m == nil {
		return nil
	}
	full := &dto.MessageFull{
		MessageBrief: dto.MessageBrief{
			ID:   m.ID,
			Date: int64(m.Date),
			Out:  m.Out,
		},
	}
	if m.FromID != nil {
		full.SenderID = tgclient.PeerIDFromClass(m.FromID)
		if users != nil {
			if u := users[full.SenderID]; u != nil {
				full.Sender = displayName(u)
			}
		}
	}
	full.Text = dto.Truncate(messageText(m), dto.MaxTextRunes)
	full.Media, full.MediaMeta = mediaKindOf(m.Media)
	if r, ok := m.GetReplyTo(); ok {
		if rm, ok := r.(*tg.MessageReplyHeader); ok {
			full.ReplyTo = rm.ReplyToMsgID
		}
	}
	if m.EditDate != 0 {
		full.EditedAt = int64(m.EditDate)
	}
	if views, ok := m.GetViews(); ok {
		full.Views = views
	}
	return full
}

// messageText 提取消息文本。
func messageText(m *tg.Message) string {
	if msg, ok := m.Media.(*tg.MessageMediaEmpty); ok {
		_ = msg
	}
	return m.Message
}

// mediaKindOf 判断媒体类型并返回元数据。
func mediaKindOf(media tg.MessageMediaClass) (string, *dto.MediaMeta) {
	switch v := media.(type) {
	case *tg.MessageMediaPhoto:
		meta := &dto.MediaMeta{Kind: "photo"}
		if photo, ok := v.Photo.(*tg.Photo); ok {
			// 取最大尺寸的宽高与大小
			var maxSize int
			for _, sz := range photo.Sizes {
				switch ps := sz.(type) {
				case *tg.PhotoSize:
					if int(ps.Size) > maxSize {
						maxSize = int(ps.Size)
						meta.Width, meta.Height = ps.W, ps.H
					}
					meta.Size += int64(ps.Size)
				case *tg.PhotoSizeProgressive:
					if len(ps.Sizes) > 0 {
						last := ps.Sizes[len(ps.Sizes)-1]
						if last > maxSize {
							maxSize = last
							meta.Width, meta.Height = ps.W, ps.H
						}
					}
				}
			}
		}
		return "photo", meta
	case *tg.MessageMediaDocument:
		doc, ok := v.Document.(*tg.Document)
		if !ok {
			return "document", &dto.MediaMeta{Kind: "document"}
		}
		kind := "document"
		for _, attr := range doc.Attributes {
			switch a := attr.(type) {
			case *tg.DocumentAttributeSticker:
				kind = "sticker"
				return kind, &dto.MediaMeta{Kind: kind, Emoji: a.Alt, Size: doc.Size}
			case *tg.DocumentAttributeAudio:
				if a.Voice {
					kind = "voice"
				} else {
					kind = "audio"
				}
				return kind, &dto.MediaMeta{
					Kind: kind, Size: doc.Size, Duration: a.Duration,
					Title: a.Title, Performer: a.Performer,
				}
			case *tg.DocumentAttributeVideo:
				kind = "video"
				return kind, &dto.MediaMeta{
					Kind: kind, Size: doc.Size, Duration: int(a.Duration),
					Width: a.W, Height: a.H,
				}
			case *tg.DocumentAttributeFilename:
				return "document", &dto.MediaMeta{Kind: "document", Size: doc.Size, FileName: a.FileName}
			}
		}
		return kind, &dto.MediaMeta{Kind: kind, Size: doc.Size, MimeType: doc.MimeType}
	case *tg.MessageMediaGeo, *tg.MessageMediaGeoLive:
		return "location", &dto.MediaMeta{Kind: "location"}
	case *tg.MessageMediaVenue:
		return "venue", &dto.MediaMeta{Kind: "venue"}
	case *tg.MessageMediaPoll:
		return "poll", &dto.MediaMeta{Kind: "poll"}
	case *tg.MessageMediaContact:
		return "contact", &dto.MediaMeta{Kind: "contact"}
	case *tg.MessageMediaWebPage:
		return "webpage", &dto.MediaMeta{Kind: "webpage"}
	case *tg.MessageMediaEmpty:
		return "", nil
	case *tg.MessageMediaUnsupported:
		return "", nil
	default:
		// 未知类型返回空而不是 "unknown"，避免纯文字被误报
		return "", nil
	}
}

// 保留 strings 导入（供后续使用）
var _ = strings.TrimSpace

// expandReplies 展开消息的引用关系，把被引用消息的摘要填进 MessageBrief。
//
// 设计：
//   - 只对 reply_to 存在的消息处理
//   - 批量拉取（一次 getMessages 拿多条），避免 N+1
//   - 被引用的消息可能是文字/图片/文件/语音，都标注类型
//   - 原消息已删除时标记 Deleted
func expandReplies(ctx context.Context, acc *tgclient.Account, msgs []*tg.Message, briefs []dto.MessageBrief, users []*tg.User) {
	if acc == nil || acc.API() == nil {
		return
	}

	// 1. 收集所有被引用的消息 ID
	needIDs := make([]int, 0, len(msgs))
	idxByMsgID := make(map[int]int, len(msgs))
	for i, m := range msgs {
		rid := replyToID(m)
		if rid > 0 {
			needIDs = append(needIDs, rid)
			idxByMsgID[m.ID] = i
		}
	}
	if len(needIDs) == 0 {
		return
	}

	// 2. 批量拉取被引用的消息
	inputMsgs := make([]tg.InputMessageClass, 0, len(needIDs))
	for _, id := range needIDs {
		inputMsgs = append(inputMsgs, &tg.InputMessageID{ID: id})
	}
	res, err := acc.API().MessagesGetMessages(ctx, inputMsgs)
	if err != nil {
		// 拉取失败：标记为已删除，不阻塞主流程
		for i := range briefs {
			if replyToID(msgs[i]) > 0 {
				briefs[i].ReplyTo = replyToID(msgs[i])
			}
		}
		return
	}

	// 3. 建立 ID → 消息映射
	repliedMsgs, repliedUsers, _ := extractMessagesFull(res)
	repliedByID := make(map[int]*tg.Message, len(repliedMsgs))
	for _, rm := range repliedMsgs {
		repliedByID[rm.ID] = rm
	}
	userMap := make(map[int64]*tg.User)
	for _, u := range users {
		userMap[u.ID] = u
	}
	for _, u := range repliedUsers {
		userMap[u.ID] = u
	}

	// 4. 填充摘要
	for _, m := range msgs {
		rid := replyToID(m)
		if rid == 0 {
			continue
		}
		idx, ok := idxByMsgID[m.ID]
		if !ok || idx >= len(briefs) {
			continue
		}
		ref := buildReplyRef(rid, repliedByID[rid], userMap)
		briefs[idx].ReplyTo = rid
		briefs[idx].Reply = ref
	}
}

// replyToID 提取消息的 reply_to 消息 ID。
func replyToID(m *tg.Message) int {
	r, ok := m.GetReplyTo()
	if !ok {
		return 0
	}
	switch v := r.(type) {
	case *tg.MessageReplyHeader:
		// 跳过 forum topic 的 reply（那是话题，不是引用消息）
		if v.ForumTopic {
			return 0
		}
		return v.ReplyToMsgID
	}
	return 0
}

// buildReplyRef 构造被引用消息的摘要。
func buildReplyRef(msgID int, original *tg.Message, users map[int64]*tg.User) *dto.ReplyRef {
	ref := &dto.ReplyRef{MsgID: msgID}
	if original == nil {
		ref.Deleted = true
		return ref
	}

	// 发送者
	if original.FromID != nil {
		if uid := tgclient.PeerIDFromClass(original.FromID); uid != 0 {
			if u := users[uid]; u != nil {
				ref.Sender = displayName(u)
			}
		}
	}

	// 文本（截断 80 字符）
	ref.Text = dto.Truncate(original.Message, 80)

	// 媒体类型 + 元数据
	kind, meta := mediaKindOf(original.Media)
	ref.Media = kind
	ref.MediaMeta = meta

	// 部分引用时的引用片段
	if r, ok := original.GetReplyTo(); ok {
		if rh, ok := r.(*tg.MessageReplyHeader); ok {
			if q, ok := rh.GetQuoteText(); ok {
				ref.QuoteText = dto.Truncate(q, 80)
			}
		}
	}

	return ref
}
