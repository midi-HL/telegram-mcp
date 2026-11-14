// Package cache 提供基于 SQLite 的本地缓存。
// 使用 modernc.org/sqlite（纯 Go，无 cgo），保证 Termux/proot 可运行。
package cache

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite"

	"github.com/midi-HL/telegram-mcp/internal/dto"
)

//go:embed schema.sql
var schemaSQL string

const (
	// MaxMessagesPerChat 是每个聊天保留的最大消息数。
	MaxMessagesPerChat = 200
	// MaxDBSizeBytes 是数据库总量上限（500MB）。
	MaxDBSizeBytes int64 = 500 * 1024 * 1024
)

// Store 是缓存存储。
type Store struct {
	db   *sql.DB
	path string
}

// Open 打开（或创建）缓存数据库。
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("创建数据目录: %w", err)
	}
	path := filepath.Join(dataDir, "cache.db")
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库 %s: %w", path, err)
	}
	// SQLite 单写多读，限制连接数避免锁竞争
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if _, err := db.Exec(schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("初始化 schema: %w", err)
	}

	s := &Store{db: db, path: path}
	if err := s.setMeta("schema_version", "1"); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭数据库。
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Path 返回数据库文件路径。
func (s *Store) Path() string { return s.path }

// setMeta 写入元数据。
func (s *Store) setMeta(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO meta(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// GetMeta 读取元数据。
func (s *Store) GetMeta(ctx context.Context, key string) (string, bool) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, key).Scan(&v)
	if err != nil {
		return "", false
	}
	return v, true
}

// UpsertChat 写入或更新聊天。
func (s *Store) UpsertChat(ctx context.Context, c dto.ChatBrief, extra ChatExtra) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO chats(id,title,type,username,about,member_count,unread,muted,pinned,
		                  can_send,is_admin,is_creator,last_text,last_at,access_hash,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			title=excluded.title, type=excluded.type, username=excluded.username,
			about=excluded.about, member_count=excluded.member_count,
			unread=excluded.unread, muted=excluded.muted, pinned=excluded.pinned,
			can_send=excluded.can_send, is_admin=excluded.is_admin,
			is_creator=excluded.is_creator, last_text=excluded.last_text,
			last_at=excluded.last_at, access_hash=excluded.access_hash,
			updated_at=excluded.updated_at`,
		c.ID, c.Title, string(c.Type), extra.Username, extra.About,
		extra.MemberCount, c.Unread, boolToInt(extra.Muted), boolToInt(extra.Pinned),
		boolToInt(extra.CanSend), boolToInt(extra.IsAdmin), boolToInt(extra.IsCreator),
		c.LastText, c.LastAt, extra.AccessHash, time.Now().Unix())
	return err
}

// ChatExtra 是 ChatBrief 之外的缓存字段。
type ChatExtra struct {
	Username    string
	About       string
	MemberCount int
	Muted       bool
	Pinned      bool
	CanSend     bool
	IsAdmin     bool
	IsCreator   bool
	AccessHash  int64
}

// ListChats 按最近活跃排序列出聊天。
func (s *Store) ListChats(ctx context.Context, limit int) ([]dto.ChatBrief, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,title,type,unread,COALESCE(last_text,''),last_at
		FROM chats ORDER BY last_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]dto.ChatBrief, 0, limit)
	for rows.Next() {
		var c dto.ChatBrief
		var typ string
		if err := rows.Scan(&c.ID, &c.Title, &typ, &c.Unread, &c.LastText, &c.LastAt); err != nil {
			return nil, err
		}
		c.Type = dto.ChatType(typ)
		c.IDString = strconv.FormatInt(c.ID, 10)
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetChat 读取单个聊天。
func (s *Store) GetChat(ctx context.Context, id int64) (*dto.ChatFull, error) {
	var c dto.ChatFull
	var typ, username, about string
	var muted, pinned, canSend, isAdmin, isCreator int
	err := s.db.QueryRowContext(ctx, `
		SELECT id,title,type,unread,COALESCE(last_text,''),last_at,
		       COALESCE(username,''),COALESCE(about,''),member_count,
		       muted,pinned,can_send,is_admin,is_creator
		FROM chats WHERE id=?`, id).Scan(
		&c.ID, &c.Title, &typ, &c.Unread, &c.LastText, &c.LastAt,
		&username, &about, &c.MemberCount, &muted, &pinned, &canSend, &isAdmin, &isCreator)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	c.Type = dto.ChatType(typ)
	c.IDString = strconv.FormatInt(c.ID, 10)
	c.Username = username
	c.About = about
	c.Muted = muted != 0
	c.Pinned = pinned != 0
	c.CanSendMessage = canSend != 0
	c.IsAdmin = isAdmin != 0
	c.IsCreator = isCreator != 0
	return &c, nil
}

// AccessHash 返回聊天的 access_hash（用于重建 InputPeer）。
func (s *Store) AccessHash(ctx context.Context, id int64) (int64, bool) {
	var h int64
	err := s.db.QueryRowContext(ctx, `SELECT access_hash FROM chats WHERE id=?`, id).Scan(&h)
	if err != nil {
		return 0, false
	}
	return h, true
}

// UpsertMessages 批量写入消息，并淘汰超出上限的旧消息。
func (s *Store) UpsertMessages(ctx context.Context, chatID int64, msgs []dto.MessageFull) error {
	if len(msgs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO messages(chat_id,msg_id,sender_id,sender_name,date,text,
		                     media_kind,media_meta,reply_to,out,edited_at,views)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(chat_id,msg_id) DO UPDATE SET
			text=excluded.text, media_kind=excluded.media_kind,
			media_meta=excluded.media_meta, edited_at=excluded.edited_at,
			views=excluded.views`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, m := range msgs {
		var mediaMeta string
		if m.MediaMeta != nil {
			b, _ := json.Marshal(m.MediaMeta)
			mediaMeta = string(b)
		}
		_, err := stmt.ExecContext(ctx, chatID, m.ID, m.SenderID, m.Sender,
			m.Date, m.Text, m.Media, mediaMeta, m.ReplyTo, boolToInt(m.Out),
			m.EditedAt, m.Views)
		if err != nil {
			return err
		}
	}

	// 淘汰：只保留最近 MaxMessagesPerChat 条
	_, err = tx.ExecContext(ctx, `
		DELETE FROM messages WHERE chat_id=? AND msg_id NOT IN (
			SELECT msg_id FROM messages WHERE chat_id=? ORDER BY date DESC LIMIT ?
		)`, chatID, chatID, MaxMessagesPerChat)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ListMessages 读取某聊天的消息（按时间倒序）。
func (s *Store) ListMessages(ctx context.Context, chatID int64, limit int) ([]dto.MessageFull, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT msg_id,date,sender_id,COALESCE(sender_name,''),COALESCE(text,''),
		       COALESCE(media_kind,''),COALESCE(media_meta,''),reply_to,out,edited_at,views
		FROM messages WHERE chat_id=? ORDER BY date DESC LIMIT ?`, chatID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]dto.MessageFull, 0, limit)
	for rows.Next() {
		var m dto.MessageFull
		var mediaMeta string
		var outFlag int
		if err := rows.Scan(&m.ID, &m.Date, &m.SenderID, &m.Sender, &m.Text,
			&m.Media, &mediaMeta, &m.ReplyTo, &outFlag, &m.EditedAt, &m.Views); err != nil {
			return nil, err
		}
		m.Out = outFlag != 0
		if mediaMeta != "" {
			var mm dto.MediaMeta
			if json.Unmarshal([]byte(mediaMeta), &mm) == nil {
				m.MediaMeta = &mm
			}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// CountMessages 统计某聊天缓存的消息数。
func (s *Store) CountMessages(ctx context.Context, chatID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE chat_id=?`, chatID).Scan(&n)
	return n, err
}

// UpsertUser 写入或更新用户资料缓存。
func (s *Store) UpsertUser(ctx context.Context, p dto.UserProfile) error {
	usernames, _ := json.Marshal(p.Usernames)
	var channelID int64
	if p.PersonalChannel != nil {
		channelID = p.PersonalChannel.ID
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users(id,access_hash,first_name,last_name,username,usernames,phone,
		                  bio,status_text,last_seen,is_bot,is_premium,is_verified,
		                  is_contact,is_mutual,birthday,common_chats,photo_id,channel_id,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			first_name=excluded.first_name, last_name=excluded.last_name,
			username=excluded.username, usernames=excluded.usernames,
			phone=excluded.phone, bio=excluded.bio, status_text=excluded.status_text,
			last_seen=excluded.last_seen, is_bot=excluded.is_bot,
			is_premium=excluded.is_premium, is_verified=excluded.is_verified,
			is_contact=excluded.is_contact, is_mutual=excluded.is_mutual,
			birthday=excluded.birthday, common_chats=excluded.common_chats,
			photo_id=excluded.photo_id, channel_id=excluded.channel_id,
			updated_at=excluded.updated_at`,
		p.ID, int64(0), p.FirstName, p.LastName, p.Username, string(usernames),
		p.Phone, p.Bio, p.Status, p.LastSeen, boolToInt(p.IsBot),
		boolToInt(p.IsPremium), boolToInt(p.IsVerified), boolToInt(p.IsContact),
		boolToInt(p.IsMutual), p.Birthday, p.CommonChats, p.PhotoID, channelID,
		time.Now().Unix())
	return err
}

// GetUser 读取用户缓存。
func (s *Store) GetUser(ctx context.Context, id int64) (*dto.UserProfile, error) {
	var p dto.UserProfile
	var usernames, status, birthday string
	var isBot, isPremium, isVerified, isContact, isMutual int
	err := s.db.QueryRowContext(ctx, `
		SELECT id,COALESCE(first_name,''),COALESCE(last_name,''),COALESCE(username,''),
		       COALESCE(usernames,'[]'),COALESCE(phone,''),COALESCE(bio,''),
		       COALESCE(status_text,''),last_seen,is_bot,is_premium,is_verified,
		       is_contact,is_mutual,COALESCE(birthday,''),common_chats,photo_id
		FROM users WHERE id=?`, id).Scan(
		&p.ID, &p.FirstName, &p.LastName, &p.Username, &usernames, &p.Phone,
		&p.Bio, &status, &p.LastSeen, &isBot, &isPremium, &isVerified,
		&isContact, &isMutual, &birthday, &p.CommonChats, &p.PhotoID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	p.Status = status
	p.Birthday = birthday
	p.IsBot = isBot != 0
	p.IsPremium = isPremium != 0
	p.IsVerified = isVerified != 0
	p.IsContact = isContact != 0
	p.IsMutual = isMutual != 0
	_ = json.Unmarshal([]byte(usernames), &p.Usernames)
	return &p, nil
}

// Stats 返回缓存统计。
func (s *Store) Stats(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	var chats, msgs, users int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chats`).Scan(&chats); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages`).Scan(&msgs); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&users); err != nil {
		return nil, err
	}
	out["chats"] = chats
	out["messages"] = msgs
	out["users"] = users
	out["db_path"] = s.path
	if fi, err := os.Stat(s.path); err == nil {
		out["db_size_bytes"] = fi.Size()
		out["db_size_mb"] = float64(fi.Size()) / 1024 / 1024
	}
	return out, nil
}

// EnforceSizeLimit 检查数据库大小，超限时删除最旧消息。
func (s *Store) EnforceSizeLimit(ctx context.Context) (deleted int64, err error) {
	fi, err := os.Stat(s.path)
	if err != nil {
		return 0, err
	}
	if fi.Size() <= MaxDBSizeBytes {
		return 0, nil
	}
	// 删除最旧的一半消息
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM messages WHERE msg_id IN (
			SELECT msg_id FROM messages ORDER BY date ASC LIMIT
			(SELECT COUNT(*)/2 FROM messages)
		)`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// QueryContext 执行查询（供其他包使用）。
func (s *Store) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, query, args...)
}

// QueryRowContext 执行单行查询。
func (s *Store) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, query, args...)
}

// ExecContext 执行写操作。
func (s *Store) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, query, args...)
}
