package mcp

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/midi-HL/telegram-mcp/internal/dto"
)

// BotUseInput 是 botuse 工具的输入。
type BotUseInput struct {
	// Action: start / buttons / click / send_text / read_replies / webapp
	Action  string `json:"action" jsonschema:"start|buttons|click|send_text|read_replies|webapp"`
	Bot     string `json:"bot,omitempty" jsonschema:"bot @username"`
	MsgID   int    `json:"msg_id,omitempty" jsonschema:"message id with buttons"`
	Data    string `json:"data,omitempty" jsonschema:"callback data (base64)"`
	Index   int    `json:"index,omitempty" jsonschema:"button index (0-based) instead of data"`
	Text    string `json:"text,omitempty" jsonschema:"text to send"`
	Limit   int    `json:"limit,omitempty" jsonschema:"default 10"`
	Account string `json:"account,omitempty" jsonschema:"account name"`
}

// Button 是按钮信息。
type Button struct {
	Index int    `json:"i"`
	Text  string `json:"text"`
	Type  string `json:"type"`             // callback/url/webview/switch_inline/other
	Data  string `json:"data,omitempty"`   // base64 for callback
	URL   string `json:"url,omitempty"`
}

// BotUseOutput 是 botuse 工具的输出。
type BotUseOutput struct {
	Action   string          `json:"action"`
	Bot      string          `json:"bot,omitempty"`
	MsgID    int             `json:"msg_id,omitempty"`
	Text     string          `json:"text,omitempty"`
	Buttons  []Button        `json:"buttons,omitempty"`
	Messages []dto.MessageBrief `json:"messages,omitempty"`
	Callback *CallbackResult `json:"callback,omitempty"`
	Count    int             `json:"count,omitempty"`
	Note     string          `json:"note,omitempty"`
}

// CallbackResult 是点击回调的返回。
type CallbackResult struct {
	Message string `json:"message,omitempty"`
	URL     string `json:"url,omitempty"`
	Alert   bool   `json:"alert,omitempty"`
}

// registerBotUseTools 注册 botuse 工具。
func registerBotUseTools(s *mcp.Server, deps *Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "botuse",
		Description: "使用别人的 Bot。action: start 启动 / buttons 读按钮 / click 点按钮 / send_text 发文字 / read_replies 读回复 / webapp 打开 WebApp",
		Annotations: &mcp.ToolAnnotations{Title: "第三方 Bot", ReadOnlyHint: false},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in BotUseInput) (*mcp.CallToolResult, BotUseOutput, error) {
		return handleBotUse(ctx, deps, in)
	})
}

func handleBotUse(ctx context.Context, deps *Deps, in BotUseInput) (*mcp.CallToolResult, BotUseOutput, error) {
	out := BotUseOutput{Action: in.Action, Bot: in.Bot}

	switch in.Action {
	case "start":
		return handleBotStart(ctx, deps, in, out)
	case "buttons":
		return handleBotButtons(ctx, deps, in, out)
	case "click":
		return handleBotClick(ctx, deps, in, out)
	case "send_text":
		return handleBotSendText(ctx, deps, in, out)
	case "read_replies":
		return handleBotReadReplies(ctx, deps, in, out)
	case "webapp":
		return handleBotWebApp(ctx, deps, in, out)
	default:
		out.Note = fmt.Sprintf("未知 action %q", in.Action)
		return nil, out, nil
	}
}

// handleBotStart 启动 Bot（/start）。
func handleBotStart(ctx context.Context, deps *Deps, in BotUseInput, out BotUseOutput) (*mcp.CallToolResult, BotUseOutput, error) {
	if in.Bot == "" {
		out.Note = "start 需要 bot 参数"
		return nil, out, nil
	}
	acc, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	peer, err := resolver.Resolve(ctx, in.Bot)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	up, ok := peer.(*tg.InputPeerUser)
	if !ok {
		out.Note = "目标不是用户"
		return nil, out, nil
	}

	// startBot 要求非空 start_param；空参数时直接发 /start 消息
	var res tg.UpdatesClass
	if in.Text != "" {
		res, err = acc.API().MessagesStartBot(ctx, &tg.MessagesStartBotRequest{
			Bot:        &tg.InputUser{UserID: up.UserID, AccessHash: up.AccessHash},
			Peer:       peer,
			RandomID:   randomID(),
			StartParam: in.Text,
		})
	} else {
		res, err = acc.API().MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
			Peer:     peer,
			Message:  "/start",
			RandomID: randomID(),
		})
	}
	if err != nil {
		out.Note = fmt.Sprintf("启动 Bot 失败: %v", err)
		return nil, out, nil
	}
	if id, _ := extractSentMessageID(res); id != 0 {
		out.MsgID = id
	}
	out.Note = "Bot 已启动，用 read_replies 读回复"
	return nil, out, nil
}

// handleBotButtons 读消息里的按钮。
func handleBotButtons(ctx context.Context, deps *Deps, in BotUseInput, out BotUseOutput) (*mcp.CallToolResult, BotUseOutput, error) {
	if in.Bot == "" || in.MsgID == 0 {
		out.Note = "buttons 需要 bot 和 msg_id"
		return nil, out, nil
	}
	acc, _, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}

	msgs, err := acc.API().MessagesGetMessages(ctx, []tg.InputMessageClass{
		&tg.InputMessageID{ID: in.MsgID},
	})
	if err != nil {
		out.Note = fmt.Sprintf("获取消息失败: %v", err)
		return nil, out, nil
	}
	msgList, _ := extractMessages(msgs)
	if len(msgList) == 0 {
		out.Note = "消息不存在"
		return nil, out, nil
	}

	markup, ok := msgList[0].GetReplyMarkup()
	if !ok {
		out.Note = "该消息没有按钮"
		return nil, out, nil
	}

	out.Buttons = extractButtons(markup)
	out.Count = len(out.Buttons)
	if out.Count == 0 {
		out.Note = "消息里没有可点击的按钮"
	} else {
		out.Note = fmt.Sprintf("共 %d 个按钮", out.Count)
	}
	return nil, out, nil
}

// extractButtons 从 reply_markup 提取按钮列表。
func extractButtons(markup tg.ReplyMarkupClass) []Button {
	var buttons []Button
	idx := 0

	switch m := markup.(type) {
	case *tg.ReplyInlineMarkup:
		for _, row := range m.Rows {
			for _, btn := range row.Buttons {
				b := Button{Index: idx, Text: btn.Text}
				switch t := btn.Type.(type) {
				case *tg.InlineButtonTypeCallback:
					b.Type = "callback"
					b.Data = base64.StdEncoding.EncodeToString(t.Data)
				case *tg.InlineButtonTypeURL:
					b.Type = "url"
					b.URL = t.URL
				case *tg.InlineButtonTypeWebView:
					b.Type = "webview"
				case *tg.InlineButtonTypeSwitchInline:
					b.Type = "switch_inline"
				case *tg.InlineButtonTypeCopy:
					b.Type = "copy"
				default:
					b.Type = "other"
				}
				buttons = append(buttons, b)
				idx++
			}
		}
	case *tg.ReplyKeyboardMarkup:
		for _, row := range m.Rows {
			for _, btn := range row.Buttons {
				buttons = append(buttons, Button{
					Index: idx,
					Text:  btn.Text,
					Type:  "reply",
				})
				idx++
			}
		}
	}
	return buttons
}

// handleBotClick 点击按钮。
func handleBotClick(ctx context.Context, deps *Deps, in BotUseInput, out BotUseOutput) (*mcp.CallToolResult, BotUseOutput, error) {
	if in.Bot == "" || in.MsgID == 0 {
		out.Note = "click 需要 bot 和 msg_id"
		return nil, out, nil
	}
	acc, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	peer, err := resolver.Resolve(ctx, in.Bot)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}

	// 取消息拿按钮
	msgs, err := acc.API().MessagesGetMessages(ctx, []tg.InputMessageClass{
		&tg.InputMessageID{ID: in.MsgID},
	})
	if err != nil {
		out.Note = fmt.Sprintf("获取消息失败: %v", err)
		return nil, out, nil
	}
	msgList, _ := extractMessages(msgs)
	if len(msgList) == 0 {
		out.Note = "消息不存在"
		return nil, out, nil
	}
	markup, ok := msgList[0].GetReplyMarkup()
	if !ok {
		out.Note = "该消息没有按钮"
		return nil, out, nil
	}

	buttons := extractButtons(markup)

	// 解析目标按钮
	var data []byte
	if in.Data != "" {
		decoded, err := base64.StdEncoding.DecodeString(in.Data)
		if err != nil {
			out.Note = fmt.Sprintf("data 不是有效的 base64: %v", err)
			return nil, out, nil
		}
		data = decoded
	} else {
		// 用 index
		if in.Index < 0 || in.Index >= len(buttons) {
			out.Note = fmt.Sprintf("index %d 越界（共 %d 个按钮）", in.Index, len(buttons))
			return nil, out, nil
		}
		btn := buttons[in.Index]
		if btn.Type != "callback" {
			out.Note = fmt.Sprintf("按钮 #%d 类型是 %s，不能点击回调（URL 类型请自行访问）", in.Index, btn.Type)
			out.Buttons = buttons
			return nil, out, nil
		}
		decoded, _ := base64.StdEncoding.DecodeString(btn.Data)
		data = decoded
	}

	// 点击
	res, err := acc.API().MessagesGetBotCallbackAnswer(ctx, &tg.MessagesGetBotCallbackAnswerRequest{
		Peer:  peer,
		MsgID: in.MsgID,
		Data:  data,
	})
	if err != nil {
		out.Note = fmt.Sprintf("点击失败: %v", err)
		return nil, out, nil
	}
	cb := &CallbackResult{}
	if msg, ok := res.GetMessage(); ok {
		cb.Message = msg
	}
	if u, ok := res.GetURL(); ok {
		cb.URL = u
	}
	cb.Alert = res.Alert
	out.Callback = cb
	out.MsgID = in.MsgID
	out.Note = "已点击按钮"
	return nil, out, nil
}

// handleBotSendText 给 Bot 发文字。
func handleBotSendText(ctx context.Context, deps *Deps, in BotUseInput, out BotUseOutput) (*mcp.CallToolResult, BotUseOutput, error) {
	if in.Bot == "" || in.Text == "" {
		out.Note = "send_text 需要 bot 和 text"
		return nil, out, nil
	}
	acc, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	peer, err := resolver.Resolve(ctx, in.Bot)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	res, err := acc.API().MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     peer,
		Message:  in.Text,
		RandomID: randomID(),
	})
	if err != nil {
		out.Note = fmt.Sprintf("发送失败: %v", err)
		return nil, out, nil
	}
	if id, _ := extractSentMessageID(res); id != 0 {
		out.MsgID = id
	}
	out.Note = "已发送，用 read_replies 读回复"
	return nil, out, nil
}

// handleBotReadReplies 读 Bot 的回复。
func handleBotReadReplies(ctx context.Context, deps *Deps, in BotUseInput, out BotUseOutput) (*mcp.CallToolResult, BotUseOutput, error) {
	if in.Bot == "" {
		out.Note = "read_replies 需要 bot 参数"
		return nil, out, nil
	}
	acc, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	peer, err := resolver.Resolve(ctx, in.Bot)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}

	res, err := acc.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:  peer,
		Limit: limit,
	})
	if err != nil {
		out.Note = fmt.Sprintf("读取失败: %v", err)
		return nil, out, nil
	}
	msgs, users, chats := extractMessagesFull(res)
	out.Messages = projectMessagesWithEntities(msgs, users, chats, nil)
	out.Messages = wrapMessagesWithTrust(ctx, deps, msgs, out.Messages)
	out.Count = len(out.Messages)
	out.Note = fmt.Sprintf("共 %d 条消息", out.Count)
	return nil, out, nil
}

// handleBotWebApp 打开 Bot 的 Web App。
func handleBotWebApp(ctx context.Context, deps *Deps, in BotUseInput, out BotUseOutput) (*mcp.CallToolResult, BotUseOutput, error) {
	if in.Bot == "" {
		out.Note = "webapp 需要 bot 参数"
		return nil, out, nil
	}
	acc, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	peer, err := resolver.Resolve(ctx, in.Bot)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}

	up, ok := peer.(*tg.InputPeerUser)
	if !ok {
		out.Note = "webapp 目标必须是 Bot（用户类型）"
		return nil, out, nil
	}
	res, err := acc.API().MessagesRequestWebView(ctx, &tg.MessagesRequestWebViewRequest{
		Peer:     peer,
		Bot:      &tg.InputUser{UserID: up.UserID, AccessHash: up.AccessHash},
		Platform: "android",
	})
	if err != nil {
		out.Note = fmt.Sprintf("打开 Web App 失败: %v（Web App 需要浏览器环境，无法在 MCP 中完整执行）", err)
		return nil, out, nil
	}
	out.Text = res.URL
	out.Note = fmt.Sprintf("Web App 已打开，URL: %s（需要浏览器交互，无法自动完成）", res.URL)
	return nil, out, nil
}
