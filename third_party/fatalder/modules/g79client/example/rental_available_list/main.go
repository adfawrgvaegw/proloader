package main

import (
	"encoding/json"
	"fmt"

	"github.com/Yeah114/g79client"
	"github.com/Yeah114/g79client/example/login"
)

func main() {
	client, err := login.Login()
	if err != nil {
		fmt.Printf("登录失败: %v", err)
	}
	list, err := client.GetAvailableRentalServers(g79client.SortTypePlayerCount, g79client.OrderTypeDesc, 0)
	if err != nil {
		fmt.Printf("获取租赁服列表失败: %v", err)
	}
	bs, err := json.Marshal(list)
	if err != nil {
		fmt.Printf("序列化租赁服列表失败: %v", err)
	}
	fmt.Printf("租赁服列表: %s", string(bs))
}