package mcp

import (
	"context"
	"fmt"

	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// BotInput 是 bot 工具的输入。
type BotInput struct {
	// Action: list / info / create / set_info / set_commands / export_token / check_username / call
	Action   string `json:"action" jsonschema:"list|info|create|set_info|set_commands|export_token|check_username|call"`
	Bot      string `json:"bot,omitempty" jsonschema:"bot @username or id"`
	Username string `json:"username,omitempty" jsonschema:"new bot username"`
	Name     string `json:"name,omitempty" jsonschema:"bot display name"`
	About    string `json:"about,omitempty" jsonschema:"bot description"`
	Commands []string `json:"commands,omitempty" jsonschema:"commands like start:Start the bot"`
	Account  string `json:"account,omitempty" jsonschema:"account name"`
}

// BotInfo 是 Bot 摘要。
type BotInfo struct {
	ID       int64  `json:"id"`
	Username string `json:"username,omitempty"`
	Name     string `json:"name"`
	About    string `json:"about,omitempty"`
	Commands []BotCommand `json:"commands,omitempty"`
}

// BotCommand 是 Bot 命令。
type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// BotOutput 是 bot 工具的输出。
type BotOutput struct {
	Action  string    `json:"action"`
	Bots    []BotInfo `json:"bots,omitempty"`
	Bot     *BotInfo  `json:"bot,omitempty"`
	Token   string    `json:"token,omitempty"`
	Count   int       `json:"count,omitempty"`
	Note    string    `json:"note,omitempty"`
}

// registerBotTools 注册 bot 工具。
func registerBotTools(s *mcp.Server, deps *Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "bot",
		Description: "管理自己的 Bot。action: list 我管理的 Bot / info 详情 / create 创建 / set_info 改信息 / set_commands 设命令 / export_token 导出 token / check_username 查用户名可用 / call 调用 Bot 方法",
		Annotations: &mcp.ToolAnnotations{Title: "Bot 管理", ReadOnlyHint: false},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in BotInput) (*mcp.CallToolResult, BotOutput, error) {
		return handleBot(ctx, deps, in)
	})
}

func handleBot(ctx context.Context, deps *Deps, in BotInput) (*mcp.CallToolResult, BotOutput, error) {
	out := BotOutput{Action: in.Action}

	switch in.Action {
	case "", "list":
		out.Action = "list"
		return handleBotList(ctx, deps, in, out)
	case "info":
		return handleBotInfo(ctx, deps, in, out)
	case "create":
		return handleBotCreate(ctx, deps, in, out)
	case "set_info":
		return handleBotSetInfo(ctx, deps, in, out)
	case "set_commands":
		return handleBotSetCommands(ctx, deps, in, out)
	case "export_token":
		return handleBotExportToken(ctx, deps, in, out)
	case "check_username":
		return handleBotCheckUsername(ctx, deps, in, out)
	default:
		out.Note = fmt.Sprintf("未知 action %q", in.Action)
		return nil, out, nil
	}
}

// handleBotList 列出我管理的 Bot。
func handleBotList(ctx context.Context, deps *Deps, in BotInput, out BotOutput) (*mcp.CallToolResult, BotOutput, error) {
	acc, _, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	users, err := acc.API().BotsGetAdminedBots(ctx)
	if err != nil {
		out.Note = fmt.Sprintf("获取 Bot 列表失败: %v", err)
		return nil, out, nil
	}
	for _, uc := range users {
		u, ok := uc.(*tg.User)
		if !ok {
			continue
		}
		info := BotInfo{
			ID:   u.ID,
			Name: displayName(u),
		}
		info.Username, _ = u.GetUsername()
		out.Bots = append(out.Bots, info)
	}
	out.Count = len(out.Bots)
	out.Note = fmt.Sprintf("共管理 %d 个 Bot", out.Count)
	return nil, out, nil
}

// handleBotInfo 获取 Bot 详情。
func handleBotInfo(ctx context.Context, deps *Deps, in BotInput, out BotOutput) (*mcp.CallToolResult, BotOutput, error) {
	if in.Bot == "" {
		out.Note = "info 需要 bot 参数"
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

	langCode := "zh"
	res, err := acc.API().BotsGetBotInfo(ctx, &tg.BotsGetBotInfoRequest{
		Bot:      &tg.InputUser{UserID: up.UserID, AccessHash: up.AccessHash},
		LangCode: langCode,
	})
	if err != nil {
		out.Note = fmt.Sprintf("获取 Bot 信息失败: %v", err)
		return nil, out, nil
	}
	info := BotInfo{
		ID:    up.UserID,
		Name:  res.Name,
		About: res.About,
	}
	if info.Name == "" {
		info.Name = res.Description
	}
	out.Bot = &info
	out.Note = "ok"
	return nil, out, nil
}

// handleBotCreate 创建 Bot。
func handleBotCreate(ctx context.Context, deps *Deps, in BotInput, out BotOutput) (*mcp.CallToolResult, BotOutput, error) {
	if in.Username == "" || in.Name == "" {
		out.Note = "create 需要 username 和 name"
		return nil, out, nil
	}
	acc, _, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}

	uc, err := acc.API().BotsCreateBot(ctx, &tg.BotsCreateBotRequest{
		Name:     in.Name,
		Username: in.Username,
	})
	if err != nil {
		out.Note = fmt.Sprintf("创建 Bot 失败: %v（用户名可能已被占用）", err)
		return nil, out, nil
	}
	u, ok := uc.(*tg.User)
	if !ok {
		out.Note = "创建成功但返回数据异常"
		return nil, out, nil
	}
	info := BotInfo{ID: u.ID, Name: displayName(u)}
	info.Username, _ = u.GetUsername()
	out.Bot = &info
	out.Note = fmt.Sprintf("Bot 已创建：@%s（ID=%d）", info.Username, info.ID)
	return nil, out, nil
}

// handleBotSetInfo 改 Bot 信息。
func handleBotSetInfo(ctx context.Context, deps *Deps, in BotInput, out BotOutput) (*mcp.CallToolResult, BotOutput, error) {
	if in.Bot == "" {
		out.Note = "set_info 需要 bot 参数"
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

	req := &tg.BotsSetBotInfoRequest{
		Bot:      &tg.InputUser{UserID: up.UserID, AccessHash: up.AccessHash},
		LangCode: "zh",
	}
	if in.Name != "" {
		req.Name = in.Name
	}
	if in.About != "" {
		req.About = in.About
	}
	_, err = acc.API().BotsSetBotInfo(ctx, req)
	if err != nil {
		out.Note = fmt.Sprintf("修改 Bot 信息失败: %v", err)
		return nil, out, nil
	}
	out.Note = "Bot 信息已更新"
	return nil, out, nil
}

// handleBotSetCommands 设置 Bot 命令菜单。
func handleBotSetCommands(ctx context.Context, deps *Deps, in BotInput, out BotOutput) (*mcp.CallToolResult, BotOutput, error) {
	if in.Bot == "" || len(in.Commands) == 0 {
		out.Note = "set_commands 需要 bot 和 commands（格式：start:启动）"
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

	cmds := make([]tg.BotCommand, 0, len(in.Commands))
	for _, c := range in.Commands {
		parts := splitOnce(c, ':')
		if len(parts) != 2 {
			out.Note = fmt.Sprintf("命令格式错误：%q（应为 cmd:描述）", c)
			return nil, out, nil
		}
		cmds = append(cmds, tg.BotCommand{
			Command:     trimSpace(parts[0]),
			Description: trimSpace(parts[1]),
		})
	}

	_, err = acc.API().BotsSetBotCommands(ctx, &tg.BotsSetBotCommandsRequest{
		Scope:    &tg.BotCommandScopeDefault{},
		LangCode: "zh",
		Commands: cmds,
	})
	if err != nil {
		out.Note = fmt.Sprintf("设置命令失败: %v", err)
		return nil, out, nil
	}
	out.Count = len(cmds)
	out.Note = fmt.Sprintf("已设置 %d 个命令", len(cmds))
	_ = up
	return nil, out, nil
}

// handleBotExportToken 导出 Bot token。
func handleBotExportToken(ctx context.Context, deps *Deps, in BotInput, out BotOutput) (*mcp.CallToolResult, BotOutput, error) {
	if in.Bot == "" {
		out.Note = "export_token 需要 bot 参数"
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

	res, err := acc.API().BotsExportBotToken(ctx, &tg.BotsExportBotTokenRequest{
		Bot: &tg.InputUser{UserID: up.UserID, AccessHash: up.AccessHash},
	})
	if err != nil {
		out.Note = fmt.Sprintf("导出 token 失败: %v（可能需要 BotFather 授权）", err)
		return nil, out, nil
	}
	out.Token = res.Token
	out.Note = "Token 已导出（请妥善保管，不要泄露）"
	return nil, out, nil
}

// handleBotCheckUsername 检查用户名可用性。
func handleBotCheckUsername(ctx context.Context, deps *Deps, in BotInput, out BotOutput) (*mcp.CallToolResult, BotOutput, error) {
	username := in.Username
	if username == "" {
		username = in.Bot
	}
	if username == "" {
		out.Note = "check_username 需要 username"
		return nil, out, nil
	}
	acc, _, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	ok, err := acc.API().BotsCheckUsername(ctx, &tg.BotsCheckUsernameRequest{Username: username})
	if err != nil {
		out.Note = fmt.Sprintf("检查失败: %v", err)
		return nil, out, nil
	}
	if ok {
		out.Note = fmt.Sprintf("@%s 可用", username)
	} else {
		out.Note = fmt.Sprintf("@%s 已被占用", username)
	}
	return nil, out, nil
}

// 辅助
func splitOnce(s string, sep byte) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
