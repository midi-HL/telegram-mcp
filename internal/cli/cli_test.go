package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/midi-HL/telegram-mcp/internal/config"
)

func newTestApp(t *testing.T) (*App, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	a := New("test", zap.NewNop())
	a.Stdout = &out
	a.Stderr = &bytes.Buffer{}
	a.DataDir = t.TempDir()
	return a, &out
}

func TestWakeFlagReorderingAndPersistence(t *testing.T) {
	a, out := newTestApp(t)
	ctx := context.Background()

	// 「wake on --data-dir ... --webui ...」这种自然写法也必须解析到 --webui。
	if err := a.Run(ctx, []string{"wake", "on"}); err != nil {
		t.Fatalf("wake on: %v", err)
	}
	cfg, err := config.Load(a.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Wake.Enabled {
		t.Fatal("wake on 应持久化 enabled=true")
	}
	if !strings.Contains(out.String(), "已开启") {
		t.Errorf("输出应包含提示: %s", out.String())
	}

	// 子命令后面的 flag 必须生效（Go flag 包默认会在第一个位置参数停止）。
	if err := a.Run(ctx, []string{"wake", "config", "--mode", "webhook", "--debounce", "7"}); err != nil {
		t.Fatalf("wake config: %v", err)
	}
	cfg, _ = config.Load(a.DataDir)
	if cfg.Wake.Mode != "webhook" || cfg.Wake.DebounceSeconds != 7 {
		t.Errorf("位置参数后的 flag 未生效: %+v", cfg.Wake)
	}

	if err := a.Run(ctx, []string{"wake", "off"}); err != nil {
		t.Fatalf("wake off: %v", err)
	}
	cfg, _ = config.Load(a.DataDir)
	if cfg.Wake.Enabled {
		t.Error("wake off 应关闭")
	}
}

func TestDoctorFreshInstallGuidance(t *testing.T) {
	a, out := newTestApp(t)
	// 避免 doctor 检查默认 8080 时结果不稳定，仍只验证关键内容。
	if err := a.Run(context.Background(), []string{"doctor"}); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	s := out.String()
	for _, want := range []string{
		"可行性自检", "已公开内置默认凭据", "开箱即用", "浏览器入口", "Agent 唤醒", "建议",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("doctor 输出缺少 %q:\n%s", want, s)
		}
	}
}

func TestDoctorReportsCustomCredentials(t *testing.T) {
	a, out := newTestApp(t)
	cfg := config.Default()
	cfg.DataDir = a.DataDir
	cfg.UpsertAccount(config.Account{Name: "main", APIID: 123, APIHash: "custom"})
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(context.Background(), []string{"doctor"}); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if !strings.Contains(out.String(), "自定义配置") {
		t.Errorf("自定义凭据未被识别:\n%s", out.String())
	}
}

func TestUsageIncludesWakeAndDoctor(t *testing.T) {
	a, _ := newTestApp(t)
	var errOut bytes.Buffer
	a.Stderr = &errOut
	if err := a.Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "wake") || !strings.Contains(errOut.String(), "doctor") {
		t.Errorf("usage 应包含 wake / doctor:\n%s", errOut.String())
	}
}

func TestWakeConfigWritesFile(t *testing.T) {
	a, _ := newTestApp(t)
	dir := a.DataDir
	path := filepath.Join(dir, "config.json")
	if err := a.Run(context.Background(), []string{"wake", "config", "--mode", "sampling", "--max-text", "99"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("config.json 未生成: %v", err)
	}
	if !strings.Contains(string(data), `"mode": "sampling"`) || !strings.Contains(string(data), `"max_text_runes": 99`) {
		t.Errorf("配置文件内容异常:\n%s", string(data))
	}
}
