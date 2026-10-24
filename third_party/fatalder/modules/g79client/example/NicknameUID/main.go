package main

import (
	"fmt"
	"log"

	"github.com/Yeah114/g79client/example/login"
)

func main() {
	client, err := login.Login()
	if err != nil {
		log.Fatalf("登录失败: %v", err)
	}

	nickname := "不吃人的魔鬼"
	resp, err := client.SearchUserByNameOrMail(nickname, 0, 20)
	if err != nil {
		log.Fatalf("搜索失败: %v", err)
	}
	if resp.Code != 0 || len(resp.Entities) == 0 {
		fmt.Printf("未找到: %s\n", nickname)
		return
	}
	for i, e := range resp.Entities {
		fmt.Printf("%d) uid=%s nickname=%s online=%v type=%v\n", i+1, e.UID.String(), e.Nickname, e.OnlineStatus.Bool(), e.OnlineType.Int64())
	}
}
