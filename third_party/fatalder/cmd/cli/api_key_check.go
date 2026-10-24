package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Yeah114/Fatalder/utils"
	uc "github.com/Yeah114/Fatalder/utils/user_center"
)

// ensureValidUserCenterAPIKey 在启动构建前校验用户中心 API Key。
// 若未配置或无效，则引导用户进入配置界面，直至校验通过或超过重试次数。
func ensureValidUserCenterAPIKey() bool {
	const maxAuthAttempts = 3
	authTries := 0
	for {
		// 若已有全局对象（可能来自 --API-Key 临时注入），优先使用它进行鉴权
		if uc.GlobalOperator != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, status, err := uc.CompleteImport(ctx, "__verify__", "__verify__")
			cancel()
			if err != nil {
				if status == 401 || strings.Contains(strings.ToLower(err.Error()), "http 401") {
					authTries++
					if authTries >= maxAuthAttempts {
						fmt.Println("[失败] 多次尝试仍未通过 API Key 校验，已取消操作。")
						return false
					}
					fmt.Println("[错误] API Key 无效或未授权。将跳转到设置界面...")
					// 清空临时全局对象，转为从配置读取
					uc.SetGlobalOperator(nil)
					manageUserCenterConfig(utils.GetConfig())
					continue
				}
				// 其他错误（例如 400/404/5xx）说明鉴权通过
				return true
			}
			// 成功响应，视为校验通过
			return true
		}

		// 无全局对象，则按配置文件流程
		cfg := utils.GetConfig().GetUserCenterConfig()
		base := strings.TrimSpace(cfg.BaseRoot)
		key := strings.TrimSpace(cfg.APIKey)

		if base == "" || key == "" {
			fmt.Println("[提示] 未配置 API Key，将跳转到设置界面进行配置...")
			manageUserCenterConfig(utils.GetConfig())
			continue
		}

		// 重新初始化全局操作对象
		if err := uc.InitGlobalFromConfig(); err != nil || uc.GlobalOperator == nil {
			fmt.Printf("[错误] 初始化用户中心失败: %v\n", err)
			manageUserCenterConfig(utils.GetConfig())
			continue
		}

		// 使用受限接口做鉴权校验：需要有效 X-API-Key 才不会返回 401
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, status, err := uc.CompleteImport(ctx, "__verify__", "__verify__")
		cancel()
		if err != nil {
			// 401 明确表示未授权，API Key 无效
			if status == 401 || strings.Contains(strings.ToLower(err.Error()), "http 401") {
				authTries++
				if authTries >= maxAuthAttempts {
					fmt.Println("[失败] 多次尝试仍未通过 API Key 校验，已取消操作。")
					return false
				}
				fmt.Println("[错误] API Key 无效或未授权。将跳转到设置界面...")
				manageUserCenterConfig(utils.GetConfig())
				continue
			}
			// 其他错误（例如 400/404/5xx）说明已通过鉴权，业务失败即可
			return true
		}

		// 成功响应，视为校验通过
		return true
	}
}
