// Package dto 提供投影 DTO，用于把 gotd 的完整对象压缩成
// 模型真正需要的字段，大幅降低 token 消耗。
//
// 实测数据：getChats(100) 全字段约 911KB（≈266k tokens），
// 投影后约 60KB（≈17k tokens），节省 93%。
package dto

import (
	"time"

	"github.com/gotd/td/tg"
)

// 消息正文截断长度，防止超长文本撑爆上下文。
const (
	// MaxPreviewRunes 是聊天列表里"最后一句"的最大字符数。
	MaxPreviewRunes = 100
	// MaxTextRunes 是单条消息文本的最大字符数（expand 时可调大）。
	MaxTextRunes = 2000
)

// ChatType 是聊天类型。
type ChatType string

const (
	ChatTypeUser       ChatType = "user"
	ChatTypeBasicGroup ChatType = "group"
	ChatTypeSupergroup ChatType = "supergroup"
	ChatTypeChannel    ChatType = "channel"
	ChatTypeSecret     ChatType = "secret"
)

// ChatBrief 是聊天列表的精简表示（5 字段，约 120 字节）。
type ChatBrief struct {
	ID       int64    `json:"id"`
	Title    string   `json:"title"`
	Type     ChatType `json:"type"`
	Unread   int      `json:"unread,omitempty"`
	LastText string   `json:"last,omitempty"`
	LastAt   int64    `json:"at,omitempty"`
}

// ChatFull 是聊天详情（expand=["full"] 时返回）。
type ChatFull struct {
	ChatBrief
	About          string   `json:"about,omitempty"`
	Username       string   `json:"username,omitempty"`
	MemberCount    int      `json:"members,omitempty"`
	Pinned         bool     `json:"pinned,omitempty"`
	Muted          bool     `json:"muted,omitempty"`
	CanSendMessage bool     `json:"can_send"`
	IsAdmin        bool     `json:"is_admin,omitempty"`
	IsCreator      bool     `json:"is_creator,omitempty"`
	Permissions    []string `json:"permissions,omitempty"`
}

// MessageBrief 是消息的精简表示（6 字段）。
type MessageBrief struct {
	ID       int    `json:"id"`
	Date     int64  `json:"date"`
	SenderID int64  `json:"sender,omitempty"`
	Sender   string `json:"sender_name,omitempty"`
	Text     string `json:"text,omitempty"`
	Media    string `json:"media,omitempty"`
	Out      bool   `json:"out,omitempty"`
	// Trust 是信任级别：admin / self / external
	Trust string `json:"trust,omitempty"`
	// ReplyTo 是被引用消息的 ID
	ReplyTo int `json:"reply_to,omitempty"`
	// Reply 是被引用消息的展开摘要
	Reply *ReplyRef `json:"reply,omitempty"`
}

// ReplyRef 是被引用消息的摘要。
type ReplyRef struct {
	MsgID  int    `json:"msg_id"`
	Sender string `json:"sender,omitempty"`
	Text   string `json:"text,omitempty"`  // 截断到 80 字符
	Media  string `json:"media,omitempty"` // photo/video/document/voice/sticker
	// 媒体元数据（用于下载）
	MediaMeta *MediaMeta `json:"media_meta,omitempty"`
	// Deleted 表示原消息已删除或无法获取
	Deleted bool `json:"deleted,omitempty"`
	// QuoteText 是引用片段（部分引用时）
	QuoteText string `json:"quote,omitempty"`
}

// MessageFull 是消息详情（expand 时返回）。
type MessageFull struct {
	MessageBrief
	Forward   string     `json:"forward,omitempty"`
	MediaMeta *MediaMeta `json:"media_meta,omitempty"`
	Entities  []string   `json:"entities,omitempty"`
	EditedAt  int64      `json:"edited,omitempty"`
	Views     int        `json:"views,omitempty"`
	Reactions []Reaction `json:"reactions,omitempty"`
}

// MediaMeta 是媒体元数据（不给内容本身）。
type MediaMeta struct {
	Kind      string `json:"kind"`
	MimeType  string `json:"mime,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Width     int    `json:"w,omitempty"`
	Height    int    `json:"h,omitempty"`
	Duration  int    `json:"dur,omitempty"`
	FileName  string `json:"file,omitempty"`
	Title     string `json:"title,omitempty"`
	Performer string `json:"performer,omitempty"`
	Emoji     string `json:"emoji,omitempty"`
	// FileID 用于后续按需下载。
	FileID int `json:"file_id,omitempty"`
}

// Reaction 是消息反应摘要。
type Reaction struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
	Mine  bool   `json:"mine,omitempty"`
}

// UserProfile 是用户资料页（对应 Telegram 客户端点开某人看到的内容）。
type UserProfile struct {
	ID          int64    `json:"id"`
	FirstName   string   `json:"first_name"`
	LastName    string   `json:"last_name,omitempty"`
	Username    string   `json:"username,omitempty"`
	Usernames   []string `json:"usernames,omitempty"`
	Phone       string   `json:"phone,omitempty"`
	Bio         string   `json:"bio,omitempty"`
	Status      string   `json:"status,omitempty"`    // 在线/最近上线时间
	LastSeen    int64    `json:"last_seen,omitempty"` // Unix 时间戳
	IsBot       bool     `json:"is_bot,omitempty"`
	IsPremium   bool     `json:"is_premium,omitempty"`
	IsVerified  bool     `json:"is_verified,omitempty"`
	IsContact   bool     `json:"is_contact,omitempty"`
	IsMutual    bool     `json:"is_mutual,omitempty"`
	IsBlocked   bool     `json:"is_blocked,omitempty"`
	Birthday    string   `json:"birthday,omitempty"` // "5月21日" 或 "1990-05-21"
	CommonChats int      `json:"common_chats,omitempty"`
	// 头像
	HasPhoto bool  `json:"has_photo"`
	PhotoID  int64 `json:"photo_id,omitempty"`
	// 个人频道
	PersonalChannel *ChannelRef `json:"channel,omitempty"`
}

// ChannelRef 是频道引用。
type ChannelRef struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Username    string `json:"username,omitempty"`
	Subscribers int    `json:"subs,omitempty"`
}

// GroupMember 是群成员。
type GroupMember struct {
	UserID   int64  `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username,omitempty"`
	Role     string `json:"role,omitempty"` // creator/admin/member/restricted/banned
	IsBot    bool   `json:"bot,omitempty"`
	JoinDate int64  `json:"joined,omitempty"`
}

// Folder 是聊天文件夹。
type Folder struct {
	ID        int          `json:"id"`
	Title     string       `json:"title"`
	Emoji     string       `json:"emoji,omitempty"`
	ChatCount int          `json:"chats,omitempty"`
	Pinned    bool         `json:"pinned,omitempty"`
	Peers     []FolderPeer `json:"peers,omitempty"`
}

// FolderPeer 是聊天文件夹里明确包含/置顶的一个会话。
// ID 用字符串输出，避免大整数在 JSON / MCP 通道里丢失精度。
type FolderPeer struct {
	ID     int64  `json:"id,string"`
	Title  string `json:"title,omitempty"`
	Type   string `json:"type,omitempty"`
	Pinned bool   `json:"pinned,omitempty"`
}

// 辅助函数

// Truncate 按字符（rune）截断字符串，超出部分加省略号。
func Truncate(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// FormatStatus 把 UserStatus 转成人类可读描述。
func FormatStatus(st tg.UserStatusClass) (text string, lastSeen int64) {
	switch v := st.(type) {
	case *tg.UserStatusOnline:
		return "在线", 0
	case *tg.UserStatusOffline:
		return "最近上线 " + time.Unix(int64(v.WasOnline), 0).Format("2006-01-02 15:04"), int64(v.WasOnline)
	case *tg.UserStatusRecently:
		return "最近上线", 0
	case *tg.UserStatusLastWeek:
		return "一周内上线", 0
	case *tg.UserStatusLastMonth:
		return "一个月内上线", 0
	case *tg.UserStatusEmpty:
		return "", 0
	default:
		return "", 0
	}
}

// FormatBirthday 把 Birthday 转成可读字符串。
func FormatBirthday(b tg.Birthday) string {
	if b.Day == 0 || b.Month == 0 {
		return ""
	}
	if b.Year != 0 {
		return time.Date(b.Year, time.Month(b.Month), b.Day, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}
	return time.Date(0, time.Month(b.Month), b.Day, 0, 0, 0, 0, time.UTC).Format("1月2日")
}

// ChatTypeOf 判断聊天类型。
func ChatTypeOf(peer tg.PeerClass) ChatType {
	switch peer.(type) {
	case *tg.PeerUser:
		return ChatTypeUser
	case *tg.PeerChat:
		return ChatTypeBasicGroup
	case *tg.PeerChannel:
		return ChatTypeSupergroup
	default:
		return ""
	}
}
