//go:generate go run -v generater.go
package constants

import (
	"encoding/json"
	"net/http"
)

var (
	Name          = "Fatalder"
	Authors       = []string{"Yeah114", "KashShinfu", "猫七街", "NearKai"}
	SpecialThanks = []string{"天劫飛飛", "bxy", "空空", "浮鸿", "Ember2024", "inyheny", "style_天枢", "jiru", "酷兔兔", "南", "夜猫子", "小虫虫", "w魏信者s", "虫子为", "MuFeng2015ye", "无名", "守卫"}
	Version       = "3.3.2"

// License = "AGPL-v3.0" // 协议这个以后再想吧
)

// 通过 https://wapi.wangyupu.com/api/nng 获取随机名字 (name键)
func GetRandomName() string {
	response, err := http.Get("https://wapi.wangyupu.com/api/nng")
	if err != nil {
		return "Tempest"
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "Tempest"
	}
	var payload struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return "Tempest"
	}
	if payload.Name == "" {
		return "Tempest"
	}
	return payload.Name
}
