package main

import (
	"encoding/json"
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
	if err := client.G79AuthenticateWithCookie(cookie); err != nil {
		log.Fatalf("认证失败: %v", err)
	}

	_, _ = client.LeaveMainCity()
	_ = client.LeaveEnteredGame()
	info, err := client.EnterMainCity()
	if err != nil {
		log.Fatalf("获取服务器地址失败: %v", err)
	}

	bs, _ := json.Marshal(info)
	fmt.Println(string(bs))
	address := fmt.Sprintf("%s:%d", info.Entity.ServerHost, info.Entity.ServerPort)
	authv2, err := client.GenerateLobbyGameAuthV2(fmt.Sprintf("%d", info.Entity.CityNo), "MHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEzmz6+EK8UC40g5XsqoAjqURAKP6uCAMmXJeEyzR/8BkZ1vVXpFTMF/AmBl3Tf+gvDFPJkT9Bm3bAO0IeXo+ssMOsJX4NFPLM4+YEohwJrJyRaMptmh1nvWue4J5+vbZW")
	if err != nil {
		log.Fatalf("生成认证v2数据失败: %v", err)
	}
	fmt.Println(string(authv2))
	chainInfo, err := client.SendAuthV2Request(authv2)
	if err != nil {
		log.Fatalf("发送认证v2请求失败: %v", err)
	}
	fmt.Println(string(chainInfo))
	fmt.Println(address)
}
