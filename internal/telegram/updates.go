package telegram

import (
	"context"

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

// RegisterMessageHandler 把新消息事件转发到 handler。
//
// 必须调用（gotd 若未设 UpdateHandler 会自动关闭更新流）。
func RegisterMessageHandler(acc *Account, handler MessageHandler, logger *zap.Logger) {
	if handler == nil {
		return
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	d := acc.Dispatcher()
	if d == nil {
		return
	}

	onNew := func(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		m, ok := u.Message.(*tg.Message)
		if !ok {
			return nil
		}
		handler(ctx, toEvent(m, e))
		return nil
	}
	onChannel := func(ctx context.Context, e tg.Entities, u *tg.UpdateNewChannelMessage) error {
		m, ok := u.Message.(*tg.Message)
		if !ok {
			return nil
		}
		handler(ctx, toEvent(m, e))
		return nil
	}

	d.OnNewMessage(onNew)
	d.OnNewChannelMessage(onChannel)
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
