package main

import (
	"encoding/json"
	"fmt"

	"github.com/Yeah114/g79client"
)

func main() {
	servers, err := g79client.GetGlobalG79TransferServers()
	if err != nil {
		fmt.Printf("获取传输服务器列表失败: %v", err)
	}
	bs, err := json.Marshal(servers)
	if err != nil {
		fmt.Printf("序列化传输服务器列表失败: %v", err)
	}
	fmt.Printf("传输服务器列表: %s", string(bs))
}
