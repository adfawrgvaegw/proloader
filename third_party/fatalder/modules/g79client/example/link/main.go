package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Yeah114/g79client/example/login"
	linkconnection "github.com/Yeah114/g79client/service/link_connection"
)

func main() {
	client, err := login.Login()
	if err != nil {
		fmt.Printf("登录失败: %v\n", err)
		return
	}

	svc, err := linkconnection.NewLinkConnectionService(client)
	if err != nil {
		fmt.Printf("创建 Link 服务失败: %v\n", err)
		return
	}

	fmt.Println("尝试建立 Link 连接...")
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	conn, err := svc.Dial(ctx)
	if err != nil {
		fmt.Printf("连接失败: %v\n", err)
		return
	}
	defer conn.Close()

	if err := conn.SendGameStart(nil); err != nil {
		fmt.Printf("发送 GameStart 失败: %v\n", err)
	}
	if err := conn.RequestChatDumpState(); err != nil {
		fmt.Printf("请求 DumpState 失败: %v\n", err)
	}
	if err := conn.RequestChatConnectState(); err != nil {
		fmt.Printf("请求 ConnectState 失败: %v\n", err)
	}
	if err := conn.SendChatOnline(); err != nil {
		fmt.Printf("发送 Online 失败: %v\n", err)
	}
	if err := conn.FetchGlobalMessages(50); err != nil {
		fmt.Printf("拉取公共消息失败: %v\n", err)
	}

	go readAndSend(conn)

	fmt.Println("连接成功，等待服务器消息（Ctrl+C 退出）...")

	for {
		select {
		case msg, ok := <-conn.Messages():
			if !ok {
				fmt.Printf("连接已关闭: %v\n", conn.Err())
				return
			}
			pretty, _ := json.MarshalIndent(msg, "", "  ")
			fmt.Printf("收到原始消息:\n%s\n", string(pretty))
			if msg.Method == "GetGlobalMessages" {
				printGlobalMessages(msg.Payload)
			}
		case push, ok := <-conn.CommonPushMessages():
			if ok {
				pretty, _ := json.MarshalIndent(push, "", "  ")
				fmt.Printf("公共频道推送:\n%s\n", string(pretty))
				printCommonPush(push)
			}
		case <-ctx.Done():
			fmt.Printf("等待被取消: %v\n", ctx.Err())
			return
		}
	}
}

func readAndSend(conn *linkconnection.LinkConnection) {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("请输入要发送的公屏消息：")
		if !scanner.Scan() {
			return
		}
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		if err := conn.SendGlobalMessage(text); err != nil {
			fmt.Printf("发送失败: %v\n", err)
		}
	}
}

func printCommonPush(push linkconnection.CommonPushData) {
	if push.Type == "GlobalMessage" && len(push.Payload) > 0 {
		printGlobalMessagePayload(push.Payload)
	}
}

func printGlobalMessages(payload json.RawMessage) {
	if len(payload) == 0 {
		return
	}
	var resp struct {
		Code   int `json:"code"`
		Entity struct {
			Msgs []map[string]any `json:"msgs"`
		} `json:"entity"`
	}
	if err := json.Unmarshal(payload, &resp); err != nil {
		fmt.Printf("解析 GetGlobalMessages 失败: %v\n", err)
		return
	}
	for _, msg := range resp.Entity.Msgs {
		fmt.Printf("[历史] %s\n", extractMessageSummary(msg))
	}
}

func printGlobalMessagePayload(payload json.RawMessage) {
	var msg map[string]any
	if err := json.Unmarshal(payload, &msg); err != nil {
		fmt.Printf("解析 GlobalMessage 失败: %v\n", err)
		return
	}
	fmt.Printf("[推送] %s\n", extractMessageSummary(msg))
}

func extractMessageSummary(entry map[string]any) string {
	if entry == nil {
		return ""
	}
	uid := fmt.Sprint(entry["uid"])
	nickname := ""
	if userInfo, ok := entry["user_info"].(map[string]any); ok {
		if name, ok := userInfo["nickname"].(string); ok {
			nickname = name
		}
	}
	content := ""
	if raw, ok := entry["msg"].(string); ok && raw != "" {
		var inner struct {
			Type string      `json:"type"`
			Data interface{} `json:"data"`
		}
		if err := json.Unmarshal([]byte(raw), &inner); err == nil {
			switch v := inner.Data.(type) {
			case string:
				content = v
			case map[string]any:
				if txt, ok := v["text"].(string); ok {
					content = txt
				} else {
					b, _ := json.Marshal(v)
					content = string(b)
				}
			default:
				b, _ := json.Marshal(v)
				content = string(b)
			}
		} else {
			content = raw
		}
	}
	if nickname != "" {
		return fmt.Sprintf("%s(%s): %s", nickname, uid, content)
	}
	return fmt.Sprintf("%s: %s", uid, content)
}
