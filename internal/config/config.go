// Package config 负责配置加载与路径管理。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// Account 表示一个 Telegram 账号的配置。
type Account struct {
	// Name 是本地别名，用于多账号切换（如 "main"、"work"）。
	Name string `json:"name"`
	// Phone 是手机号，用于登录。扫码登录时可为空。
	Phone string `json:"phone,omitempty"`
	// APIID / APIHash 是该账号使用的 Telegram 应用凭据。
	APIID   int    `json:"api_id"`
	APIHash string `json:"api_hash"`
	// SessionDir 是该账号的 session 存储目录。
	SessionDir string `json:"session_dir"`
}

// WebUI 是 Web UI 的配置。
type WebUI struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
}

// ASR 是语音转写的配置。
type ASR struct {
	Backend   string `json:"backend"`
	ModelDir  string `json:"model_dir,omitempty"`
	CLIPath   string `json:"cli_path,omitempty"`
	APIKey    string `json:"api_key,omitempty"`
	Endpoint  string `json:"endpoint,omitempty"`
}

// Config 是完整配置。
type Config struct {
	// DataDir 是数据根目录（session、缓存数据库、配置）。
	DataDir string `json:"data_dir"`
	// DefaultAccount 是默认账号名。
	DefaultAccount string `json:"default_account"`
	// Accounts 是所有账号配置。
	Accounts []Account `json:"accounts"`
	// WebUI 配置。
	WebUI WebUI `json:"webui"`
	// ASR 配置。
	ASR ASR `json:"asr"`
}

// DefaultDataDir 返回平台默认数据目录。
func DefaultDataDir() string {
	if dir := os.Getenv("TGMCP_DATA_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".tgmcp"
	}
	return filepath.Join(home, ".tgmcp")
}

// Default 返回默认配置。
func Default() *Config {
	dataDir := DefaultDataDir()
	return &Config{
		DataDir:        dataDir,
		DefaultAccount: "main",
		Accounts:       nil,
		WebUI: WebUI{
			Enabled: true,
			Host:    "0.0.0.0",
			Port:    8080,
		},
		ASR: ASR{
			Backend: "none",
		},
	}
}

// Path 返回配置文件路径。
func Path(dataDir string) string {
	return filepath.Join(dataDir, "config.json")
}

// Load 从 dataDir 加载配置。文件不存在时返回默认配置。
func Load(dataDir string) (*Config, error) {
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}
	path := Path(dataDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := Default()
			cfg.DataDir = dataDir
			return cfg, nil
		}
		return nil, fmt.Errorf("读取配置 %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置 %s: %w", path, err)
	}
	if cfg.DataDir == "" {
		cfg.DataDir = dataDir
	}
	// 填充默认值
	if cfg.DefaultAccount == "" {
		cfg.DefaultAccount = "main"
	}
	if cfg.WebUI.Port == 0 {
		cfg.WebUI.Port = 8080
	}
	if cfg.WebUI.Host == "" {
		cfg.WebUI.Host = "0.0.0.0"
	}
	if cfg.ASR.Backend == "" {
		cfg.ASR.Backend = "none"
	}
	return &cfg, nil
}

// Save 写入配置文件。
func (c *Config) Save() error {
	if err := os.MkdirAll(c.DataDir, 0o700); err != nil {
		return fmt.Errorf("创建数据目录: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置: %w", err)
	}
	path := Path(c.DataDir)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("写入配置 %s: %w", path, err)
	}
	return nil
}

// Account 按名称查找账号，找不到返回 nil。
func (c *Config) Account(name string) *Account {
	for i := range c.Accounts {
		if c.Accounts[i].Name == name {
			return &c.Accounts[i]
		}
	}
	return nil
}

// UpsertAccount 新增或更新账号配置。
func (c *Config) UpsertAccount(acc Account) {
	if acc.SessionDir == "" {
		acc.SessionDir = filepath.Join(c.DataDir, "sessions", acc.Name)
	}
	for i := range c.Accounts {
		if c.Accounts[i].Name == acc.Name {
			c.Accounts[i] = acc
			return
		}
	}
	c.Accounts = append(c.Accounts, acc)
}

// EnvCredentials 从环境变量读取 API 凭据（TELEGRAM_API_ID / TELEGRAM_API_HASH）。
// 返回 ok=false 表示环境变量不完整。
func EnvCredentials() (id int, hash string, ok bool) {
	idStr := os.Getenv("TELEGRAM_API_ID")
	hash = os.Getenv("TELEGRAM_API_HASH")
	if idStr == "" || hash == "" {
		return 0, "", false
	}
	n, err := strconv.Atoi(idStr)
	if err != nil || n <= 0 {
		return 0, "", false
	}
	return n, hash, true
}
