//go:build ignore

// wake_probe 验证运行中的 MCP HTTP 服务能主动推送唤醒通知。
// 用法：go run tools/smoketest/wake_probe.go http://127.0.0.1:18093/mcp
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	endpoint := "http://127.0.0.1:18093/mcp"
	webui := "http://127.0.0.1:8080"
	if len(os.Args) > 1 {
		endpoint = os.Args[1]
	}
	if len(os.Args) > 2 {
		webui = os.Args[2]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	got := make(chan string, 8)
	client := mcp.NewClient(&mcp.Implementation{Name: "wake-probe", Version: "1"}, &mcp.ClientOptions{
		LoggingMessageHandler: func(_ context.Context, req *mcp.LoggingMessageRequest) {
			b, _ := json.Marshal(req.Params.Data)
			select {
			case got <- "log:" + string(b):
			default:
			}
		},
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			select {
			case got <- "progress:" + req.Params.Message:
			default:
			}
		},
	})
	cs, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint}, nil)
	if err != nil {
		fmt.Println("CONNECT_ERROR:", err)
		os.Exit(1)
	}
	defer cs.Close()
	if r := cs.InitializeResult(); r != nil {
		fmt.Println("CONNECTED", r.ProtocolVersion)
	}

	// 登录后触发一次真实投递（必须在客户端连接建立之后）。
	loginBody := strings.NewReader(`{"password":"Admin"}`)
	loginReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, webui+"/api/login", loginBody)
	loginReq.Header.Set("Content-Type", "application/json")
	loginRes, err := http.DefaultClient.Do(loginReq)
	if err != nil {
		fmt.Println("LOGIN_ERROR:", err)
		os.Exit(1)
	}
	defer loginRes.Body.Close()
	for _, c := range loginRes.Cookies() {
		if c.Name != "tgmcp_session" {
			continue
		}
		testReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, webui+"/api/wake/test", strings.NewReader(`{"text":"PROBE_WAKE_MESSAGE"}`))
		testReq.Header.Set("Content-Type", "application/json")
		testReq.AddCookie(c)
		testRes, err := http.DefaultClient.Do(testReq)
		if err == nil {
			_, _ = io.Copy(io.Discard, testRes.Body)
			_ = testRes.Body.Close()
		}
		break
	}

	for {
		select {
		case data := <-got:
			if strings.Contains(data, "PROBE_WAKE_MESSAGE") && strings.Contains(data, "untrusted_content") {
				fmt.Println("WAKE_NOTIFICATION_OK", data[:min(80, len(data))])
				return
			}
			fmt.Println("OTHER_NOTIFICATION:", data[:min(200, len(data))])
		case <-ctx.Done():
			fmt.Println("TIMEOUT_NO_NOTIFICATION")
			os.Exit(1)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
