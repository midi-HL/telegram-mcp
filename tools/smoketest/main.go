package main

// 阶段 0 的 MCP 冒烟测试：启动 tgmcp serve，通过 stdio 发送 JSON-RPC，
// 验证 initialize / tools/list / tools/call(system) 三个环节。
//
// 用法: go run ./tools/smoketest <tgmcp 可执行文件路径>

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type rpcReq struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

func main() {
	args := os.Args[1:]
	stage1 := false
	stage1send := false
	stage1group := false
	stage2 := false
	if len(args) > 0 {
		switch args[0] {
		case "-stage1":
			stage1 = true
			args = args[1:]
		case "-stage1send":
			stage1send = true
			args = args[1:]
		case "-stage1group":
			stage1group = true
			args = args[1:]
		case "-stage2":
			stage2 = true
			args = args[1:]
		}
	}
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "用法: smoketest [-stage1] <tgmcp 路径>")
		os.Exit(2)
	}
	bin := args[0]

	if stage1 {
		runStage1(bin)
		return
	}
	if stage1send {
		runStage1Send(bin)
		return
	}
	if stage1group {
		runStage1Group(bin)
		return
	}
	if stage2 {
		runStage2(bin)
		return
	}

	cmd := exec.Command(bin, "serve")
	cmd.Env = append(os.Environ(),
		"TGMCP_DATA_DIR="+os.Getenv("TGMCP_DATA_DIR"),
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		fatal("stdin: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fatal("stdout: %v", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		fatal("start: %v", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	reader := bufio.NewReaderSize(stdout, 1<<20)
	send := func(v any) {
		b, _ := json.Marshal(v)
		if _, err := stdin.Write(append(b, '\n')); err != nil {
			fatal("write: %v", err)
		}
	}
	readResp := func(wantID int) map[string]any {
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			line, err := reader.ReadString('\n')
			if err != nil {
				fatal("read: %v", err)
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(line), &m); err != nil {
				fmt.Printf("  (非 JSON 行: %s)\n", truncate(line, 120))
				continue
			}
			if id, ok := m["id"].(float64); ok && int(id) == wantID {
				return m
			}
			fmt.Printf("  (跳过通知: method=%v)\n", m["method"])
		}
		fatal("等待响应 id=%d 超时", wantID)
		return nil
	}

	// 1. initialize
	fmt.Println("[1] initialize")
	send(rpcReq{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": "2025-11-25",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "smoketest", "version": "1.0"},
		},
	})
	resp := readResp(1)
	if e, ok := resp["error"]; ok {
		fatal("initialize 失败: %v", e)
	}
	res := resp["result"].(map[string]any)
	serverInfo, _ := res["serverInfo"].(map[string]any)
	fmt.Printf("    ✅ server=%v version=%v\n", serverInfo["name"], serverInfo["version"])

	// 发送 initialized 通知
	send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})

	// 2. tools/list
	fmt.Println("[2] tools/list")
	send(rpcReq{JSONRPC: "2.0", ID: 2, Method: "tools/list"})
	resp = readResp(2)
	if e, ok := resp["error"]; ok {
		fatal("tools/list 失败: %v", e)
	}
	tools, _ := resp["result"].(map[string]any)["tools"].([]any)
	fmt.Printf("    ✅ 工具数 = %d\n", len(tools))
	for _, t := range tools {
		tm := t.(map[string]any)
		fmt.Printf("       - %v\n", tm["name"])
	}
	if len(tools) == 0 {
		fatal("tools/list 返回空")
	}

	// 3. tools/call system doctor
	fmt.Println("[3] tools/call system(doctor)")
	send(rpcReq{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "system",
			"arguments": map[string]any{"action": "doctor"},
		},
	})
	resp = readResp(3)
	if e, ok := resp["error"]; ok {
		fatal("tools/call 失败: %v", e)
	}
	content, _ := resp["result"].(map[string]any)["content"].([]any)
	if len(content) == 0 {
		fatal("tools/call 返回空 content")
	}
	text := content[0].(map[string]any)["text"].(string)
	fmt.Printf("    ✅ 返回 %d 字节\n", len(text))
	fmt.Printf("    %s\n", truncate(text, 300))

	// 4. tools/call system status（真实账号信息）
	fmt.Println("[4] tools/call system(status)")
	send(rpcReq{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "system",
			"arguments": map[string]any{"action": "status"},
		},
	})
	resp = readResp(4)
	content, _ = resp["result"].(map[string]any)["content"].([]any)
	if len(content) > 0 {
		text := content[0].(map[string]any)["text"].(string)
		fmt.Printf("    ✅ %s\n", truncate(text, 400))
	}

	fmt.Println("\n=== 阶段 0 MCP 冒烟测试全部通过 ===")
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "❌ "+format+"\n", args...)
	os.Exit(1)
}

var _ = io.Discard
