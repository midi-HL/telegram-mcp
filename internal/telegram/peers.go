package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/gotd/td/tg"
)

// PeerResolver 负责把用户输入（ID / @用户名 / 手机号 / t.me 链接）
// 解析成 gotd 的 InputPeer，并缓存 access_hash。
//
// gotd 的 telegram/peers 包官方标注 WIP，因此这里自建轻量缓存。
type PeerResolver struct {
	api *tg.Client

	mu    sync.RWMutex
	cache map[int64]tg.InputPeerClass // 按 peer ID 缓存
	byName map[string]int64           // 用户名 → ID
}

// NewPeerResolver 创建解析器。
func NewPeerResolver(api *tg.Client) *PeerResolver {
	return &PeerResolver{
		api:    api,
		cache:  make(map[int64]tg.InputPeerClass),
		byName: make(map[string]int64),
	}
}

// PeerID 从 InputPeer 提取数字 ID。
func PeerID(p tg.InputPeerClass) int64 {
	switch v := p.(type) {
	case *tg.InputPeerUser:
		return v.UserID
	case *tg.InputPeerChat:
		return v.ChatID
	case *tg.InputPeerChannel:
		return v.ChannelID
	case *tg.InputPeerSelf:
		return 0
	default:
		return 0
	}
}

// PeerIDFromClass 从 Peer 提取数字 ID。
func PeerIDFromClass(p tg.PeerClass) int64 {
	switch v := p.(type) {
	case *tg.PeerUser:
		return v.UserID
	case *tg.PeerChat:
		return v.ChatID
	case *tg.PeerChannel:
		return v.ChannelID
	default:
		return 0
	}
}

// Resolve 把用户输入解析成 InputPeer。
// 支持：
//   - 数字 ID（正数=用户，负数=群/频道，Telegram 约定 -100xxx 为频道）
//   - @username
//   - t.me/xxx 或 https://t.me/xxx
//   - "me" / "self"（自己）
func (r *PeerResolver) Resolve(ctx context.Context, input string) (tg.InputPeerClass, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("peer 输入为空")
	}

	// 自己
	if input == "me" || input == "self" {
		return &tg.InputPeerSelf{}, nil
	}

	// 先查缓存
	if id := r.lookupCache(input); id != 0 {
		r.mu.RLock()
		if p, ok := r.cache[id]; ok {
			r.mu.RUnlock()
			return p, nil
		}
		r.mu.RUnlock()
	}

	// @username 或 t.me 链接
	if name, ok := extractUsername(input); ok {
		return r.resolveUsername(ctx, name)
	}

	// 数字 ID
	if id, err := strconv.ParseInt(input, 10, 64); err == nil {
		return r.resolveID(ctx, id)
	}

	// 兜底：当作用户名尝试
	return r.resolveUsername(ctx, input)
}

// lookupCache 从用户名缓存查 ID。
func (r *PeerResolver) lookupCache(input string) int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if id, ok := r.byName[strings.ToLower(strings.TrimPrefix(input, "@"))]; ok {
		return id
	}
	return 0
}

// resolveUsername 通过 contacts.resolveUsername 解析。
func (r *PeerResolver) resolveUsername(ctx context.Context, username string) (tg.InputPeerClass, error) {
	username = strings.TrimPrefix(username, "@")
	res, err := r.api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: username})
	if err != nil {
		return nil, fmt.Errorf("解析 @%s: %w", username, err)
	}

	for _, u := range res.Users {
		if user, ok := u.(*tg.User); ok {
			uname, _ := user.GetUsername()
			if strings.EqualFold(uname, username) {
				peer := &tg.InputPeerUser{UserID: user.ID, AccessHash: user.AccessHash}
				r.store(user.ID, peer, username)
				return peer, nil
			}
			// 主用户名不匹配时看其他用户名
			for _, un := range user.Usernames {
				if strings.EqualFold(un.Username, username) {
					peer := &tg.InputPeerUser{UserID: user.ID, AccessHash: user.AccessHash}
					r.store(user.ID, peer, username)
					return peer, nil
				}
			}
		}
	}

	for _, c := range res.Chats {
		switch ch := c.(type) {
		case *tg.Channel:
			uname, _ := ch.GetUsername()
			if strings.EqualFold(uname, username) {
				peer := &tg.InputPeerChannel{ChannelID: ch.ID, AccessHash: ch.AccessHash}
				r.store(ch.ID, peer, username)
				return peer, nil
			}
			for _, un := range ch.Usernames {
				if strings.EqualFold(un.Username, username) {
					peer := &tg.InputPeerChannel{ChannelID: ch.ID, AccessHash: ch.AccessHash}
					r.store(ch.ID, peer, username)
					return peer, nil
				}
			}
		case *tg.Chat:
			peer := &tg.InputPeerChat{ChatID: ch.ID}
			r.store(ch.ID, peer, username)
			return peer, nil
		}
	}

	return nil, fmt.Errorf("找不到 @%s（可能是用户名不存在或已更改）", username)
}

// resolveID 通过数字 ID 解析。需要先拉取对话列表建立映射。
func (r *PeerResolver) resolveID(ctx context.Context, id int64) (tg.InputPeerClass, error) {
	// 负数：群/频道（Telegram 约定）
	if id < 0 {
		// 超级群/频道 ID 形如 -100xxxxxxxxxx
		realID := -id
		if realID > 1000000000000 {
			realID -= 1000000000000
		}
		// 尝试从对话列表里找
		if peer, err := r.findInDialogs(ctx, realID); err == nil {
			return peer, nil
		}
		return nil, fmt.Errorf("找不到 ID %d 的群/频道（需要先与它有对话）", id)
	}

	// 正数：用户
	if peer, err := r.findInDialogs(ctx, id); err == nil {
		return peer, nil
	}
	return nil, fmt.Errorf("找不到 ID %d 的用户（需要先与它有对话，或改用 @用户名）", id)
}

// findInDialogs 从对话列表里查找 peer 并缓存 access_hash。
func (r *PeerResolver) findInDialogs(ctx context.Context, targetID int64) (tg.InputPeerClass, error) {
	// 先查缓存
	r.mu.RLock()
	if p, ok := r.cache[targetID]; ok {
		r.mu.RUnlock()
		return p, nil
	}
	r.mu.RUnlock()

	// 分页拉取对话（最多 500 个）
	const pageSize = 100
	const maxDialogs = 500
	offsetPeer := tg.InputPeerClass(&tg.InputPeerEmpty{})
	offsetDate := 0
	offsetID := 0

	for fetched := 0; fetched < maxDialogs; fetched += pageSize {
		res, err := r.api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			OffsetDate: offsetDate,
			OffsetID:   offsetID,
			OffsetPeer: offsetPeer,
			Limit:      pageSize,
		})
		if err != nil {
			return nil, fmt.Errorf("拉取对话列表: %w", err)
		}

		users, chats := extractEntities(res)
		for _, u := range users {
			peer := &tg.InputPeerUser{UserID: u.ID, AccessHash: u.AccessHash}
			uname, _ := u.GetUsername()
			r.store(u.ID, peer, uname)
			if u.ID == targetID {
				return peer, nil
			}
		}
		for _, c := range chats {
			switch ch := c.(type) {
			case *tg.Channel:
				peer := &tg.InputPeerChannel{ChannelID: ch.ID, AccessHash: ch.AccessHash}
				uname, _ := ch.GetUsername()
				r.store(ch.ID, peer, uname)
				if ch.ID == targetID {
					return peer, nil
				}
			case *tg.Chat:
				peer := &tg.InputPeerChat{ChatID: ch.ID}
				r.store(ch.ID, peer, "")
				if ch.ID == targetID {
					return peer, nil
				}
			}
		}

		// 取下一页游标：用最后一个对话的 TopMessage 作为 offsetID
		dialogs, ok := dialogsFrom(res)
		if !ok || len(dialogs) < pageSize {
			break
		}
		last := dialogs[len(dialogs)-1]
		peer, topMsg := dialogPeerAndTop(last)
		if peer == nil {
			break
		}
		offsetID = topMsg
		offsetDate = 0
		r.mu.RLock()
		p, found := r.cache[PeerIDFromClass(peer)]
		r.mu.RUnlock()
		if !found {
			break
		}
		offsetPeer = p
	}

	return nil, fmt.Errorf("对话列表里没有 ID %d", targetID)
}

// dialogPeerAndTop 从 DialogClass 取出 peer 和 topMessage。
// DialogClass 没有 GetPeer/GetTopMessage 接口方法，需要类型断言。
func dialogPeerAndTop(d tg.DialogClass) (tg.PeerClass, int) {
	switch v := d.(type) {
	case *tg.Dialog:
		return v.Peer, v.TopMessage
	case *tg.DialogFolder:
		return v.Peer, v.TopMessage
	default:
		return nil, 0
	}
}

// store 写入缓存。
func (r *PeerResolver) store(id int64, peer tg.InputPeerClass, username string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache[id] = peer
	if username != "" {
		r.byName[strings.ToLower(username)] = id
	}
}

// extractUsername 从各种输入形式提取用户名。
func extractUsername(input string) (string, bool) {
	if strings.HasPrefix(input, "@") {
		return strings.TrimPrefix(input, "@"), true
	}
	// t.me/xxx 或 https://t.me/xxx
	for _, prefix := range []string{"https://t.me/", "http://t.me/", "t.me/", "telegram.me/"} {
		if strings.HasPrefix(strings.ToLower(input), prefix) {
			rest := input[len(prefix):]
			// 去掉路径后续部分
			if i := strings.IndexAny(rest, "/?#"); i >= 0 {
				rest = rest[:i]
			}
			// 邀请链接 +xxx 不是用户名
			if strings.HasPrefix(rest, "+") || strings.HasPrefix(rest, "joinchat") {
				return "", false
			}
			if rest != "" {
				return rest, true
			}
		}
	}
	return "", false
}

// extractEntities 从 MessagesDialogsClass 里取出 users 和 chats。
func extractEntities(res tg.MessagesDialogsClass) ([]*tg.User, []tg.ChatClass) {
	var users []tg.UserClass
	var chats []tg.ChatClass
	switch v := res.(type) {
	case *tg.MessagesDialogs:
		users, chats = v.Users, v.Chats
	case *tg.MessagesDialogsSlice:
		users, chats = v.Users, v.Chats
	default:
		return nil, nil
	}
	out := make([]*tg.User, 0, len(users))
	for _, u := range users {
		if user, ok := u.(*tg.User); ok {
			out = append(out, user)
		}
	}
	return out, chats
}

// dialogsFrom 取出对话列表。
func dialogsFrom(res tg.MessagesDialogsClass) ([]tg.DialogClass, bool) {
	switch v := res.(type) {
	case *tg.MessagesDialogs:
		return v.Dialogs, true
	case *tg.MessagesDialogsSlice:
		return v.Dialogs, true
	default:
		return nil, false
	}
}
