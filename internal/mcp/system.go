package mcp

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

// SystemInput 是 system 工具的输入。
type SystemInput struct {
	// Action: status / accounts / doctor
	Action  string `json:"action" jsonschema:"status|accounts|doctor"`
	Account string `json:"account,omitempty" jsonschema:"account name"`
}

// SystemOutput 是 system 工具的输出。
type SystemOutput struct {
	Action  string `json:"action"`
	Version string `json:"version"`
	Data    any    `json:"data,omitempty"`
	Message string `json:"message,omitempty"`
}

// registerSystemTools 注册 system 工具。
func registerSystemTools(s *mcp.Server, deps *Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "system",
		Description: "服务状态与账号信息。action=status 查登录状态；accounts 列出所有账号；doctor 环境自检",
		Annotations: &mcp.ToolAnnotations{
			Title:        "系统状态",
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in SystemInput) (*mcp.CallToolResult, SystemOutput, error) {
		return handleSystem(ctx, deps, in)
	})
}

func handleSystem(ctx context.Context, deps *Deps, in SystemInput) (*mcp.CallToolResult, SystemOutput, error) {
	out := SystemOutput{Action: in.Action, Version: Version}

	switch in.Action {
	case "", "status":
		out.Action = "status"
		name := in.Account
		if name == "" {
			name = deps.DefaultAccount
		}
		acc, ok := deps.Pool.Get(name)
		if !ok {
			out.Message = fmt.Sprintf("账号 %q 不存在。已配置账号：%v", name, deps.Pool.Names())
			return nil, out, nil
		}
		info := map[string]any{
			"name":         acc.Name,
			"session_path": acc.SessionPath(),
			"has_self":     acc.Self() != nil,
		}
		if self := acc.Self(); self != nil {
			username, _ := self.GetUsername()
			info["user_id"] = self.ID
			info["username"] = username
			info["first_name"] = self.FirstName
			info["last_name"] = self.LastName
			info["phone"], _ = self.GetPhone()
		}
		out.Data = info
		out.Message = "ok"
		return nil, out, nil

	case "accounts":
		list := make([]map[string]any, 0, deps.Pool.Len())
		for _, n := range deps.Pool.Names() {
			acc, _ := deps.Pool.Get(n)
			item := map[string]any{"name": n, "is_default": n == deps.DefaultAccount}
			if acc != nil {
				item["session_path"] = acc.SessionPath()
				if self := acc.Self(); self != nil {
					username, _ := self.GetUsername()
					item["user_id"] = self.ID
					item["username"] = username
				}
			}
			list = append(list, item)
		}
		out.Data = list
		out.Message = fmt.Sprintf("共 %d 个账号", len(list))
		return nil, out, nil

	case "doctor":
		out.Data = map[string]any{
			"go_version":    runtime.Version(),
			"os":            runtime.GOOS,
			"arch":          runtime.GOARCH,
			"cgo_enabled":   cgoEnabled,
			"time":          time.Now().Format(time.RFC3339),
			"account_count": deps.Pool.Len(),
			"accounts":      deps.Pool.Names(),
			"default":       deps.DefaultAccount,
		}
		out.Message = "环境自检完成"
		return nil, out, nil

	default:
		out.Message = fmt.Sprintf("未知 action %q（支持 status / accounts / doctor）", in.Action)
		return nil, out, nil
	}
}

// logDeps 保留给后续阶段使用，避免未使用告警。
var _ = zap.NewNop
