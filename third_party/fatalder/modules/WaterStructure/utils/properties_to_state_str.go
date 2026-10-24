package utils

import (
	"errors"
	"reflect"
	"strconv"
)

// PropertiesToStateStr 将属性映射转换为状态字符串
func PropertiesToStateStr(properties map[string]any) (stateStr string) {
	if len(properties) == 0 {
		return "[]"
	}
	stateStr = "["
	for key, value := range properties {
		if stateStr != "[" {
			stateStr = stateStr + ","
		}
		stateStr = stateStr + `"` + key + `"=`
		switch v := value.(type) {
		case string:
			stateStr = stateStr + `"` + v + `"`
		case byte:
			if v == 0 {
				stateStr = stateStr + "false"
			} else {
				stateStr = stateStr + "true"
			}
		case int32:
			stateStr = stateStr + strconv.Itoa(int(v))
		default:
			panic(errors.New("未知的属性值类型: " + reflect.TypeOf(value).String()))
		}
	}
	return stateStr + "]"
}
