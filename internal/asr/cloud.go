package asr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// OpenAI 是 OpenAI Whisper API 后端。
type OpenAI struct {
	APIKey   string
	Endpoint string
	Language string
	Model    string // 默认 whisper-1
}

// Name 返回后端名。
func (o *OpenAI) Name() string { return "openai" }

// Transcribe 调用 OpenAI 音频转写 API。
func (o *OpenAI) Transcribe(ctx context.Context, audio []byte, mime string) (*Result, error) {
	model := o.Model
	if model == "" {
		model = "whisper-1"
	}

	// 构造 multipart 请求
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	// file 字段
	ext := extFromMime(mime)
	fw, err := w.CreateFormFile("file", "audio"+ext)
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write(audio); err != nil {
		return nil, err
	}
	// model 字段
	_ = w.WriteField("model", model)
	// language 字段（可选）
	if o.Language != "" {
		_ = w.WriteField("language", o.Language)
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.Endpoint, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+o.APIKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 OpenAI API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API 返回 %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}

	var out struct {
		Text     string  `json:"text"`
		Language string  `json:"language"`
		Duration float64 `json:"duration"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("解析响应: %w", err)
	}

	return &Result{
		Backend:  "openai",
		Text:     out.Text,
		Language: out.Language,
		Duration: out.Duration,
	}, nil
}

// 辅助函数

// encodeBase64 编码为 base64。
func encodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// writeTempAudio 把音频写入临时文件。
func writeTempAudio(audio []byte) (string, error) {
	f, err := os.CreateTemp("", "tgmcp_asr_*.wav")
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(audio); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// cleanupTemp 删除临时文件。
func cleanupTemp(path string) {
	_ = os.Remove(path)
}

// extFromMime 从 MIME 猜扩展名。
func extFromMime(mime string) string {
	switch {
	case contains(mime, "ogg"):
		return ".ogg"
	case contains(mime, "mpeg"), contains(mime, "mp3"):
		return ".mp3"
	case contains(mime, "mp4"), contains(mime, "m4a"):
		return ".m4a"
	case contains(mime, "wav"):
		return ".wav"
	case contains(mime, "webm"):
		return ".webm"
	default:
		return ".ogg"
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ModelDirName 返回模型目录下的默认文件名（用于文档）。
func ModelDirName() string {
	return filepath.Join("models", "sherpa-onnx-paraformer-zh-small")
}
