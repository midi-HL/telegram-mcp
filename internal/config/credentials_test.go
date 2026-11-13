package config

import (
	"encoding/json"
	"os"
	"testing"
)

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestBuiltinCredentials(t *testing.T) {
	c := BuiltinCredentials()
	if c.APIID != 34353880 || c.APIHash != "35edeb69a4182e939b3ed765cd5524be" {
		t.Fatalf("内置凭据 = %+v", c)
	}
	if c.Source != "builtin" {
		t.Errorf("Source = %q, want builtin", c.Source)
	}
}

func TestResolveCredentialsDefault(t *testing.T) {
	unsetCredEnv(t)
	c := ResolveCredentials(0, "")
	if c.APIID != BuiltinAPIID || c.APIHash != BuiltinAPIHash {
		t.Errorf("空参数应使用内置凭据，got %+v", c)
	}
	if c.Source != "builtin" {
		t.Errorf("Source = %q, want builtin", c.Source)
	}
}

func TestResolveCredentialsOverride(t *testing.T) {
	unsetCredEnv(t)
	c := ResolveCredentials(12345, "myhash")
	if c.APIID != 12345 || c.APIHash != "myhash" || c.Source != "configured" {
		t.Errorf("显式凭据 = %+v", c)
	}
	c = ResolveCredentials(12345, "")
	if c.APIID != 12345 || c.APIHash != BuiltinAPIHash {
		t.Errorf("只覆盖 api_id = %+v", c)
	}
	c = ResolveCredentials(0, "myhash")
	if c.APIID != BuiltinAPIID || c.APIHash != "myhash" {
		t.Errorf("只覆盖 api_hash = %+v", c)
	}
}

func TestResolveCredentialsEnvWins(t *testing.T) {
	unsetCredEnv(t)
	t.Setenv("TELEGRAM_API_ID", "999")
	t.Setenv("TELEGRAM_API_HASH", "envhash")
	c := ResolveCredentials(123, "confighash")
	if c.APIID != 999 || c.APIHash != "envhash" || c.Source != "env" {
		t.Errorf("环境变量应优先，got %+v", c)
	}
}

func TestUpsertAccountFillsBuiltin(t *testing.T) {
	unsetCredEnv(t)
	dir := t.TempDir()
	cfg := Default()
	cfg.DataDir = dir
	cfg.UpsertAccount(Account{Name: "main"})
	acc := cfg.Account("main")
	if acc == nil || acc.APIID != BuiltinAPIID || acc.APIHash != BuiltinAPIHash {
		t.Fatalf("UpsertAccount 未填内置凭据: %+v", acc)
	}
	if !cfg.BuiltinCredentialsUsed() {
		t.Error("应检测到内置凭据")
	}
	cfg.UpsertAccount(Account{Name: "main", APIID: 42, APIHash: "custom"})
	if cfg.BuiltinCredentialsUsed() {
		t.Error("自定义凭据不应被识别为内置")
	}
}

func TestLoadFillsBuiltinCredentials(t *testing.T) {
	unsetCredEnv(t)
	dir := t.TempDir()
	raw := `{"data_dir":"","default_account":"main","accounts":[{"name":"a","session_dir":` + jsonString(dir+"/sessions/a") + `}],"webui":{"enabled":true,"host":"0.0.0.0","port":8080}}`
	if err := os.WriteFile(Path(dir), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Accounts[0].APIID != BuiltinAPIID || cfg.Accounts[0].APIHash != BuiltinAPIHash {
		t.Errorf("Load 应补内置凭据: %+v", cfg.Accounts[0])
	}
	if cfg.Wake.Mode != "notify" || cfg.Wake.MaxTextRunes != 280 {
		t.Errorf("旧配置应补 Wake 默认数值: %+v", cfg.Wake)
	}
	if !cfg.Wake.IncludeText || cfg.Wake.DebounceSeconds != 2 {
		t.Errorf("旧配置缺少 wake 字段时应给完整默认值: %+v", cfg.Wake)
	}
}

func TestLoadPreservesExplicitWakeFalse(t *testing.T) {
	unsetCredEnv(t)
	dir := t.TempDir()
	raw := `{"data_dir":"","wake":{"enabled":false,"mode":"webhook","include_text":false,"debounce_seconds":0,"webhook_url":"http://127.0.0.1:9000/hook"}}`
	if err := os.WriteFile(Path(dir), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Wake.IncludeText {
		t.Error("显式 include_text=false 不应被重置为 true")
	}
	if cfg.Wake.Mode != "webhook" || cfg.Wake.DebounceSeconds != 0 {
		t.Errorf("显式 wake 配置应保持: %+v", cfg.Wake)
	}
}

func unsetCredEnv(t *testing.T) {
	t.Helper()
	oldID, hadID := os.LookupEnv("TELEGRAM_API_ID")
	oldHash, hadHash := os.LookupEnv("TELEGRAM_API_HASH")
	t.Cleanup(func() {
		if hadID {
			_ = os.Setenv("TELEGRAM_API_ID", oldID)
		} else {
			_ = os.Unsetenv("TELEGRAM_API_ID")
		}
		if hadHash {
			_ = os.Setenv("TELEGRAM_API_HASH", oldHash)
		} else {
			_ = os.Unsetenv("TELEGRAM_API_HASH")
		}
	})
	_ = os.Unsetenv("TELEGRAM_API_ID")
	_ = os.Unsetenv("TELEGRAM_API_HASH")
}
