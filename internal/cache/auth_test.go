package cache

import (
	"context"
	"testing"
	"time"
)

func TestEnsureWebUICredential(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	isDefault, err := s.EnsureWebUICredential(ctx)
	if err != nil {
		t.Fatalf("EnsureWebUICredential: %v", err)
	}
	if !isDefault {
		t.Error("首次创建应是默认密码")
	}

	// 再次调用不应改变
	isDefault2, err := s.EnsureWebUICredential(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !isDefault2 {
		t.Error("重复调用应仍是默认密码")
	}
}

func TestVerifyWebUIPassword(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_, _ = s.EnsureWebUICredential(ctx)

	ok, err := s.VerifyWebUIPassword(ctx, "Admin")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("默认密码 Admin 应校验通过")
	}

	ok, _ = s.VerifyWebUIPassword(ctx, "wrong")
	if ok {
		t.Error("错误密码应校验失败")
	}
}

func TestSetWebUIPassword(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_, _ = s.EnsureWebUICredential(ctx)

	if err := s.SetWebUIPassword(ctx, "NewSecret123"); err != nil {
		t.Fatalf("SetWebUIPassword: %v", err)
	}

	// 旧密码失效
	ok, _ := s.VerifyWebUIPassword(ctx, "Admin")
	if ok {
		t.Error("旧密码应失效")
	}
	// 新密码有效
	ok, _ = s.VerifyWebUIPassword(ctx, "NewSecret123")
	if !ok {
		t.Error("新密码应有效")
	}
	// is_default 应变为 false
	if s.IsDefaultPassword(ctx) {
		t.Error("改密码后 is_default 应为 false")
	}
}

func TestSessionLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	sid, err := s.CreateSession(ctx)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if len(sid) != 64 {
		t.Errorf("session ID 长度 = %d, want 64（32 字节 hex）", len(sid))
	}

	ok, err := s.ValidateSession(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("新建会话应有效")
	}

	// 无效 ID
	ok, _ = s.ValidateSession(ctx, "nonexistent")
	if ok {
		t.Error("不存在的会话应无效")
	}

	// 删除
	_ = s.DeleteSession(ctx, sid)
	ok, _ = s.ValidateSession(ctx, sid)
	if ok {
		t.Error("删除后会话应无效")
	}
}

func TestDeleteAllSessions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s1, _ := s.CreateSession(ctx)
	s2, _ := s.CreateSession(ctx)

	if err := s.DeleteAllSessions(ctx); err != nil {
		t.Fatal(err)
	}

	for _, sid := range []string{s1, s2} {
		if ok, _ := s.ValidateSession(ctx, sid); ok {
			t.Errorf("会话 %s 应已被删除", sid[:8])
		}
	}
}

func TestExpiredSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	sid, _ := s.CreateSession(ctx)
	// 手动改成过期
	_, err := s.db.ExecContext(ctx,
		`UPDATE webui_session SET expires_at=? WHERE session_id=?`,
		time.Now().Add(-time.Hour).Unix(), sid)
	if err != nil {
		t.Fatal(err)
	}

	ok, _ := s.ValidateSession(ctx, sid)
	if ok {
		t.Error("过期会话应无效")
	}
}

func TestLoginLockout(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ip := "1.2.3.4"

	// 初始未锁定
	locked, remaining, err := s.CheckLoginLock(ctx, ip)
	if err != nil {
		t.Fatal(err)
	}
	if locked {
		t.Error("初始不应锁定")
	}
	if remaining != MaxLoginAttempts {
		t.Errorf("剩余次数 = %d, want %d", remaining, MaxLoginAttempts)
	}

	// 连续失败
	for i := 1; i < MaxLoginAttempts; i++ {
		lockedNow, err := s.RecordLoginFailure(ctx, ip)
		if err != nil {
			t.Fatal(err)
		}
		if lockedNow {
			t.Errorf("第 %d 次失败不应锁定", i)
		}
	}

	// 最后一次触发锁定
	lockedNow, err := s.RecordLoginFailure(ctx, ip)
	if err != nil {
		t.Fatal(err)
	}
	if !lockedNow {
		t.Errorf("第 %d 次失败应触发锁定", MaxLoginAttempts)
	}

	locked, _, _ = s.CheckLoginLock(ctx, ip)
	if !locked {
		t.Error("应处于锁定状态")
	}
}

func TestClearLoginFailures(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ip := "5.6.7.8"

	_, _ = s.RecordLoginFailure(ctx, ip)
	_, _ = s.RecordLoginFailure(ctx, ip)

	if err := s.ClearLoginFailures(ctx, ip); err != nil {
		t.Fatal(err)
	}

	locked, remaining, _ := s.CheckLoginLock(ctx, ip)
	if locked {
		t.Error("清理后不应锁定")
	}
	if remaining != MaxLoginAttempts {
		t.Errorf("清理后剩余次数 = %d, want %d", remaining, MaxLoginAttempts)
	}
}

func TestAdminsCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// 添加
	if err := s.InsertAdmin(ctx, 123456, "alice"); err != nil {
		t.Fatalf("InsertAdmin: %v", err)
	}

	// 查询
	ok, err := s.IsAdmin(ctx, 123456)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("123456 应是管理员")
	}

	ok, _ = s.IsAdmin(ctx, 999)
	if ok {
		t.Error("999 不应是管理员")
	}

	// 列表
	rows, err := s.QueryAdmins(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].UserID != 123456 || rows[0].Username != "alice" {
		t.Errorf("管理员列表错误：%+v", rows)
	}

	// 重复添加应更新用户名而不是新增
	if err := s.InsertAdmin(ctx, 123456, "alice_new"); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.QueryAdmins(ctx)
	if len(rows) != 1 {
		t.Errorf("重复添加后应仍是 1 条，got %d", len(rows))
	}
	if rows[0].Username != "alice_new" {
		t.Errorf("用户名应更新为 alice_new，got %q", rows[0].Username)
	}

	// 删除
	if err := s.DeleteAdmin(ctx, 123456); err != nil {
		t.Fatal(err)
	}
	ok, _ = s.IsAdmin(ctx, 123456)
	if ok {
		t.Error("删除后不应是管理员")
	}
}

func TestAdminIDs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_ = s.InsertAdmin(ctx, 111, "a")
	_ = s.InsertAdmin(ctx, 222, "b")

	ids, err := s.AdminIDs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || !ids[111] || !ids[222] {
		t.Errorf("AdminIDs = %v, want {111:true, 222:true}", ids)
	}
}
