package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Yeah114/Fatalder/cmd/cli/ui"
	"github.com/Yeah114/Fatalder/defines"
	"github.com/Yeah114/Fatalder/utils"
	l10n "github.com/Yeah114/Fatalder/utils/localization"
)

// manageServerConfigs 管理服务器配置
func manageServerConfigs() {
	fmt.Printf("\n=== %s ===\n", l10n.T(l10n.KeyConfigManageServer))

	storagePath := utils.GetStoragePath()
	configDir := filepath.Join(storagePath, "server_configs")

	for {
		// 列出所有配置文件
		files, err := os.ReadDir(configDir)
		if err != nil {
			fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
			return
		}

		var configFiles []string
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
				configFiles = append(configFiles, file.Name())
			}
		}

		if len(configFiles) == 0 {
			fmt.Println(l10n.T(l10n.KeyConfigNoFiles))
			if ui.Confirm(l10n.T(l10n.KeyConfigCreateNew)) {
				createNewServerConfig()
			}
			return
		}

		// 添加操作选项
		options := make([]string, len(configFiles)+2)
		copy(options, configFiles)
		options[len(configFiles)] = l10n.T(l10n.KeyConfigCreateNew)
		options[len(configFiles)+1] = l10n.T(l10n.KeyReturn)

		_, selected := ui.Select(l10n.T(l10n.KeyServerConfigSelect), options)

		if selected == l10n.T(l10n.KeyReturn) {
			return
		}

		if selected == l10n.T(l10n.KeyConfigCreateNew) {
			createNewServerConfig()
			continue
		}

		// 管理选中的配置
		manageServerConfig(filepath.Join(configDir, selected))
	}
}

// manageServerConfig 管理单个服务器配置
func manageServerConfig(configPath string) {
	configName := strings.TrimSuffix(filepath.Base(configPath), ".json")

	// 读取配置
	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("%s: %v\n", l10n.T(l10n.KeyErrorReadConfigFile), err)
		return
	}

	var config defines.ControlConfig
	if err := json.Unmarshal(data, &config); err != nil {
		fmt.Printf("%s: %v\n", l10n.T(l10n.KeyErrorParseConfigFile), err)
		return
	}

	for {
		fmt.Printf("\n=== "+l10n.T(l10n.KeyServerConfigTitle)+" ===\n", configName)
		fmt.Printf("%s: %s\n", l10n.T(l10n.KeyServerConfigNumber), config.RentalServerCode)
		fmt.Printf("%s: %s\n", l10n.T(l10n.KeyServerConfigPassword), maskPassword(config.RentalServerPasscode))

		options := []string{
			l10n.T(l10n.KeyServerConfigModifyNumber),
			l10n.T(l10n.KeyServerConfigModifyPass),
			l10n.T(l10n.KeyTaskRename),
			l10n.T(l10n.KeyTaskDelete),
			l10n.T(l10n.KeyReturn),
		}

		index, _ := ui.Select(l10n.T(l10n.KeySelectOperation), options)

		switch index {
		case 0:
			config.RentalServerCode = ui.Input(l10n.T(l10n.KeyServerConfigInputNumber) + ": ")
		case 1:
			config.RentalServerPasscode = ui.Password(l10n.T(l10n.KeyServerConfigInputPass) + ": ")
		case 2:
			if renameServerConfig(configPath) {
				return
			}
		case 3:
			if deleteServerConfig(configPath) {
				return
			}
		case 4:
			return
		}

		// 保存修改（仅当修改了编号或密码时）
		if index == 0 || index == 1 {
			data, _ := json.MarshalIndent(config, "", "  ")
			os.WriteFile(configPath, data, 0644)
			fmt.Println(l10n.T(l10n.KeyConfigSaved))
		}
	}
}

// maskPassword 遮盖密码显示
func maskPassword(password string) string {
	if len(password) == 0 {
		return "[" + l10n.T(l10n.KeyEmpty) + "]"
	}
	if len(password) <= 4 {
		return strings.Repeat("*", len(password))
	}
	return password[:2] + strings.Repeat("*", len(password)-4) + password[len(password)-2:]
}

// renameServerConfig 重命名服务器配置
func renameServerConfig(configPath string) bool {
	oldName := strings.TrimSuffix(filepath.Base(configPath), ".json")
	newName := ui.Input(fmt.Sprintf(l10n.T(l10n.KeyServerConfigInputName)+": ", oldName))

	if newName == "" || newName == oldName {
		fmt.Println(l10n.T(l10n.KeyNameUnchanged))
		return false
	}

	newConfigPath := filepath.Join(filepath.Dir(configPath), newName+".json")

	if err := os.Rename(configPath, newConfigPath); err != nil {
		fmt.Printf("%s: %v\n", l10n.T(l10n.KeyErrorRenameFailed), err)
		return false
	} else {
		fmt.Printf(l10n.T(l10n.KeyServerConfigRenamed)+"\n", newName)
		return true
	}
}

// deleteServerConfig 删除服务器配置
func deleteServerConfig(configPath string) bool {
	configName := strings.TrimSuffix(filepath.Base(configPath), ".json")

	if ui.Confirm(fmt.Sprintf(l10n.T(l10n.KeyServerConfigConfirmDel), configName)) {
		if err := os.Remove(configPath); err != nil {
			fmt.Printf("%s: %v\n", l10n.T(l10n.KeyErrorDeleteFailed), err)
			return false
		} else {
			fmt.Printf(l10n.T(l10n.KeyServerConfigDeleted)+"\n", configName)
			return true
		}
	}
	return false
}
