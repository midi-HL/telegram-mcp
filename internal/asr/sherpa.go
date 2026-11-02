package asr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Sherpa 通过子进程调用 sherpa-onnx-offline。
//
// 实现要点（来自实地部署验证）：
//  1. sherpa-onnx 只接受 WAV（RIFF），Ogg/Opus 必须先转码
//  2. shared 构建需要 LD_LIBRARY_PATH 指向 CLI 所在目录
//  3. 输出是 stdout 上的 JSON（取 .text 字段），不是 "0: 文本" 格式
//  4. 模型文件名可能是 model.onnx / model.int8.onnx / encoder.onnx
type Sherpa struct {
	// CLIPath 是可执行文件完整路径
	CLIPath string
	// ModelDir 是模型目录
	ModelDir string
	// FFmpegPath 是 ffmpeg 路径（空则自动查找）
	FFmpegPath string
	// NumThreads 是 CPU 线程数（0 则默认 2）
	NumThreads int
}

// Name 返回后端名。
func (s *Sherpa) Name() string { return "sherpa" }

// Transcribe 转写音频。
func (s *Sherpa) Transcribe(ctx context.Context, audio []byte, mime string) (*Result, error) {
	// 1. 确保是 WAV（sherpa 只认 RIFF）
	wavPath, cleanup, err := s.ensureWAV(ctx, audio, mime)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// 2. 找到模型文件和词表
	modelFile, tokensFile, err := s.resolveModelFiles()
	if err != nil {
		return nil, err
	}

	// 3. 构造命令
	threads := s.NumThreads
	if threads <= 0 {
		threads = 2
	}
	args := []string{
		"--tokens=" + tokensFile,
		"--paraformer=" + modelFile,
		"--num-threads=" + itoa(threads),
		"--decoding-method=greedy_search",
		wavPath,
	}

	cmd := exec.CommandContext(ctx, s.CLIPath, args...)
	// ★ 关键：shared 构建需要 LD_LIBRARY_PATH 指向 CLI 目录
	cmd.Env = s.buildEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("sherpa 执行失败: %w\nstderr: %s",
			err, truncate(stderr.String(), 500))
	}

	// 4. 解析 JSON 输出
	text, err := parseSherpaJSON(stdout.String())
	if err != nil {
		return nil, fmt.Errorf("解析 sherpa 输出失败: %w\nstdout: %s",
			err, truncate(stdout.String(), 500))
	}

	return &Result{
		Backend: "sherpa",
		Text:    text,
	}, nil
}

// buildEnv 构造带 LD_LIBRARY_PATH 的环境变量。
func (s *Sherpa) buildEnv() []string {
	env := os.Environ()
	libDir := filepath.Dir(s.CLIPath)

	// 按平台决定库路径变量名
	var key string
	switch runtime.GOOS {
	case "darwin":
		key = "DYLD_LIBRARY_PATH"
	case "windows":
		// Windows 用 PATH
		key = "PATH"
	default:
		key = "LD_LIBRARY_PATH"
	}

	// 已有值则追加，否则新建
	found := false
	for i, e := range env {
		if strings.HasPrefix(e, key+"=") {
			existing := strings.TrimPrefix(e, key+"=")
			if existing != "" {
				env[i] = key + "=" + libDir + string(os.PathListSeparator) + existing
			} else {
				env[i] = key + "=" + libDir
			}
			found = true
			break
		}
	}
	if !found {
		env = append(env, key+"="+libDir)
	}
	return env
}

// resolveModelFiles 找到模型文件和词表文件。
func (s *Sherpa) resolveModelFiles() (modelFile, tokensFile string, err error) {
	// 词表
	for _, name := range []string{"tokens.txt", "tokens"} {
		p := filepath.Join(s.ModelDir, name)
		if fileExists(p) {
			tokensFile = p
			break
		}
	}
	if tokensFile == "" {
		return "", "", fmt.Errorf("模型目录 %s 里找不到 tokens.txt", s.ModelDir)
	}

	// 模型文件（按优先级）
	candidates := []string{
		"model.int8.onnx",   // ★ 实测部署用的就是这个
		"model.onnx",
		"encoder.int8.onnx",
		"encoder.onnx",
		"paraformer.onnx",
		"model.int8.onnx",
	}
	for _, name := range candidates {
		p := filepath.Join(s.ModelDir, name)
		if fileExists(p) {
			modelFile = p
			break
		}
	}
	if modelFile == "" {
		return "", "", fmt.Errorf("模型目录 %s 里找不到模型文件（尝试过 %v）",
			s.ModelDir, candidates)
	}
	return modelFile, tokensFile, nil
}

// ensureWAV 确保音频是 WAV 格式。Ogg/MP3 等需先用 ffmpeg 转码。
func (s *Sherpa) ensureWAV(ctx context.Context, audio []byte, mime string) (string, func(), error) {
	// 检查是否已经是 WAV（RIFF 头）
	if isWAV(audio) {
		p, err := writeTemp(audio, ".wav")
		if err != nil {
			return "", nil, err
		}
		return p, func() { _ = os.Remove(p) }, nil
	}

	// 需要转码
	ffmpeg := s.FFmpegPath
	if ffmpeg == "" {
		ffmpeg = findFFmpeg()
	}
	if ffmpeg == "" {
		return "", nil, fmt.Errorf(
			"音频是 %s 格式，sherpa 只接受 WAV，但找不到 ffmpeg。\n"+
				"请安装 ffmpeg，或用 --ffmpeg 指定路径", mime)
	}

	// 写原始音频到临时文件
	ext := extFromMime(mime)
	srcPath, err := writeTemp(audio, ext)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = os.Remove(srcPath) }()

	// ffmpeg 转 WAV：16kHz 单声道 PCM 16-bit
	dstPath := srcPath + ".wav"
	args := []string{
		"-y", "-hide_banner", "-loglevel", "error",
		"-i", srcPath,
		"-ar", "16000",   // 采样率
		"-ac", "1",       // 单声道
		"-c:a", "pcm_s16le", // PCM 16-bit 小端
		dstPath,
	}
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", nil, fmt.Errorf("ffmpeg 转码失败: %w\n%s", err, truncate(stderr.String(), 300))
	}

	return dstPath, func() { _ = os.Remove(dstPath) }, nil
}

// isWAV 判断是否是 WAV 文件（RIFF 头）。
func isWAV(data []byte) bool {
	if len(data) < 12 {
		return false
	}
	return string(data[0:4]) == "RIFF" && string(data[8:12]) == "WAVE"
}

// writeTemp 写入临时文件。
func writeTemp(data []byte, ext string) (string, error) {
	f, err := os.CreateTemp("", "tgmcp_asr_*"+ext)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(data); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// findFFmpeg 查找 ffmpeg 可执行文件。
func findFFmpeg() string {
	for _, name := range []string{"ffmpeg", "ffmpeg.exe"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	// 常见路径
	for _, p := range []string{"/usr/bin/ffmpeg", "/usr/local/bin/ffmpeg"} {
		if fileExists(p) {
			return p
		}
	}
	return ""
}

// parseSherpaJSON 解析 sherpa 的 JSON 输出，取 .text 字段。
//
// 实际输出格式（每行一个 JSON，一次调用一个文件时只有一行）：
//
//	{"lang":"", "text":"识别结果", "tokens":[...], ...}
func parseSherpaJSON(out string) (string, error) {
	var texts []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var obj struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue
		}
		if obj.Text != "" {
			texts = append(texts, obj.Text)
		}
	}
	if len(texts) == 0 {
		return "", fmt.Errorf("输出里没有有效的 JSON text 字段")
	}
	return strings.Join(texts, " "), nil
}

// fileExists 检查文件存在。
func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// itoa 简单整数转字符串。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
