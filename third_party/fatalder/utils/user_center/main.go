package user_center

import (
	"context"
	"fmt"
	"strings"

	"github.com/Yeah114/Fatalder/utils"
)

// GlobalOperator 全局用户中心加密客户端
var GlobalOperator *UserCenterOperator

// InitGlobalFromConfig 从全局配置读取并初始化全局操作对象。
// 要求：配置中的 BaseRoot(不含 /api) 与 APIKey 均非空时才初始化。
func InitGlobalFromConfig() error {
	cfg := utils.GetConfig().GetUserCenterConfig()
	base := strings.TrimSpace(cfg.BaseRoot)
	key := strings.TrimSpace(cfg.APIKey)
	if base == "" || key == "" {
		// 未配置，跳过初始化，不报错
		return nil
	}
	op, err := NewUserCenterOperater(base, key)
	if err != nil {
		return fmt.Errorf("初始化用户中心加密客户端失败: %w", err)
	}
	GlobalOperator = op
	return nil
}

// SetGlobalOperator 允许手动注入全局对象
func SetGlobalOperator(op *UserCenterOperator) { GlobalOperator = op }

// ChargeImportCost 封装：使用全局操作对象发起扣费请求。
// 若未初始化全局操作对象（未配置 BaseRoot 或 APIKey），返回错误。
func ChargeImportCost(ctx context.Context, payload CostChargePayload) (CostChargeResponse, int, error) {
	if GlobalOperator == nil {
		return CostChargeResponse{}, 0, fmt.Errorf("user center not initialized")
	}
	return GlobalOperator.ChargeImportCost(ctx, payload)
}

// CompleteImport 全局封装：删除 (server, file_hash) 去重记录
func CompleteImport(ctx context.Context, server, fileHash string) (CompleteResponse, int, error) {
	if GlobalOperator == nil {
		return CompleteResponse{}, 0, fmt.Errorf("user center not initialized")
	}
	return GlobalOperator.CompleteImport(ctx, server, fileHash)
}


// GetBillingMode 全局封装：读取计费模式
func GetBillingMode(ctx context.Context) (BillingModeResponse, int, error) {
	if GlobalOperator == nil {
		return BillingModeResponse{}, 0, fmt.Errorf("user center not initialized")
	}
	return GlobalOperator.GetBillingMode(ctx)
}
