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
	loc, err := client.FindPlayerByNickname(nickname)
	if err != nil {
		fmt.Printf("查询失败: %v\n", err)
		return
	}
	fmt.Printf("昵称=%s 的位置: room_id=%s, server_no=%s, server_id=%s\n", nickname, loc.RoomID, loc.ServerNo, loc.ServerID)
}


