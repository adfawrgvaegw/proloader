package main

import (
	"fmt"
	"os"

	"github.com/Yeah114/g79client"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: cookietest <cookie.json 路径>")
		os.Exit(1)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Printf("读取 cookie 失败: %v\n", err)
		os.Exit(1)
	}
	cookieStr := string(data)

	fmt.Println("创建客户端（会自动 bootstrap 获取版本信息）...")
	client, err := g79client.NewClient()
	if err != nil {
		fmt.Printf("NewClient 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("EngineVersion=%s\n", client.EngineVersion)
	fmt.Printf("G79LatestVersion=%s\n", client.G79LatestVersion)

	fmt.Println("\n执行 PE 认证...")
	err = client.G79AuthenticateWithCookie(cookieStr)
	if err != nil {
		fmt.Printf("❌ 认证失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ 认证成功! UserID=%s\n", client.UserID)
	fmt.Printf("   UserToken=%s...\n", client.UserToken[:20])
}
