package cli

import (
	"archive/tar"
	"compress/bzip2"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/midi-HL/telegram-mcp/internal/asr"
	"github.com/midi-HL/telegram-mcp/internal/config"
)

// 模型下载地址（sherpa-onnx 官方发布）
const (
	modelName = "sherpa-onnx-paraformer-zh-small"
	modelURL  = "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-paraformer-zh-small-2024-03-09.tar.bz2"
	modelSize = "74 MB"
)

// runASR 执行 asr 子命令。
func (a *App) runASR(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("asr", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	a.commonFlags(fs)

	var (
		backend = fs.String("backend", "", "ASR 后端：sherpa / openai / native / none")
		cliPath = fs.String("cli", "", "sherpa-onnx 可执行文件路径")
		apiKey  = fs.String("api-key", "", "云服务 API key")
		endpoint = fs.String("endpoint", "", "云服务 endpoint")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return fmt.Errorf("用法：tgmcp asr <setup|status|config>")
	}

	cfg, err := a.resolveConfig()
	if err != nil {
		return err
	}

	switch fs.Arg(0) {
	case "setup":
		return a.asrSetup(ctx, cfg, *cliPath)
	case "status":
		return a.asrStatus(cfg)
	case "config":
		if *backend != "" {
			cfg.ASR.Backend = *backend
		}
		if *cliPath != "" {
			cfg.ASR.CLIPath = *cliPath
		}
		if *apiKey != "" {
			cfg.ASR.APIKey = *apiKey
		}
		if *endpoint != "" {
			cfg.ASR.Endpoint = *endpoint
		}
		if err := cfg.Save(); err != nil {
			return err
		}
		fmt.Fprintf(a.Stdout, "配置已保存\n")
		return a.asrStatus(cfg)
	default:
		return fmt.Errorf("未知子命令 %q（支持 setup/status/config）", fs.Arg(0))
	}
}

// asrStatus 显示 ASR 配置状态。
func (a *App) asrStatus(cfg *config.Config) error {
	fmt.Fprintf(a.Stdout, "ASR 配置\n")
	fmt.Fprintf(a.Stdout, "  后端     : %s\n", cfg.ASR.Backend)
	fmt.Fprintf(a.Stdout, "  模型目录 : %s\n", cfg.ASR.ModelDir)
	fmt.Fprintf(a.Stdout, "  CLI 路径 : %s\n", cfg.ASR.CLIPath)

	if cfg.ASR.ModelDir != "" {
		if _, err := os.Stat(filepath.Join(cfg.ASR.ModelDir, "model.onnx")); err == nil {
			fmt.Fprintf(a.Stdout, "  模型状态 : ✅ 已安装\n")
		} else {
			fmt.Fprintf(a.Stdout, "  模型状态 : ❌ 未安装（运行 tgmcp asr setup）\n")
		}
	}
	if cfg.ASR.CLIPath != "" {
		if _, err := os.Stat(cfg.ASR.CLIPath); err == nil {
			fmt.Fprintf(a.Stdout, "  CLI 状态 : ✅ 存在\n")
		} else {
			fmt.Fprintf(a.Stdout, "  CLI 状态 : ❌ 不存在\n")
		}
	}
	return nil
}

// asrSetup 下载并解压 sherpa-onnx 模型。
func (a *App) asrSetup(ctx context.Context, cfg *config.Config, cliPath string) error {
	modelDir := filepath.Join(cfg.DataDir, "models", modelName)
	if _, err := os.Stat(filepath.Join(modelDir, "model.onnx")); err == nil {
		fmt.Fprintf(a.Stdout, "模型已存在：%s\n", modelDir)
	} else {
		fmt.Fprintf(a.Stdout, "下载模型（%s）...\n", modelSize)
		fmt.Fprintf(a.Stdout, "  来源：%s\n", modelURL)

		if err := downloadAndExtract(ctx, modelURL, modelDir, a.Logger); err != nil {
			return fmt.Errorf("下载模型失败: %w", err)
		}
		fmt.Fprintf(a.Stdout, "✅ 模型已安装：%s\n", modelDir)
	}

	// 检查 CLI
	if cliPath == "" {
		cliPath = findSherpaCLI()
	}
	if cliPath == "" {
		fmt.Fprintf(a.Stdout, "\n⚠ 未找到 sherpa-onnx 可执行文件\n")
		fmt.Fprintf(a.Stdout, "  请下载对应平台的 CLI：\n")
		fmt.Fprintf(a.Stdout, "    Windows: sherpa-onnx-non-streaming-asr-x64-v2.0.1.exe\n")
		fmt.Fprintf(a.Stdout, "    Linux:   sherpa-onnx-v2.0.1-linux-x64-shared.tar.bz2\n")
		fmt.Fprintf(a.Stdout, "    Termux:  sherpa-onnx-v2.0.1-android-aarch64-termux-shared.tar.bz2\n")
		fmt.Fprintf(a.Stdout, "  下载后运行：tgmcp asr config --cli <路径>\n")
	} else {
		fmt.Fprintf(a.Stdout, "✅ 找到 CLI：%s\n", cliPath)
	}

	// 保存配置
	cfg.ASR.Backend = "sherpa"
	cfg.ASR.ModelDir = modelDir
	if cliPath != "" {
		cfg.ASR.CLIPath = cliPath
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "\n配置已保存\n")
	return a.asrStatus(cfg)
}

// downloadAndExtract 下载 tar.bz2 并解压到目标目录。
func downloadAndExtract(ctx context.Context, url, destDir string, logger *zap.Logger) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// bzip2 解压 → tar 解析
	bzReader := bzip2.NewReader(resp.Body)
	tarReader := tar.NewReader(bzReader)

	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return err
	}

	extracted := 0
	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取 tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		// 去掉顶层目录
		name := hdr.Name
		if i := strings.Index(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		if name == "" || strings.Contains(name, "..") {
			continue
		}
		target := filepath.Join(destDir, name)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		f, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, tarReader); err != nil {
			_ = f.Close()
			return err
		}
		_ = f.Close()
		extracted++
		if extracted%10 == 0 {
			logger.Info("解压进度", zap.Int("files", extracted))
		}
	}
	logger.Info("模型解压完成", zap.Int("files", extracted))
	return nil
}

// findSherpaCLI 在常见位置查找 sherpa-onnx 可执行文件。
func findSherpaCLI() string {
	candidates := []string{
		"sherpa-onnx-offline",
		"sherpa-onnx-non-streaming-asr",
		"sherpa-onnx.exe",
		"sherpa-onnx-non-streaming-asr-x64-v2.0.1.exe",
	}
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return ""
}

// 确保 asr 包被引用
var _ = asr.ModelDirName
