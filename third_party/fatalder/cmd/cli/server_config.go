package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Yeah114/Fatalder/cmd/cli/ui"
	"github.com/Yeah114/Fatalder/defines"
	"github.com/Yeah114/Fatalder/utils"
	l10n "github.com/Yeah114/Fatalder/utils/localization"
)

// LastServerConfig 上次使用的服务器配置
type LastServerConfig struct {
	Config     defines.ControlConfig `json:"config"`      // 服务器配置
	UsedAt     time.Time             `json:"used_at"`     // 使用时间
	ConfigFile string                `json:"config_file"` // 配置文件名
}

// selectServerConfig 选择服务器配置
func selectServerConfig() (defines.ControlConfig, error) {
	storagePath := utils.GetStoragePath()
	lastConfigPath := filepath.Join(storagePath, "last_server.json")

	// 检查是否存在上次配置
	if _, err := os.Stat(lastConfigPath); err == nil {
		if ui.ConfirmWithDefault(l10n.T(l10n.KeyServerConfigUseLast), true) {
			return loadLastServerConfig(lastConfigPath)
		}
	}

	// 显示配置选择菜单
	options := []string{
		l10n.T(l10n.KeyServerConfigCreateNew),
		l10n.T(l10n.KeyServerConfigSelectExist),
	}

	index, _ := ui.Select(l10n.T(l10n.KeyServerConfigSelect), options)

	switch index {
	case 0:
		return createNewServerConfig()
	case 1:
		return selectExistingServerConfig()
	default:
		return defines.ControlConfig{}, fmt.Errorf("operation cancelled")
	}
}

// loadLastServerConfig 加载上次使用的服务器配置
func loadLastServerConfig(path string) (defines.ControlConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return defines.ControlConfig{}, err
	}

	var lastConfig LastServerConfig
	if err := json.Unmarshal(data, &lastConfig); err != nil {
		return defines.ControlConfig{}, err
	}

	return lastConfig.Config, nil
}

// createNewServerConfig 创建新的服务器配置
func createNewServerConfig() (defines.ControlConfig, error) {
	config := defines.ControlConfig{}

	// 输入服务器信息
	config.RentalServerCode = ui.Input(l10n.T(l10n.KeyServerNumber) + ": ")
	if config.RentalServerCode == "" {
		return config, fmt.Errorf("server number is required")
	}

	password := ui.Password(l10n.T(l10n.KeyServerPassword) + ": ")
	if password == "" {
		password = "" // 默认密码
	}
	config.RentalServerPasscode = password

	// 询问是否保存配置
	if ui.Confirm(l10n.T(l10n.KeyServerSaveConfig)) {
		fileName := ui.InputWithDefault(l10n.T(l10n.KeyServerFileName), config.RentalServerCode)
		if err := saveServerConfig(config, fileName); err != nil {
			fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
		} else {
			fmt.Println(l10n.T(l10n.KeyServerConfigSaved))
		}
	}

	// 保存为上次使用的配置
	saveLastServerConfig(config, "")

	return config, nil
}

// selectExistingServerConfig 选择已有的服务器配置
func selectExistingServerConfig() (defines.ControlConfig, error) {
	storagePath := utils.GetStoragePath()
	configDir := filepath.Join(storagePath, "server_configs")

	// 读取配置文件列表
	files, err := os.ReadDir(configDir)
	if err != nil {
		return defines.ControlConfig{}, err
	}

	var configFiles []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			configFiles = append(configFiles, file.Name())
		}
	}

	if len(configFiles) == 0 {
		fmt.Println(l10n.T(l10n.KeyServerConfigNoFiles))
		return defines.ControlConfig{}, fmt.Errorf("no config files found")
	}

	// 显示文件选择
	index, selected := ui.Select(l10n.T(l10n.KeyServerConfigSelectExist), configFiles)
	if index < 0 {
		return defines.ControlConfig{}, fmt.Errorf("operation cancelled")
	}

	// 加载选中的配置
	configPath := filepath.Join(configDir, selected)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return defines.ControlConfig{}, err
	}

	var config defines.ControlConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return defines.ControlConfig{}, err
	}

	// 保存为上次使用的配置
	saveLastServerConfig(config, selected)

	return config, nil
}

// saveServerConfig 保存服务器配置
func saveServerConfig(config defines.ControlConfig, fileName string) error {
	storagePath := utils.GetStoragePath()
	configDir := filepath.Join(storagePath, "server_configs")

	if !strings.HasSuffix(fileName, ".json") {
		fileName += ".json"
	}

	configPath := filepath.Join(configDir, fileName)
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// saveLastServerConfig 保存上次使用的服务器配置
func saveLastServerConfig(config defines.ControlConfig, configFile string) error {
	storagePath := utils.GetStoragePath()
	lastConfigPath := filepath.Join(storagePath, "last_server.json")

	lastConfig := LastServerConfig{
		Config:     config,
		UsedAt:     time.Now(),
		ConfigFile: configFile,
	}

	data, err := json.MarshalIndent(lastConfig, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(lastConfigPath, data, 0644)
}
