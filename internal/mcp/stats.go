package mcp

import (
	"context"
	"fmt"

	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// StatsInput 是 stats 工具的输入。
type StatsInput struct {
	// Action: group / chat / cache
	Action  string `json:"action" jsonschema:"group|chat|cache"`
	Chat    string `json:"chat,omitempty" jsonschema:"chat id"`
	Account string `json:"account,omitempty" jsonschema:"account name"`
}

// StatsOutput 是 stats 工具的输出。
type StatsOutput struct {
	Action string         `json:"action"`
	Data   map[string]any `json:"data,omitempty"`
	Note   string         `json:"note,omitempty"`
}

// registerStatsTools 注册 stats 工具。
func registerStatsTools(s *mcp.Server, deps *Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "stats",
		Description: "统计信息。action: group 群统计 / chat 会话统计 / cache 本地缓存统计",
		Annotations: &mcp.ToolAnnotations{Title: "统计", ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in StatsInput) (*mcp.CallToolResult, StatsOutput, error) {
		return handleStats(ctx, deps, in)
	})
}

func handleStats(ctx context.Context, deps *Deps, in StatsInput) (*mcp.CallToolResult, StatsOutput, error) {
	out := StatsOutput{Action: in.Action}

	switch in.Action {
	case "group":
		return handleGroupStats(ctx, deps, in, out)
	case "", "chat":
		out.Action = "chat"
		return handleChatStats(ctx, deps, in, out)
	case "cache":
		return handleCacheStats(ctx, deps, in, out)
	default:
		out.Note = fmt.Sprintf("未知 action %q", in.Action)
		return nil, out, nil
	}
}

// handleGroupStats 获取群统计。
func handleGroupStats(ctx context.Context, deps *Deps, in StatsInput, out StatsOutput) (*mcp.CallToolResult, StatsOutput, error) {
	if in.Chat == "" {
		out.Note = "group 需要 chat 参数"
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
	ch, ok := peer.(*tg.InputPeerChannel)
	if !ok {
		out.Note = "group 统计只支持频道/超级群"
		return nil, out, nil
	}

	// 拉取频道完整信息（含成员数等基础统计）
	res, err := acc.API().ChannelsGetFullChannel(ctx, &tg.InputChannel{
		ChannelID: ch.ChannelID, AccessHash: ch.AccessHash,
	})
	if err != nil {
		out.Note = fmt.Sprintf("获取群信息失败: %v", err)
		return nil, out, nil
	}
	var cf *tg.ChannelFull
	for _, fc := range []tg.ChatFullClass{res.FullChat} {
		if c, ok := fc.(*tg.ChannelFull); ok {
			cf = c
		}
	}
	if cf == nil {
		out.Note = "群信息不可用"
		return nil, out, nil
	}

	data := map[string]any{
		"members":        cf.ParticipantsCount,
		"admins":         cf.AdminsCount,
		"kicked":         cf.KickedCount,
		"banned":         cf.BannedCount,
		"online":         cf.OnlineCount,
	}
	if cf.About != "" {
		data["about"] = cf.About
	}

	// 尝试拉取详细统计（需要管理员权限 + 群人数足够）
	if cf.CanViewStats {
		if stats, err := fetchChannelStats(ctx, acc.API(), ch); err == nil && stats != nil {
			for k, v := range stats {
				data[k] = v
			}
		} else {
			data["stats_note"] = "详细统计需要管理员权限且群人数足够"
		}
	} else {
		data["stats_note"] = "无统计权限（需要群主或管理员）"
	}

	out.Data = data
	out.Note = fmt.Sprintf("成员 %d，在线 %d", cf.ParticipantsCount, cf.OnlineCount)
	return nil, out, nil
}

// fetchChannelStats 拉取频道详细统计。
func fetchChannelStats(ctx context.Context, api *tg.Client, ch *tg.InputPeerChannel) (map[string]any, error) {
	// 先尝试广播统计（频道）
	// 先尝试超级群统计
	res2, err2 := api.StatsGetMegagroupStats(ctx, &tg.StatsGetMegagroupStatsRequest{
		Channel: &tg.InputChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash},
	})
	if err2 == nil && res2 != nil {
		return map[string]any{
			"members_prev":    res2.Members.Previous,
			"members_current": res2.Members.Current,
			"messages_prev":   res2.Messages.Previous,
			"messages_current": res2.Messages.Current,
			"posters_current": res2.Posters.Current,
			"viewers_current": res2.Viewers.Current,
		}, nil
	}

	// 再尝试广播统计（频道）
	res, err := api.StatsGetBroadcastStats(ctx, &tg.StatsGetBroadcastStatsRequest{
		Channel: &tg.InputChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash},
	})
	if err == nil && res != nil {
		return map[string]any{
			"followers_prev":    res.Followers.Previous,
			"followers_current": res.Followers.Current,
			"views_per_post":    res.ViewsPerPost.Current,
			"shares_per_post":   res.SharesPerPost.Current,
		}, nil
	}
	return nil, err
}

// handleChatStats 获取单个会话统计（从本地缓存）。
func handleChatStats(ctx context.Context, deps *Deps, in StatsInput, out StatsOutput) (*mcp.CallToolResult, StatsOutput, error) {
	if deps.Cache == nil {
		out.Note = "缓存不可用"
		return nil, out, nil
	}
	if in.Chat == "" {
		// 全局统计
		stats, err := deps.Cache.Stats(ctx)
		if err != nil {
			out.Note = err.Error()
			return nil, out, nil
		}
		out.Data = stats
		out.Note = "全局缓存统计"
		return nil, out, nil
	}

	_, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	peer, err := resolver.Resolve(ctx, in.Chat)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	chatID := PeerIDOf(peer)

	count, err := deps.Cache.CountMessages(ctx, chatID)
	if err != nil {
		out.Note = fmt.Sprintf("统计失败: %v", err)
		return nil, out, nil
	}
	full, _ := deps.Cache.GetChat(ctx, chatID)
	data := map[string]any{
		"chat_id":         chatID,
		"cached_messages": count,
	}
	if full != nil {
		data["title"] = full.Title
		data["type"] = string(full.Type)
		data["unread"] = full.Unread
		data["members"] = full.MemberCount
	}
	out.Data = data
	out.Note = fmt.Sprintf("缓存了 %d 条消息", count)
	return nil, out, nil
}

// handleCacheStats 本地缓存统计。
func handleCacheStats(ctx context.Context, deps *Deps, in StatsInput, out StatsOutput) (*mcp.CallToolResult, StatsOutput, error) {
	if deps.Cache == nil {
		out.Note = "缓存不可用"
		return nil, out, nil
	}
	stats, err := deps.Cache.Stats(ctx)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	out.Data = stats
	out.Note = "本地缓存统计"
	return nil, out, nil
}
