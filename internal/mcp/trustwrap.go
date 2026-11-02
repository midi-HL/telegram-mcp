package mcp

import (
	"context"

	"github.com/gotd/td/tg"

	"github.com/midi-HL/telegram-mcp/internal/dto"
	"github.com/midi-HL/telegram-mcp/internal/trust"
)

// wrapMessagesWithTrust 按信任级别包装消息文本。
// 管理员/自己的消息原样返回；外部内容包 <untrusted_content>。
func wrapMessagesWithTrust(ctx context.Context, deps *Deps, msgs []*tg.Message, briefs []dto.MessageBrief) []dto.MessageBrief {
	if deps.Guard == nil {
		return briefs // 未配置 Guard 时不包装
	}
	// 建立 msgID → 原始消息的映射
	byID := make(map[int]*tg.Message, len(msgs))
	for _, m := range msgs {
		byID[m.ID] = m
	}

	for i := range briefs {
		raw := byID[briefs[i].ID]
		if raw == nil {
			continue
		}
		level := deps.Guard.Classify(ctx, raw)
		briefs[i].Text = deps.Guard.WrapSender(level, briefs[i].Sender, briefs[i].Text)
		briefs[i].Trust = level.String()
	}
	return briefs
}

// TrustLevelOf 返回某条消息的信任级别字符串（用于输出）。
func TrustLevelOf(ctx context.Context, deps *Deps, msg *tg.Message) string {
	if deps.Guard == nil {
		return "admin"
	}
	return deps.Guard.Classify(ctx, msg).String()
}

// ensureTrustImports 确保 trust 包被引用。
var _ = trust.LevelAdmin
