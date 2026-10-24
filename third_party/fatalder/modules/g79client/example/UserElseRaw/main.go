package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/Yeah114/g79client"
	"github.com/Yeah114/g79client/example/login"
)

func main() {
	client, err := login.Login()
	if err != nil {
		log.Fatalf("登录失败: %v", err)
	}

	nickname := "不吃人的魔鬼"
	body := map[string]any{
		"is_need_recharge_benefit": 1,
		"uids":                     nickname,
	}
	b, _ := json.Marshal(body)
	api := "/user-else-detail-many/"
	token := g79client.CalculateDynamicToken(api, string(b), client.UserToken)
	req, _ := http.NewRequest("POST", client.ReleaseJSON.ApiGatewayUrl+api, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("user-id", client.UserID)
	req.Header.Set("user-token", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	fmt.Printf("/user-else-detail-many/ 原始响应: %s\n", string(raw))
}
