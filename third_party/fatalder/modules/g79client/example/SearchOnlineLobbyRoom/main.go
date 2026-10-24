package main

import (
	"fmt"
	"log"

	"github.com/Yeah114/g79client"
)

func main() {
	// 替换为你的 Cookie（与 example/main.go 相同来源）
	cookie := `{"sauth_json":"{\"gameid\":\"x19\",\"login_channel\":\"netease\",\"app_channel\":\"netease\",\"platform\":\"pc\",\"sdkuid\":\"aibgraaesciluppl\",\"sessionid\":\"1-eyJzIjogImRscmdoa2RnaTh1eXF6ZmcyZDdrM3UwbXduNWtzNTg0IiwgIm9kaSI6ICJhbWF3cmFhYWF3cjV0Mm9lLWQiLCAic2kiOiAiYTA0NzFiYTY4MjEzZmUyZGZlMDMwZWRmZmQ0NTQyNDljNGY1Mjk4NyIsICJ1IjogImFpYmdyYWFlc2NpbHVwcGwiLCAidCI6IDIsICJnX2kiOiAiYWVjZnJ4b2R5cWFhYWFqcCJ9\",\"sdk_version\":\"3.9.0\",\"udid\":\"sznjy5jkn80387y93rsc1wm3z23iws3q\",\"deviceid\":\"amawraaaawr5t2oe-d\",\"aim_info\":\"{\\\"aim\\\":\\\"127.0.0.1\\\",\\\"country\\\":\\\"CN\\\",\\\"tz\\\":\\\"+0800\\\",\\\"tzid\\\":\\\"\\\"}\",\"client_login_sn\":\"C14DB363E5934FE0F529E6642EBA4D0E\",\"gas_token\":\"\",\"source_platform\":\"pc\",\"ip\":\"127.0.0.1\"}"}`

	client, err := g79client.NewClient()
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}

	// 认证
	if err := client.X19AuthenticateWithCookie(cookie); err != nil {
		log.Fatalf("认证失败: %v", err)
	}
	fmt.Printf("登录成功, 用户ID: %v, 昵称: %s\n", client.UserID, client.UserDetail.Name)
	// 搜索在线大厅房间
	keyword := "86020"
	searchResp, err := client.SearchOnlineLobbyRoomByKeyword(keyword, 10, 0)
	if err != nil {
		log.Fatalf("搜索在线大厅房间失败: %v", err)
	}
	if searchResp.Code != 0 {
		log.Fatalf("搜索在线大厅房间失败(%d): %s", searchResp.Code, searchResp.Message)
	}
	fmt.Printf("搜索在线大厅房间成功, 共找到 %d 个房间:\n", searchResp.Total.Int64())
	for _, room := range searchResp.Entities {
		fmt.Printf("- 房间: %v\n", room)
	}
}
