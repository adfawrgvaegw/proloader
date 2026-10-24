package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/Yeah114/g79client/example/login"
)

func main() {
	client, err := login.Login()
	if err != nil {
		log.Fatalf("登录失败: %v", err)
	}

	uids := []string{"2779636056", "2828379402", "2851954627", "2893605562", "2907414095", "2943012955", "2956215411", "2983061407"}
	// 强类型调用
	resp, err := client.GetUserElseDetailMany(uids)
	if err != nil {
		log.Fatalf("请求失败: %v", err)
	}

	// 简要打印
	fmt.Printf("返回用户数: %d\n", len(resp.Entities))
	for i, e := range resp.Entities {
		fmt.Printf("%d) id=%d nickname=%s lv=%d online=%v\n", i+1, e.ID.Int64(), e.Nickname, e.PEGrowth.Lv.Int64(), e.OnlineStatus.Bool())
	}

	fmt.Printf("summary_md5: %s\n", resp.SummaryMD5)
	fmt.Printf("查询UIDs: %s\n", strings.Join(uids, ";"))
}
