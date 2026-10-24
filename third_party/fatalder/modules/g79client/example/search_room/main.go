package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Yeah114/g79client"
)

func main() {
	// 使用用户提供的 cookie
	cookie := `{"sauth_json":"{\"gameid\":\"x19\",\"login_channel\":\"netease\",\"app_channel\":\"netease\",\"platform\":\"pc\",\"sdkuid\":\"aibgraygckinkcmz\",\"sessionid\":\"1-eyJzIjogInJ1MWtsaGJ2ejhzZ2Z0c29oejdvNTN1eW91dXkzMjdiIiwgIm9kaSI6ICJhbWF3cmF5YWF3c2tyZTRwLWQiLCAic2kiOiAiMjlkNzYwNGNlNDY2N2Y4YmRmZmQwMzY3NDUxZDkzMTNmZWExYWNjNCIsICJ1IjogImFpYmdyYXlnY2tpbmtjbXoiLCAidCI6IDIsICJnX2kiOiAiYWVjZnJ4b2R5cWFhYWFqcCJ9\",\"sdk_version\":\"3.9.0\",\"udid\":\"7k5u9u8zto4cc5wl2flu50cgcrfgeil3\",\"deviceid\":\"amawrayaawskre4p-d\",\"aim_info\":\"{\\\"aim\\\":\\\"127.0.0.1\\\",\\\"country\\\":\\\"CN\\\",\\\"tz\\\":\\\"+0800\\\",\\\"tzid\\\":\\\"\\\"}\",\"client_login_sn\":\"1D73C1A26B40F95EE7D932A96884B8F2\",\"gas_token\":\"\",\"source_platform\":\"pc\",\"ip\":\"127.0.0.1\"}"}`

	client, err := g79client.NewClient()
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}

	if err := client.X19AuthenticateWithCookie(cookie); err != nil {
		log.Fatalf("认证失败: %v", err)
	}

	keyword := "93560"
	if len(os.Args) > 1 && os.Args[1] != "" {
		keyword = os.Args[1]
	}
	// 发起搜索
	api := "/online-lobby-room/query/search-by-name-v2"
	body := map[string]interface{}{
		"length":  20,
		"version": "1.21.2",
		"res_id":  "",
		"keyword": keyword,
		"offset":  0,
	}
	b, _ := json.Marshal(body)
	doRequest := func() (int, []byte, map[string]any) {
		token := g79client.CalculateDynamicToken(api, string(b), client.UserToken)
		req, err := http.NewRequest("POST", "https://x19apigatewayobt.nie.netease.com"+api, bytes.NewReader(b))
		if err != nil {
			log.Fatalf("构造请求失败: %v", err)
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
		req.Header.Set("user-id", client.UserID)
		req.Header.Set("user-token", token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Fatalf("请求失败: %v", err)
		}
		defer resp.Body.Close()
		var reader io.Reader = resp.Body
		if resp.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(resp.Body)
			if err != nil {
				log.Fatalf("解压失败: %v", err)
			}
			defer gz.Close()
			reader = gz
		}
		raw, _ := io.ReadAll(reader)
		var obj map[string]any
		fmt.Println(string(raw))
		_ = json.Unmarshal(raw, &obj)
		code := 0
		if v, ok := obj["code"].(float64); ok {
			code = int(v)
		}
		return code, raw, obj
	}

	code, _, obj := doRequest()
	if code == 2311 {
		time.Sleep(3 * time.Second)
		code, _, obj = doRequest()
	}

	if code != 0 {
		fmt.Printf("接口返回错误 code=%d, message=%v\n", code, obj["message"])
		return
	}

	entities, _ := obj["entities"].([]any)
	if len(entities) == 0 {
		fmt.Println("房间总数: 0")
		return
	}

	// 读取总数
	total := 0
	if f, ok := obj["total"].(float64); ok {
		total = int(f)
	}

	// 收集 owner_id 列表并去重
	ownerIDs := make([]string, 0, len(entities))
	ownerIDSet := make(map[string]struct{})
	for _, it := range entities {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		ownerID := firstNonEmpty(m["owner_id"]).String()
		if ownerID == "" {
			continue
		}
		if _, exists := ownerIDSet[ownerID]; !exists {
			ownerIDSet[ownerID] = struct{}{}
			ownerIDs = append(ownerIDs, ownerID)
		}
	}

	// 批量拉取昵称
	uidToNickname := map[string]string{}
	if len(ownerIDs) > 0 {
		elseResp, err := client.GetUserElseDetailMany(ownerIDs)
		if err == nil && elseResp != nil && elseResp.Code == 0 {
			for _, e := range elseResp.Entities {
				uidToNickname[e.ID.String()] = e.Nickname
			}
		}
	}

	// 仅输出所需字段
	for _, it := range entities {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		rid := firstNonEmpty(m["room_id"], m["entity_id"], m["id"], m["roomId"]).String()
		rname := firstNonEmpty(m["room_name"], m["name"]).String()
		ownerID := firstNonEmpty(m["owner_id"]).String()
		ownerName := uidToNickname[ownerID]
		if ownerName == "" {
			ownerName = firstNonEmpty(m["owner_name"], m["owner"]).String()
		}
		resName := firstNonEmpty(m["res_name"]).String()
		slogan := firstNonEmpty(m["slogan"]).String()
		fmt.Printf("room_id=%s, room_name=%s, owner_name=%s, res_name=%s, slogan=%s\n", rid, rname, ownerName, resName, slogan)
	}

	// 输出总数
	fmt.Printf("房间总数: %d\n", total)
}

// 工具方法：从多个可能字段取第一个非空字符串
type anyString struct{ v any }

func (a anyString) String() string {
	switch t := a.v.(type) {
	case nil:
		return ""
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func firstNonEmpty(vals ...any) anyString {
	for _, v := range vals {
		if v == nil {
			continue
		}
		s := fmt.Sprint(v)
		if s != "" && s != "0" && s != "<nil>" {
			return anyString{v}
		}
	}
	return anyString{""}
}
