package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/Yeah114/g79client"
)

func main() {
	client, err := g79client.NewClient()
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}

	cookie := `{"sauth_json":"{\"gameid\":\"x19\",\"login_channel\":\"netease\",\"app_channel\":\"netease\",\"platform\":\"pc\",\"sdkuid\":\"aibgraaesciluppl\",\"sessionid\":\"1-eyJzIjogImRscmdoa2RnaTh1eXF6ZmcyZDdrM3UwbXduNWtzNTg0IiwgIm9kaSI6ICJhbWF3cmFhYWF3cjV0Mm9lLWQiLCAic2kiOiAiYTA0NzFiYTY4MjEzZmUyZGZlMDMwZWRmZmQ0NTQyNDljNGY1Mjk4NyIsICJ1IjogImFpYmdyYWFlc2NpbHVwcGwiLCAidCI6IDIsICJnX2kiOiAiYWVjZnJ4b2R5cWFhYWFqcCJ9\",\"sdk_version\":\"3.9.0\",\"udid\":\"sznjy5jkn80387y93rsc1wm3z23iws3q\",\"deviceid\":\"amawraaaawr5t2oe-d\",\"aim_info\":\"{\\\"aim\\\":\\\"127.0.0.1\\\",\\\"country\\\":\\\"CN\\\",\\\"tz\\\":\\\"+0800\\\",\\\"tzid\\\":\\\"\\\"}\",\"client_login_sn\":\"C14DB363E5934FE0F529E6642EBA4D0E\",\"gas_token\":\"\",\"source_platform\":\"pc\",\"ip\":\"127.0.0.1\"}"}`

	if err := client.X19AuthenticateWithCookie(cookie); err != nil {
		log.Fatalf("X19 登录失败: %v", err)
	}

	api := "/user-item-purchase"

	payload := map[string]any{
		"entity_id":       0,
		"item_id":         "4641585872553142281",
		"item_level":      0,
		"user_id":         client.UserID,
		"purchase_time":   0,
		"last_play_time":  0,
		"total_play_time": 0,
		"receiver_id":     "",
		"buy_path":        "PC_H5_COMPONENT_DETAIL",
		"coupon_ids":      []any{},
		"diamond":         0,
		"activity_name":   "",
		"batch_count":     1,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Fatalf("序列化请求失败: %v", err)
	}

	req, err := http.NewRequest("POST", client.X19ReleaseJSON.DCWebURL+api, bytes.NewReader(body))
	if err != nil {
		log.Fatalf("创建请求失败: %v", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("user-id", client.UserID)

	token := g79client.CalculateDynamicToken(api, string(body), client.UserToken)
	req.Header.Set("user-token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("读取响应失败: %v", err)
	}

	fmt.Printf("/user-item-purchase 原始响应: %s\n", string(raw))
}
