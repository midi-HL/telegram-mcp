package autoreply

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/tg"
	"go.uber.org/zap"

	tgclient "github.com/midi-HL/telegram-mcp/internal/telegram"
)

// ReplyFunc 是生成回复的函数（由 MCP 层或外部注入）。
// 返回空字符串表示不回复。
type ReplyFunc func(ctx context.Context, chatID int64, sender string, text string, rule *Rule) (string, error)

// Event 是一条待处理的新消息。
type Event struct {
	ChatID   int64
	MsgID    int
	Sender   string
	SenderID int64
	Text     string
	IsOut    bool
}

// Engine 是自动回复引擎。
type Engine struct {
	rules   *RuleStore
	pool    *tgclient.Pool
	reply   ReplyFunc
	logger  *zap.Logger

	mu      sync.RWMutex
	enabled bool
	stop    chan struct{}
	wg      sync.WaitGroup
}

// Config 是引擎配置。
type Config struct {
	Rules  *RuleStore
	Pool   *tgclient.Pool
	Reply  ReplyFunc
	Logger *zap.Logger
}

// New 创建自动回复引擎。
func New(cfg Config) *Engine {
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Engine{
		rules:  cfg.Rules,
		pool:   cfg.Pool,
		reply:  cfg.Reply,
		logger: logger,
		stop:   make(chan struct{}),
	}
}

// SetEnabled 开关自动回复。
func (e *Engine) SetEnabled(v bool) {
	e.mu.Lock()
	e.enabled = v
	e.mu.Unlock()
}

// Enabled 返回是否开启。
func (e *Engine) Enabled() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.enabled
}

// Handle 处理一条新消息（由更新流调用）。
func (e *Engine) Handle(ctx context.Context, ev Event) {
	if !e.Enabled() {
		return
	}
	// 忽略自己发的
	if ev.IsOut {
		return
	}
	if ev.Text == "" {
		return
	}

	rules, err := e.rules.List(ctx)
	if err != nil {
		e.logger.Error("读取规则失败", zap.Error(err))
		return
	}

	for i := range rules {
		rule := &rules[i]
		if !rule.Match(ev.ChatID, ev.Text) {
			continue
		}
		e.logger.Info("规则命中",
			zap.Int64("rule_id", rule.ID),
			zap.Int64("chat_id", ev.ChatID),
			zap.Int("msg_id", ev.MsgID))

		if e.reply == nil {
			_ = e.rules.Log(ctx, &LogEntry{
				RuleID: rule.ID, ChatID: ev.ChatID, InMsgID: ev.MsgID,
				Error: "未配置回复函数",
			})
			return
		}

		outText, err := e.reply(ctx, ev.ChatID, ev.Sender, ev.Text, rule)
		if err != nil {
			e.logger.Error("生成回复失败", zap.Error(err))
			_ = e.rules.Log(ctx, &LogEntry{
				RuleID: rule.ID, ChatID: ev.ChatID, InMsgID: ev.MsgID,
				Error: err.Error(),
			})
			return
		}
		if outText == "" {
			// 不回复
			return
		}

		// 发送回复
		if err := e.send(ctx, ev.ChatID, outText); err != nil {
			e.logger.Error("发送回复失败", zap.Error(err))
			_ = e.rules.Log(ctx, &LogEntry{
				RuleID: rule.ID, ChatID: ev.ChatID, InMsgID: ev.MsgID,
				Error: err.Error(),
			})
			return
		}
		_ = e.rules.Log(ctx, &LogEntry{
			RuleID: rule.ID, ChatID: ev.ChatID, InMsgID: ev.MsgID,
			OutText: outText,
		})
		e.logger.Info("自动回复已发送",
			zap.Int64("chat_id", ev.ChatID), zap.String("text", truncate(outText, 80)))
		return // 只匹配第一条规则
	}
}

// send 发送回复消息。
func (e *Engine) send(ctx context.Context, chatID int64, text string) error {
	// 找第一个就绪的账号
	for _, name := range e.pool.Names() {
		acc, ok := e.pool.Get(name)
		if !ok || acc.API() == nil || acc.Self() == nil {
			continue
		}
		peer, err := resolvePeerForChat(ctx, acc, chatID)
		if err != nil {
			continue
		}
		_, err = acc.API().MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
			Peer:     peer,
			Message:  text,
			RandomID: time.Now().UnixNano(),
		})
		return err
	}
	return fmt.Errorf("没有可用账号")
}

// resolvePeerForChat 把 chatID 解析成 InputPeer。
func resolvePeerForChat(ctx context.Context, acc *tgclient.Account, chatID int64) (tg.InputPeerClass, error) {
	r := tgclient.NewPeerResolver(acc.API())
	return r.Resolve(ctx, itoa(chatID))
}

// SimpleReply 是一个默认的回复函数：直接回固定文本或规则里的 prompt。
func SimpleReply(ctx context.Context, chatID int64, sender, text string, rule *Rule) (string, error) {
	if rule.Prompt != "" {
		// 支持简单占位符
		out := rule.Prompt
		out = strings.ReplaceAll(out, "{sender}", sender)
		out = strings.ReplaceAll(out, "{text}", text)
		return out, nil
	}
	return "已收到你的消息", nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
