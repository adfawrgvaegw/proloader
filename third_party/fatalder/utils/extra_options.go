package utils

import (
	"encoding/json"
	"os"
)

// ExtraOptions 存储从环境变量读取的配置项
type ExtraOptions struct {
	// 是否跳过导出操作员权限检查，默认false
	SkipExportOpCheck bool `json:"skip_export_op_check"`
	// 是否使用无操作员权限的传送，默认false
	UseOplessTp bool `json:"use_opless_tp"`
	// 是否解除导出的限制，默认false
	UnlimitedExport bool `json:"unlimited_export"`
}

// 全局实例，供外部包直接使用
var GlobalExtraOptions ExtraOptions

func GetExtraOptions() ExtraOptions {
	return GlobalExtraOptions
}

func init() {
	// 读取环境变量 ExtraOptions 的值
	envVal := os.Getenv("ExtraOptions")
	if envVal == "" {
		// 环境变量为空时，使用默认值（false），不报错（静默）
		GlobalExtraOptions = ExtraOptions{}
		return
	}

	// JSON解码到结构体
	if err := json.Unmarshal([]byte(envVal), &GlobalExtraOptions); err != nil {
		// 解码失败时初始化默认值
		GlobalExtraOptions = ExtraOptions{}
	}
}
