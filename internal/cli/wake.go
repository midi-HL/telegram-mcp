package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/midi-HL/telegram-mcp/internal/config"
	"github.com/midi-HL/telegram-mcp/internal/wake"
)

// runWake 管理「新消息唤醒 Agent」。
func (a *App) runWake(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("wake", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	a.commonFlags(fs)
	var (
		mode     = fs.String("mode", "", "notify / sampling / webhook / all")
		out      = fs.Bool("include-out", false, "自己发的消息也唤醒（配合 config）")
		noText   = fs.Bool("no-text", false, "通知里不附带消息正文")
		debounce = fs.Int("debounce", -1, "同一会话防抖秒数（0 关闭）")
		maxText  = fs.Int("max-text", 0, "正文最大字符数")
		webhook  = fs.String("webhook", "", "Webhook URL")
		token    = fs.String("webhook-token", "", "Webhook Bearer token")
		prompt   = fs.String("prompt", "", "给 Agent 的提示语")
		text     = fs.String("text", "", "test 用的消息正文")
		webuiURL = fs.String("webui", "", "已运行服务的 Web UI 地址（如 http://127.0.0.1:8080），用于在线 test/status")
	)
	// Go 的 flag 包遇到第一个位置参数就停止解析，因此
	// 「wake test --webui http://...」这种自然写法会丢掉后面的 flag。
	// 这里把 action 移到末尾，让所有 flag 都能被解析；
	// 「wake --webui http://... test」也仍然可用。
	parseArgs := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		parseArgs = append(append([]string{}, args[1:]...), args[0])
	}
	if err := fs.Parse(parseArgs); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("用法：tgmcp wake <status|on|off|config|test|pending|ack> [flags]")
	}
	action := fs.Arg(0)
	cfg, err := a.resolveConfig()
	if err != nil {
		return err
	}

	// on/off/config 写本地配置；指定 --webui 时同步热更新运行中服务。
	switch action {
	case "on":
		cfg.Wake.Enabled = true
		if err := cfg.Save(); err != nil {
			return err
		}
		if *webuiURL != "" {
			_, err := a.wakeRemoteResult(ctx, *webuiURL, "on", *text, cfg)
			return err
		}
		return a.wakeStatus(cfg, "已开启（下一次启动生效；运行中服务可用 Web UI 或 --webui 同步）")
	case "off":
		cfg.Wake.Enabled = false
		if err := cfg.Save(); err != nil {
			return err
		}
		if *webuiURL != "" {
			_, err := a.wakeRemoteResult(ctx, *webuiURL, "off", *text, cfg)
			return err
		}
		return a.wakeStatus(cfg, "已关闭（下一次启动生效；运行中服务可用 Web UI 或 --webui 同步）")
	case "config":
		if *mode != "" {
			switch *mode {
			case "notify", "sampling", "webhook", "all":
				cfg.Wake.Mode = *mode
			default:
				return fmt.Errorf("未知 --mode %q（notify/sampling/webhook/all）", *mode)
			}
		}
		if *out {
			cfg.Wake.IncludeOut = true
		}
		if *noText {
			cfg.Wake.IncludeText = false
		}
		if *debounce >= 0 {
			cfg.Wake.DebounceSeconds = *debounce
		}
		if *maxText > 0 {
			cfg.Wake.MaxTextRunes = *maxText
		}
		if *webhook != "" {
			if err := validateWebhookURL(*webhook); err != nil {
				return err
			}
			cfg.Wake.WebhookURL = *webhook
		}
		if *token != "" {
			cfg.Wake.WebhookToken = *token
		}
		if *prompt != "" {
			cfg.Wake.Prompt = *prompt
		}
		config.NormalizeWake(&cfg.Wake)
		if err := cfg.Save(); err != nil {
			return err
		}
		if *webuiURL != "" {
			_, err := a.wakeRemoteResult(ctx, *webuiURL, "config", *text, cfg)
			return err
		}
		return a.wakeStatus(cfg, "配置已保存（下一次启动生效）")
	case "status", "pending", "ack", "test":
		// 继续
	default:
		return fmt.Errorf("未知子命令 %q（status/on/off/config/test/pending/ack）", action)
	}

	if *webuiURL != "" {
		_, err := a.wakeRemoteResult(ctx, *webuiURL, action, *text, cfg)
		if err != nil {
			return err
		}
		return nil
	}
	return a.wakeLocal(ctx, cfg, action, *text)
}

func (a *App) wakeStatus(cfg *config.Config, msg string) error {
	fmt.Fprintf(a.Stdout, "%s\n\n", msg)
	printWakeConfig(a.Stdout, cfg.Wake)
	return nil
}

func (a *App) wakeLocal(ctx context.Context, cfg *config.Config, action, text string) error {
	// 本地进程没有已连接的 MCP 会话：status/config 直接读盘；
	// test 仍可验证 webhook 与待读队列，MCP 通道会明确标为未就绪。
	e := wake.New(wake.Options{Config: cfg.Wake, Logger: a.Logger, Transport: "cli"})
	switch action {
	case "status":
		printWakeConfig(a.Stdout, e.Config())
		fmt.Fprintf(a.Stdout, "\n提示：查看运行中服务的实时状态可用 --webui http://127.0.0.1:8080\n")
		return nil
	case "pending":
		list := e.Pending()
		fmt.Fprintf(a.Stdout, "本地 CLI 进程待读 %d 条（运行中服务的队列请用 --webui 查看）\n", len(list))
		return nil
	case "ack":
		fmt.Fprintf(a.Stdout, "本地 CLI 进程已清空 %d 条（运行中服务的队列请用 --webui 清空）\n", e.Ack())
		return nil
	case "test":
		note := e.Test(ctx, text)
		enc := json.NewEncoder(a.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(note); err != nil {
			return err
		}
		fmt.Fprintf(a.Stdout, "\n说明：CLI 独立进程没有 MCP 会话；notify/sampling 会显示「MCP server 尚未就绪」。\n")
		fmt.Fprintf(a.Stdout, "要验证真实 Agent 推送，请对运行中服务用：tgmcp wake test --webui http://127.0.0.1:8080\n")
		return nil
	}
	return nil
}

// wakeRemoteResult 通过 Web UI 已登录会话操作运行中服务。
// 口令从 TGMCP_WEBUI_PASSWORD 读取；没有设置时用默认 Admin。
func (a *App) wakeRemoteResult(ctx context.Context, base, action, text string, cfg *config.Config) (any, error) {
	password := getenv("TGMCP_WEBUI_PASSWORD")
	if password == "" {
		password = "Admin"
	}
	base = strings.TrimRight(base, "/")
	client := &http.Client{Timeout: 15 * time.Second}

	// 登录拿 Cookie
	loginBody, _ := json.Marshal(map[string]string{"password": password})
	loginReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/login", strings.NewReader(string(loginBody)))
	if err != nil {
		return nil, err
	}
	loginReq.Header.Set("Content-Type", "application/json")
	loginRes, err := client.Do(loginReq)
	if err != nil {
		return nil, fmt.Errorf("连接 Web UI %s: %w", base, err)
	}
	defer func() { _ = loginRes.Body.Close() }()
	if loginRes.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Web UI 登录失败（HTTP %d）；可用 TGMCP_WEBUI_PASSWORD 指定密码", loginRes.StatusCode)
	}
	var cookies []*http.Cookie
	for _, c := range loginRes.Cookies() {
		cookies = append(cookies, c)
	}

	var method, path string
	var body []byte
	switch action {
	case "status":
		method, path = http.MethodGet, "/api/wake"
	case "on", "off":
		method, path = http.MethodPost, "/api/wake/toggle"
		body, _ = json.Marshal(map[string]bool{"enabled": action == "on"})
	case "config":
		method, path = http.MethodPost, "/api/wake"
		body, _ = json.Marshal(cfg.Wake)
	case "test":
		method, path = http.MethodPost, "/api/wake/test"
		body, _ = json.Marshal(map[string]string{"text": text})
	case "pending":
		method, path = http.MethodGet, "/api/wake/pending"
	case "ack":
		method, path = http.MethodPost, "/api/wake/ack"
		body = []byte("{}")
	default:
		return nil, fmt.Errorf("未知 action %q", action)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	var v any
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		return nil, fmt.Errorf("解析响应: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return v, fmt.Errorf("Web UI 返回 HTTP %d: %v", res.StatusCode, v)
	}
	enc := json.NewEncoder(a.Stdout)
	enc.SetIndent("", "  ")
	return v, enc.Encode(v)
}

func printWakeConfig(w interface{ Write([]byte) (int, error) }, c config.Wake) {
	data, _ := json.MarshalIndent(c, "", "  ")
	fmt.Fprintf(w, "Agent 唤醒配置:\n%s\n", string(data))
}

func validateWebhookURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("--webhook 必须是完整的 http:// 或 https:// 地址")
	}
	return nil
}

func getenv(key string) string {
	return os.Getenv(key)
}
