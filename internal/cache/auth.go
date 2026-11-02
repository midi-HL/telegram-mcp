package cache

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// 默认密码（首次启动时写入）
const DefaultWebUIPassword = "Admin"

// bcrypt 成本因子。12 在现代硬件上约 250ms，平衡安全与体验。
// 测试时通过 SetBcryptCost 降低到 4（约 1ms），避免测试超时。
var bcryptCost = 12

// SetBcryptCost 修改 bcrypt 成本（仅供测试）。
func SetBcryptCost(c int) {
	if c >= bcrypt.MinCost && c <= bcrypt.MaxCost {
		bcryptCost = c
	}
}

// SessionTTL 是 Web UI 会话有效期。
const SessionTTL = 24 * time.Hour

// 登录失败限流参数
const (
	MaxLoginAttempts = 5
	LockDuration     = 15 * time.Minute
)

// EnsureWebUICredential 确保存在一条凭据记录。首次调用时写入默认密码。
// 返回 isDefault 表示当前是否仍是默认密码。
func (s *Store) EnsureWebUICredential(ctx context.Context) (isDefault bool, err error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM webui_credential`).Scan(&count); err != nil {
		return false, err
	}
	if count > 0 {
		var d int
		err := s.db.QueryRowContext(ctx, `SELECT is_default FROM webui_credential WHERE id=1`).Scan(&d)
		return d != 0, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(DefaultWebUIPassword), bcryptCost)
	if err != nil {
		return false, err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO webui_credential(id, password_hash, is_default, updated_at) VALUES(1, ?, 1, ?)`,
		string(hash), time.Now().Unix())
	return true, err
}

// VerifyWebUIPassword 校验密码。
func (s *Store) VerifyWebUIPassword(ctx context.Context, password string) (bool, error) {
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM webui_credential WHERE id=1`).Scan(&hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return false, nil
	}
	return true, nil
}

// SetWebUIPassword 修改密码，并把 is_default 置 false。
func (s *Store) SetWebUIPassword(ctx context.Context, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcryptCost)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE webui_credential SET password_hash=?, is_default=0, updated_at=? WHERE id=1`,
		string(hash), time.Now().Unix())
	return err
}

// IsDefaultPassword 返回是否仍使用默认密码。
func (s *Store) IsDefaultPassword(ctx context.Context) bool {
	var d int
	if err := s.db.QueryRowContext(ctx, `SELECT is_default FROM webui_credential WHERE id=1`).Scan(&d); err != nil {
		return false
	}
	return d != 0
}

// CreateSession 创建会话，返回 session ID。
func (s *Store) CreateSession(ctx context.Context) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	id := hex.EncodeToString(buf)
	now := time.Now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO webui_session(session_id, created_at, expires_at, last_seen) VALUES(?,?,?,?)`,
		id, now.Unix(), now.Add(SessionTTL).Unix(), now.Unix())
	return id, err
}

// ValidateSession 校验会话并滑动续期。
func (s *Store) ValidateSession(ctx context.Context, id string) (bool, error) {
	if id == "" {
		return false, nil
	}
	var expires, lastSeen int64
	err := s.db.QueryRowContext(ctx,
		`SELECT expires_at, last_seen FROM webui_session WHERE session_id=?`, id).Scan(&expires, &lastSeen)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	now := time.Now()
	if now.Unix() > expires {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM webui_session WHERE session_id=?`, id)
		return false, nil
	}
	// 滑动续期：超过 1 分钟才更新，避免频繁写
	if now.Unix()-lastSeen > 60 {
		_, _ = s.db.ExecContext(ctx,
			`UPDATE webui_session SET last_seen=?, expires_at=? WHERE session_id=?`,
			now.Unix(), now.Add(SessionTTL).Unix(), id)
	}
	return true, nil
}

// DeleteSession 删除单个会话。
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM webui_session WHERE session_id=?`, id)
	return err
}

// DeleteAllSessions 删除所有会话（改密码后强制登出）。
func (s *Store) DeleteAllSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM webui_session`)
	return err
}

// CleanExpiredSessions 清理过期会话。
func (s *Store) CleanExpiredSessions(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM webui_session WHERE expires_at < ?`, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// LoginAttempt 检查 IP 是否被锁定。
func (s *Store) CheckLoginLock(ctx context.Context, ip string) (locked bool, remaining int, err error) {
	var failCount int
	var lockedUntil int64
	err = s.db.QueryRowContext(ctx,
		`SELECT fail_count, locked_until FROM login_attempt WHERE ip=?`, ip).Scan(&failCount, &lockedUntil)
	if err != nil {
		if err == sql.ErrNoRows {
			// 无记录 = 从未失败，剩余全部次数
			return false, MaxLoginAttempts, nil
		}
		return false, 0, err
	}
	now := time.Now().Unix()
	if lockedUntil > now {
		return true, int(lockedUntil - now), nil
	}
	return false, MaxLoginAttempts - failCount, nil
}

// RecordLoginFailure 记录失败，达阈值则锁定。
func (s *Store) RecordLoginFailure(ctx context.Context, ip string) (locked bool, err error) {
	now := time.Now()
	var failCount int
	var lockedUntil int64
	scanErr := s.db.QueryRowContext(ctx,
		`SELECT fail_count, locked_until FROM login_attempt WHERE ip=?`, ip).Scan(&failCount, &lockedUntil)

	if scanErr == sql.ErrNoRows {
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO login_attempt(ip, fail_count, locked_until) VALUES(?, 1, 0)`, ip)
		return false, err
	}
	if scanErr != nil {
		return false, scanErr
	}

	// 锁定期已过，重置计数
	if lockedUntil > 0 && lockedUntil <= now.Unix() {
		failCount = 0
	}
	failCount++
	var newLockedUntil int64
	if failCount >= MaxLoginAttempts {
		newLockedUntil = now.Add(LockDuration).Unix()
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE login_attempt SET fail_count=?, locked_until=? WHERE ip=?`,
		failCount, newLockedUntil, ip)
	return newLockedUntil > 0, err
}

// ClearLoginFailures 登录成功后清空失败记录。
func (s *Store) ClearLoginFailures(ctx context.Context, ip string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM login_attempt WHERE ip=?`, ip)
	return err
}
