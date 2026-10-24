package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Yeah114/Fatalder/cmd/cli/ui"
	"github.com/Yeah114/Fatalder/utils"
	l10n "github.com/Yeah114/Fatalder/utils/localization"
	uc "github.com/Yeah114/Fatalder/utils/user_center"
)

// manageFrameworkConfig 管理框架配置
func manageFrameworkConfig() {
	config := utils.GetConfig()

	for {
		buildConfig := config.GetBuildTaskConfig()
		updateConfig := config.GetUpdateConfig()
		extraConfig := config.GetExtraConfig()

		fmt.Printf("\n%s:\n", l10n.T(l10n.KeyFrameworkConfigCurrent))
		fmt.Printf("%s: %s\n", l10n.T(l10n.KeyFrameworkConfigLanguage), l10n.GetLanguageDisplayName(config.GetLanguage()))
		fmt.Printf("%s: %s\n", l10n.T(l10n.KeyFrameworkConfigStorage), utils.GetStoragePath())
		fmt.Printf("%s: %d, %.1f%%, %d\n", l10n.T(l10n.KeyBuildTaskConfigTitle),
			buildConfig.MinOnceParseChunkNum, buildConfig.MinMatchingDegree, buildConfig.MaxFixDepth)
		fmt.Printf("GitHub: %s\n", updateConfig.GitHubBaseURL)
		// 显示用户中心配置简要
		ucCfg := config.GetUserCenterConfig()
		fmt.Printf("UserCenter BaseRoot: %s\n", strings.TrimSpace(ucCfg.BaseRoot))
		if strings.TrimSpace(ucCfg.APIKey) != "" {
			fmt.Printf("UserCenter APIKey: ****** (已设置)\n")
		} else {
			fmt.Printf("UserCenter APIKey: (未设置)\n")
		}
		fmt.Printf("%s: %t\n", l10n.T(l10n.KeyExtraConfigTanLobbyMode), extraConfig.TanLobbyRoomOperateMode)

		options := []string{
			l10n.T(l10n.KeyFrameworkConfigModifyLang),
			l10n.T(l10n.KeyFrameworkConfigModifyPath),
			l10n.T(l10n.KeyFrameworkConfigModifyBuildTask),
			l10n.T(l10n.KeyFrameworkConfigModifyUpdate),
			l10n.T(l10n.KeyFrameworkConfigModifyExtra),
			"配置api-key（用户中心获取）",
			l10n.T(l10n.KeyFrameworkConfigReset),
			l10n.T(l10n.KeyReturn),
		}

		index, _ := ui.Select(l10n.T(l10n.KeyUISelectPrompt), options)

		switch index {
		case 0:
			changeLanguage(config)
		case 1:
			changeStoragePath(config)
		case 2:
			manageBuildTaskConfig(config)
		case 3:
			manageUpdateConfig(config)
		case 4:
			manageExtraConfig(config)
		case 5:
			manageUserCenterConfig(config)
		case 6:
			resetFrameworkConfig(config)
		case 7:
			return
		default:
			continue
		}
	}
}

// manageUserCenterConfig 管理用户中心加密客户端配置
func manageUserCenterConfig(config *utils.Config) {
	current := config.GetUserCenterConfig()
	// 仅允许修改 API Key
	fmt.Printf("当前 APIKey: %s\n", func() string {
		if strings.TrimSpace(current.APIKey) == "" {
			return "(未设置)"
		}
		return "****** (已设置)"
	}())
	apiKey := ui.Input("请输入 X-API-Key（可留空保持不变）: ")
	if strings.TrimSpace(apiKey) == "" {
		apiKey = current.APIKey
	}

	newCfg := current
	newCfg.APIKey = strings.TrimSpace(apiKey)
	if err := config.SetUserCenterConfig(newCfg); err != nil {
		fmt.Printf("保存失败: %v\n", err)
		return
	}
	// 重新初始化全局操作对象
	if err := uc.InitGlobalFromConfig(); err != nil {
		fmt.Printf("初始化失败: %v\n", err)
		return
	}
	fmt.Println("API Key 已更新")
}

// manageUpdateConfig 管理更新配置
func manageUpdateConfig(config *utils.Config) {
	current := config.GetUpdateConfig()
	fmt.Printf("当前 GitHub 源: %s\n", current.GitHubBaseURL)
	newURL := ui.Input("请输入新的 GitHub 基础 URL (默认 https://github.com): ")
	if strings.TrimSpace(newURL) == "" {
		newURL = "https://github.com"
	}
	if err := config.SetGitHubBaseURL(newURL); err != nil {
		fmt.Printf("保存失败: %v\n", err)
		return
	}
	fmt.Printf("已设置 GitHub 源为: %s\n", newURL)
}

// manageExtraConfig 管理扩展配置
func manageExtraConfig(config *utils.Config) {
	extra := config.GetExtraConfig()
	fmt.Printf("\n%s\n", l10n.T(l10n.KeyExtraConfigTitle))
	fmt.Printf("%s: %t\n", l10n.T(l10n.KeyExtraConfigTanLobbyMode), extra.TanLobbyRoomOperateMode)
	newValue := ui.ConfirmWithDefault(l10n.T(l10n.KeyExtraConfigTanLobbyModePrompt), extra.TanLobbyRoomOperateMode)
	if newValue == extra.TanLobbyRoomOperateMode {
		return
	}
	extra.TanLobbyRoomOperateMode = newValue
	if err := config.SetExtraConfig(extra); err != nil {
		fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
		return
	}
	fmt.Println(l10n.T(l10n.KeyOperationSuccess))
}

// changeLanguage 修改语言设置
func changeLanguage(config *utils.Config) {
	// 获取所有可用语言及其显示名称
	languagesWithNames := l10n.GetAvailableLanguagesWithNames()

	// 将 map 转换为有序的切片
	var languages []string
	var languageNames []string

	for lang, displayName := range languagesWithNames {
		languages = append(languages, lang)
		languageNames = append(languageNames, displayName)
	}

	fmt.Printf(l10n.T(l10n.KeyFrameworkConfigCurrentLang)+"\n", l10n.GetLanguageDisplayName(config.GetLanguage()))

	options := make([]string, len(languages))
	for i, lang := range languages {
		if lang == config.GetLanguage() {
			options[i] = fmt.Sprintf("%s (%s)", languageNames[i], l10n.T(l10n.KeyCurrent))
		} else {
			options[i] = languageNames[i]
		}
	}

	index, _ := ui.Select(l10n.T(l10n.KeySelectLanguage), options)

	if index >= 0 && index < len(languages) {
		newLang := languages[index]
		if newLang != config.GetLanguage() {
			config.SetLanguage(newLang)
			l10n.SetCurrentLanguage(newLang)
			fmt.Printf(l10n.T(l10n.KeyFrameworkConfigLangSet)+"\n", languageNames[index])
		}
	}
}

// changeStoragePath 修改存储路径
func changeStoragePath(config *utils.Config) {
	currentPath := utils.GetStoragePath()
	fmt.Printf(l10n.T(l10n.KeyFrameworkConfigCurrentPath)+"\n", currentPath)

	newPath := ui.Input(l10n.T(l10n.KeyFrameworkConfigInputPath) + ": ")
	if newPath == "" {
		return
	}

	// 验证路径
	if err := os.MkdirAll(newPath, 0755); err != nil {
		fmt.Printf("%s: %v\n", l10n.T(l10n.KeyErrorInvalidPath), err)
		return
	}

	config.SetStoragePathInConfig(newPath)
	fmt.Printf(l10n.T(l10n.KeyFrameworkConfigPathSet)+"\n", newPath)
}

// resetFrameworkConfig 重置框架配置
func resetFrameworkConfig(config *utils.Config) {
	if ui.Confirm(l10n.T(l10n.KeyFrameworkConfigConfirmReset)) {
		config.SetLanguage("zh")
		l10n.SetCurrentLanguage("zh")
		config.SetStoragePathInConfig(utils.GetStoragePath())
		// 重置更新配置
		_ = config.SetGitHubBaseURL("https://github.com")
		// 重置用户中心配置
		_ = config.SetUserCenterConfig(utils.UserCenterConfig{
			BaseRoot: "https://fatalder.yeah114.top",
			APIKey:   "",
		})
		fmt.Println(l10n.T(l10n.KeyFrameworkConfigResetDone))
	}
}

// manageBuildTaskConfig 管理构建任务配置
func manageBuildTaskConfig(config *utils.Config) {
	for {
		buildConfig := config.GetBuildTaskConfig()

		fmt.Printf("\n%s:\n", l10n.T(l10n.KeyBuildTaskConfigCurrent))
		fmt.Printf("%s: %d\n", l10n.T(l10n.KeyBuildTaskConfigMinOnceParseChunkNum), buildConfig.MinOnceParseChunkNum)
		fmt.Printf("%s: %.1f%%\n", l10n.T(l10n.KeyBuildTaskConfigMinMatchingDegree), buildConfig.MinMatchingDegree)
		fmt.Printf("%s: %d\n", l10n.T(l10n.KeyBuildTaskConfigMaxFixDepth), buildConfig.MaxFixDepth)

		options := []string{
			l10n.T(l10n.KeyBuildTaskConfigModifyMinOnceParseChunkNum),
			l10n.T(l10n.KeyBuildTaskConfigModifyMinMatchingDegree),
			l10n.T(l10n.KeyBuildTaskConfigModifyMaxFixDepth),
			l10n.T(l10n.KeyReturn),
		}

		index, _ := ui.Select(l10n.T(l10n.KeyUISelectPrompt), options)

		switch index {
		case 0:
			changeMinOnceParseChunkNum(config)
		case 1:
			changeMinMatchingDegree(config)
		case 2:
			changeMaxFixDepth(config)
		case 3:
			return
		default:
			continue
		}
	}
}

// changeMinOnceParseChunkNum 修改单次解析区块数量最小值
func changeMinOnceParseChunkNum(config *utils.Config) {
	currentConfig := config.GetBuildTaskConfig()

	value := ui.InputIntWithDefault(
		fmt.Sprintf(l10n.T(l10n.KeyBuildTaskConfigInputMinOnceParseChunkNum), currentConfig.MinOnceParseChunkNum),
		currentConfig.MinOnceParseChunkNum,
	)

	if value < 1 {
		fmt.Println(l10n.T(l10n.KeyBuildTaskConfigErrorMinOnceParseChunkNum))
		return
	}

	newConfig := currentConfig
	newConfig.MinOnceParseChunkNum = value
	if err := config.SetBuildTaskConfig(newConfig); err != nil {
		fmt.Printf(l10n.T(l10n.KeyBuildTaskConfigErrorSetFailed)+"\n", err)
		return
	}

	fmt.Printf(l10n.T(l10n.KeyBuildTaskConfigMinOnceParseChunkNumSet)+"\n", value)
}

// changeMinMatchingDegree 修改匹配度最小值
func changeMinMatchingDegree(config *utils.Config) {
	currentConfig := config.GetBuildTaskConfig()

	// 使用InputFloat获取浮点数输入
	prompt := fmt.Sprintf(l10n.T(l10n.KeyBuildTaskConfigInputMinMatchingDegree), currentConfig.MinMatchingDegree)
	fmt.Printf("%s: ", prompt)
	value := ui.InputFloat("")

	if value < 0 || value > 100 {
		fmt.Println(l10n.T(l10n.KeyBuildTaskConfigErrorMinMatchingDegree))
		return
	}

	newConfig := currentConfig
	newConfig.MinMatchingDegree = value
	if err := config.SetBuildTaskConfig(newConfig); err != nil {
		fmt.Printf(l10n.T(l10n.KeyBuildTaskConfigErrorSetFailed)+"\n", err)
		return
	}

	fmt.Printf(l10n.T(l10n.KeyBuildTaskConfigMinMatchingDegreeSet)+"\n", value)
}

// changeMaxFixDepth 修改最大修补深度
func changeMaxFixDepth(config *utils.Config) {
	currentConfig := config.GetBuildTaskConfig()

	value := ui.InputIntWithDefault(
		fmt.Sprintf(l10n.T(l10n.KeyBuildTaskConfigInputMaxFixDepth), currentConfig.MaxFixDepth),
		currentConfig.MaxFixDepth,
	)

	if value < 1 {
		fmt.Println(l10n.T(l10n.KeyBuildTaskConfigErrorMaxFixDepth))
		return
	}

	newConfig := currentConfig
	newConfig.MaxFixDepth = value
	if err := config.SetBuildTaskConfig(newConfig); err != nil {
		fmt.Printf(l10n.T(l10n.KeyBuildTaskConfigErrorSetFailed)+"\n", err)
		return
	}

	fmt.Printf(l10n.T(l10n.KeyBuildTaskConfigMaxFixDepthSet)+"\n", value)
}

// toggleWaitChunkLoad removed – handled per build task
