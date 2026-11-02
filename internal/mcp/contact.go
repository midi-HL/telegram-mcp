package mcp

import (
	"context"
	"fmt"

	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/midi-HL/telegram-mcp/internal/dto"
)

// ContactInput 是 contact 工具的输入。
type ContactInput struct {
	// Action: info / list / search / add / block / resolve
	Action  string `json:"action" jsonschema:"info|list|search|add|block|resolve"`
	User    string `json:"user,omitempty" jsonschema:"user"`
	Phone   string `json:"phone,omitempty" jsonschema:"phone"`
	FirstName string `json:"first_name,omitempty" jsonschema:"first name"`
	LastName  string `json:"last_name,omitempty" jsonschema:"last name"`
	Query   string `json:"query,omitempty" jsonschema:"query"`
	Limit   int    `json:"limit,omitempty" jsonschema:"default 50"`
	Expand  []string `json:"expand,omitempty" jsonschema:"full"`
	Account string `json:"account,omitempty" jsonschema:"account name"`
}

// ContactOutput 是 contact 工具的输出。
type ContactOutput struct {
	Action   string              `json:"action"`
	Profile  *dto.UserProfile    `json:"profile,omitempty"`
	Contacts []dto.UserProfile   `json:"contacts,omitempty"`
	Count    int                 `json:"count,omitempty"`
	Note     string              `json:"note,omitempty"`
}

// registerContactTools 注册 contact 工具。
func registerContactTools(s *mcp.Server, deps *Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "contact",
		Description: "联系人与用户资料。action: info 用户资料页 / list 联系人 / search 搜索 / add 添加 / block 拉黑 / resolve 解析用户名",
		Annotations: &mcp.ToolAnnotations{Title: "联系人", ReadOnlyHint: false},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ContactInput) (*mcp.CallToolResult, ContactOutput, error) {
		return handleContact(ctx, deps, in)
	})
}

func handleContact(ctx context.Context, deps *Deps, in ContactInput) (*mcp.CallToolResult, ContactOutput, error) {
	out := ContactOutput{Action: in.Action}

	switch in.Action {
	case "info":
		return handleContactInfo(ctx, deps, in, out)
	case "", "list":
		out.Action = "list"
		return handleContactList(ctx, deps, in, out)
	case "search":
		return handleContactSearch(ctx, deps, in, out)
	case "add":
		return handleContactAdd(ctx, deps, in, out)
	case "block":
		return handleContactBlock(ctx, deps, in, out)
	case "resolve":
		return handleContactResolve(ctx, deps, in, out)
	default:
		out.Note = fmt.Sprintf("未知 action %q", in.Action)
		return nil, out, nil
	}
}

// handleContactInfo 获取用户资料页（对应客户端点开某人看到的内容）。
func handleContactInfo(ctx context.Context, deps *Deps, in ContactInput, out ContactOutput) (*mcp.CallToolResult, ContactOutput, error) {
	if in.User == "" {
		out.Note = "info 需要 user 参数"
		return nil, out, nil
	}
	acc, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	peer, err := resolver.Resolve(ctx, in.User)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	up, ok := peer.(*tg.InputPeerUser)
	if !ok {
		out.Note = "info 只支持用户（群/频道请用 chat info）"
		return nil, out, nil
	}

	// 拉取完整资料
	res, err := acc.API().UsersGetFullUser(ctx, &tg.InputUser{UserID: up.UserID, AccessHash: up.AccessHash})
	if err != nil {
		out.Note = fmt.Sprintf("获取资料失败: %v", err)
		return nil, out, nil
	}
	var u *tg.User
	for _, uc := range res.Users {
		if user, ok := uc.(*tg.User); ok {
			u = user
			break
		}
	}
	if u == nil {
		out.Note = "用户资料为空"
		return nil, out, nil
	}

	profile := projectUserProfile(u, &res.FullUser)

	// 个人频道
	if res.FullUser.PersonalChannelID != 0 {
		if ref := fetchChannelRef(ctx, acc.API(), res.FullUser.PersonalChannelID); ref != nil {
			profile.PersonalChannel = ref
		}
	}

	// 写缓存
	if deps.Cache != nil {
		_ = deps.Cache.UpsertUser(ctx, *profile)
	}

	out.Profile = profile
	out.Note = "ok"
	return nil, out, nil
}

// handleContactList 列出联系人。
func handleContactList(ctx context.Context, deps *Deps, in ContactInput, out ContactOutput) (*mcp.CallToolResult, ContactOutput, error) {
	acc, _, err := deps.Account(in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}

	res, err := acc.API().ContactsGetContacts(ctx, 0)
	if err != nil {
		out.Note = fmt.Sprintf("获取联系人失败: %v", err)
		return nil, out, nil
	}

	var users []tg.UserClass
	if list, ok := res.(*tg.ContactsContacts); ok {
		users = list.Users
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 50
	}

	for _, uc := range users {
		if len(out.Contacts) >= limit {
			break
		}
		u, ok := uc.(*tg.User)
		if !ok {
			continue
		}
		out.Contacts = append(out.Contacts, dto.UserProfile{
			ID:        u.ID,
			FirstName: u.FirstName,
			LastName:  u.LastName,
			Username:  mustUsername(u),
			Phone:     mustPhone(u),
			IsContact: true,
		})
	}
	out.Count = len(out.Contacts)
	out.Note = fmt.Sprintf("共 %d 个联系人", out.Count)
	return nil, out, nil
}

// handleContactSearch 搜索联系人。
func handleContactSearch(ctx context.Context, deps *Deps, in ContactInput, out ContactOutput) (*mcp.CallToolResult, ContactOutput, error) {
	acc, _, err := deps.Account(in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}

	res, err := acc.API().ContactsSearch(ctx, &tg.ContactsSearchRequest{
		Q:     in.Query,
		Limit: limit,
	})
	if err != nil {
		out.Note = fmt.Sprintf("搜索失败: %v", err)
		return nil, out, nil
	}
	for _, uc := range res.Users {
		if u, ok := uc.(*tg.User); ok {
			out.Contacts = append(out.Contacts, dto.UserProfile{
				ID:        u.ID,
				FirstName: u.FirstName,
				LastName:  u.LastName,
				Username:  mustUsername(u),
			})
		}
	}
	out.Count = len(out.Contacts)
	out.Note = fmt.Sprintf("找到 %d 个", out.Count)
	return nil, out, nil
}

// handleContactAdd 添加联系人。
func handleContactAdd(ctx context.Context, deps *Deps, in ContactInput, out ContactOutput) (*mcp.CallToolResult, ContactOutput, error) {
	acc, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}

	var userID int64
	var accessHash int64

	if in.User != "" {
		peer, err := resolver.Resolve(ctx, in.User)
		if err != nil {
			out.Note = err.Error()
			return nil, out, nil
		}
		if up, ok := peer.(*tg.InputPeerUser); ok {
			userID = up.UserID
			accessHash = up.AccessHash
		}
	} else if in.Phone != "" {
		// 通过手机号导入
		res, err := acc.API().ContactsImportContacts(ctx, []tg.InputPhoneContact{
			{
				ClientID:  randomID(),
				Phone:     in.Phone,
				FirstName: in.FirstName,
				LastName:  in.LastName,
			},
		})
		if err != nil {
			out.Note = fmt.Sprintf("导入联系人失败: %v", err)
			return nil, out, nil
		}
		if len(res.Imported) > 0 {
			userID = res.Imported[0].UserID
			for _, uc := range res.Users {
				if u, ok := uc.(*tg.User); ok && u.ID == userID {
					accessHash = u.AccessHash
					break
				}
			}
		}
	} else {
		out.Note = "add 需要 user 或 phone"
		return nil, out, nil
	}

	if userID == 0 {
		out.Note = "未能解析到用户（手机号可能未注册 Telegram）"
		return nil, out, nil
	}

	// 如果已有 access_hash，用 addContact
	if accessHash != 0 {
		_, err = acc.API().ContactsAddContact(ctx, &tg.ContactsAddContactRequest{
			ID: &tg.InputUser{UserID: userID, AccessHash: accessHash},
			FirstName: in.FirstName,
			LastName:  in.LastName,
			Phone:     in.Phone,
		})
		if err != nil {
			out.Note = fmt.Sprintf("添加失败: %v", err)
			return nil, out, nil
		}
	}

	out.Note = fmt.Sprintf("已添加联系人（user_id=%d）", userID)
	return nil, out, nil
}

// handleContactBlock 拉黑/取消拉黑。
func handleContactBlock(ctx context.Context, deps *Deps, in ContactInput, out ContactOutput) (*mcp.CallToolResult, ContactOutput, error) {
	if in.User == "" {
		out.Note = "block 需要 user 参数"
		return nil, out, nil
	}
	acc, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	peer, err := resolver.Resolve(ctx, in.User)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}

	block := in.Query != "unblock"
	if block {
		_, err = acc.API().ContactsBlock(ctx, &tg.ContactsBlockRequest{ID: peer})
	} else {
		_, err = acc.API().ContactsUnblock(ctx, &tg.ContactsUnblockRequest{ID: peer})
	}
	if err != nil {
		out.Note = fmt.Sprintf("操作失败: %v", err)
		return nil, out, nil
	}
	if block {
		out.Note = "已拉黑"
	} else {
		out.Note = "已取消拉黑"
	}
	return nil, out, nil
}

// handleContactResolve 只解析用户名，不拉资料。
func handleContactResolve(ctx context.Context, deps *Deps, in ContactInput, out ContactOutput) (*mcp.CallToolResult, ContactOutput, error) {
	if in.User == "" {
		out.Note = "resolve 需要 user 参数"
		return nil, out, nil
	}
	_, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	peer, err := resolver.Resolve(ctx, in.User)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	id := PeerIDOf(peer)
	out.Profile = &dto.UserProfile{ID: id}
	out.Note = fmt.Sprintf("解析成功：ID=%d", id)
	return nil, out, nil
}

// projectUserProfile 把 gotd User + UserFull 投影成 UserProfile。
func projectUserProfile(u *tg.User, full *tg.UserFull) *dto.UserProfile {
	p := &dto.UserProfile{
		ID:         u.ID,
		FirstName:  u.FirstName,
		LastName:   u.LastName,
		IsBot:      u.Bot,
		IsPremium:  u.Premium,
		IsVerified: u.Verified,
		IsContact:  u.Contact,
		IsMutual:   u.MutualContact,
	}
	p.Username, _ = u.GetUsername()
	p.Phone, _ = u.GetPhone()
	for _, un := range u.Usernames {
		p.Usernames = append(p.Usernames, un.Username)
	}

	if st := u.Status; st != nil {
		p.Status, p.LastSeen = dto.FormatStatus(st)
	}

	if u.Photo != nil {
		if ph, ok := u.Photo.(*tg.UserProfilePhoto); ok {
			p.HasPhoto = true
			p.PhotoID = ph.PhotoID
		}
	}

	if full != nil {
		p.Bio = full.About
		p.IsBlocked = full.Blocked
		p.CommonChats = full.CommonChatsCount
		if b, ok := full.GetBirthday(); ok {
			p.Birthday = dto.FormatBirthday(b)
		}
	}
	return p
}

// fetchChannelRef 获取频道简要信息。
func fetchChannelRef(ctx context.Context, api *tg.Client, channelID int64) *dto.ChannelRef {
	// 需要 access_hash，从对话列表里查
	res, err := api.ChannelsGetChannels(ctx, []tg.InputChannelClass{
		&tg.InputChannel{ChannelID: channelID},
	})
	if err != nil {
		return nil
	}
	var chats []tg.ChatClass
	switch v := res.(type) {
	case *tg.MessagesChats:
		chats = v.Chats
	case *tg.MessagesChatsSlice:
		chats = v.Chats
	}
	for _, c := range chats {
		if ch, ok := c.(*tg.Channel); ok {
			ref := &dto.ChannelRef{
				ID:    ch.ID,
				Title: ch.Title,
			}
			ref.Username, _ = ch.GetUsername()
			if p, ok := ch.GetParticipantsCount(); ok {
				ref.Subscribers = p
			}
			return ref
		}
	}
	return nil
}

func mustUsername(u *tg.User) string {
	v, _ := u.GetUsername()
	return v
}

func mustPhone(u *tg.User) string {
	v, _ := u.GetPhone()
	return v
}
