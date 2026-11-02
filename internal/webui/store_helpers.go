package webui

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"time"

	"github.com/midi-HL/telegram-mcp/internal/cache"
)

// newJSONDecoder 创建 JSON 解码器（限制请求体大小）。
func newJSONDecoder(r io.Reader) *json.Decoder {
	dec := json.NewDecoder(io.LimitReader(r, 1<<20)) // 1MB 上限
	dec.DisallowUnknownFields()
	return dec
}

// listAdmins 列出所有管理员。
func (s *Server) listAdmins(ctx context.Context) ([]AdminView, error) {
	rows, err := s.store.QueryAdmins(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AdminView, 0, len(rows))
	for _, r := range rows {
		out = append(out, AdminView{
			UserID:   r.UserID,
			Username: r.Username,
			AddedAt:  r.AddedAt,
			AddedBy:  r.AddedBy,
		})
	}
	return out, nil
}

// addAdmin 添加管理员（存数字 ID）。
func (s *Server) addAdmin(ctx context.Context, userID int64, username string) error {
	return s.store.InsertAdmin(ctx, userID, username)
}

// removeAdmin 删除管理员。
func (s *Server) removeAdmin(ctx context.Context, userID int64) error {
	return s.store.DeleteAdmin(ctx, userID)
}

// 确保 cache 包被引用（用于常量）
var _ = cache.MaxLoginAttempts
var _ = sql.ErrNoRows
var _ = time.Now
