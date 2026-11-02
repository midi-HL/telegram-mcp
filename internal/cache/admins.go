package cache

import (
	"context"
	"time"
)

// AdminRow 是管理员记录。
type AdminRow struct {
	UserID   int64
	Username string
	AddedAt  int64
	AddedBy  int64
}

// InsertAdmin 添加管理员（按数字 UserID 去重）。
func (s *Store) InsertAdmin(ctx context.Context, userID int64, username string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO admins(user_id, username_snapshot, added_at, added_by)
		VALUES(?, ?, ?, 0)
		ON CONFLICT(user_id) DO UPDATE SET username_snapshot=excluded.username_snapshot`,
		userID, username, time.Now().Unix())
	return err
}

// DeleteAdmin 删除管理员。
func (s *Store) DeleteAdmin(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM admins WHERE user_id=?`, userID)
	return err
}

// QueryAdmins 列出所有管理员。
func (s *Store) QueryAdmins(ctx context.Context) ([]AdminRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT user_id, COALESCE(username_snapshot,''), added_at, added_by
		 FROM admins ORDER BY added_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]AdminRow, 0)
	for rows.Next() {
		var r AdminRow
		if err := rows.Scan(&r.UserID, &r.Username, &r.AddedAt, &r.AddedBy); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// IsAdmin 判断某个 UserID 是否是管理员（★ 只认数字 ID）。
func (s *Store) IsAdmin(ctx context.Context, userID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM admins WHERE user_id=?`, userID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// AdminIDs 返回所有管理员 UserID（用于批量判断）。
func (s *Store) AdminIDs(ctx context.Context) (map[int64]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT user_id FROM admins`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
