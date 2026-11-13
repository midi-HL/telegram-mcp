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

// fetchDialogs 分页拉取会话列表并投影。
// target 是累计需要的会话数，offset 用于跳过前 N 个（给上层做分页）。
func fetchDialogs(ctx context.Context, api *tg.Client, target int, offset int) ([]dialogItem, error) {
	if target <= 0 {
		target = 50
	}
	need := offset + target
	if need > 1000 {
		need = 1000
	}
	const pageSize = 100

	out := make([]dialogItem, 0, target)
	seen := make(map[int64]bool)
	var offsetDate, offsetID int
	var offsetPeer tg.InputPeerClass = &tg.InputPeerEmpty{}

	for len(out) < need {
		batchLimit := pageSize
		if remain := need - len(out); remain < batchLimit {
			batchLimit = remain
		}
		res, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			OffsetDate: offsetDate,
			OffsetID:   offsetID,
			OffsetPeer: offsetPeer,
			Limit:      batchLimit,
		})
		if err != nil {
			return nil, err
		}

		var dialogs []tg.DialogClass
		var users []tg.UserClass
		var chats []tg.ChatClass
		var messages []tg.MessageClass
		switch v := res.(type) {
		case *tg.MessagesDialogs:
			dialogs, users, chats, messages = v.Dialogs, v.Users, v.Chats, v.Messages
		case *tg.MessagesDialogsSlice:
			dialogs, users, chats, messages = v.Dialogs, v.Users, v.Chats, v.Messages
		default:
			break
		}
		if len(dialogs) == 0 {
			break
		}

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
				chatMap[ch.ID], channelMap[ch.ID] = ch, ch
			case *tg.Chat:
				chatMap[ch.ID] = ch
			}
		}
		msgMap := make(map[int]*tg.Message, len(messages))
		for _, m := range messages {
			if msg, ok := m.(*tg.Message); ok {
				msgMap[msg.ID] = msg
			}
		}

		batchCount := 0
		var lastPeer tg.InputPeerClass
		for _, d := range dialogs {
			dialog, ok := d.(*tg.Dialog)
			if !ok {
				continue // DialogFolder 等条目跳过
			}
			batchCount++
			item := dialogItem{Unread: dialog.UnreadCount, Pinned: dialog.Pinned}
			if mute, ok := dialog.NotifySettings.GetMuteUntil(); ok {
				item.Muted = mute > 0
			}
			switch peer := dialog.Peer.(type) {
			case *tg.PeerUser:
				u := userMap[peer.UserID]
				if u == nil {
					lastPeer = &tg.InputPeerUser{UserID: peer.UserID}
					continue
				}
				item.ID, item.Type = u.ID, dto.ChatTypeUser
				item.Title = displayName(u)
				item.Username, _ = u.GetUsername()
				item.AccessHash, item.CanSend = u.AccessHash, true
				lastPeer = &tg.InputPeerUser{UserID: u.ID, AccessHash: u.AccessHash}
			case *tg.PeerChat:
				c, ok := chatMap[peer.ChatID].(*tg.Chat)
				if !ok {
					lastPeer = &tg.InputPeerChat{ChatID: peer.ChatID}
					continue
				}
				item.ID, item.Type = c.ID, dto.ChatTypeBasicGroup
				item.Title, item.MemberCount, item.CanSend = c.Title, c.ParticipantsCount, true
				lastPeer = &tg.InputPeerChat{ChatID: c.ID}
			case *tg.PeerChannel:
				ch := channelMap[peer.ChannelID]
				if ch == nil {
					lastPeer = &tg.InputPeerChannel{ChannelID: peer.ChannelID}
					continue
				}
				item.ID, item.Title = ch.ID, ch.Title
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
				lastPeer = &tg.InputPeerChannel{ChannelID: ch.ID, AccessHash: ch.AccessHash}
			}
			if item.ID == 0 || seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			if msg := msgMap[dialog.TopMessage]; msg != nil {
				item.LastText, item.LastAt = msg.Message, int64(msg.Date)
			} else if dialog.TopMessage != 0 {
				item.LastAt = int64(dialog.TopMessage)
			}
			out = append(out, item)
			if v, ok := dialogPeerToInput(dialog.Peer); ok {
				lastPeer = v
			}
			offsetID = dialog.TopMessage
			if msg := msgMap[dialog.TopMessage]; msg != nil {
				offsetDate = msg.Date
			}
		}
		if batchCount == 0 || lastPeer == nil {
			break
		}
		if _, complete := res.(*tg.MessagesDialogs); complete {
			break // 全量返回，没有更多页
		}
		offsetPeer = lastPeer
	}
	if offset >= len(out) {
		return []dialogItem{}, nil
	}
	out = out[offset:]
	if len(out) > target {
		out = out[:target]
	}
	return out, nil
}

func dialogPeerToInput(p tg.PeerClass) (tg.InputPeerClass, bool) {
	switch v := p.(type) {
	case *tg.PeerUser:
		return &tg.InputPeerUser{UserID: v.UserID}, true
	case *tg.PeerChat:
		return &tg.InputPeerChat{ChatID: v.ChatID}, true
	case *tg.PeerChannel:
		return &tg.InputPeerChannel{ChannelID: v.ChannelID}, true
	default:
		return nil, false
	}
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

// folderPeerInfo 从分类里的 InputPeer 提取会话 ID 和类型。
func folderPeerInfo(p tg.InputPeerClass) (int64, string) {
	switch v := p.(type) {
	case *tg.InputPeerUser:
		return v.UserID, string(dto.ChatTypeUser)
	case *tg.InputPeerChat:
		return v.ChatID, string(dto.ChatTypeBasicGroup)
	case *tg.InputPeerChannel:
		return v.ChannelID, string(dto.ChatTypeChannel)
	default:
		return 0, ""
	}
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

// FolderChatIDs 实时展开指定分类里的会话 ID，供 Agent 唤醒过滤使用。
func FolderChatIDs(ctx context.Context, api *tg.Client, folderIDs []int) map[int64]bool {
	out := make(map[int64]bool)
	if len(folderIDs) == 0 {
		return out
	}
	want := make(map[int]bool, len(folderIDs))
	for _, id := range folderIDs {
		want[id] = true
	}
	res, err := api.MessagesGetDialogFilters(ctx)
	if err != nil {
		return out
	}
	for _, f := range res.Filters {
		df, ok := f.(*tg.DialogFilter)
		if !ok || !want[df.ID] {
			continue
		}
		for _, p := range append(append([]tg.InputPeerClass(nil), df.PinnedPeers...), df.IncludePeers...) {
			if id, _ := folderPeerInfo(p); id != 0 {
				out[id] = true
			}
		}
		for _, p := range df.ExcludePeers {
			if id, _ := folderPeerInfo(p); id != 0 {
				delete(out, id)
			}
		}
	}
	return out
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
		seen := make(map[int64]bool)
		for pinned, peers := range map[bool][]tg.InputPeerClass{true: df.PinnedPeers, false: df.IncludePeers} {
			for _, p := range peers {
				id, typ := folderPeerInfo(p)
				if id == 0 || seen[id] {
					continue
				}
				seen[id] = true
				item.Peers = append(item.Peers, dto.FolderPeer{ID: id, Type: typ, Pinned: pinned})
			}
		}
		for _, p := range df.ExcludePeers {
			id, _ := folderPeerInfo(p)
			delete(seen, id)
		}
		item.ChatCount = len(seen)
		out = append(out, item)
	}
	return out, nil
}
