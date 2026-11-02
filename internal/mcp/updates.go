package mcp

import (
	"github.com/gotd/td/tg"
)

// extractSentMessageID 从各种 Updates 返回结构中提取刚发送消息的 ID。
//
// Telegram 发送消息可能返回三种结构：
//   - Updates / UpdatesCombined：含 UpdateNewMessage 或 UpdateNewChannelMessage
//   - UpdateShortSentMessage：只含消息 ID（常见于私聊）
//   - UpdateShort：含一个 update
func extractSentMessageID(res tg.UpdatesClass) (int, *tg.Message) {
	switch v := res.(type) {
	case *tg.Updates:
		for _, u := range v.Updates {
			if id, msg := messageFromUpdate(u); id != 0 {
				return id, msg
			}
		}
	case *tg.UpdatesCombined:
		for _, u := range v.Updates {
			if id, msg := messageFromUpdate(u); id != 0 {
				return id, msg
			}
		}
	case *tg.UpdateShortSentMessage:
		return v.ID, nil
	case *tg.UpdateShort:
		if id, msg := messageFromUpdate(v.Update); id != 0 {
			return id, msg
		}
	}
	return 0, nil
}

// messageFromUpdate 从单个 update 中提取消息。
func messageFromUpdate(u tg.UpdateClass) (int, *tg.Message) {
	switch v := u.(type) {
	case *tg.UpdateNewMessage:
		if m, ok := v.Message.(*tg.Message); ok {
			return m.ID, m
		}
	case *tg.UpdateNewChannelMessage:
		if m, ok := v.Message.(*tg.Message); ok {
			return m.ID, m
		}
	case *tg.UpdateMessageID:
		return v.ID, nil
	case *tg.UpdateEditMessage:
		if m, ok := v.Message.(*tg.Message); ok {
			return m.ID, m
		}
	case *tg.UpdateEditChannelMessage:
		if m, ok := v.Message.(*tg.Message); ok {
			return m.ID, m
		}
	}
	return 0, nil
}
