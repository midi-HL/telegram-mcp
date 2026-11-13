// Package telegram 提供账号常驻运行器，支持运行中动态激活新登录账号。
package telegram

import (
	"context"
	"sync"

	"go.uber.org/zap"
)

// Runtime 管理所有账号的长期 client.Run 循环。
//
// 启动时账号与 Web UI 登录成功后的账号共用同一套 activate 逻辑，
// 保证后者无需重启即可被 MCP / Web UI / 自动回复 / 唤醒链路使用。
type Runtime struct {
	ctx    context.Context
	pool   *Pool
	logger *zap.Logger

	activate AccountActivator

	mu      sync.Mutex
	running map[string]*runCtl
	wg      sync.WaitGroup
}

// AccountActivator 在 client.Run 内、账号授权成功后调用。
// 典型职责：SetAPI、CheckStatus、注册新消息 handler、更新默认账号等。
type AccountActivator func(ctx context.Context, acc *Account) error

type runCtl struct {
	cancel context.CancelFunc
	ready  chan struct{}
	err    error
	mu     sync.Mutex
}

// NewRuntime 创建账号运行器。
func NewRuntime(ctx context.Context, pool *Pool, activate AccountActivator, logger *zap.Logger) *Runtime {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Runtime{
		ctx:      ctx,
		pool:     pool,
		logger:   logger,
		activate: activate,
		running:  make(map[string]*runCtl),
	}
}

// Activate 把一个账号投入常驻运行循环，并返回可等待就绪的控制块。
// 同名账号已运行时，先取消旧循环再启动新循环（Web 重新登录场景）。
func (r *Runtime) Activate(acc *Account) *runCtl {
	if acc == nil {
		return nil
	}
	ctl := &runCtl{ready: make(chan struct{})}

	r.mu.Lock()
	if old := r.running[acc.Name]; old != nil && old.cancel != nil {
		old.cancel()
	}
	r.running[acc.Name] = ctl
	runCtx, cancel := context.WithCancel(r.ctx)
	ctl.cancel = cancel
	// 放入共享账号池，使 MCP / Web UI / 自动回复立即可见。
	r.pool.Add(acc)
	r.wg.Add(1)
	r.mu.Unlock()

	go func() {
		defer r.wg.Done()
		waiter := NewFloodWaiter(r.logger)
		err := waiter.Run(runCtx, func(ctx context.Context) error {
			return acc.Client().Run(ctx, func(ctx context.Context) error {
				// tg.Client 在 Run 回调内有效，先缓存给所有工具层使用。
				acc.SetAPI(acc.Client().API())

				activationErr := error(nil)
				if r.activate != nil {
					activationErr = r.activate(ctx, acc)
				}
				ctl.mu.Lock()
				ctl.err = activationErr
				ctl.mu.Unlock()
				select {
				case <-ctl.ready:
				default:
					close(ctl.ready)
				}

				// 保持 Run 回调直到服务退出；返回会关闭 Client，
				// 而 gotd 不允许对同一 Client 再次 Run。
				<-ctx.Done()
				return nil
			})
		})
		if err != nil && runCtx.Err() == nil {
			r.logger.Error("账号运行失败", zap.String("account", acc.Name), zap.Error(err))
		}
		if err != nil {
			ctl.mu.Lock()
			if ctl.err == nil {
				ctl.err = err
			}
			ctl.mu.Unlock()
		}
		select {
		case <-ctl.ready:
		default:
			close(ctl.ready)
		}
	}()
	return ctl
}

// ActivateExisting 把池里现有账号全部启动。
func (r *Runtime) ActivateExisting() []*runCtl {
	out := make([]*runCtl, 0, r.pool.Len())
	for _, name := range r.pool.Names() {
		acc, ok := r.pool.Get(name)
		if !ok || acc == nil {
			continue
		}
		r.Activate(acc)
		r.mu.Lock()
		out = append(out, r.running[name])
		r.mu.Unlock()
	}
	return out
}

// WaitReady 等待账号进入 Run 回调并完成激活（或失败）。
func (r *Runtime) WaitReady(ctx context.Context, ctl *runCtl) error {
	if ctl == nil {
		return nil
	}
	select {
	case <-ctl.ready:
	case <-ctx.Done():
		return ctx.Err()
	}
	ctl.mu.Lock()
	defer ctl.mu.Unlock()
	return ctl.err
}

// Wait 等待全部账号运行循环退出（服务关闭时调用）。
func (r *Runtime) Wait() { r.wg.Wait() }
