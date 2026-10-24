package main

import (
	"fmt"

	"github.com/Yeah114/g79client/example/login"
)

func main() {
	client, err := login.Login()
	if err != nil {
		fmt.Printf("登录失败: %v\n", err)
		return
	}

	targetUID := "2719712857"

	searchResp, err := client.GetOtherUserDetail(targetUID, false)
	if err != nil {
		fmt.Printf("搜索玩家失败: %v\n", err)
		return
	}

	targetMomentID := searchResp.Entity.MomentID
	fmt.Printf("找到玩家 %s, MomentID=%s\n", targetUID, targetMomentID)

	messagesResp, err := client.GetUserMessages(targetMomentID, 1, 10)
	if err != nil {
		fmt.Printf("拉取玩家动态失败: %v\n", err)
		return
	}

	if messagesResp.Status != 200 {
		fmt.Printf("动态接口返回异常状态: %d\n", messagesResp.Status)
		return
	}

	if len(messagesResp.Data.Data) == 0 {
		fmt.Printf("玩家 %s 暂无动态\n", targetUID)
		return
	}

	for _, moment := range messagesResp.Data.Data {
		likeResp, err := client.LikeMoment(moment.MsgID, 0)
		if err != nil {
			fmt.Printf("点赞动态失败: %v\n", err)
			continue
		}
		if likeResp.Code != 0 {
			fmt.Printf("点赞动态失败: %s\n", likeResp.Message)
			continue
		}
		fmt.Printf("动态 %s 内容: %s\n", moment.MsgID, moment.Content)
		if len(moment.RecentComment) == 0 {
			fmt.Println("  没有评论")
			continue
		}
		for _, comment := range moment.RecentComment {
			fmt.Printf("  评论ID: %s, 用户: %s, 内容: %s\n", comment.CommentID, comment.Name, comment.Comment)
		}
	}
}
