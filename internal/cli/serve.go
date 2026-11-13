package cli

import (
	"context"
	"flag"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/midi-HL/telegram-mcp/internal/autoreply"
	"github.com/midi-HL/telegram-mcp/internal/cache"
	"github.com/midi-HL/telegram-mcp/internal/config"
	tgmcp "github.com/midi-HL/telegram-mcp/internal/mcp"
	tgclient "github.com/midi-HL/telegram-mcp/internal/telegram"
	"github.com/midi-HL/telegram-mcp/internal/trust"
	"github.com/midi-HL/telegram-mcp/internal/wake"
	"github.com/midi-HL/telegram-mcp/internal/webui"
)

// runServe 启动 MCP 服务。
func (a *App) runServe(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	a.commonFlags(fs)

	var (
		accountName = fs.String("account", "", "默认账号名（留空用配置里的 default_account）")
		useHTTP     = fs.Bool("http", false, "使用 Streamable HTTP 传输（阶段 3 实现）")
		port        = fs.Int("port", 0, "HTTP 端口（--http 时生效）")
		httpToken   = fs.String("token", "", "HTTP Bearer token（留空则不鉴权，仅限本机）")
		readyWait   = fs.Duration("ready-timeout", 60*time.Second, "等待账号就绪的超时时间")
		wakeOn      = fs.Bool("wake", false, "开启新消息唤醒 Agent（覆盖配置里的 wake.enabled）")
		wakeOff     = fs.Bool("no-wake", false, "关闭新消息唤醒 Agent")
		wakeMode    = fs.String("wake-mode", "", "唤醒模式：notify / sampling / webhook / all")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *wakeOn && *wakeOff {
		return fmt.Errorf("--wake 与 --no-wake 不能同时使用")
	}

	cfg, err := a.resolveConfig()
	if err != nil {
		return err
	}
	// 命令行临时覆盖（不写入 config.json）
	if *wakeOn {
		cfg.Wake.Enabled = true
	}
	if *wakeOff {
		cfg.Wake.Enabled = false
	}
	if *wakeMode != "" {
		cfg.Wake.Mode = *wakeMode
	}
	config.NormalizeWake(&cfg.Wake)

	// ★ 不因"没有账号"而拒绝启动：Web UI 必须先起来，让用户在浏览器里
	//   填 API 凭据 + 扫码/手机号登录。账号就绪与否不影响 Web UI。
	defaultAccount := *accountName
	if defaultAccount == "" {
		defaultAccount = cfg.DefaultAccount
	}
	if defaultAccount == "" {
		defaultAccount = "main"
	}

	// 构造账号池（配置里没有账号时是空池，登录成功后动态加入）
	pool := tgclient.NewPool(a.Logger)
	for _, ac := range cfg.Accounts {
		acc, err := tgclient.NewAccount(ac)
		if err != nil {
			// 单个账号配置错误不应阻止整个服务启动
			a.Logger.Warn("账号初始化失败，已跳过",
				zap.String("account", ac.Name), zap.Error(err))
			continue
		}
		pool.Add(acc)
	}
	if pool.Len() == 0 {
		a.Logger.Info("当前没有可用账号，请在 Web UI 中完成登录",
			zap.String("webui", fmt.Sprintf("http://%s:%d/ui", cfg.WebUI.Host, cfg.WebUI.Port)))
	}

	// 自动回复引擎（延迟到 store 打开后初始化）
	var autoReplyEngine *autoreply.Engine

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// ★ 先打开缓存 + 启动 Web UI，再连账号。
	//   这样即使账号未登录/连接失败，浏览器也能访问 UI 完成登录。
	store, err := cache.Open(cfg.DataDir)
	if err != nil {
		a.Logger.Warn("缓存不可用，将直接走 Telegram API", zap.Error(err))
	} else {
		defer func() { _ = store.Close() }()
		a.Logger.Info("缓存已打开", zap.String("path", store.Path()))
	}

	// 信任体系
	var guard *trust.Guard
	var adminChecker interface {
		IsAdmin(ctx context.Context, userID int64) (bool, error)
	}
	if store != nil {
		guard = trust.New(store)
		adminChecker = store
	}

	// 自动回复引擎
	if store != nil {
		rs := autoreply.NewRuleStore(store)
		autoReplyEngine = autoreply.New(autoreply.Config{
			Rules:  rs,
			Pool:   pool,
			Reply:  autoreply.SimpleReply,
			Logger: a.Logger,
		})
	}

	// 新消息唤醒引擎：在 MCP server 创建前注入 provider，HTTP/stdio 都能主动通知。
	wakeEngine := wake.New(wake.Options{
		Config:    cfg.Wake,
		Logger:    a.Logger,
		Admins:    adminChecker,
		Server:    tgmcp.CurrentServer,
		Transport: map[bool]string{true: "http", false: "stdio"}[*useHTTP],
	})

	deps := &tgmcp.Deps{
		Config:         cfg,
		Pool:           pool,
		Cache:          store,
		Logger:         a.Logger,
		DefaultAccount: defaultAccount,
		Guard:          guard,
		AutoReply:      autoReplyEngine,
		Wake:           wakeEngine,
	}

	// ★ Web UI 立即启动（独立端口），不等待任何账号
	if cfg.WebUI.Enabled {
		var ui *webui.Server
		ui, err = webui.NewWithAutoReply(cfg, store, pool, autoReplyEngine, wakeEngine, a.Logger)
		if err != nil {
			a.Logger.Error("Web UI 初始化失败", zap.Error(err))
		} else {
			go func() {
				if err := ui.Run(runCtx); err != nil && runCtx.Err() == nil {
					a.Logger.Error("Web UI 运行失败", zap.Error(err))
				}
			}()
			a.Logger.Info("Web UI 已就绪，可立即在浏览器中登录",
				zap.String("url", fmt.Sprintf("http://%s:%d/ui", cfg.WebUI.Host, cfg.WebUI.Port)))
		}
	}

	// 每个账号就绪后关闭自己的 ready channel
	type readyState struct {
		acc   *tgclient.Account
		ready chan struct{}
		err   error
		mu    sync.Mutex
	}
	states := make([]*readyState, 0, pool.Len())
	for _, name := range pool.Names() {
		acc, _ := pool.Get(name)
		if acc == nil {
			continue
		}
		states = append(states, &readyState{acc: acc, ready: make(chan struct{})})
	}

	var wg sync.WaitGroup
	for _, st := range states {
		wg.Add(1)
		go func(st *readyState) {
			defer wg.Done()
			waiter := tgclient.NewFloodWaiter(a.Logger)
			err := waiter.Run(runCtx, func(ctx context.Context) error {
				return st.acc.Client().Run(ctx, func(ctx context.Context) error {
					// 缓存 tg.Client，供 MCP 工具层使用
					st.acc.SetAPI(st.acc.Client().API())
					status, err := tgclient.CheckStatus(ctx, st.acc)
					st.mu.Lock()
					if err != nil {
						st.err = err
					}
					st.mu.Unlock()
					// 无论成功失败都标记就绪，避免 MCP 启动被永久阻塞
					close(st.ready)

					if err != nil {
						a.Logger.Warn("账号状态检查失败",
							zap.String("account", st.acc.Name), zap.Error(err))
						<-ctx.Done()
						return nil
					}
					if !status.Authorized {
						a.Logger.Warn("账号未授权，请先运行 tgmcp auth",
							zap.String("account", st.acc.Name))
						<-ctx.Done()
						return nil
					}
					a.Logger.Info("账号已就绪",
						zap.String("account", st.acc.Name),
						zap.Int64("user_id", status.UserID),
						zap.String("username", status.Username))
					// ★ 自动回复 + Agent 唤醒共用同一个 dispatcher handler。
					//   gotd 的 OnNewMessage 是覆盖式注册，分两次注册会让其中一个静默失效。
					var handlers []tgclient.MessageHandler
					if autoReplyEngine != nil {
						handlers = append(handlers, func(hctx context.Context, ev tgclient.MessageEvent) {
							autoReplyEngine.Handle(hctx, autoreply.Event{
								ChatID:   ev.ChatID,
								MsgID:    ev.MsgID,
								Sender:   ev.Sender,
								SenderID: ev.SenderID,
								Text:     ev.Text,
								IsOut:    ev.Out,
							})
						})
					}
					handlers = append(handlers, func(hctx context.Context, ev tgclient.MessageEvent) {
						wakeEngine.Handle(hctx, wake.Event{
							Account:  st.acc.Name,
							ChatID:   ev.ChatID,
							MsgID:    ev.MsgID,
							SenderID: ev.SenderID,
							Sender:   ev.Sender,
							Text:     ev.Text,
							Out:      ev.Out,
						})
					})
					tgclient.RegisterMessageHandlers(st.acc, a.Logger, handlers...)
					<-ctx.Done()
					return nil
				})
			})
			if err != nil && runCtx.Err() == nil {
				a.Logger.Error("账号运行失败",
					zap.String("account", st.acc.Name), zap.Error(err))
			}
		}(st)
	}

	// 后台等待账号就绪（不阻塞 Web UI / MCP 传输）
	go func() {
		if len(states) == 0 {
			return
		}
		readyCtx, readyCancel := context.WithTimeout(runCtx, *readyWait)
		defer readyCancel()
		for _, st := range states {
			select {
			case <-st.ready:
			case <-readyCtx.Done():
				a.Logger.Warn("账号就绪超时（服务继续运行，可在 Web UI 中登录）",
					zap.String("account", st.acc.Name))
				return
			}
		}
		a.Logger.Info("全部账号已就绪")
	}()

	if *useHTTP {
		httpPort := *port
		if httpPort == 0 {
			httpPort = cfg.WebUI.Port + 1 // 默认用 WebUI 端口 +1
		}
		addr := fmt.Sprintf("%s:%d", cfg.WebUI.Host, httpPort)
		a.Logger.Info("启动 MCP 服务", zap.String("transport", "http"),
			zap.String("addr", addr),
			zap.Bool("auth", *httpToken != ""),
			zap.Bool("wake", cfg.Wake.Enabled))
		h := tgmcp.NewHTTPServer(deps, *httpToken)
		err = h.RunHTTP(ctx, addr, a.Logger)
		cancel()
		wg.Wait()
		return err
	}

	a.Logger.Info("启动 MCP 服务", zap.String("transport", "stdio"),
		zap.String("default_account", defaultAccount))

	err = tgmcp.RunStdio(ctx, deps)
	cancel()
	wg.Wait()
	return err
}

func accountNames(cfg *config.Config) []string {
	out := make([]string, 0, len(cfg.Accounts))
	for _, a := range cfg.Accounts {
		out = append(out, a.Name)
	}
	return out
}
