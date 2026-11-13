package cli

import (
	"context"
	"flag"
	"fmt"
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
		useHTTP     = fs.Bool("http", false, "使用 Streamable HTTP 传输")
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

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 先打开缓存 + 启动 Web UI，再连账号。
	store, err := cache.Open(cfg.DataDir)
	if err != nil {
		a.Logger.Warn("缓存不可用，将直接走 Telegram API", zap.Error(err))
	} else {
		defer func() { _ = store.Close() }()
		a.Logger.Info("缓存已打开", zap.String("path", store.Path()))
	}

	var guard *trust.Guard
	var adminChecker interface {
		IsAdmin(ctx context.Context, userID int64) (bool, error)
	}
	if store != nil {
		guard = trust.New(store)
		adminChecker = store
	}

	var autoReplyEngine *autoreply.Engine
	if store != nil {
		rs := autoreply.NewRuleStore(store)
		autoReplyEngine = autoreply.New(autoreply.Config{
			Rules:  rs,
			Pool:   pool,
			Reply:  autoreply.SimpleReply,
			Logger: a.Logger,
		})
	}

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

	// 统一账号激活逻辑：启动时已有账号与 Web 登录新增账号共用。
	activate := tgclient.AccountActivator(func(actx context.Context, acc *tgclient.Account) error {
		status, err := tgclient.CheckStatus(actx, acc)
		if err != nil {
			return err
		}
		if !status.Authorized {
			return fmt.Errorf("账号 %q 未授权", acc.Name)
		}

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
				Account:  acc.Name,
				ChatID:   ev.ChatID,
				MsgID:    ev.MsgID,
				SenderID: ev.SenderID,
				Sender:   ev.Sender,
				Text:     ev.Text,
				Out:      ev.Out,
			})
		})
		// gotd 的 OnNewMessage 是覆盖式注册，自动回复与唤醒必须一次注册。
		tgclient.RegisterMessageHandlers(acc, a.Logger, handlers...)

		// 替换同名账号后旧 resolver 持有旧 tg.Client，必须失效。
		deps.InvalidateResolver(acc.Name)
		// 零账号启动 / 默认账号不存在时，把第一个就绪账号设为默认。
		if _, ok := pool.Get(deps.DefaultAccount); !ok || deps.DefaultAccount == "" {
			deps.DefaultAccount = acc.Name
			cfg.DefaultAccount = acc.Name
		}

		a.Logger.Info("账号已就绪，可立即使用",
			zap.String("account", acc.Name),
			zap.Int64("user_id", status.UserID),
			zap.String("username", status.Username))
		return nil
	})

	runtime := tgclient.NewRuntime(runCtx, pool, activate, a.Logger)
	initial := runtime.ActivateExisting()

	// Web 登录成功回调：先构造并排队常驻 Account，但延迟启动其 client.Run，
	// 等临时登录连接关闭后再启动，避免两个连接共用同一 session 文件。
	loginActivator := webui.LoginActivator(func(acc *tgclient.Account) error {
		saved := cfg.Account(acc.Name)
		if saved == nil {
			return fmt.Errorf("账号 %q 配置不存在", acc.Name)
		}
		persistent, err := tgclient.NewAccount(*saved)
		if err != nil {
			return err
		}
		go func() {
			// 临时登录 Run 返回后 session.json 已落盘；留一点时间给
			// gotd 关闭连接/释放文件句柄，再建立服务级常驻连接。
			select {
			case <-time.After(200 * time.Millisecond):
			case <-runCtx.Done():
				return
			}
			ctl := runtime.Activate(persistent)
			readyCtx, cancel := context.WithTimeout(runCtx, 30*time.Second)
			defer cancel()
			if err := runtime.WaitReady(readyCtx, ctl); err != nil && readyCtx.Err() == nil {
				a.Logger.Error("新登录账号激活失败",
					zap.String("account", acc.Name), zap.Error(err))
			}
		}()
		return nil
	})

	// Web UI 立即启动，不等待任何账号。
	if cfg.WebUI.Enabled {
		ui, err := webui.NewWithAutoReply(cfg, store, pool, autoReplyEngine, wakeEngine, loginActivator, a.Logger)
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

	// 后台等待启动账号就绪（不阻塞 Web UI / MCP 传输）。
	go func() {
		if len(initial) == 0 {
			return
		}
		readyCtx, readyCancel := context.WithTimeout(runCtx, *readyWait)
		defer readyCancel()
		for _, ctl := range initial {
			if err := runtime.WaitReady(readyCtx, ctl); err != nil && readyCtx.Err() == nil {
				a.Logger.Warn("账号就绪检查失败（服务继续运行，可在 Web UI 中登录）", zap.Error(err))
			}
		}
		a.Logger.Info("启动账号已检查完毕")
	}()

	if *useHTTP {
		httpPort := *port
		if httpPort == 0 {
			httpPort = cfg.WebUI.Port + 1
		}
		addr := fmt.Sprintf("%s:%d", cfg.WebUI.Host, httpPort)
		a.Logger.Info("启动 MCP 服务", zap.String("transport", "http"),
			zap.String("addr", addr),
			zap.Bool("auth", *httpToken != ""),
			zap.Bool("wake", cfg.Wake.Enabled))
		h := tgmcp.NewHTTPServer(deps, *httpToken)
		err = h.RunHTTP(ctx, addr, a.Logger)
		cancel()
		runtime.Wait()
		return err
	}

	a.Logger.Info("启动 MCP 服务", zap.String("transport", "stdio"),
		zap.String("default_account", deps.DefaultAccount))

	err = tgmcp.RunStdio(ctx, deps)
	cancel()
	runtime.Wait()
	return err
}

func accountNames(cfg *config.Config) []string {
	out := make([]string, 0, len(cfg.Accounts))
	for _, a := range cfg.Accounts {
		out = append(out, a.Name)
	}
	return out
}
