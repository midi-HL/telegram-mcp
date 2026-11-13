// Package config 负责配置加载与路径管理。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
//
// 路径解析优先级（从高到低）：
//  1. 环境变量 TGMCP_ASR_MODEL_DIR / TGMCP_ASR_CLI
//  2. config.json 里的 model_dir / cli_path
//  3. 默认 {DataDir}/models/sherpa-onnx-paraformer-zh-small
type ASR struct {
	Backend  string `json:"backend"`
	ModelDir string `json:"model_dir,omitempty"`
	CLIPath  string `json:"cli_path,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	Language string `json:"language,omitempty"`
	// FFmpegPath 是 ffmpeg 路径（空则自动查找），用于把 Ogg/Opus 转成 WAV
	FFmpegPath string `json:"ffmpeg_path,omitempty"`
	// NumThreads 是 sherpa 的 CPU 线程数（0 则默认 2）
	NumThreads int `json:"num_threads,omitempty"`
}

// ResolveModelDir 按优先级解析模型目录。
func (a *ASR) ResolveModelDir(dataDir string) string {
	if v := os.Getenv("TGMCP_ASR_MODEL_DIR"); v != "" {
		return v
	}
	if a.ModelDir != "" {
		return a.ModelDir
	}
	return filepath.Join(dataDir, "models", "sherpa-onnx-paraformer-zh-small")
}

// ResolveCLIPath 按优先级解析 CLI 路径。
func (a *ASR) ResolveCLIPath() string {
	if v := os.Getenv("TGMCP_ASR_CLI"); v != "" {
		return v
	}
	return a.CLIPath
}

// Wake 是「新消息唤醒 Agent」的配置。
type Wake struct {
	Enabled bool `json:"enabled"`
	// Mode: notify / sampling / webhook / all。
	// notify 推送 MCP notifications/message；sampling 向 Agent 发一条消息；
	// webhook 调用外部 HTTP；all 同时执行三种。
	Mode string `json:"mode"`
	// IncludeOut 是否连自己发出的消息也唤醒（默认 false）。
	IncludeOut bool `json:"include_out"`
	// IncludeText 是否在通知里携带消息正文（默认 true，外部内容会包装）。
	IncludeText bool `json:"include_text"`
	// 消息正文最多保留的字符数。
	MaxTextRunes int `json:"max_text_runes,omitempty"`
	// 同一会话两次唤醒之间的最小间隔（秒）。
	DebounceSeconds int `json:"debounce_seconds,omitempty"`
	// Webhook 地址（mode=webhook/all 时使用）。
	WebhookURL string `json:"webhook_url,omitempty"`
	// Webhook Bearer token。
	WebhookToken string `json:"webhook_token,omitempty"`
	// 交付给 Agent 的提示语模板，留空使用默认。
	Prompt string `json:"prompt,omitempty"`
}

// DefaultWake 返回唤醒配置的默认值。
func DefaultWake() Wake {
	return Wake{
		Enabled:         false,
		Mode:            "notify",
		IncludeText:     true,
		MaxTextRunes:    280,
		DebounceSeconds: 2,
	}
}

// NormalizeWake 修正唤醒配置中的空值/非法值。
func NormalizeWake(w *Wake) {
	if w == nil {
		return
	}
	d := DefaultWake()
	if w.Mode == "" {
		w.Mode = d.Mode
	}
	switch w.Mode {
	case "notify", "sampling", "webhook", "all":
	default:
		w.Mode = d.Mode
	}
	if w.MaxTextRunes <= 0 {
		w.MaxTextRunes = d.MaxTextRunes
	}
	if w.DebounceSeconds < 0 {
		w.DebounceSeconds = d.DebounceSeconds
	}
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
	// Wake 是新消息唤醒 Agent 的配置。
	Wake Wake `json:"wake"`
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
		Wake: DefaultWake(),
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
	// 区分「旧配置没有 wake 字段」与「用户显式写了 false」：
	// 前者给完整默认值（小白开箱即用），后者尊重用户意图。
	var raw struct {
		Wake json.RawMessage `json:"wake"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("解析配置 %s: %w", path, err)
	}
	hasWake := len(raw.Wake) > 0 && string(raw.Wake) != "null"
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
	if !hasWake {
		cfg.Wake = DefaultWake()
	} else {
		// 只修正空/非法值，不因零值布尔字段把用户显式关闭的设置重置。
		NormalizeWake(&cfg.Wake)
	}
	// 强制收录内置凭据：空值补默认，显式值保持可替换。
	// 环境变量不写进配置，撤销后会自动回到文件里的值。
	for i := range cfg.Accounts {
		FillBuiltinCredentials(&cfg.Accounts[i])
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
// api_id / api_hash 留空时自动写入公开内置默认凭据。
func (c *Config) UpsertAccount(acc Account) {
	if acc.SessionDir == "" {
		acc.SessionDir = filepath.Join(c.DataDir, "sessions", acc.Name)
	}
	FillBuiltinCredentials(&acc)
	for i := range c.Accounts {
		if c.Accounts[i].Name == acc.Name {
			c.Accounts[i] = acc
			return
		}
	}
	c.Accounts = append(c.Accounts, acc)
	// 首个账号自动成为默认账号：零账号启动后 Web 登录也能立即作为默认使用。
	if len(c.Accounts) == 1 {
		c.DefaultAccount = acc.Name
	}
}

// EffectiveCredentials 返回指定账号实际生效的凭据及来源。
// 账号不存在时返回全局解析结果。
func (c *Config) EffectiveCredentials(name string) Credentials {
	if acc := c.Account(name); acc != nil {
		return ResolveCredentials(acc.APIID, acc.APIHash)
	}
	return ResolveCredentials(0, "")
}

// BuiltinCredentialsUsed 判断是否至少有一个账号仍在使用内置凭据。
func (c *Config) BuiltinCredentialsUsed() bool {
	for i := range c.Accounts {
		if IsBuiltinCredentials(c.Accounts[i].APIID, c.Accounts[i].APIHash) {
			return true
		}
	}
	return false
}
