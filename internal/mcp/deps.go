package mcp

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/midi-HL/telegram-mcp/internal/autoreply"
	"github.com/midi-HL/telegram-mcp/internal/cache"
	"github.com/midi-HL/telegram-mcp/internal/config"
	tgclient "github.com/midi-HL/telegram-mcp/internal/telegram"
	"github.com/midi-HL/telegram-mcp/internal/trust"
	"github.com/midi-HL/telegram-mcp/internal/wake"
)

// Deps 是工具共享的依赖。
type Deps struct {
	Config         *config.Config
	Pool           *tgclient.Pool
	Cache          *cache.Store
	Logger         *zap.Logger
	DefaultAccount string
	// Guard 负责信任分级与内容包装（可为 nil，此时全部按管理员处理）
	Guard *trust.Guard
	// AutoReply 是自动回复引擎（可为 nil）
	AutoReply *autoreply.Engine
	// Wake 是新消息唤醒 Agent 引擎（可为 nil）
	Wake *wake.Engine

	// 每个账号一个 resolver，延迟初始化
	resolverMu sync.Mutex
	resolvers  map[string]*tgclient.PeerResolver
}

// Resolver 返回指定账号的 peer 解析器。
func (d *Deps) Resolver(account string) (*tgclient.PeerResolver, error) {
	acc, ok := d.Pool.Get(account)
	if !ok {
		return nil, fmt.Errorf("账号 %q 不存在", account)
	}
	if acc.Self() == nil {
		return nil, fmt.Errorf("账号 %q 尚未就绪", account)
	}

	d.resolverMu.Lock()
	defer d.resolverMu.Unlock()
	if d.resolvers == nil {
		d.resolvers = make(map[string]*tgclient.PeerResolver)
	}
	if r, ok := d.resolvers[account]; ok {
		return r, nil
	}

	// resolver 需要 tg.Client，只能在 Run 上下文内获取。
	// 这里用账号持有的 api 引用。
	api := acc.API()
	if api == nil {
		return nil, fmt.Errorf("账号 %q 的 API 客户端不可用", account)
	}
	r := tgclient.NewPeerResolver(api)
	d.resolvers[account] = r
	return r, nil
}

// Account 返回账号，未指定时用默认账号。
func (d *Deps) Account(name string) (*tgclient.Account, string, error) {
	if name == "" {
		name = d.DefaultAccount
	}
	acc, ok := d.Pool.Get(name)
	if !ok {
		return nil, name, fmt.Errorf("账号 %q 不存在，可用：%v", name, d.Pool.Names())
	}
	if acc.Self() == nil {
		return nil, name, fmt.Errorf("账号 %q 尚未就绪（可能未登录或连接中）", name)
	}
	return acc, name, nil
}

// InvalidateResolver 丢弃某个账号的 peer 解析缓存。
// Web 登录替换同名 Account 后必须调用，否则 resolver 仍持有旧 tg.Client。
func (d *Deps) InvalidateResolver(account string) {
	d.resolverMu.Lock()
	delete(d.resolvers, account)
	d.resolverMu.Unlock()
}

// withAccount 是工具里获取账号的便捷封装。
func (d *Deps) withAccount(ctx context.Context, name string) (*tgclient.Account, *tgclient.PeerResolver, string, error) {
	acc, resolved, err := d.Account(name)
	if err != nil {
		return nil, nil, resolved, err
	}
	r, err := d.Resolver(resolved)
	if err != nil {
		return nil, nil, resolved, err
	}
	return acc, r, resolved, nil
}
