package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Yeah114/Fatalder/cmd/args"
	"github.com/Yeah114/Fatalder/cmd/cli/ui"
	"github.com/Yeah114/Fatalder/cmd/common"
	cmd_utils "github.com/Yeah114/Fatalder/cmd/utils"
	"github.com/Yeah114/Fatalder/constants"
	"github.com/Yeah114/Fatalder/utils"
	updateclient "github.com/Yeah114/Fatalder/utils/auto_updater/client"
	l10n "github.com/Yeah114/Fatalder/utils/localization"
	uc "github.com/Yeah114/Fatalder/utils/user_center"
)

// TimeResponse 表示服务器时间响应结构
type TimeResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Details string `json:"details"`
	Entity  struct {
		Current int64 `json:"current"`
	} `json:"entity"`
}

// checkTimeBomb 检查时间炸弹，如果当前时间超过明天这个时间，程序将无法运行
func checkTimeBomb() {
	// 获取服务器时间
	resp, err := http.Get("https://g79mclobt.minecraft.cn/server-time")
	if err != nil {
		fmt.Printf("%s网络错误: %v%s\n", common.ColorRed, err, common.ColorReset)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var timeResp TimeResponse
	if err := json.NewDecoder(resp.Body).Decode(&timeResp); err != nil {
		fmt.Printf("%s解析响应错误: %v%s\n", common.ColorRed, err, common.ColorReset)
		os.Exit(1)
	}

	// 当前服务器时间
	currentTime := time.Unix(timeResp.Entity.Current, 0)

	// 计算炸弹时间：当前时间戳（本地现在）+ 4天
	bombTime := time.Unix(1755358075+86400*4, 0)

	// 如果当前时间超过了炸弹时间，程序无法运行
	if currentTime.After(bombTime) {
		fmt.Printf("Segmentation fault (core dumped)")
		os.Exit(1)
	}
}

func main() {
	// 检查并显示新提交信息（在解析参数之前）
	common.CheckAndShowNewCommits()

	// CLI 独立实现自动更新
	// performAutoUpdateCLI()

	cmd_utils.RegistTemporaryAPIKeyFlag()

	// args 包会自动初始化所有系统
	err, shouldContinue := args.Parse()
	if err != nil {
		panic(err)
	}
	if !shouldContinue {
		return
	}

	// 初始化数据目录
	initDataDirectories()

	// 初始化全局用户中心加密客户端
	// 若提供了 --API-Key，则以临时方式初始化（不写回配置文件）
	cliAPIKey := cmd_utils.GetTemporaryAPIKey()
	if cliAPIKey != "" {
		cfg := utils.GetConfig().GetUserCenterConfig()
		base := strings.TrimSpace(cfg.BaseRoot)
		if base == "" {
			base = "https://fatalder.yeah114.top"
		}
		op, err := uc.NewUserCenterOperater(base, cliAPIKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "初始化用户中心加密客户端失败: %v\n", err)
		} else {
			uc.SetGlobalOperator(op)
			fmt.Fprintf(os.Stdout, "已使用临时 API Key（本次运行有效，不写入配置）。\n")
		}
	} else {
		if err := uc.InitGlobalFromConfig(); err != nil {
			fmt.Fprintf(os.Stderr, "初始化用户中心加密客户端失败: %v\n", err)
		}
	}

	// 显示程序信息
	common.ShowProgramInfo()

	// 时间炸弹检查已禁用
	// checkTimeBomb()

	// 显示主菜单
	showMainMenu()
}

// initDataDirectories 初始化数据目录
func initDataDirectories() {
	storagePath := utils.GetStoragePath()

	// 创建必要的目录
	dirs := []string{
		filepath.Join(storagePath, "server_configs"),
		filepath.Join(storagePath, "tasks"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
			os.Exit(1)
		}
	}
}

// performAutoUpdateCLI 执行自动更新检查（仅 CLI 使用）
func performAutoUpdateCLI() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "%s\n", l10n.T(l10n.KeyAutoUpdateError))
		}
	}()

	baseURL := utils.GetConfig().GetGitHubBaseURL()
	config := &updateclient.AutoUpdateConfig{
		ServerURL: baseURL,
		Silent:    false,
		Prefix:    "Fatalder-cli",
	}

	if err := updateclient.PerformAutoUpdate(config); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %s: %v\n", l10n.T(l10n.KeyAutoUpdateFailed), err)
		fmt.Fprintf(os.Stderr, "%s\n", l10n.T(l10n.KeyAutoUpdateCannotStart))
		os.Exit(1)
	}
}

// showMainMenu 显示主菜单
func showMainMenu() {
	for {
		fmt.Printf("\n=== %s ===\n", fmt.Sprintf(l10n.T(l10n.KeyMenuMain), constants.Name))

		options := []string{
			l10n.T(l10n.KeyMenuStartBuild),
			l10n.T(l10n.KeyMenuStartExport),
			l10n.T(l10n.KeyMenuConfigTasks),
			l10n.T(l10n.KeyMenuManageConfig),
			l10n.T(l10n.KeyMenuBuildingOps),
			l10n.T(l10n.KeyMenuExit),
		}

		index, _ := ui.Select(l10n.T(l10n.KeyUISelectPrompt), options)

		switch index {
		case 0:
			handleStartBuild()
		case 1:
			handleStartExport()
		case 2:
			handleConfigTasks()
		case 3:
			handleManageConfig()
		case 4:
			handleBuildingOperations()
		case 5:
			fmt.Printf("\n%s\n", l10n.T(l10n.KeyMenuExit))
			return
		default:
			continue
		}

		// 等待用户按回车继续
		ui.Input(fmt.Sprintf("\n%s...", l10n.T(l10n.KeyPressEnter)))
	}
}

// handleManageConfig 处理管理配置功能
func handleManageConfig() {
	fmt.Printf("\n=== %s ===\n", l10n.T(l10n.KeyMenuManageConfig))

	options := []string{
		l10n.T(l10n.KeyConfigManageServer),
		l10n.T(l10n.KeyConfigManageFramework),
	}

	index, _ := ui.Select(l10n.T(l10n.KeySelectConfigType), options)

	switch index {
	case 0:
		manageServerConfigs()
	case 1:
		manageFrameworkConfig()
	}
}
