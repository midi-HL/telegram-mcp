package telegram

import (
	"context"
	"sync"

	"github.com/gotd/td/tg"
	"go.uber.org/zap"
)

// MessageEvent 是一条新消息事件。
type MessageEvent struct {
	ChatID   int64
	MsgID    int
	SenderID int64
	Sender   string
	Text     string
	Out      bool
}

// MessageHandler 处理新消息事件。
type MessageHandler func(ctx context.Context, ev MessageEvent)

// HandlerSet 把多个新消息处理器合并为一次 dispatcher 注册。
//
// ★ gotd 的 UpdateDispatcher.OnNewMessage 是「按类型 ID 覆盖」而非追加，
//
//	自动回复与 Agent 唤醒必须共用同一个 handler，否则后注册者会静默覆盖前者。
type HandlerSet struct {
	mu       sync.RWMutex
	handlers []MessageHandler
}

// NewHandlerSet 创建处理器集合。
func NewHandlerSet(handlers ...MessageHandler) *HandlerSet {
	return &HandlerSet{handlers: append([]MessageHandler(nil), handlers...)}
}

// Add 追加一个处理器（可动态增删，线程安全）。
func (h *HandlerSet) Add(handler MessageHandler) {
	if handler == nil {
		return
	}
	h.mu.Lock()
	h.handlers = append(h.handlers, handler)
	h.mu.Unlock()
}

// Handle 把事件分发给全部处理器；单个处理器 panic 不影响其他处理器。
func (h *HandlerSet) Handle(ctx context.Context, ev MessageEvent) {
	h.mu.RLock()
	handlers := append([]MessageHandler(nil), h.handlers...)
	h.mu.RUnlock()
	for _, fn := range handlers {
		func() {
			defer func() { _ = recover() }()
			fn(ctx, ev)
		}()
	}
}

// RegisterMessageHandler 把新消息事件转发到 handler。
//
// 必须调用（gotd 若未设 UpdateHandler 会自动关闭更新流）。
func RegisterMessageHandler(acc *Account, handler MessageHandler, logger *zap.Logger) {
	RegisterMessageHandlers(acc, logger, handler)
}

// RegisterMessageHandlers 一次性注册多个处理器（推荐）。
func RegisterMessageHandlers(acc *Account, logger *zap.Logger, handlers ...MessageHandler) *HandlerSet {
	if logger == nil {
		logger = zap.NewNop()
	}
	d := acc.Dispatcher()
	if d == nil {
		return nil
	}
	set := NewHandlerSet(handlers...)

	onNew := func(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		m, ok := u.Message.(*tg.Message)
		if !ok {
			return nil
		}
		set.Handle(ctx, toEvent(m, e))
		return nil
	}
	onChannel := func(ctx context.Context, e tg.Entities, u *tg.UpdateNewChannelMessage) error {
		m, ok := u.Message.(*tg.Message)
		if !ok {
			return nil
		}
		set.Handle(ctx, toEvent(m, e))
		return nil
	}

	d.OnNewMessage(onNew)
	d.OnNewChannelMessage(onChannel)
	return set
}

// toEvent 把 tg.Message 转成 MessageEvent。
func toEvent(m *tg.Message, entities tg.Entities) MessageEvent {
	ev := MessageEvent{
		MsgID: m.ID,
		Out:   m.Out,
		Text:  m.Message,
	}
	// 会话 ID
	if m.PeerID != nil {
		ev.ChatID = PeerIDFromClass(m.PeerID)
	}
	// 发送者
	if m.FromID != nil {
		ev.SenderID = PeerIDFromClass(m.FromID)
		if u, ok := m.FromID.(*tg.PeerUser); ok {
			if user, exists := entities.Users[u.UserID]; exists {
				ev.Sender = user.FirstName
				if user.LastName != "" {
					if ev.Sender != "" {
						ev.Sender += " "
					}
					ev.Sender += user.LastName
				}
			}
		}
	}
	return ev
}
