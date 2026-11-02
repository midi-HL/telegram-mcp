package webui

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// DepKind 是依赖类型。
type DepKind string

const (
	DepFFmpeg DepKind = "ffmpeg"
	DepSherpa DepKind = "sherpa"
)

// DepInstallState 是依赖安装进度。
type DepInstallState struct {
	Running    bool     `json:"running"`
	Kind       DepKind  `json:"kind"`
	Phase      string   `json:"phase"` // checking / downloading / extracting / done / error / unsupported
	Downloaded int64    `json:"downloaded"`
	Total      int64    `json:"total"`
	Percent    float64  `json:"percent"`
	Message    string   `json:"message,omitempty"`
	Error      string   `json:"error,omitempty"`
	Installed  string   `json:"installed_path,omitempty"`
	Steps      []string `json:"steps,omitempty"`
}

// DepInstaller 负责自动安装 ffmpeg 和 sherpa-onnx CLI。
type DepInstaller struct {
	mu    sync.RWMutex
	state DepInstallState
}

// NewDepInstaller 创建安装器。
func NewDepInstaller() *DepInstaller {
	return &DepInstaller{}
}

// State 返回当前状态。
func (d *DepInstaller) State() DepInstallState {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state
}

// setState 更新状态。
func (d *DepInstaller) setState(fn func(*DepInstallState)) {
	d.mu.Lock()
	fn(&d.state)
	d.mu.Unlock()
}

// Start 启动安装（异步）。
func (d *DepInstaller) Start(ctx context.Context, kind DepKind, dataDir string, logger *zap.Logger) error {
	d.mu.Lock()
	if d.state.Running {
		d.mu.Unlock()
		return fmt.Errorf("已有安装任务进行中")
	}
	d.state = DepInstallState{
		Running: true,
		Kind:    kind,
		Phase:   "checking",
	}
	d.mu.Unlock()

	switch kind {
	case DepFFmpeg:
		go d.installFFmpeg(dataDir, logger)
	case DepSherpa:
		go d.installSherpa(dataDir, logger)
	default:
		d.setState(func(s *DepInstallState) {
			s.Running = false
			s.Phase = "error"
			s.Error = "未知依赖类型"
		})
		return fmt.Errorf("未知依赖类型 %q", kind)
	}
	return nil
}

// addStep 追加步骤日志。
func (d *DepInstaller) addStep(msg string) {
	d.setState(func(s *DepInstallState) {
		s.Steps = append(s.Steps, msg)
	})
}

// installFFmpeg 安装 ffmpeg。
//
// 策略：
//  1. Linux: 尝试包管理器（apt/dnf/apk/pacman）
//  2. macOS: brew
//  3. Windows: 下载 gyan.dev 的静态构建并解压
//  4. 兜底：提示手动安装
func (d *DepInstaller) installFFmpeg(dataDir string, logger *zap.Logger) {
	defer func() {
		d.setState(func(s *DepInstallState) { s.Running = false })
	}()

	// 先检查是否已安装
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		d.setState(func(s *DepInstallState) {
			s.Phase = "done"
			s.Percent = 100
			s.Installed = p
			s.Message = "ffmpeg 已存在，无需安装"
		})
		return
	}

	switch runtime.GOOS {
	case "linux":
		d.installFFmpegLinux()
	case "darwin":
		d.installFFmpegDarwin()
	case "windows":
		d.installFFmpegWindows(dataDir)
	default:
		d.setState(func(s *DepInstallState) {
			s.Phase = "unsupported"
			s.Error = fmt.Sprintf("不支持自动安装 ffmpeg 的平台：%s，请手动安装", runtime.GOOS)
		})
	}
	_ = logger
}

// installFFmpegLinux 用包管理器安装。
func (d *DepInstaller) installFFmpegLinux() {
	// 检测包管理器
	type pm struct {
		cmd  string
		args []string
	}
	var candidates []pm
	if _, err := exec.LookPath("apt-get"); err == nil {
		candidates = append(candidates, pm{"apt-get", []string{"install", "-y", "ffmpeg"}})
	}
	if _, err := exec.LookPath("dnf"); err == nil {
		candidates = append(candidates, pm{"dnf", []string{"install", "-y", "ffmpeg"}})
	}
	if _, err := exec.LookPath("yum"); err == nil {
		candidates = append(candidates, pm{"yum", []string{"install", "-y", "ffmpeg"}})
	}
	if _, err := exec.LookPath("apk"); err == nil {
		candidates = append(candidates, pm{"apk", []string{"add", "ffmpeg"}})
	}
	if _, err := exec.LookPath("pacman"); err == nil {
		candidates = append(candidates, pm{"pacman", []string{"-S", "--noconfirm", "ffmpeg"}})
	}

	if len(candidates) == 0 {
		d.setState(func(s *DepInstallState) {
			s.Phase = "unsupported"
			s.Error = "未找到包管理器（apt/dnf/yum/apk/pacman），请手动安装 ffmpeg"
		})
		return
	}

	d.setState(func(s *DepInstallState) { s.Phase = "extracting" })

	for _, c := range candidates {
		d.addStep(fmt.Sprintf("尝试 %s %s", c.cmd, strings.Join(c.args, " ")))
		// apt 需要先 update
		if c.cmd == "apt-get" {
			upd := exec.Command("apt-get", "update", "-qq")
			upd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
			_ = upd.Run()
		}
		cmd := exec.Command(c.cmd, c.args...)
		cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
		out, err := cmd.CombinedOutput()
		if err != nil {
			d.addStep(fmt.Sprintf("%s 失败: %v", c.cmd, truncStr(string(out), 200)))
			continue
		}
		if p, err := exec.LookPath("ffmpeg"); err == nil {
			d.setState(func(s *DepInstallState) {
				s.Phase = "done"
				s.Percent = 100
				s.Installed = p
				s.Message = "ffmpeg 安装成功"
			})
			return
		}
	}

	d.setState(func(s *DepInstallState) {
		s.Phase = "error"
		s.Error = "所有包管理器均失败。可能需要在容器内以 root 运行，或手动安装 ffmpeg"
	})
}

// installFFmpegDarwin 用 brew 安装。
func (d *DepInstaller) installFFmpegDarwin() {
	if _, err := exec.LookPath("brew"); err != nil {
		d.setState(func(s *DepInstallState) {
			s.Phase = "unsupported"
			s.Error = "未找到 Homebrew，请先安装 brew 或手动安装 ffmpeg"
		})
		return
	}
	d.setState(func(s *DepInstallState) { s.Phase = "extracting" })
	d.addStep("brew install ffmpeg")
	cmd := exec.Command("brew", "install", "ffmpeg")
	out, err := cmd.CombinedOutput()
	if err != nil {
		d.setState(func(s *DepInstallState) {
			s.Phase = "error"
			s.Error = "brew install 失败: " + truncStr(string(out), 300)
		})
		return
	}
	p, _ := exec.LookPath("ffmpeg")
	d.setState(func(s *DepInstallState) {
		s.Phase = "done"
		s.Percent = 100
		s.Installed = p
		s.Message = "ffmpeg 安装成功"
	})
}

// installFFmpegWindows 下载静态构建。
func (d *DepInstaller) installFFmpegWindows(dataDir string) {
	// 用 gyan.dev 的 essentials 构建
	const url = "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"

	binDir := filepath.Join(dataDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		d.setState(func(s *DepInstallState) {
			s.Phase = "error"
			s.Error = err.Error()
		})
		return
	}

	d.setState(func(s *DepInstallState) { s.Phase = "downloading" })
	zipPath := filepath.Join(binDir, "ffmpeg.zip")

	if err := d.downloadFile(url, zipPath); err != nil {
		d.setState(func(s *DepInstallState) {
			s.Phase = "error"
			s.Error = "下载失败: " + err.Error()
		})
		return
	}
	defer func() { _ = os.Remove(zipPath) }()

	d.setState(func(s *DepInstallState) { s.Phase = "extracting" })
	d.addStep("解压 ffmpeg.zip")

	if err := unzipFFmpeg(zipPath, binDir); err != nil {
		d.setState(func(s *DepInstallState) {
			s.Phase = "error"
			s.Error = "解压失败: " + err.Error()
		})
		return
	}

	// 找 ffmpeg.exe
	var found string
	_ = filepath.Walk(binDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.EqualFold(info.Name(), "ffmpeg.exe") {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if found == "" {
		d.setState(func(s *DepInstallState) {
			s.Phase = "error"
			s.Error = "解压后未找到 ffmpeg.exe"
		})
		return
	}

	d.setState(func(s *DepInstallState) {
		s.Phase = "done"
		s.Percent = 100
		s.Installed = found
		s.Message = "ffmpeg 安装成功，路径：" + found
	})
}

// installSherpa 下载 sherpa-onnx CLI。
func (d *DepInstaller) installSherpa(dataDir string, logger *zap.Logger) {
	defer func() {
		d.setState(func(s *DepInstallState) { s.Running = false })
	}()

	// 检查已安装
	if p := findSherpaCLIPath(); p != "" {
		d.setState(func(s *DepInstallState) {
			s.Phase = "done"
			s.Percent = 100
			s.Installed = p
			s.Message = "sherpa-onnx CLI 已存在，无需安装"
		})
		return
	}

	// 按平台选下载地址
	url, archiveName := sherpaDownloadURL()
	if url == "" {
		d.setState(func(s *DepInstallState) {
			s.Phase = "unsupported"
			s.Error = fmt.Sprintf("暂不支持自动安装 sherpa-onnx 的平台：%s/%s，请手动下载", runtime.GOOS, runtime.GOARCH)
		})
		return
	}

	// 解压到 data/sherpa/（压缩包内已有 bin/ lib/ include/ 结构）
	binDir := filepath.Join(dataDir, "sherpa")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		d.setState(func(s *DepInstallState) {
			s.Phase = "error"
			s.Error = err.Error()
		})
		return
	}

	d.setState(func(s *DepInstallState) {
		s.Phase = "downloading"
		s.Message = "下载 " + archiveName
	})
	d.addStep("下载 " + url)

	archivePath := filepath.Join(binDir, archiveName)
	if err := d.downloadFile(url, archivePath); err != nil {
		d.setState(func(s *DepInstallState) {
			s.Phase = "error"
			s.Error = "下载失败: " + err.Error()
		})
		return
	}
	defer func() { _ = os.Remove(archivePath) }()

	d.setState(func(s *DepInstallState) { s.Phase = "extracting" })
	d.addStep("解压 " + archiveName)

	if err := extractArchive(archivePath, binDir); err != nil {
		d.setState(func(s *DepInstallState) {
			s.Phase = "error"
			s.Error = "解压失败: " + err.Error()
		})
		return
	}

	// 找到 sherpa-onnx-offline
	var found string
	_ = filepath.Walk(binDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		base := info.Name()
		if base == "sherpa-onnx-offline" || base == "sherpa-onnx-offline.exe" ||
			base == "sherpa-onnx-non-streaming-asr" || base == "sherpa-onnx-non-streaming-asr.exe" {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if found == "" {
		d.setState(func(s *DepInstallState) {
			s.Phase = "error"
			s.Error = "解压后未找到 sherpa-onnx CLI 可执行文件"
		})
		return
	}

	// Unix 上补执行权限
	if runtime.GOOS != "windows" {
		_ = os.Chmod(found, 0o755)
		// 同目录下的 .so 也要可读
		dir := filepath.Dir(found)
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".so") || strings.Contains(e.Name(), ".so.") {
				_ = os.Chmod(filepath.Join(dir, e.Name()), 0o755)
			}
		}
	}

	logger.Info("sherpa-onnx CLI 安装完成", zap.String("path", found))
	d.setState(func(s *DepInstallState) {
		s.Phase = "done"
		s.Percent = 100
		s.Installed = found
		s.Message = "sherpa-onnx CLI 安装成功"
	})
}

// sherpaDownloadURL 按平台返回下载地址。
//
// 资产名经 GitHub API 核实（v2.0.1 共 288 个资产），全部为 tar.bz2：
//   - linux-x64-shared.tar.bz2            (Linux x86_64, glibc)
//   - linux-aarch64-shared-cpu.tar.bz2    (Linux ARM64, glibc)
//   - linux-arm-gnueabihf-shared.tar.bz2  (Linux ARMv7, glibc)
//   - osx-arm64-shared.tar.bz2            (macOS Apple Silicon)
//   - osx-x64-shared.tar.bz2              (macOS Intel)
//   - win-x64-shared-MD-Release.tar.bz2   (Windows x64, MD 运行时)
//
// 注意：Windows 没有 .zip 资产，只有 tar.bz2。
func sherpaDownloadURL() (url, name string) {
	const base = "https://github.com/k2-fsa/sherpa-onnx/releases/download/v2.0.1/"
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		return base + "sherpa-onnx-v2.0.1-linux-x64-shared.tar.bz2", "sherpa.tar.bz2"
	case "linux/arm64":
		return base + "sherpa-onnx-v2.0.1-linux-aarch64-shared-cpu.tar.bz2", "sherpa.tar.bz2"
	case "linux/arm":
		return base + "sherpa-onnx-v2.0.1-linux-arm-gnueabihf-shared.tar.bz2", "sherpa.tar.bz2"
	case "darwin/arm64":
		return base + "sherpa-onnx-v2.0.1-osx-arm64-shared.tar.bz2", "sherpa.tar.bz2"
	case "darwin/amd64":
		return base + "sherpa-onnx-v2.0.1-osx-x64-shared.tar.bz2", "sherpa.tar.bz2"
	case "windows/amd64":
		// Windows 只有 tar.bz2；MD 运行时（与 MSVC 动态运行时匹配）
		return base + "sherpa-onnx-v2.0.1-win-x64-shared-MD-Release.tar.bz2", "sherpa.tar.bz2"
	default:
		return "", ""
	}
}

// findSherpaCLIPath 查找已安装的 sherpa CLI。
func findSherpaCLIPath() string {
	for _, name := range []string{
		"sherpa-onnx-offline", "sherpa-onnx-offline.exe",
		"sherpa-onnx-non-streaming-asr", "sherpa-onnx-non-streaming-asr.exe",
	} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// downloadFile 下载文件并报告进度。
func (d *DepInstaller) downloadFile(url, dst string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "tgmcp/1.0")
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	d.setState(func(s *DepInstallState) { s.Total = resp.ContentLength })

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	reader := &progressReader{
		r: resp.Body,
		onProgress: func(n int64) {
			d.setState(func(s *DepInstallState) {
				s.Downloaded = n
				if s.Total > 0 {
					s.Percent = float64(n) / float64(s.Total) * 100
				}
			})
		},
	}
	_, err = io.Copy(f, reader)
	return err
}

// extractArchive 按扩展名解压。
func extractArchive(path, dstDir string) error {
	switch {
	case strings.HasSuffix(path, ".tar.bz2"):
		return extractTarBz2(path, dstDir)
	case strings.HasSuffix(path, ".tar.gz"):
		return extractTarGz(path, dstDir)
	case strings.HasSuffix(path, ".zip"):
		return extractZip(path, dstDir)
	default:
		return fmt.Errorf("不支持的压缩格式: %s", path)
	}
}

// extractTarBz2 解压 tar.bz2。
func extractTarBz2(path, dstDir string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return extractTarStream(bzip2.NewReader(f), dstDir)
}

// extractTarGz 解压 tar.gz。
func extractTarGz(path, dstDir string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	return extractTarStream(gz, dstDir)
}

// extractTarStream 从 Reader 解压 tar。
func extractTarStream(r io.Reader, dstDir string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		// 去掉顶层目录
		name := hdr.Name
		if idx := strings.Index(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		if name == "" || strings.Contains(name, "..") {
			continue
		}
		dst := filepath.Join(dstDir, name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			_ = os.MkdirAll(dst, 0o755)
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return err
			}
			_ = f.Close()
		}
	}
}

// extractZip 解压 zip。
func extractZip(path, dstDir string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		name := f.Name
		if idx := strings.Index(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		if name == "" || strings.Contains(name, "..") {
			continue
		}
		dst := filepath.Join(dstDir, name)
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(dst, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		_ = out.Close()
		_ = rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// unzipFFmpeg 专门解压 ffmpeg zip（Windows）。
func unzipFFmpeg(path, dstDir string) error {
	return extractZip(path, dstDir)
}

// truncStr 截断字符串。
func truncStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
