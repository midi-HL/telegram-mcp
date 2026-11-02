package asr

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
)

func TestNewNone(t *testing.T) {
	tr, err := New(Config{Backend: "none"})
	if err != nil {
		t.Fatalf("New(none): %v", err)
	}
	if tr.Name() != "none" {
		t.Errorf("Name = %q, want none", tr.Name())
	}
}

func TestNewDefaultIsNone(t *testing.T) {
	tr, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Name() != "none" {
		t.Errorf("空配置应默认 none，got %q", tr.Name())
	}
}

func TestNewNative(t *testing.T) {
	tr, err := New(Config{Backend: "native"})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Name() != "native" {
		t.Errorf("Name = %q, want native", tr.Name())
	}
}

func TestNewSherpaRequiresPaths(t *testing.T) {
	if _, err := New(Config{Backend: "sherpa"}); err == nil {
		t.Error("缺少 cli_path 应报错")
	}
	if _, err := New(Config{Backend: "sherpa", CLIPath: "/x"}); err == nil {
		t.Error("缺少 model_dir 应报错")
	}
	tr, err := New(Config{Backend: "sherpa", CLIPath: "/x", ModelDir: "/m"})
	if err != nil {
		t.Fatalf("完整配置应成功: %v", err)
	}
	if tr.Name() != "sherpa" {
		t.Errorf("Name = %q", tr.Name())
	}
}

func TestNewOpenAIRequiresKey(t *testing.T) {
	if _, err := New(Config{Backend: "openai"}); err == nil {
		t.Error("缺少 api_key 应报错")
	}
	tr, err := New(Config{Backend: "openai", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Name() != "openai" {
		t.Errorf("Name = %q", tr.Name())
	}
}

func TestNewUnknownBackend(t *testing.T) {
	if _, err := New(Config{Backend: "bogus"}); err == nil {
		t.Error("未知后端应报错")
	}
}

func TestNoneTranscribe(t *testing.T) {
	tr, _ := New(Config{Backend: "none"})
	res, err := tr.Transcribe(context.Background(), []byte("fake audio"), "audio/ogg")
	if err != nil {
		t.Fatal(err)
	}
	if res.Backend != "none" {
		t.Errorf("Backend = %q", res.Backend)
	}
	if res.Text != "" {
		t.Errorf("none 后端不应返回文本，got %q", res.Text)
	}
	if !strings.Contains(res.Note, "未配置") {
		t.Errorf("Note 应提示未配置，got %q", res.Note)
	}
}

func TestNativeTranscribe(t *testing.T) {
	tr, _ := New(Config{Backend: "native"})
	audio := []byte("fake audio data")
	res, err := tr.Transcribe(context.Background(), audio, "audio/ogg")
	if err != nil {
		t.Fatal(err)
	}
	if res.Backend != "native" {
		t.Errorf("Backend = %q", res.Backend)
	}
	// 验证 base64 可解码
	decoded, err := base64.StdEncoding.DecodeString(res.AudioBase64)
	if err != nil {
		t.Fatalf("base64 解码失败: %v", err)
	}
	if string(decoded) != string(audio) {
		t.Error("base64 往返不一致")
	}
}

func TestParseSherpaOutput(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"0: 你好世界", "你好世界"},
		{"1: 第二句话", "第二句话"},
		{"--some-flag\n0: 文本", "文本"},
		{"纯文本没有前缀", "纯文本没有前缀"},
		{"", ""},
	}
	for _, tt := range tests {
		got := parseSherpaOutput(tt.input)
		if got != tt.want {
			t.Errorf("parseSherpaOutput(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtFromMime(t *testing.T) {
	tests := []struct {
		mime string
		want string
	}{
		{"audio/ogg", ".ogg"},
		{"audio/mpeg", ".mp3"},
		{"audio/mp4", ".m4a"},
		{"audio/wav", ".wav"},
		{"audio/webm", ".webm"},
		{"unknown/type", ".ogg"},
	}
	for _, tt := range tests {
		if got := extFromMime(tt.mime); got != tt.want {
			t.Errorf("extFromMime(%q) = %q, want %q", tt.mime, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("got %q", got)
	}
	if got := truncate("very long text here", 5); got != "very …" {
		t.Errorf("got %q", got)
	}
}
