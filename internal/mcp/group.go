package mcp

import (
	"context"
	"fmt"

	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/midi-HL/telegram-mcp/internal/dto"
	tgclient "github.com/midi-HL/telegram-mcp/internal/telegram"
)

// GroupInput 是 group 工具的输入。
type GroupInput struct {
	// Action: members / invite / kick / promote / permissions / approve / join / leave / invites / comments
	Action string `json:"action" jsonschema:"members|invite|kick|promote|permissions|approve|join|leave|invites|comments"`
	Chat   string `json:"chat,omitempty" jsonschema:"chat id"`
	User   string `json:"user,omitempty" jsonschema:"user"`
	Users  []string `json:"users,omitempty" jsonschema:"users"`
	Limit  int    `json:"limit,omitempty" jsonschema:"default 50"`
	Query  string `json:"query,omitempty" jsonschema:"query"`
	Link   string `json:"link,omitempty" jsonschema:"invite link"`
	Role   string `json:"role,omitempty" jsonschema:"admin|member|banned|reject"`
	Rights []string `json:"rights,omitempty" jsonschema:"管理员权限：change_info/delete_messages/ban_users/invite_users/pin_messages/add_admins"`
	MsgID  int    `json:"msg_id,omitempty" jsonschema:"msg id"`
	Text   string `json:"text,omitempty" jsonschema:"text"`
	Account string `json:"account,omitempty" jsonschema:"account name"`
}

// GroupOutput 是 group 工具的输出。
type GroupOutput struct {
	Action  string             `json:"action"`
	Members []dto.GroupMember  `json:"members,omitempty"`
	Chat    *dto.ChatFull      `json:"chat,omitempty"`
	Invites []InviteLink       `json:"invites,omitempty"`
	Comments []dto.MessageBrief `json:"comments,omitempty"`
	MsgID    int                `json:"msg_id,omitempty"`
	Count   int                `json:"count,omitempty"`
	Note    string             `json:"note,omitempty"`
}

// InviteLink 是邀请链接。
type InviteLink struct {
	Link      string `json:"link"`
	Title     string `json:"title,omitempty"`
	Members   int    `json:"members,omitempty"`
	Permanent bool   `json:"permanent,omitempty"`
	ExpiresAt int64  `json:"expires,omitempty"`
}

// registerGroupTools 注册 group 工具。
func registerGroupTools(s *mcp.Server, deps *Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "group",
		Description: "群组管理。action: members 成员 / invite 拉人 / kick 踢人 / promote 设管理员 / permissions 权限 / approve 审批 / join 加入 / leave 退出 / invites 邀请链接 / comments 评论区",
		Annotations: &mcp.ToolAnnotations{Title: "群组", ReadOnlyHint: false},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in GroupInput) (*mcp.CallToolResult, GroupOutput, error) {
		return handleGroup(ctx, deps, in)
	})
}

func handleGroup(ctx context.Context, deps *Deps, in GroupInput) (*mcp.CallToolResult, GroupOutput, error) {
	out := GroupOutput{Action: in.Action}

	switch in.Action {
	case "members":
		return handleMembers(ctx, deps, in, out)
	case "invite":
		return handleInvite(ctx, deps, in, out)
	case "kick":
		return handleKick(ctx, deps, in, out)
	case "promote":
		return handlePromote(ctx, deps, in, out)
	case "permissions":
		return handlePermissions(ctx, deps, in, out)
	case "approve":
		return handleApprove(ctx, deps, in, out)
	case "join":
		return handleJoin(ctx, deps, in, out)
	case "leave":
		return handleLeave(ctx, deps, in, out)
	case "invites":
		return handleInvites(ctx, deps, in, out)
	case "comments":
		return handleComments(ctx, deps, in, out)
	default:
		out.Note = fmt.Sprintf("未知 action %q", in.Action)
		return nil, out, nil
	}
}

// handleMembers 列出群成员。
func handleMembers(ctx context.Context, deps *Deps, in GroupInput, out GroupOutput) (*mcp.CallToolResult, GroupOutput, error) {
	if in.Chat == "" {
		out.Note = "members 需要 chat 参数"
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
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	ch, ok := peer.(*tg.InputPeerChannel)
	if !ok {
		out.Note = "members 只支持超级群/频道（基础群暂不支持）"
		return nil, out, nil
	}

	req := &tg.ChannelsGetParticipantsRequest{
		Channel: &tg.InputChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash},
		Filter:  &tg.ChannelParticipantsRecent{},
		Limit:   limit,
	}
	if in.Query != "" {
		req.Filter = &tg.ChannelParticipantsSearch{Q: in.Query}
	}

	res, err := acc.API().ChannelsGetParticipants(ctx, req)
	if err != nil {
		out.Note = fmt.Sprintf("获取成员失败: %v", err)
		return nil, out, nil
	}

	var participants []tg.ChannelParticipantClass
	var userClasses []tg.UserClass
	switch v := res.(type) {
	case *tg.ChannelsChannelParticipants:
		participants, userClasses = v.Participants, v.Users
	case *tg.ChannelsChannelParticipantsNotModified:
		out.Note = "成员列表未变更（无缓存数据）"
		return nil, out, nil
	}

	userMap := make(map[int64]*tg.User)
	for _, u := range userClasses {
		if user, ok := u.(*tg.User); ok {
			userMap[user.ID] = user
		}
	}

	for _, p := range participants {
		member := dto.GroupMember{}
		switch pp := p.(type) {
		case *tg.ChannelParticipant:
			member.UserID = pp.UserID
			member.Role = "member"
			member.JoinDate = int64(pp.Date)
		case *tg.ChannelParticipantSelf:
			member.UserID = pp.UserID
			member.Role = "member"
			member.JoinDate = int64(pp.Date)
		case *tg.ChannelParticipantAdmin:
			member.UserID = pp.UserID
			member.Role = "admin"
			member.JoinDate = int64(pp.Date)
		case *tg.ChannelParticipantCreator:
			member.UserID = pp.UserID
			member.Role = "creator"
		case *tg.ChannelParticipantBanned:
			member.UserID = tgclient.PeerIDFromClass(pp.Peer)
			member.Role = "banned"
		}
		if u := userMap[member.UserID]; u != nil {
			member.Name = displayName(u)
			member.Username, _ = u.GetUsername()
			member.IsBot = u.Bot
		}
		if member.UserID != 0 {
			out.Members = append(out.Members, member)
		}
	}

	out.Count = len(out.Members)
	out.Note = fmt.Sprintf("共 %d 名成员", out.Count)
	return nil, out, nil
}

// handleInvite 拉人进群。
func handleInvite(ctx context.Context, deps *Deps, in GroupInput, out GroupOutput) (*mcp.CallToolResult, GroupOutput, error) {
	users := in.Users
	if in.User != "" {
		users = append(users, in.User)
	}
	if in.Chat == "" || len(users) == 0 {
		out.Note = "invite 需要 chat 和 user/users"
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
		out.Note = "invite 只支持超级群/频道"
		return nil, out, nil
	}

	// 解析用户
	inputUsers := make([]tg.InputUserClass, 0, len(users))
	for _, u := range users {
		up, err := resolver.Resolve(ctx, u)
		if err != nil {
			out.Note = fmt.Sprintf("解析用户 %s: %v", u, err)
			return nil, out, nil
		}
		if iu, ok := up.(*tg.InputPeerUser); ok {
			inputUsers = append(inputUsers, &tg.InputUser{UserID: iu.UserID, AccessHash: iu.AccessHash})
		}
	}

	_, err = acc.API().ChannelsInviteToChannel(ctx, &tg.ChannelsInviteToChannelRequest{
		Channel: &tg.InputChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash},
		Users:   inputUsers,
	})
	if err != nil {
		out.Note = fmt.Sprintf("邀请失败: %v", err)
		return nil, out, nil
	}
	out.Count = len(inputUsers)
	out.Note = fmt.Sprintf("已邀请 %d 人", out.Count)
	return nil, out, nil
}

// handleKick 踢人。
func handleKick(ctx context.Context, deps *Deps, in GroupInput, out GroupOutput) (*mcp.CallToolResult, GroupOutput, error) {
	if in.Chat == "" || in.User == "" {
		out.Note = "kick 需要 chat 和 user"
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
		out.Note = "kick 只支持超级群/频道"
		return nil, out, nil
	}
	up, err := resolver.Resolve(ctx, in.User)
	if err != nil {
		out.Note = fmt.Sprintf("解析用户: %v", err)
		return nil, out, nil
	}
	iu, ok := up.(*tg.InputPeerUser)
	if !ok {
		out.Note = "目标不是用户"
		return nil, out, nil
	}

	_, err = acc.API().ChannelsEditBanned(ctx, &tg.ChannelsEditBannedRequest{
		Channel: &tg.InputChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash},
		Participant: &tg.InputPeerUser{UserID: iu.UserID, AccessHash: iu.AccessHash},
		BannedRights: tg.ChatBannedRights{
			ViewMessages: true,
			UntilDate:    0, // 永久
		},
	})
	if err != nil {
		out.Note = fmt.Sprintf("踢出失败: %v", err)
		return nil, out, nil
	}
	out.Note = "已踢出"
	return nil, out, nil
}

// handlePromote 设/撤管理员。
func handlePromote(ctx context.Context, deps *Deps, in GroupInput, out GroupOutput) (*mcp.CallToolResult, GroupOutput, error) {
	if in.Chat == "" || in.User == "" {
		out.Note = "promote 需要 chat 和 user"
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
		out.Note = "promote 只支持超级群/频道"
		return nil, out, nil
	}
	up, err := resolver.Resolve(ctx, in.User)
	if err != nil {
		out.Note = fmt.Sprintf("解析用户: %v", err)
		return nil, out, nil
	}
	iu, ok := up.(*tg.InputPeerUser)
	if !ok {
		out.Note = "目标不是用户"
		return nil, out, nil
	}

	// 构建权限
	rights := tg.ChatAdminRights{}
	for _, r := range in.Rights {
		switch r {
		case "change_info":
			rights.ChangeInfo = true
		case "delete_messages":
			rights.DeleteMessages = true
		case "ban_users":
			rights.BanUsers = true
		case "invite_users":
			rights.InviteUsers = true
		case "pin_messages":
			rights.PinMessages = true
		case "add_admins":
			rights.AddAdmins = true
		case "manage_topics":
			rights.ManageTopics = true
		case "post_messages":
			rights.PostMessages = true
		case "edit_messages":
			rights.EditMessages = true
		}
	}

	_, err = acc.API().ChannelsEditAdmin(ctx, &tg.ChannelsEditAdminRequest{
		Channel:     &tg.InputChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash},
		UserID:      &tg.InputUser{UserID: iu.UserID, AccessHash: iu.AccessHash},
		AdminRights: rights,
		Rank:        "",
	})
	if err != nil {
		out.Note = fmt.Sprintf("设置管理员失败: %v", err)
		return nil, out, nil
	}
	if in.Role == "member" {
		out.Note = "已撤销管理员"
	} else {
		out.Note = "已设为管理员"
	}
	return nil, out, nil
}

// handlePermissions 查看群权限。
func handlePermissions(ctx context.Context, deps *Deps, in GroupInput, out GroupOutput) (*mcp.CallToolResult, GroupOutput, error) {
	if in.Chat == "" {
		out.Note = "permissions 需要 chat 参数"
		return nil, out, nil
	}
	full, err := fetchChatFull(ctx, deps, in.Account, mustResolve(ctx, deps, in.Account, in.Chat), in.Rights)
	if err != nil {
		out.Note = fmt.Sprintf("获取权限失败: %v", err)
		return nil, out, nil
	}
	out.Chat = full
	out.Note = "ok"
	return nil, out, nil
}

// handleApprove 审批入群请求。
func handleApprove(ctx context.Context, deps *Deps, in GroupInput, out GroupOutput) (*mcp.CallToolResult, GroupOutput, error) {
	if in.Chat == "" {
		out.Note = "approve 需要 chat 参数"
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
		out.Note = "approve 只支持超级群/频道"
		return nil, out, nil
	}

	// 审批动作
	up, err := resolver.Resolve(ctx, in.User)
	if err != nil {
		out.Note = fmt.Sprintf("解析用户: %v", err)
		return nil, out, nil
	}
	iu, ok := up.(*tg.InputPeerUser)
	if !ok {
		out.Note = "目标不是用户"
		return nil, out, nil
	}

	approve := in.Role != "reject"
	_, err = acc.API().MessagesHideChatJoinRequest(ctx, &tg.MessagesHideChatJoinRequestRequest{
		Peer:     peer,
		UserID:   &tg.InputUser{UserID: iu.UserID, AccessHash: iu.AccessHash},
		Approved: approve,
	})
	if err != nil {
		out.Note = fmt.Sprintf("审批失败: %v", err)
		return nil, out, nil
	}
	if approve {
		out.Note = "已批准入群"
	} else {
		out.Note = "已拒绝入群"
	}
	_ = ch
	return nil, out, nil
}

// handleJoin 加入群/频道。
func handleJoin(ctx context.Context, deps *Deps, in GroupInput, out GroupOutput) (*mcp.CallToolResult, GroupOutput, error) {
	target := in.Chat
	if in.Link != "" {
		target = in.Link
	}
	if target == "" {
		out.Note = "join 需要 chat 或 link"
		return nil, out, nil
	}
	acc, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}

	// 邀请链接走 importChatInvite
	if in.Link != "" || isInviteLink(target) {
		hash := extractInviteHash(target)
		if hash == "" {
			out.Note = "无法解析邀请链接"
			return nil, out, nil
		}
		// 先检查邀请信息
		inv, err := acc.API().MessagesCheckChatInvite(ctx, hash)
		if err == nil {
			if _, already := inv.(*tg.ChatInviteAlready); already {
				out.Note = "已在群中"
				return nil, out, nil
			}
			if ci, ok := inv.(*tg.ChatInvite); ok && ci.RequestNeeded {
				out.Note = fmt.Sprintf("该群需要管理员审批（群名：%s，%d 人），已提交申请", ci.Title, ci.ParticipantsCount)
			}
		}
		res, err := acc.API().MessagesImportChatInvite(ctx, hash)
		if err != nil {
			out.Note = fmt.Sprintf("加入失败: %v", err)
			return nil, out, nil
		}
		out.Note = describeJoinResult(res)
		return nil, out, nil
	}

	// 公开群/频道走 joinChannel
	peer, err := resolver.Resolve(ctx, target)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	ch, ok := peer.(*tg.InputPeerChannel)
	if !ok {
		out.Note = "join 只支持频道/超级群"
		return nil, out, nil
	}
	res, err := acc.API().ChannelsJoinChannel(ctx, &tg.InputChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash})
	if err != nil {
		out.Note = fmt.Sprintf("加入失败: %v", err)
		return nil, out, nil
	}
	out.Note = describeJoinResult(res)
	return nil, out, nil
}

// handleLeave 退出群/频道。
func handleLeave(ctx context.Context, deps *Deps, in GroupInput, out GroupOutput) (*mcp.CallToolResult, GroupOutput, error) {
	if in.Chat == "" {
		out.Note = "leave 需要 chat 参数"
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
		out.Note = "leave 只支持频道/超级群"
		return nil, out, nil
	}
	_, err = acc.API().ChannelsLeaveChannel(ctx, &tg.InputChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash})
	if err != nil {
		out.Note = fmt.Sprintf("退出失败: %v", err)
		return nil, out, nil
	}
	out.Note = "已退出"
	return nil, out, nil
}

// handleInvites 管理邀请链接。
func handleInvites(ctx context.Context, deps *Deps, in GroupInput, out GroupOutput) (*mcp.CallToolResult, GroupOutput, error) {
	if in.Chat == "" {
		out.Note = "invites 需要 chat 参数"
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
		out.Note = "invites 只支持频道/超级群"
		return nil, out, nil
	}

	// 新建一个永久邀请链接
	res, err := acc.API().MessagesExportChatInvite(ctx, &tg.MessagesExportChatInviteRequest{
		Peer: peer,
	})
	if err != nil {
		out.Note = fmt.Sprintf("创建邀请链接失败: %v", err)
		return nil, out, nil
	}
	link := InviteLink{}
	switch v := res.(type) {
	case *tg.ChatInviteExported:
		link.Link = v.Link
		if t, ok := v.GetTitle(); ok {
			link.Title = t
		}
		link.Permanent = v.Permanent
		if exp, ok := v.GetExpireDate(); ok {
			link.ExpiresAt = int64(exp)
		}
	case *tg.ChatInvitePublicJoinRequests:
		link.Link = ""
		link.Title = "需要审批的邀请链接"
	}
	out.Invites = []InviteLink{link}
	out.Note = "已生成邀请链接"
	_ = ch
	return nil, out, nil
}

// handleComments 频道评论区。
func handleComments(ctx context.Context, deps *Deps, in GroupInput, out GroupOutput) (*mcp.CallToolResult, GroupOutput, error) {
	if in.Chat == "" || in.MsgID == 0 {
		out.Note = "comments 需要 chat（频道）和 msg_id（帖子）"
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
		out.Note = "comments 需要频道/超级群"
		return nil, out, nil
	}

	// 发评论
	if in.Text != "" {
		res, err := acc.API().MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
			Peer:     peer,
			Message:  in.Text,
			RandomID: randomID(),
			ReplyTo:  &tg.InputReplyToMessage{ReplyToMsgID: in.MsgID},
		})
		if err != nil {
			out.Note = fmt.Sprintf("发评论失败: %v", err)
			return nil, out, nil
		}
		if id, _ := extractSentMessageID(res); id != 0 {
			out.MsgID = id
			out.Note = fmt.Sprintf("已发评论，消息 ID %d", id)
			return nil, out, nil
		}
		out.Note = "已发评论"
		return nil, out, nil
	}

	// 读评论
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	res, err := acc.API().MessagesGetReplies(ctx, &tg.MessagesGetRepliesRequest{
		Peer:      peer,
		MsgID:     in.MsgID,
		OffsetID:  0,
		OffsetDate: 0,
		AddOffset: 0,
		Limit:     limit,
		MaxID:     0,
		MinID:     0,
	})
	if err != nil {
		out.Note = fmt.Sprintf("读取评论失败: %v", err)
		return nil, out, nil
	}
	msgs, users, chats := extractMessagesFull(res)
	out.Comments = projectMessagesWithEntities(msgs, users, chats, nil)
	out.Comments = wrapMessagesWithTrust(ctx, deps, msgs, out.Comments)
	out.Count = len(out.Comments)
	out.Note = fmt.Sprintf("共 %d 条评论", out.Count)
	_ = ch
	return nil, out, nil
}

// 辅助函数

func isInviteLink(s string) bool {
	return len(s) > 0 && (containsStr(s, "t.me/+") || containsStr(s, "joinchat/"))
}

func extractInviteHash(s string) string {
	if i := indexStr(s, "t.me/+"); i >= 0 {
		return s[i+len("t.me/+"):]
	}
	if i := indexStr(s, "joinchat/"); i >= 0 {
		return s[i+len("joinchat/"):]
	}
	return ""
}

func containsStr(s, sub string) bool { return indexStr(s, sub) >= 0 }

func indexStr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func describeJoinResult(res tg.MessagesChatInviteJoinResultClass) string {
	switch v := res.(type) {
	case *tg.MessagesChatInviteJoinResultOk:
		if upd, ok := v.Updates.(*tg.Updates); ok {
			for _, u := range upd.Updates {
				if _, ok := u.(*tg.UpdateChannelParticipant); ok {
					return "已加入"
				}
			}
		}
		return "已加入"
	case *tg.MessagesChatInviteJoinResultWebView:
		return fmt.Sprintf("需要 Web App 验证（Bot ID %d），请用 botuse 打开", v.BotID)
	default:
		return "操作完成"
	}
}

func mustResolve(ctx context.Context, deps *Deps, account, target string) tg.InputPeerClass {
	_, r, _, err := deps.withAccount(ctx, account)
	if err != nil {
		return nil
	}
	peer, err := r.Resolve(ctx, target)
	if err != nil {
		return nil
	}
	return peer
}
