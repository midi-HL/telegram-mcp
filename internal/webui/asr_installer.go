package webui

import (
	"archive/tar"
	"compress/bzip2"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// 可选模型（名称 → 下载地址 + 说明）
type modelOption struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Size    string `json:"size"`
	Desc    string `json:"desc"`
	URL     string `json:"-"`
	DirName string `json:"dir_name"`
}

var availableModels = []modelOption{
	{
		ID:      "paraformer-zh-small",
		Name:    "Paraformer 中文小型",
		Size:    "74 MB",
		Desc:    "中文识别，速度快，内存占用低（推荐）",
		URL:     "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-paraformer-zh-small-2024-03-09.tar.bz2",
		DirName: "sherpa-onnx-paraformer-zh-small-2024-03-09",
	},
	{
		ID:      "paraformer-zh-int8",
		Name:    "Paraformer 中文 int8 量化",
		Size:    "218 MB",
		Desc:    "中文识别，量化版，精度略高于 small",
		URL:     "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-paraformer-zh-int8-2025-10-07.tar.bz2",
		DirName: "sherpa-onnx-paraformer-zh-int8-2025-10-07",
	},
	{
		ID:      "paraformer-bilingual",
		Name:    "Paraformer 中英双语",
		Size:    "999 MB",
		Desc:    "中英混合识别，体积大，适合双语场景",
		URL:     "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-streaming-paraformer-bilingual-zh-en.tar.bz2",
		DirName: "sherpa-onnx-streaming-paraformer-bilingual-zh-en",
	},
}

// ASRInstallState 是安装进度状态。
type ASRInstallState struct {
	Running   bool    `json:"running"`
	ModelID   string  `json:"model_id"`
	Downloaded int64  `json:"downloaded"`
	Total     int64   `json:"total"`
	Percent   float64 `json:"percent"`
	Phase     string  `json:"phase"` // downloading / extracting / done / error
	Error     string  `json:"error,omitempty"`
	TargetDir string  `json:"target_dir,omitempty"`
}

// ASRInstaller 管理模型下载。
type ASRInstaller struct {
	mu    sync.RWMutex
	state ASRInstallState
}

// NewASRInstaller 创建安装器。
func NewASRInstaller() *ASRInstaller {
	return &ASRInstaller{}
}

// State 返回当前状态。
func (i *ASRInstaller) State() ASRInstallState {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.state
}

// setState 更新状态。
func (i *ASRInstaller) setState(fn func(*ASRInstallState)) {
	i.mu.Lock()
	fn(&i.state)
	i.mu.Unlock()
}

// Start 启动下载（异步）。
func (i *ASRInstaller) Start(ctx context.Context, modelID, dataDir string, logger *zap.Logger) error {
	var model *modelOption
	for idx := range availableModels {
		if availableModels[idx].ID == modelID {
			model = &availableModels[idx]
			break
		}
	}
	if model == nil {
		return fmt.Errorf("未知模型 %q", modelID)
	}

	i.mu.Lock()
	if i.state.Running {
		i.mu.Unlock()
		return fmt.Errorf("已有下载任务进行中")
	}
	targetDir := filepath.Join(dataDir, "models", model.DirName)
	i.state = ASRInstallState{
		Running:   true,
		ModelID:   modelID,
		Phase:     "downloading",
		TargetDir: targetDir,
	}
	i.mu.Unlock()

	go i.download(context.Background(), model, targetDir, logger)
	return nil
}

// download 执行下载 + 解压。
func (i *ASRInstaller) download(ctx context.Context, model *modelOption, targetDir string, logger *zap.Logger) {
	defer func() {
		i.setState(func(s *ASRInstallState) { s.Running = false })
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, model.URL, nil)
	if err != nil {
		i.fail(err)
		return
	}
	client := &http.Client{Timeout: 60 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		i.fail(err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		i.fail(fmt.Errorf("HTTP %d", resp.StatusCode))
		return
	}

	total := resp.ContentLength
	i.setState(func(s *ASRInstallState) { s.Total = total })

	// 带进度计数
	reader := &progressReader{
		r: resp.Body,
		onProgress: func(n int64) {
			i.setState(func(s *ASRInstallState) {
				s.Downloaded = n
				if s.Total > 0 {
					s.Percent = float64(n) / float64(s.Total) * 100
				}
			})
		},
	}

	// 解压 tar.bz2
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		i.fail(err)
		return
	}
	bzReader := bzip2.NewReader(reader)
	tarReader := tar.NewReader(bzReader)

	i.setState(func(s *ASRInstallState) { s.Phase = "extracting" })
	extracted := 0
	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			i.fail(fmt.Errorf("解压失败: %w", err))
			return
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		// 去掉顶层目录
		name := hdr.Name
		if idx := strings.Index(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		if name == "" || strings.Contains(name, "..") {
			continue
		}
		dst := filepath.Join(targetDir, name)
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			i.fail(err)
			return
		}
		f, err := os.Create(dst)
		if err != nil {
			i.fail(err)
			return
		}
		if _, err := io.Copy(f, tarReader); err != nil {
			_ = f.Close()
			i.fail(err)
			return
		}
		_ = f.Close()
		extracted++
	}

	logger.Info("模型下载完成",
		zap.String("model", model.ID),
		zap.Int("files", extracted),
		zap.String("dir", targetDir))

	i.setState(func(s *ASRInstallState) {
		s.Phase = "done"
		s.Percent = 100
	})
}

// fail 记录失败。
func (i *ASRInstaller) fail(err error) {
	i.setState(func(s *ASRInstallState) {
		s.Phase = "error"
		s.Error = err.Error()
	})
}

// progressReader 是带进度回调的 Reader。
type progressReader struct {
	r          io.Reader
	onProgress func(int64)
	total      int64
	lastReport time.Time
}

// Read 实现 io.Reader。
func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.total += int64(n)
		// 每 200ms 报告一次，避免过于频繁
		if time.Since(p.lastReport) > 200*time.Millisecond {
			p.onProgress(p.total)
			p.lastReport = time.Now()
		}
	}
	if err == io.EOF {
		p.onProgress(p.total)
	}
	return n, err
}

// EnvironmentCheck 是环境检测结果。
type EnvironmentCheck struct {
	FFmpeg     bool   `json:"ffmpeg"`
	FFmpegPath string `json:"ffmpeg_path,omitempty"`
	SherpaCLI  bool   `json:"sherpa_cli"`
	SherpaPath string `json:"sherpa_path,omitempty"`
	ModelFound bool   `json:"model_found"`
	ModelPath  string `json:"model_path,omitempty"`
	OS         string `json:"os"`
	Notes      []string `json:"notes,omitempty"`
}

// CheckEnvironment 检测 ASR 相关环境。
func CheckEnvironment(modelDir, cliPath string) EnvironmentCheck {
	env := EnvironmentCheck{}

	// ffmpeg
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		env.FFmpeg = true
		env.FFmpegPath = p
	} else {
		for _, p := range []string{"/usr/bin/ffmpeg", "/usr/local/bin/ffmpeg", "C:\\ffmpeg\\bin\\ffmpeg.exe"} {
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
				env.FFmpeg = true
				env.FFmpegPath = p
				break
			}
		}
	}

	// sherpa CLI
	if cliPath != "" {
		if fi, err := os.Stat(cliPath); err == nil && !fi.IsDir() {
			env.SherpaCLI = true
			env.SherpaPath = cliPath
		}
	}
	if !env.SherpaCLI {
		for _, name := range []string{"sherpa-onnx-offline", "sherpa-onnx-non-streaming-asr"} {
			if p, err := exec.LookPath(name); err == nil {
				env.SherpaCLI = true
				env.SherpaPath = p
				break
			}
		}
	}

	// 模型
	if modelDir != "" {
		for _, name := range []string{"model.onnx", "model.int8.onnx", "encoder.onnx", "encoder.int8.onnx"} {
			p := filepath.Join(modelDir, name)
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
				env.ModelFound = true
				env.ModelPath = p
				break
			}
		}
	}

	// 提示
	if !env.FFmpeg {
		env.Notes = append(env.Notes, "未找到 ffmpeg：Telegram 语音是 OGG 格式，sherpa 只接受 WAV，必须转码。请安装 ffmpeg。")
	}
	if !env.SherpaCLI {
		env.Notes = append(env.Notes, "未找到 sherpa-onnx CLI：请从 GitHub releases 下载对应平台版本。")
	}
	if !env.ModelFound {
		env.Notes = append(env.Notes, "未找到模型文件：可用上方按钮一键下载。")
	}
	return env
}
