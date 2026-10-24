package data_manager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/Yeah114/Fatalder/defines"
)

const (
	serverConfigFileSuffix = ".json"
)

// ListServerConfigs 列出所有服务器配置（原有方法，保持不变）
func (dm *DataManager) ListServerConfigs() (map[string]defines.ControlConfig, error) {
	dm.serverConfigMu.RLock()
	defer dm.serverConfigMu.RUnlock()

	serverConfigs := make(map[string]defines.ControlConfig)
	files, err := os.ReadDir(dm.serverConfigsDirPath)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if !file.IsDir() {
			if configName, ok := strings.CutSuffix(file.Name(), serverConfigFileSuffix); ok {
				content, err := os.ReadFile(filepath.Join(dm.serverConfigsDirPath, file.Name()))
				if err != nil {
					return nil, err
				}

				var config defines.ControlConfig
				if err := json.Unmarshal(content, &config); err != nil {
					return nil, err
				}

				serverConfigs[configName] = config
			}
		}
	}

	return serverConfigs, nil
}

// GetServerConfig 读取单个服务器配置
// configName: 配置名称（不含 .json 后缀）
// 返回值: 配置内容、是否存在、错误信息
func (dm *DataManager) GetServerConfig(configName string) (defines.ControlConfig, bool, error) {
	dm.serverConfigMu.RLock()
	defer dm.serverConfigMu.RUnlock()

	// 拼接完整文件路径
	filePath := filepath.Join(dm.serverConfigsDirPath, configName+serverConfigFileSuffix)

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return defines.ControlConfig{}, false, nil // 文件不存在，无错误
	} else if err != nil {
		return defines.ControlConfig{}, false, err // 其他错误（如权限问题）
	}

	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		return defines.ControlConfig{}, false, err
	}

	// 解析 JSON
	var config defines.ControlConfig
	if err := json.Unmarshal(content, &config); err != nil {
		return defines.ControlConfig{}, false, err
	}

	return config, true, nil
}

// SaveServerConfig 创建或更新服务器配置
// configName: 配置名称（不含 .json 后缀）
// config: 配置内容
// 特性: 自动创建父目录、原子写入（避免文件损坏）、覆盖已有配置
func (dm *DataManager) SaveServerConfig(configName string, config defines.ControlConfig) error {
	dm.serverConfigMu.Lock()
	defer dm.serverConfigMu.Unlock()

	// 1. 确保父目录存在（首次创建时自动创建）
	if err := os.MkdirAll(dm.serverConfigsDirPath, 0755); err != nil {
		return err
	}

	// 2. 拼接文件路径
	filePath := filepath.Join(dm.serverConfigsDirPath, configName+serverConfigFileSuffix)

	// 3. JSON 序列化（带缩进，提高可读性）
	content, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	// 4. 原子写入（先写临时文件，成功后替换目标文件，避免中途崩溃导致文件损坏）
	tempFilePath := filePath + ".tmp"
	if err := os.WriteFile(tempFilePath, content, 0644); err != nil {
		return err
	}

	// 5. 替换目标文件（原子操作，跨平台兼容）
	return os.Rename(tempFilePath, filePath)
}

// DeleteServerConfig 删除服务器配置（可选补充，增强完整性）
func (dm *DataManager) DeleteServerConfig(configName string) error {
	dm.serverConfigMu.Lock()
	defer dm.serverConfigMu.Unlock()

	filePath := filepath.Join(dm.serverConfigsDirPath, configName+serverConfigFileSuffix)
	return os.Remove(filePath)
}
