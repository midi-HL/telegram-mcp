package main

// 阶段 1 冒烟测试：验证 chat / message / contact / group 工具的真实行为。
// 用 go run ./tools/smoketest -stage1 <tgmcp 路径>

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type rpcClient struct {
	stdin  *os.File
	reader *bufio.Reader
	nextID int
}

func newRPCClient(bin string) (*rpcClient, func(), error) {
	cmd := exec.Command(bin, "serve")
	cmd.Env = os.Environ()
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	c := &rpcClient{stdin: in.(*os.File), reader: bufio.NewReaderSize(out, 4<<20), nextID: 1}
	cleanup := func() {
		_ = in.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	return c, cleanup, nil
}

func (c *rpcClient) call(method string, params any) (map[string]any, error) {
	id := c.nextID
	c.nextID++
	req := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		req["params"] = params
	}
	b, _ := json.Marshal(req)
	if _, err := c.stdin.Write(append(b, '\n')); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) != nil {
			continue
		}
		if gotID, ok := m["id"].(float64); ok && int(gotID) == id {
			return m, nil
		}
	}
	return nil, fmt.Errorf("等待 %s 响应超时", method)
}

func (c *rpcClient) notify(method string) {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method})
	_, _ = c.stdin.Write(append(b, '\n'))
}

// toolCall 调用工具并返回解析后的 JSON 文本。
func (c *rpcClient) toolCall(name string, args map[string]any) (string, error) {
	resp, err := c.call("tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return "", err
	}
	if e, ok := resp["error"]; ok {
		return "", fmt.Errorf("RPC 错误: %v", e)
	}
	result, _ := resp["result"].(map[string]any)
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		return "", fmt.Errorf("空 content")
	}
	return content[0].(map[string]any)["text"].(string), nil
}

func runStage1(bin string) {
	c, cleanup, err := newRPCClient(bin)
	if err != nil {
		fatal("启动: %v", err)
	}
	defer cleanup()

	// initialize
	if _, err := c.call("initialize", map[string]any{
		"protocolVersion": "2025-11-25",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "stage1test", "version": "1"},
	}); err != nil {
		fatal("initialize: %v", err)
	}
	c.notify("notifications/initialized")

	pass, fail := 0, 0
	check := func(name string, fn func() error) {
		fmt.Printf("\n[%s]\n", name)
		if err := fn(); err != nil {
			fmt.Printf("  ❌ %v\n", err)
			fail++
		} else {
			fmt.Printf("  ✅\n")
			pass++
		}
	}

	// 1. chat list
	check("chat list (limit=10)", func() error {
		text, err := c.toolCall("chat", map[string]any{"action": "list", "limit": 10})
		if err != nil {
			return err
		}
		fmt.Printf("  返回 %d 字节\n", len(text))
		var out struct {
			Chats []struct {
				ID     int64  `json:"id"`
				Title  string `json:"title"`
				Type   string `json:"type"`
				Unread int    `json:"unread"`
			} `json:"chats"`
			Total int `json:"total"`
		}
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			return fmt.Errorf("解析: %v (原文: %s)", err, truncate(text, 300))
		}
		fmt.Printf("  会话数 %d\n", out.Total)
		for i, ch := range out.Chats {
			if i >= 5 {
				break
			}
			fmt.Printf("    %d. [%s] %s (未读 %d)\n", i+1, ch.Type, ch.Title, ch.Unread)
		}
		if out.Total == 0 {
			return fmt.Errorf("会话数为 0（可能账号没有对话）")
		}
		return nil
	})

	// 2. chat unread
	check("chat unread", func() error {
		text, err := c.toolCall("chat", map[string]any{"action": "unread"})
		if err != nil {
			return err
		}
		var out struct {
			Unread struct {
				TotalChats  int `json:"total_chats"`
				TotalUnread int `json:"total_unread"`
			} `json:"unread"`
		}
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			return fmt.Errorf("解析: %v", err)
		}
		fmt.Printf("  总会话 %d，总未读 %d\n", out.Unread.TotalChats, out.Unread.TotalUnread)
		return nil
	})

	// 3. contact info 自己
	check("contact info (自己 @HL_Mutsumi)", func() error {
		text, err := c.toolCall("contact", map[string]any{"action": "info", "user": "@HL_Mutsumi"})
		if err != nil {
			return err
		}
		var out struct {
			Profile struct {
				ID        int64  `json:"id"`
				FirstName string `json:"first_name"`
				Username  string `json:"username"`
				Phone     string `json:"phone"`
				Bio       string `json:"bio"`
				Status    string `json:"status"`
				Birthday  string `json:"birthday"`
			} `json:"profile"`
			Note string `json:"note"`
		}
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			return fmt.Errorf("解析: %v (原文 %s)", err, truncate(text, 300))
		}
		fmt.Printf("  ID=%d 名字=%s @%s\n", out.Profile.ID, out.Profile.FirstName, out.Profile.Username)
		fmt.Printf("  手机=%s 状态=%s\n", out.Profile.Phone, out.Profile.Status)
		if out.Profile.Bio != "" {
			fmt.Printf("  简介=%s\n", truncate(out.Profile.Bio, 60))
		}
		if out.Profile.Birthday != "" {
			fmt.Printf("  生日=%s\n", out.Profile.Birthday)
		}
		if out.Profile.ID == 0 {
			return fmt.Errorf("ID 为 0，资料获取失败: %s", out.Note)
		}
		return nil
	})

	// 4. contact info 大号
	check("contact info (@Edwin_hlcx 你的大号)", func() error {
		text, err := c.toolCall("contact", map[string]any{"action": "info", "user": "@Edwin_hlcx"})
		if err != nil {
			return err
		}
		var out struct {
			Profile struct {
				ID              int64  `json:"id"`
				FirstName       string `json:"first_name"`
				LastName        string `json:"last_name"`
				Username        string `json:"username"`
				Phone           string `json:"phone"`
				Bio             string `json:"bio"`
				Status          string `json:"status"`
				Birthday        string `json:"birthday"`
				HasPhoto        bool   `json:"has_photo"`
				IsContact       bool   `json:"is_contact"`
				CommonChats     int    `json:"common_chats"`
				PersonalChannel *struct {
					Title       string `json:"title"`
					Subscribers int    `json:"subs"`
				} `json:"channel"`
			} `json:"profile"`
			Note string `json:"note"`
		}
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			return fmt.Errorf("解析: %v", err)
		}
		p := out.Profile
		fmt.Printf("  ID=%d 名字=%s %s @%s\n", p.ID, p.FirstName, p.LastName, p.Username)
		fmt.Printf("  手机=%s 状态=%s 头像=%v\n", p.Phone, p.Status, p.HasPhoto)
		if p.Bio != "" {
			fmt.Printf("  简介=%s\n", truncate(p.Bio, 80))
		}
		if p.Birthday != "" {
			fmt.Printf("  生日=%s\n", p.Birthday)
		}
		if p.PersonalChannel != nil {
			fmt.Printf("  个人频道=%s (%d 订阅)\n", p.PersonalChannel.Title, p.PersonalChannel.Subscribers)
		}
		fmt.Printf("  共同群=%d 是联系人=%v\n", p.CommonChats, p.IsContact)
		if p.ID == 0 {
			return fmt.Errorf("获取失败: %s", out.Note)
		}
		return nil
	})

	fmt.Printf("\n=== 阶段 1 冒烟: %d 通过, %d 失败 ===\n", pass, fail)
	if fail > 0 {
		os.Exit(1)
	}
}

// runStage1Send 测试发消息、编辑、删除、历史。
func runStage1Send(bin string) {
	c, cleanup, err := newRPCClient(bin)
	if err != nil {
		fatal("启动: %v", err)
	}
	defer cleanup()

	if _, err := c.call("initialize", map[string]any{
		"protocolVersion": "2025-11-25",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "stage1send", "version": "1"},
	}); err != nil {
		fatal("initialize: %v", err)
	}
	c.notify("notifications/initialized")

	target := os.Getenv("TGMCP_TEST_TARGET")
	if target == "" {
		target = "@Edwin_hlcx"
	}
	msgText := fmt.Sprintf("[tgmcp 自动测试] %s", time.Now().Format("15:04:05"))

	fmt.Printf("\n[发送消息到 %s]\n", target)
	text, err := c.toolCall("message", map[string]any{
		"action": "send", "chat": target, "text": msgText,
	})
	if err != nil {
		fatal("send: %v", err)
	}
	fmt.Printf("  %s\n", truncate(text, 300))

	var sent struct {
		MsgID int    `json:"msg_id"`
		Note  string `json:"note"`
	}
	_ = json.Unmarshal([]byte(text), &sent)
	if sent.MsgID == 0 {
		fatal("发送失败: %s", sent.Note)
	}
	fmt.Printf("  ✅ 已发送，消息 ID=%d\n", sent.MsgID)

	// 读历史验证
	fmt.Printf("\n[读取历史验证]\n")
	text, err = c.toolCall("message", map[string]any{
		"action": "history", "chat": target, "limit": 5,
	})
	if err != nil {
		fatal("history: %v", err)
	}
	var hist struct {
		Messages []struct {
			ID   int    `json:"id"`
			Text string `json:"text"`
			Out  bool   `json:"out"`
		} `json:"messages"`
		Count int `json:"count"`
	}
	_ = json.Unmarshal([]byte(text), &hist)
	fmt.Printf("  返回 %d 条\n", hist.Count)
	found := false
	for _, m := range hist.Messages {
		mark := " "
		if m.Out {
			mark = "→"
		}
		fmt.Printf("    %s [%d] %s\n", mark, m.ID, truncate(m.Text, 50))
		if strings.Contains(m.Text, "[tgmcp 自动测试]") {
			found = true
		}
	}
	if !found {
		fmt.Printf("  ⚠ 历史里没找到刚发的消息（可能缓存延迟）\n")
	} else {
		fmt.Printf("  ✅ 历史里确认到刚发的消息\n")
	}

	// 编辑
	fmt.Printf("\n[编辑消息]\n")
	edited := msgText + " (已编辑)"
	text, err = c.toolCall("message", map[string]any{
		"action": "edit", "chat": target, "msg_id": sent.MsgID, "text": edited,
	})
	if err != nil {
		fatal("edit: %v", err)
	}
	fmt.Printf("  %s\n", truncate(text, 200))

	// 删除
	fmt.Printf("\n[删除消息]\n")
	text, err = c.toolCall("message", map[string]any{
		"action": "delete", "chat": target, "msg_id": sent.MsgID,
	})
	if err != nil {
		fatal("delete: %v", err)
	}
	fmt.Printf("  %s\n", truncate(text, 200))

	fmt.Printf("\n=== 阶段 1 收发测试完成 ===\n")
}

// runStage1Group 测试群管理。
func runStage1Group(bin string) {
	c, cleanup, err := newRPCClient(bin)
	if err != nil {
		fatal("启动: %v", err)
	}
	defer cleanup()

	if _, err := c.call("initialize", map[string]any{
		"protocolVersion": "2025-11-25",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "stage1group", "version": "1"},
	}); err != nil {
		fatal("initialize: %v", err)
	}
	c.notify("notifications/initialized")

	target := os.Getenv("TGMCP_TEST_GROUP")
	if target == "" {
		target = "Testing for agent"
	}

	fmt.Printf("\n[group members: %s]\n", target)
	text, err := c.toolCall("group", map[string]any{
		"action": "members", "chat": target, "limit": 20,
	})
	if err != nil {
		fatal("members: %v", err)
	}
	var mout struct {
		Members []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"members"`
		Count int    `json:"count"`
		Note  string `json:"note"`
	}
	_ = json.Unmarshal([]byte(text), &mout)
	fmt.Printf("  %s\n", truncate(text, 400))
	fmt.Printf("  成员数 %d\n", mout.Count)
	for i, m := range mout.Members {
		if i >= 5 {
			break
		}
		fmt.Printf("    %d. %s [%s]\n", i+1, m.Name, m.Role)
	}

	fmt.Printf("\n[group invites: %s]\n", target)
	text, err = c.toolCall("group", map[string]any{"action": "invites", "chat": target})
	if err != nil {
		fatal("invites: %v", err)
	}
	fmt.Printf("  %s\n", truncate(text, 300))

	fmt.Printf("\n[chat info: %s]\n", target)
	text, err = c.toolCall("chat", map[string]any{"action": "info", "chat": target})
	if err != nil {
		fatal("chat info: %v", err)
	}
	fmt.Printf("  %s\n", truncate(text, 400))

	fmt.Printf("\n=== 群管理测试完成 ===\n")
}

// runStage2 测试阶段 2 功能。
func runStage2(bin string) {
	c, cleanup, err := newRPCClient(bin)
	if err != nil {
		fatal("启动: %v", err)
	}
	defer cleanup()
	if _, err := c.call("initialize", map[string]any{
		"protocolVersion": "2025-11-25",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "stage2", "version": "1"},
	}); err != nil {
		fatal("initialize: %v", err)
	}
	c.notify("notifications/initialized")

	pass, fail := 0, 0
	check := func(name string, fn func() error) {
		fmt.Printf("\n[%s]\n", name)
		if err := fn(); err != nil {
			fmt.Printf("  ❌ %v\n", err)
			fail++
		} else {
			fmt.Printf("  ✅\n")
			pass++
		}
	}

	// 1. account profile
	check("account profile", func() error {
		text, err := c.toolCall("account", map[string]any{"action": "profile"})
		if err != nil {
			return err
		}
		fmt.Printf("  %s\n", truncate(text, 400))
		var o struct{ Profile struct{ ID int64; Username string } `json:"profile"` }
		_ = json.Unmarshal([]byte(text), &o)
		if o.Profile.ID == 0 {
			return fmt.Errorf("ID 为 0")
		}
		return nil
	})

	// 2. account sessions
	check("account sessions", func() error {
		text, err := c.toolCall("account", map[string]any{"action": "sessions"})
		if err != nil {
			return err
		}
		fmt.Printf("  %s\n", truncate(text, 500))
		return nil
	})

	// 3. sticker search
	check("sticker search_sets", func() error {
		text, err := c.toolCall("sticker", map[string]any{"action": "search_sets", "query": "cat", "limit": 5})
		if err != nil {
			return err
		}
		fmt.Printf("  %s\n", truncate(text, 400))
		return nil
	})

	// 4. bot list
	check("bot list", func() error {
		text, err := c.toolCall("bot", map[string]any{"action": "list"})
		if err != nil {
			return err
		}
		fmt.Printf("  %s\n", truncate(text, 400))
		return nil
	})

	// 5. botuse start 福利 Bot
	check("botuse start @gemini_plusv_bot", func() error {
		text, err := c.toolCall("botuse", map[string]any{"action": "start", "bot": "@gemini_plusv_bot"})
		if err != nil {
			return err
		}
		fmt.Printf("  %s\n", truncate(text, 300))
		return nil
	})

	fmt.Printf("\n=== 阶段 2 冒烟: %d 通过, %d 失败 ===\n", pass, fail)
	if fail > 0 {
		os.Exit(1)
	}
}
