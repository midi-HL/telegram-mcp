package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// WakeInput 是 wake 工具的输入。
type WakeInput struct {
	// Action: status / on / off / config / test / pending / ack
	Action string `json:"action" jsonschema:"status|on|off|config|test|pending|ack"`
	Mode   string `json:"mode,omitempty" jsonschema:"notify|sampling|webhook|all"`
	// Enabled 用于 config action。
	Enabled *bool `json:"enabled,omitempty" jsonschema:"enable or disable wake"`
	// IncludeOut / IncludeText 用于 config action。
	IncludeOut  *bool `json:"include_out,omitempty"`
	IncludeText *bool `json:"include_text,omitempty"`
	// WebhookURL / WebhookToken / Prompt 用于 config action。
	WebhookURL   string `json:"webhook_url,omitempty"`
	WebhookToken string `json:"webhook_token,omitempty"`
	Prompt       string `json:"prompt,omitempty"`
	// Text 用于 test action。
	Text string `json:"text,omitempty"`
}

// WakeOutput 是 wake 工具的输出。
type WakeOutput struct {
	Action  string `json:"action"`
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// registerWakeTools 注册 wake 工具。
func registerWakeTools(s *mcp.Server, deps *Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "wake",
		Description: "新消息唤醒 Agent：status 查询 / on off 开关 / config 配置 / test 测试 / pending 查看待读 / ack 清空待读",
		Annotations: &mcp.ToolAnnotations{Title: "Agent 唤醒", ReadOnlyHint: false},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in WakeInput) (*mcp.CallToolResult, WakeOutput, error) {
		return handleWake(ctx, deps, in)
	})
}

func handleWake(ctx context.Context, deps *Deps, in WakeInput) (*mcp.CallToolResult, WakeOutput, error) {
	out := WakeOutput{Action: in.Action}
	if deps.Wake == nil {
		out.Message = "唤醒引擎未初始化"
		return nil, out, nil
	}
	switch in.Action {
	case "", "status":
		out.Action = "status"
		out.OK = true
		out.Data = deps.Wake.Stats()
		out.Message = "Agent 唤醒状态"
	case "on":
		deps.Wake.SetEnabled(true)
		if deps.Config != nil {
			deps.Config.Wake.Enabled = true
		}
		out.OK = true
		out.Message = "已开启：新消息会通知 Agent 查看（不自动回复）"
	case "off":
		deps.Wake.SetEnabled(false)
		if deps.Config != nil {
			deps.Config.Wake.Enabled = false
		}
		out.OK = true
		out.Message = "已关闭"
	case "config":
		cfg := deps.Wake.Config()
		if in.Mode != "" {
			switch in.Mode {
			case "notify", "sampling", "webhook", "all":
				cfg.Mode = in.Mode
			default:
				out.Message = fmt.Sprintf("未知 mode %q（notify/sampling/webhook/all）", in.Mode)
				return nil, out, nil
			}
		}
		if in.Enabled != nil {
			cfg.Enabled = *in.Enabled
		}
		if in.IncludeOut != nil {
			cfg.IncludeOut = *in.IncludeOut
		}
		if in.IncludeText != nil {
			cfg.IncludeText = *in.IncludeText
		}
		if in.WebhookURL != "" {
			cfg.WebhookURL = in.WebhookURL
		}
		if in.WebhookToken != "" {
			cfg.WebhookToken = in.WebhookToken
		}
		if in.Prompt != "" {
			cfg.Prompt = in.Prompt
		}
		deps.Wake.UpdateConfig(cfg)
		if deps.Config != nil {
			deps.Config.Wake = cfg
		}
		out.OK = true
		out.Data = deps.Wake.Stats()
		out.Message = "唤醒配置已更新（当前进程生效）"
	case "test":
		note := deps.Wake.Test(ctx, in.Text)
		out.OK = true
		out.Data = note
		if len(note.Errors) > 0 {
			out.Message = "测试已投递，部分通道失败：" + strings.Join(note.Errors, "; ")
		} else {
			out.Message = "测试已投递到 " + strings.Join(note.Channels, ", ")
		}
	case "pending":
		out.OK = true
		out.Data = deps.Wake.Pending()
		out.Message = fmt.Sprintf("待读 %d 条", len(deps.Wake.Pending()))
	case "ack":
		n := deps.Wake.Ack()
		out.OK = true
		out.Message = fmt.Sprintf("已清空 %d 条待读", n)
	default:
		out.Message = fmt.Sprintf("未知 action %q（status/on/off/config/test/pending/ack）", in.Action)
	}
	return nil, out, nil
}
