// Package autoreply 提供自动回复引擎。
package autoreply

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/midi-HL/telegram-mcp/internal/cache"
)

// Rule 是自动回复规则。
type Rule struct {
	ID        int64    `json:"id"`
	Enabled   bool     `json:"enabled"`
	ChatScope []string `json:"chat_scope,omitempty"` // 空=全部聊天
	Keyword   string   `json:"keyword,omitempty"`    // 空=不限制
	Prompt    string   `json:"prompt,omitempty"`     // 回复模板/提示
	CreatedAt int64    `json:"created_at"`
}

// LogEntry 是自动回复日志。
type LogEntry struct {
	ID        int64  `json:"id"`
	RuleID    int64  `json:"rule_id"`
	ChatID    int64  `json:"chat_id"`
	InMsgID   int    `json:"in_msg_id"`
	OutText   string `json:"out_text,omitempty"`
	Error     string `json:"error,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// RuleStore 管理规则的持久化。
type RuleStore struct {
	store *cache.Store
}

// NewRuleStore 创建规则存储。
func NewRuleStore(s *cache.Store) *RuleStore {
	return &RuleStore{store: s}
}

// DB 返回底层 Store（用于直接查询）。
func (r *RuleStore) DB() *cache.Store { return r.store }

// List 列出所有规则。
func (r *RuleStore) List(ctx context.Context) ([]Rule, error) {
	rows, err := r.store.QueryContext(ctx,
		`SELECT id, enabled, COALESCE(chat_scope,''), COALESCE(keyword,''),
		        COALESCE(prompt,''), created_at FROM autoreply_rules ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Rule, 0)
	for rows.Next() {
		var rule Rule
		var enabled int
		var scope string
		if err := rows.Scan(&rule.ID, &enabled, &scope, &rule.Keyword,
			&rule.Prompt, &rule.CreatedAt); err != nil {
			return nil, err
		}
		rule.Enabled = enabled != 0
		if scope != "" {
			_ = json.Unmarshal([]byte(scope), &rule.ChatScope)
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

// Get 读取单条规则。
func (r *RuleStore) Get(ctx context.Context, id int64) (*Rule, error) {
	var rule Rule
	var enabled int
	var scope string
	err := r.store.QueryRowContext(ctx,
		`SELECT id, enabled, COALESCE(chat_scope,''), COALESCE(keyword,''),
		        COALESCE(prompt,''), created_at FROM autoreply_rules WHERE id=?`, id).
		Scan(&rule.ID, &enabled, &scope, &rule.Keyword, &rule.Prompt, &rule.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	rule.Enabled = enabled != 0
	if scope != "" {
		_ = json.Unmarshal([]byte(scope), &rule.ChatScope)
	}
	return &rule, nil
}

// Save 新增或更新规则。
func (r *RuleStore) Save(ctx context.Context, rule *Rule) (int64, error) {
	scope := ""
	if len(rule.ChatScope) > 0 {
		b, _ := json.Marshal(rule.ChatScope)
		scope = string(b)
	}
	enabled := 0
	if rule.Enabled {
		enabled = 1
	}

	if rule.ID == 0 {
		res, err := r.store.ExecContext(ctx,
			`INSERT INTO autoreply_rules(enabled, chat_scope, keyword, prompt, created_at)
			 VALUES(?,?,?,?,?)`,
			enabled, scope, rule.Keyword, rule.Prompt, time.Now().Unix())
		if err != nil {
			return 0, err
		}
		id, _ := res.LastInsertId()
		rule.ID = id
		return id, nil
	}

	_, err := r.store.ExecContext(ctx,
		`UPDATE autoreply_rules SET enabled=?, chat_scope=?, keyword=?, prompt=? WHERE id=?`,
		enabled, scope, rule.Keyword, rule.Prompt, rule.ID)
	return rule.ID, err
}

// Delete 删除规则。
func (r *RuleStore) Delete(ctx context.Context, id int64) error {
	_, err := r.store.ExecContext(ctx, `DELETE FROM autoreply_rules WHERE id=?`, id)
	return err
}

// Log 写入自动回复日志。
func (r *RuleStore) Log(ctx context.Context, e *LogEntry) error {
	_, err := r.store.ExecContext(ctx,
		`INSERT INTO autoreply_log(rule_id, chat_id, in_msg_id, out_text, error, created_at)
		 VALUES(?,?,?,?,?,?)`,
		e.RuleID, e.ChatID, e.InMsgID, e.OutText, e.Error, time.Now().Unix())
	return err
}

// RecentLogs 读取最近日志。
func (r *RuleStore) RecentLogs(ctx context.Context, limit int) ([]LogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.store.QueryContext(ctx,
		`SELECT id, COALESCE(rule_id,0), COALESCE(chat_id,0), COALESCE(in_msg_id,0),
		        COALESCE(out_text,''), COALESCE(error,''), created_at
		 FROM autoreply_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]LogEntry, 0, limit)
	for rows.Next() {
		var e LogEntry
		if err := rows.Scan(&e.ID, &e.RuleID, &e.ChatID, &e.InMsgID,
			&e.OutText, &e.Error, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Match 判断规则是否匹配某条消息。
func (r *Rule) Match(chatID int64, text string) bool {
	if !r.Enabled {
		return false
	}
	// 聊天范围
	if len(r.ChatScope) > 0 {
		found := false
		target := formatChatScope(chatID)
		for _, s := range r.ChatScope {
			if strings.EqualFold(s, target) || s == "*" {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	// 关键词
	if r.Keyword != "" {
		if !strings.Contains(strings.ToLower(text), strings.ToLower(r.Keyword)) {
			return false
		}
	}
	return true
}

// formatChatScope 把 chat ID 格式化成 "user:123" / "group:456" 形式。
func formatChatScope(id int64) string {
	if id < 0 {
		return "group:" + itoa(-id)
	}
	return "user:" + itoa(id)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
