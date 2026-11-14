package mcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/midi-HL/telegram-mcp/internal/cache"
	"github.com/midi-HL/telegram-mcp/internal/dto"
)

// ChatInput 是 chat 工具的输入。
type ChatInput struct {
	// Action: list / info / unread / mark_read / folders / resolve
	Action  string   `json:"action" jsonschema:"list|info|unread|mark_read|folders|resolve"`
	Chat    string   `json:"chat,omitempty" jsonschema:"chat id string or @username or title keyword"`
	Limit   int      `json:"limit,omitempty" jsonschema:"default 50, max 200"`
	Offset  int      `json:"offset,omitempty" jsonschema:"pagination offset for list"`
	Expand  []string `json:"expand,omitempty" jsonschema:"full|members|permissions"`
	Account string   `json:"account,omitempty" jsonschema:"account name"`
}

// ChatOutput 是 chat 工具的输出。
type ChatOutput struct {
	Action  string          `json:"action"`
	Chats   []dto.ChatBrief `json:"chats,omitempty"`
	Chat    *dto.ChatFull   `json:"chat,omitempty"`
	Folders []dto.Folder    `json:"folders,omitempty"`
	Unread  *UnreadSummary  `json:"unread,omitempty"`
	Message string          `json:"message,omitempty"`
	Total   int             `json:"total,omitempty"`
}

// UnreadSummary 是未读概览。
type UnreadSummary struct {
	TotalChats  int             `json:"total_chats"`
	TotalUnread int             `json:"total_unread"`
	TopUnread   []dto.ChatBrief `json:"top,omitempty"`
}

// chatFail 返回 MCP 错误结果，避免把失败伪装成「成功 + message/note」。
func chatFail(format string, args ...any) (*mcp.CallToolResult, ChatOutput, error) {
	r, _ := fail(format, args...)
	return r, ChatOutput{}, nil
}

// registerChatTools 注册 chat 工具。
func registerChatTools(s *mcp.Server, deps *Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "chat",
		Description: "会话管理。所有 id 输出均为字符串；chat 输入也用字符串。action: list / info / unread / mark_read / folders / resolve（按标题或 ID 找候选）",
		Annotations: &mcp.ToolAnnotations{Title: "会话", ReadOnlyHint: false},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ChatInput) (*mcp.CallToolResult, ChatOutput, error) {
		return handleChat(ctx, deps, in)
	})
}

func handleChat(ctx context.Context, deps *Deps, in ChatInput) (*mcp.CallToolResult, ChatOutput, error) {
	out := ChatOutput{Action: in.Action}

	switch in.Action {
	case "", "list":
		out.Action = "list"
		return handleChatList(ctx, deps, in, out)
	case "info":
		out.Action = "info"
		return handleChatInfo(ctx, deps, in, out)
	case "unread":
		out.Action = "unread"
		return handleChatUnread(ctx, deps, in, out)
	case "mark_read":
		out.Action = "mark_read"
		return handleChatMarkRead(ctx, deps, in, out)
	case "folders":
		out.Action = "folders"
		return handleChatFolders(ctx, deps, in, out)
	case "resolve":
		out.Action = "resolve"
		return handleChatResolve(ctx, deps, in, out)
	default:
		return chatFail("未知 action %q（支持 list / info / unread / mark_read / folders / resolve）", in.Action)
	}
}

// handleChatList 列出会话。
func handleChatList(ctx context.Context, deps *Deps, in ChatInput, out ChatOutput) (*mcp.CallToolResult, ChatOutput, error) {
	acc, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		return chatFail("%v", err)
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	// 从 Telegram 拉取会话列表；limit 是请求目标，内部自动翻页直到取够/取完。
	dialogs, err := fetchDialogsCached(ctx, acc.API(), limit, in.Offset)
	if err != nil {
		return chatFail("拉取会话失败: %v", err)
	}

	// 写入缓存 + 投影
	briefs := make([]dto.ChatBrief, 0, len(dialogs))
	for _, d := range dialogs {
		brief, extra := projectDialog(d)
		if brief.ID == 0 {
			continue
		}
		briefs = append(briefs, brief)
		if d.InputPeer != nil {
			resolver.StorePeer(d.InputPeer, d.Username)
		}
		if deps.Cache != nil {
			_ = deps.Cache.UpsertChat(ctx, brief, extra)
		}
	}

	out.Chats = briefs
	out.Total = len(briefs)
	out.Message = fmt.Sprintf("共 %d 个会话", len(briefs))
	return nil, out, nil
}

// handleChatInfo 获取单个会话详情。
func handleChatInfo(ctx context.Context, deps *Deps, in ChatInput, out ChatOutput) (*mcp.CallToolResult, ChatOutput, error) {
	if in.Chat == "" {
		return chatFail("info 需要 chat 参数")
	}
	_, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		return chatFail("%v", err)
	}

	peer, err := resolver.Resolve(ctx, in.Chat)
	if err != nil {
		return chatFail("%v。可先用 chat resolve 按标题查找候选，或用 chat list 刷新 peer 缓存", err)
	}

	// 先尝试缓存
	if deps.Cache != nil {
		if cached, err := deps.Cache.GetChat(ctx, PeerIDOf(peer)); err == nil && cached != nil {
			out.Chat = cached
			out.Message = "来自缓存"
			return nil, out, nil
		}
	}

	// 从 Telegram 拉取
	full, err := fetchChatFull(ctx, deps, in.Account, peer, in.Expand)
	if err != nil {
		return chatFail("获取会话详情失败: %v", err)
	}
	out.Chat = full
	out.Message = "ok"
	return nil, out, nil
}

// handleChatUnread 未读概览。
func handleChatUnread(ctx context.Context, deps *Deps, in ChatInput, out ChatOutput) (*mcp.CallToolResult, ChatOutput, error) {
	acc, _, err := deps.Account(in.Account)
	if err != nil {
		return chatFail("%v", err)
	}

	// 拉取足够多的会话以统计未读
	dialogs, err := fetchDialogsCached(ctx, acc.API(), 100, 0)
	if err != nil {
		return chatFail("拉取会话失败: %v", err)
	}

	summary := &UnreadSummary{}
	unreadChats := make([]dto.ChatBrief, 0)
	for _, d := range dialogs {
		brief, _ := projectDialog(d)
		if brief.ID == 0 {
			continue
		}
		summary.TotalChats++
		if brief.Unread > 0 {
			summary.TotalUnread += brief.Unread
			unreadChats = append(unreadChats, brief)
		}
	}

	// 按未读数排序，取前 10
	sortByUnread(unreadChats)
	if len(unreadChats) > 10 {
		unreadChats = unreadChats[:10]
	}
	summary.TopUnread = unreadChats
	out.Unread = summary
	out.Message = fmt.Sprintf("%d 个会话有未读，共 %d 条", len(unreadChats), summary.TotalUnread)
	return nil, out, nil
}

// handleChatMarkRead 标记已读。
func handleChatMarkRead(ctx context.Context, deps *Deps, in ChatInput, out ChatOutput) (*mcp.CallToolResult, ChatOutput, error) {
	if in.Chat == "" {
		return chatFail("mark_read 需要 chat 参数")
	}
	acc, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		return chatFail("%v", err)
	}
	peer, err := resolver.Resolve(ctx, in.Chat)
	if err != nil {
		return chatFail("%v", err)
	}

	if err := markPeerRead(ctx, acc.API(), peer); err != nil {
		return chatFail("标记已读失败: %v", err)
	}
	out.Message = "已标记为已读"
	return nil, out, nil
}

// handleChatFolders 列出聊天文件夹。
func handleChatFolders(ctx context.Context, deps *Deps, in ChatInput, out ChatOutput) (*mcp.CallToolResult, ChatOutput, error) {
	acc, _, err := deps.Account(in.Account)
	if err != nil {
		return chatFail("%v", err)
	}

	folders, err := fetchFolders(ctx, acc.API())
	if err != nil {
		return chatFail("获取文件夹失败: %v", err)
	}
	out.Folders = folders
	out.Message = fmt.Sprintf("共 %d 个文件夹", len(folders))
	return nil, out, nil
}

// 辅助：从对话对象投影出 ChatBrief + 缓存字段
func projectDialog(d dialogItem) (dto.ChatBrief, cache.ChatExtra) {
	b := dto.ChatBrief{
		ID:              d.ID,
		IDString:        strconv.FormatInt(d.ID, 10),
		Title:           d.Title,
		MetadataMissing: d.LastAt == 0 && d.LastText == "",
		Type:            d.Type,
		Unread:          d.Unread,
		LastAt:          d.LastAt,
	}
	if d.LastText != "" {
		b.LastText = dto.Truncate(d.LastText, dto.MaxPreviewRunes)
	}
	extra := cache.ChatExtra{
		Username:    d.Username,
		About:       d.About,
		MemberCount: d.MemberCount,
		Muted:       d.Muted,
		Pinned:      d.Pinned,
		CanSend:     d.CanSend,
		IsAdmin:     d.IsAdmin,
		IsCreator:   d.IsCreator,
		AccessHash:  d.AccessHash,
	}
	return b, extra
}

// sortByUnread 按未读数降序排序。
func sortByUnread(cs []dto.ChatBrief) {
	for i := 1; i < len(cs); i++ {
		for j := i; j > 0 && cs[j].Unread > cs[j-1].Unread; j-- {
			cs[j], cs[j-1] = cs[j-1], cs[j]
		}
	}
}

// PeerIDOf 从 InputPeer 提取数字 ID（mcp 包内用）。
func PeerIDOf(p tg.InputPeerClass) int64 {
	switch v := p.(type) {
	case *tg.InputPeerUser:
		return v.UserID
	case *tg.InputPeerChat:
		return v.ChatID
	case *tg.InputPeerChannel:
		return v.ChannelID
	default:
		return 0
	}
}

// handleChatResolve 按标题或 ID 从会话列表里找候选，并把命中项写入 peer 缓存。
func handleChatResolve(ctx context.Context, deps *Deps, in ChatInput, out ChatOutput) (*mcp.CallToolResult, ChatOutput, error) {
	query := strings.TrimSpace(in.Chat)
	if query == "" {
		return chatFail("resolve 需要 chat（标题关键字或 ID 字符串）")
	}
	acc, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		return chatFail("%v", err)
	}
	dialogs, err := fetchDialogsCached(ctx, acc.API(), 200, 0)
	if err != nil {
		return chatFail("拉取会话列表失败: %v", err)
	}
	q := strings.ToLower(query)
	candidates := make([]dto.ChatBrief, 0, 8)
	for _, d := range dialogs {
		if d.InputPeer != nil {
			resolver.StorePeer(d.InputPeer, d.Username)
		}
		brief, _ := projectDialog(d)
		if brief.ID == 0 {
			continue
		}
		id := strconv.FormatInt(brief.ID, 10)
		if strings.Contains(strings.ToLower(brief.Title), q) || id == query || "-100"+id == query {
			candidates = append(candidates, brief)
		}
	}
	out.Chats = candidates
	out.Total = len(candidates)
	out.Message = fmt.Sprintf("找到 %d 个候选", len(candidates))
	return nil, out, nil
}
