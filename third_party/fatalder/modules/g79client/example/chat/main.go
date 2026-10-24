package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Yeah114/g79client/example/login"
	chatconnection "github.com/Yeah114/g79client/service/chat_connection"
)

func main() {
	client, err := login.Login()
	if err != nil {
		fmt.Printf("登录失败: %v\n", err)
		return
	}
	fmt.Println(client.UserDetail.Name)
	_, _ = client.ApplyFriend(2719712857)

	svc, err := chatconnection.NewChatConnectionService(client)
	if err != nil {
		fmt.Printf("创建聊天服务失败: %v\n", err)
		return
	}

	endpoint, err := svc.SelectServer(0)
	if err != nil {
		fmt.Printf("获取聊天服务器信息失败: %v\n", err)
		return
	}

	fmt.Printf("准备连接聊天服务器: %s:%d\n", endpoint.Host, endpoint.Port)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := svc.DialWithEndpoint(ctx, endpoint)
	if err != nil {
		fmt.Printf("连接聊天服务器失败: %v\n", err)
		return
	}
	defer conn.Close()
	id, err := conn.ChatTo(2719712857, "Hello, world!")
	if err != nil {
		fmt.Printf("发送消息失败: %v\n", err)
		return
	}
	fmt.Println("发送消息: SN=", id)

	fmt.Println("连接成功，开始读取聊天消息...")

	for msg := range conn.Messages() {
		fmt.Printf("收到消息: SN=%d, CMD=%d, Data=%s\n", msg.Sequence, msg.Command, string(msg.Payload))
	}
}
