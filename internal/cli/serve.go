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
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := a.resolveConfig()
	if err != nil {
		return err
	}
	if len(cfg.Accounts) == 0 {
		return fmt.Errorf("没有已配置的账号，请先运行 `tgmcp auth`")
	}

	defaultAccount := *accountName
	if defaultAccount == "" {
		defaultAccount = cfg.DefaultAccount
	}
	if cfg.Account(defaultAccount) == nil {
		return fmt.Errorf("账号 %q 不存在，已配置：%v", defaultAccount, accountNames(cfg))
	}

	// 构造账号池
	pool := tgclient.NewPool(a.Logger)
	for _, ac := range cfg.Accounts {
		acc, err := tgclient.NewAccount(ac)
		if err != nil {
			return fmt.Errorf("初始化账号 %q: %w", ac.Name, err)
		}
		pool.Add(acc)
	}

	// 自动回复引擎（延迟到 store 打开后初始化）
	var autoReplyEngine *autoreply.Engine

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

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
					// 注册新消息处理器（供自动回复）
					if autoReplyEngine != nil {
						tgclient.RegisterMessageHandler(st.acc, func(hctx context.Context, ev tgclient.MessageEvent) {
							autoReplyEngine.Handle(hctx, autoreply.Event{
								ChatID:   ev.ChatID,
								MsgID:    ev.MsgID,
								Sender:   ev.Sender,
								SenderID: ev.SenderID,
								Text:     ev.Text,
								IsOut:    ev.Out,
							})
						}, a.Logger)
					}
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

	// 等待所有账号就绪（或超时）
	readyCtx, readyCancel := context.WithTimeout(runCtx, *readyWait)
	defer readyCancel()
	allReady := true
	for _, st := range states {
		select {
		case <-st.ready:
		case <-readyCtx.Done():
			a.Logger.Warn("等待账号就绪超时，继续启动 MCP 服务",
				zap.String("account", st.acc.Name))
			allReady = false
		}
	}
	if !allReady {
		a.Logger.Warn("部分账号未就绪，工具调用可能失败")
	}

	// 打开缓存
	store, err := cache.Open(cfg.DataDir)
	if err != nil {
		a.Logger.Warn("缓存不可用，将直接走 Telegram API", zap.Error(err))
	} else {
		defer func() { _ = store.Close() }()
		a.Logger.Info("缓存已打开", zap.String("path", store.Path()))
	}

	// 信任体系
	var guard *trust.Guard
	if store != nil {
		guard = trust.New(store)
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

	deps := &tgmcp.Deps{
		Config:         cfg,
		Pool:           pool,
		Cache:          store,
		Logger:         a.Logger,
		DefaultAccount: defaultAccount,
		Guard:          guard,
		AutoReply:      autoReplyEngine,
	}

	// 启动 Web UI（独立端口）
	var webErr chan error
	if cfg.WebUI.Enabled {
		ui, err := webui.NewWithAutoReply(cfg, store, pool, autoReplyEngine, a.Logger)
		if err != nil {
			a.Logger.Warn("Web UI 初始化失败", zap.Error(err))
		} else {
			webErr = make(chan error, 1)
			go func() {
				webErr <- ui.Run(runCtx)
			}()
		}
	}

	if *useHTTP {
		httpPort := *port
		if httpPort == 0 {
			httpPort = cfg.WebUI.Port + 1 // 默认用 WebUI 端口 +1
		}
		addr := fmt.Sprintf("%s:%d", cfg.WebUI.Host, httpPort)
		a.Logger.Info("启动 MCP 服务", zap.String("transport", "http"),
			zap.String("addr", addr),
			zap.Bool("auth", *httpToken != ""))
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
