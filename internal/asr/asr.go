// Package asr 提供可插拔的语音转写后端。
//
// 支持五种后端：
//   - sherpa：子进程调 sherpa-onnx（离线免费）
//   - openai：OpenAI Whisper API
//   - aliyun / tencent：国内云服务
//   - native：音频 base64 直接给模型（模型需支持音频输入）
//   - none：不转写，只返回元数据
package asr

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Result 是转写结果。
type Result struct {
	Text     string  `json:"text"`
	Backend  string  `json:"backend"`
	Language string  `json:"language,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	// AudioBase64 是 native 后端返回的音频数据
	AudioBase64 string `json:"audio_base64,omitempty"`
	// Note 是提示信息
	Note string `json:"note,omitempty"`
}

// Transcriber 是转写后端接口。
type Transcriber interface {
	// Transcribe 转写音频。
	Transcribe(ctx context.Context, audio []byte, mime string) (*Result, error)
	// Name 返回后端名称。
	Name() string
}

// Config 是 ASR 配置。
type Config struct {
	Backend  string
	// sherpa 相关
	CLIPath  string
	ModelDir string
	// 云服务相关
	APIKey   string
	Endpoint string
	Language string
}

// New 按配置创建转写器。
func New(cfg Config) (Transcriber, error) {
	switch strings.ToLower(cfg.Backend) {
	case "", "none":
		return &None{}, nil
	case "sherpa":
		if cfg.CLIPath == "" {
			return nil, fmt.Errorf("sherpa 后端需要 cli_path")
		}
		if cfg.ModelDir == "" {
			return nil, fmt.Errorf("sherpa 后端需要 model_dir")
		}
		return &Sherpa{CLIPath: cfg.CLIPath, ModelDir: cfg.ModelDir}, nil
	case "openai":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("openai 后端需要 api_key")
		}
		ep := cfg.Endpoint
		if ep == "" {
			ep = "https://api.openai.com/v1/audio/transcriptions"
		}
		return &OpenAI{APIKey: cfg.APIKey, Endpoint: ep, Language: cfg.Language}, nil
	case "native":
		return &Native{}, nil
	default:
		return nil, fmt.Errorf("未知 ASR 后端 %q（支持 sherpa/openai/native/none）", cfg.Backend)
	}
}

// None 是空后端，只返回元数据。
type None struct{}

// Name 返回后端名。
func (n *None) Name() string { return "none" }

// Transcribe 不做转写。
func (n *None) Transcribe(ctx context.Context, audio []byte, mime string) (*Result, error) {
	return &Result{
		Backend: "none",
		Note:    fmt.Sprintf("未配置 ASR 后端，音频 %d 字节（%s）", len(audio), mime),
	}, nil
}

// Native 把音频原样返回（base64），交给模型自己处理。
type Native struct{}

// Name 返回后端名。
func (n *Native) Name() string { return "native" }

// Transcribe 返回 base64 音频。
func (n *Native) Transcribe(ctx context.Context, audio []byte, mime string) (*Result, error) {
	return &Result{
		Backend:     "native",
		AudioBase64: encodeBase64(audio),
		Note:        fmt.Sprintf("音频 %d 字节，已转 base64（需模型支持音频输入）", len(audio)),
	}, nil
}

// Sherpa 通过子进程调用 sherpa-onnx。
type Sherpa struct {
	CLIPath  string
	ModelDir string
}

// Name 返回后端名。
func (s *Sherpa) Name() string { return "sherpa" }

// Transcribe 调用 sherpa-onnx-offline 命令行。
//
// 用法：
//
//	sherpa-onnx-offline --tokens=... --paraformer=... --model-type=paraformer input.wav
func (s *Sherpa) Transcribe(ctx context.Context, audio []byte, mime string) (*Result, error) {
	// 写入临时 wav 文件
	tmp, err := writeTempAudio(audio)
	if err != nil {
		return nil, err
	}
	defer cleanupTemp(tmp)

	args := []string{
		"--tokens=" + s.ModelDir + "/tokens.txt",
		"--paraformer=" + s.ModelDir + "/model.onnx",
		"--model-type=paraformer",
		tmp,
	}

	cmd := exec.CommandContext(ctx, s.CLIPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("sherpa 执行失败: %w (输出: %s)", err, truncate(string(out), 300))
	}

	text := parseSherpaOutput(string(out))
	return &Result{
		Backend: "sherpa",
		Text:    text,
	}, nil
}

// parseSherpaOutput 从 sherpa 输出里提取识别文本。
func parseSherpaOutput(out string) string {
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "--") || strings.HasPrefix(line, "sherpa") {
			continue
		}
		// 去掉 "0:" 之类的时间戳前缀
		if i := strings.Index(line, ":"); i > 0 && i < 6 {
			line = strings.TrimSpace(line[i+1:])
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, " ")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
