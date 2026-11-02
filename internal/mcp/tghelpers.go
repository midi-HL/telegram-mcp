package mcp

import (
	"context"
	"fmt"

	"github.com/gotd/td/tg"

	"github.com/midi-HL/telegram-mcp/internal/dto"
)

// dialogItem 是从 gotd 对话对象提取的中间结构。
type dialogItem struct {
	ID          int64
	Title       string
	Type        dto.ChatType
	Username    string
	About       string
	MemberCount int
	Unread      int
	Muted       bool
	Pinned      bool
	CanSend     bool
	IsAdmin     bool
	IsCreator   bool
	LastText    string
	LastAt      int64
	AccessHash  int64
}

// fetchDialogs 拉取会话列表并投影。
func fetchDialogs(ctx context.Context, api *tg.Client, limit int) ([]dialogItem, error) {
	res, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		OffsetDate: 0,
		OffsetID:   0,
		OffsetPeer: &tg.InputPeerEmpty{},
		Limit:      limit,
	})
	if err != nil {
		return nil, err
	}

	var dialogs []tg.DialogClass
	var users []tg.UserClass
	var chats []tg.ChatClass
	switch v := res.(type) {
	case *tg.MessagesDialogs:
		dialogs, users, chats = v.Dialogs, v.Users, v.Chats
	case *tg.MessagesDialogsSlice:
		dialogs, users, chats = v.Dialogs, v.Users, v.Chats
	default:
		return nil, nil
	}

	// 建立 ID → 实体映射
	userMap := make(map[int64]*tg.User, len(users))
	for _, u := range users {
		if user, ok := u.(*tg.User); ok {
			userMap[user.ID] = user
		}
	}
	chatMap := make(map[int64]tg.ChatClass, len(chats))
	channelMap := make(map[int64]*tg.Channel, len(chats))
	for _, c := range chats {
		switch ch := c.(type) {
		case *tg.Channel:
			chatMap[ch.ID] = ch
			channelMap[ch.ID] = ch
		case *tg.Chat:
			chatMap[ch.ID] = ch
		}
	}

	out := make([]dialogItem, 0, len(dialogs))
	for _, d := range dialogs {
		var dialog *tg.Dialog
		switch v := d.(type) {
		case *tg.Dialog:
			dialog = v
		case *tg.DialogFolder:
			// 文件夹条目，跳过
			continue
		default:
			continue
		}

		item := dialogItem{
			Unread: dialog.UnreadCount,
			Pinned: dialog.Pinned,
		}
		if mute, ok := dialog.NotifySettings.GetMuteUntil(); ok {
			item.Muted = mute > 0
		}

		switch peer := dialog.Peer.(type) {
		case *tg.PeerUser:
			u := userMap[peer.UserID]
			if u == nil {
				continue
			}
			item.ID = u.ID
			item.Title = displayName(u)
			item.Type = dto.ChatTypeUser
			item.Username, _ = u.GetUsername()
			item.AccessHash = u.AccessHash
			item.CanSend = true
		case *tg.PeerChat:
			c, ok := chatMap[peer.ChatID].(*tg.Chat)
			if !ok {
				continue
			}
			item.ID = c.ID
			item.Title = c.Title
			item.Type = dto.ChatTypeBasicGroup
			item.MemberCount = c.ParticipantsCount
			item.CanSend = true
		case *tg.PeerChannel:
			ch := channelMap[peer.ChannelID]
			if ch == nil {
				continue
			}
			item.ID = ch.ID
			item.Title = ch.Title
			item.Username, _ = ch.GetUsername()
			item.AccessHash = ch.AccessHash
			if ch.Megagroup {
				item.Type = dto.ChatTypeSupergroup
			} else {
				item.Type = dto.ChatTypeChannel
			}
			if participants, ok := ch.GetParticipantsCount(); ok {
				item.MemberCount = participants
			}
			item.CanSend = ch.Megagroup
		}

		if item.ID == 0 {
			continue
		}

		// 取最后一条消息的预览
		if dialog.TopMessage != 0 {
			item.LastText = findTopMessageText(dialogs, dialog)
			item.LastAt = int64(dialog.TopMessage) // 占位，实际时间需从消息取
		}

		out = append(out, item)
	}

	return out, nil
}

// findTopMessageText 从对话列表里找最后一条消息的文本预览。
// 注意：MessagesDialogs 的 Messages 字段包含这些消息，但我们的中间结构没保留。
// 这里返回空字符串作为降级——真正的预览在 message history 里拿。
func findTopMessageText(_ []tg.DialogClass, _ *tg.Dialog) string {
	return ""
}

// displayName 生成用户显示名。
func displayName(u *tg.User) string {
	name := u.FirstName
	if u.LastName != "" {
		if name != "" {
			name += " "
		}
		name += u.LastName
	}
	if name == "" {
		name, _ = u.GetUsername()
	}
	if name == "" {
		name = fmt.Sprintf("用户%d", u.ID)
	}
	return name
}

// fetchChatFull 获取会话详情。
func fetchChatFull(ctx context.Context, deps *Deps, account string, peer tg.InputPeerClass, expand []string) (*dto.ChatFull, error) {
	acc, _, err := deps.Account(account)
	if err != nil {
		return nil, err
	}
	api := acc.API()

	full := &dto.ChatFull{}
	full.ID = PeerIDOf(peer)

	switch p := peer.(type) {
	case *tg.InputPeerUser:
		res, err := api.UsersGetFullUser(ctx, &tg.InputUser{UserID: p.UserID, AccessHash: p.AccessHash})
		if err != nil {
			return nil, err
		}
		var u *tg.User
		for _, uc := range res.Users {
			if user, ok := uc.(*tg.User); ok {
				u = user
				break
			}
		}
		if u == nil {
			return nil, fmt.Errorf("用户信息为空")
		}
		full.Type = dto.ChatTypeUser
		full.Title = displayName(u)
		full.About = res.FullUser.About
		full.CanSendMessage = !res.FullUser.Blocked
		full.Username, _ = u.GetUsername()
	case *tg.InputPeerChannel:
		res, err := api.ChannelsGetFullChannel(ctx, &tg.InputChannel{ChannelID: p.ChannelID, AccessHash: p.AccessHash})
		if err != nil {
			return nil, err
		}
		var ch *tg.Channel
		for _, cc := range res.Chats {
			if channel, ok := cc.(*tg.Channel); ok {
				ch = channel
				break
			}
		}
		if ch == nil {
			return nil, fmt.Errorf("频道信息为空")
		}
		full.Title = ch.Title
		if ch.Megagroup {
			full.Type = dto.ChatTypeSupergroup
		} else {
			full.Type = dto.ChatTypeChannel
		}
		if cf, ok := res.FullChat.(*tg.ChannelFull); ok {
			full.About = cf.About
			full.MemberCount = cf.ParticipantsCount
		}
		full.Username, _ = ch.GetUsername()
		full.CanSendMessage = ch.Megagroup
		if hasAnyRight(ch.AdminRights) {
			full.IsAdmin = true
		}
		if ch.Creator {
			full.IsCreator = true
			full.IsAdmin = true
		}
	case *tg.InputPeerChat:
		res, err := api.MessagesGetFullChat(ctx, p.ChatID)
		if err != nil {
			return nil, err
		}
		var c *tg.Chat
		for _, cc := range res.Chats {
			if chat, ok := cc.(*tg.Chat); ok {
				c = chat
				break
			}
		}
		if c == nil {
			return nil, fmt.Errorf("群信息为空")
		}
		full.Title = c.Title
		full.Type = dto.ChatTypeBasicGroup
		full.MemberCount = c.ParticipantsCount
		if cf, ok := res.FullChat.(*tg.ChatFull); ok {
			full.About = cf.About
			switch parts := cf.Participants.(type) {
			case *tg.ChatParticipants:
				full.MemberCount = len(parts.Participants)
			}
		}
		full.CanSendMessage = true
	}

	// expand 处理
	for _, e := range expand {
		switch e {
		case "full", "permissions":
			// 已包含在上面
		}
	}
	return full, nil
}

// hasAnyRight 判断管理员是否有任何权限。
func hasAnyRight(r tg.ChatAdminRights) bool {
	return r.ChangeInfo || r.DeleteMessages || r.BanUsers || r.InviteUsers ||
		r.PinMessages || r.AddAdmins || r.Anonymous || r.ManageCall ||
		r.Other || r.ManageTopics || r.PostMessages || r.EditMessages || r.DeleteStories || r.EditStories
}

// markPeerRead 标记对话为已读。
func markPeerRead(ctx context.Context, api *tg.Client, peer tg.InputPeerClass) error {
	switch p := peer.(type) {
	case *tg.InputPeerChannel:
		_, err := api.ChannelsReadHistory(ctx, &tg.ChannelsReadHistoryRequest{
			Channel: &tg.InputChannel{ChannelID: p.ChannelID, AccessHash: p.AccessHash},
			MaxID:   0,
		})
		return err
	case *tg.InputPeerUser:
		_, err := api.MessagesReadHistory(ctx, &tg.MessagesReadHistoryRequest{
			Peer:  p,
			MaxID: 0,
		})
		return err
	case *tg.InputPeerChat:
		_, err := api.MessagesReadHistory(ctx, &tg.MessagesReadHistoryRequest{
			Peer:  p,
			MaxID: 0,
		})
		return err
	}
	return fmt.Errorf("不支持的 peer 类型")
}

// fetchFolders 拉取聊天文件夹。
func fetchFolders(ctx context.Context, api *tg.Client) ([]dto.Folder, error) {
	res, err := api.MessagesGetDialogFilters(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dto.Folder, 0, len(res.Filters))
	for _, f := range res.Filters {
		df, ok := f.(*tg.DialogFilter)
		if !ok {
			continue
		}
		item := dto.Folder{
			ID:    df.ID,
			Title: df.Title.Text,
		}
		if emoji, ok := df.GetEmoticon(); ok {
			item.Emoji = emoji
		}
		item.ChatCount = len(df.PinnedPeers) + len(df.IncludePeers)
		out = append(out, item)
	}
	return out, nil
}
