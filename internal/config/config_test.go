package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.DefaultAccount != "main" {
		t.Errorf("DefaultAccount = %q, want main", cfg.DefaultAccount)
	}
	if cfg.WebUI.Port != 8080 {
		t.Errorf("WebUI.Port = %d, want 8080", cfg.WebUI.Port)
	}
	if cfg.WebUI.Host != "0.0.0.0" {
		t.Errorf("WebUI.Host = %q, want 0.0.0.0", cfg.WebUI.Host)
	}
	if cfg.ASR.Backend != "none" {
		t.Errorf("ASR.Backend = %q, want none", cfg.ASR.Backend)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := Default()
	cfg.DataDir = dir
	cfg.UpsertAccount(Account{
		Name:    "test",
		Phone:   "+1234567890",
		APIID:   12345,
		APIHash: "abcdef",
	})

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Accounts) != 1 {
		t.Fatalf("Accounts len = %d, want 1", len(loaded.Accounts))
	}
	acc := loaded.Account("test")
	if acc == nil {
		t.Fatal("账号 test 不存在")
	}
	if acc.APIID != 12345 {
		t.Errorf("APIID = %d, want 12345", acc.APIID)
	}
	if acc.APIHash != "abcdef" {
		t.Errorf("APIHash = %q, want abcdef", acc.APIHash)
	}
	// SessionDir 应被自动填充
	wantSession := filepath.Join(dir, "sessions", "test")
	if acc.SessionDir != wantSession {
		t.Errorf("SessionDir = %q, want %q", acc.SessionDir, wantSession)
	}
}

func TestLoadMissingReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load 不存在的配置应返回默认值，却报错: %v", err)
	}
	if cfg.DataDir != dir {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, dir)
	}
	if len(cfg.Accounts) != 0 {
		t.Errorf("Accounts len = %d, want 0", len(cfg.Accounts))
	}
}

func TestUpsertAccountUpdates(t *testing.T) {
	cfg := Default()
	cfg.UpsertAccount(Account{Name: "a", APIID: 1})
	cfg.UpsertAccount(Account{Name: "b", APIID: 2})
	cfg.UpsertAccount(Account{Name: "a", APIID: 99})

	if len(cfg.Accounts) != 2 {
		t.Fatalf("Accounts len = %d, want 2（更新不应新增）", len(cfg.Accounts))
	}
	if got := cfg.Account("a").APIID; got != 99 {
		t.Errorf("账号 a APIID = %d, want 99", got)
	}
}

func TestEnvCredentials(t *testing.T) {
	// 保存并恢复环境变量
	oldID, hadID := os.LookupEnv("TELEGRAM_API_ID")
	oldHash, hadHash := os.LookupEnv("TELEGRAM_API_HASH")
	defer func() {
		if hadID {
			os.Setenv("TELEGRAM_API_ID", oldID)
		} else {
			os.Unsetenv("TELEGRAM_API_ID")
		}
		if hadHash {
			os.Setenv("TELEGRAM_API_HASH", oldHash)
		} else {
			os.Unsetenv("TELEGRAM_API_HASH")
		}
	}()

	os.Unsetenv("TELEGRAM_API_ID")
	os.Unsetenv("TELEGRAM_API_HASH")
	if _, _, ok := EnvCredentials(); ok {
		t.Error("未设置时应返回 ok=false")
	}

	os.Setenv("TELEGRAM_API_ID", "12345")
	os.Setenv("TELEGRAM_API_HASH", "hash")
	id, hash, ok := EnvCredentials()
	if !ok {
		t.Fatal("应返回 ok=true")
	}
	if id != 12345 || hash != "hash" {
		t.Errorf("got (%d, %q), want (12345, hash)", id, hash)
	}

	// 非法 id
	os.Setenv("TELEGRAM_API_ID", "abc")
	if _, _, ok := EnvCredentials(); ok {
		t.Error("非法 api_id 应返回 ok=false")
	}
}

func TestSavePermission(t *testing.T) {
	dir := t.TempDir()
	cfg := Default()
	cfg.DataDir = dir
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(Path(dir))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	// Windows 上权限位不可靠，仅在类 Unix 检查
	if info.Mode().Perm()&0o077 != 0 {
		t.Logf("警告：配置文件权限 %v 允许其他用户读取", info.Mode().Perm())
	}
}
