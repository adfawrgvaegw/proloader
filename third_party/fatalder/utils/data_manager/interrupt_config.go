package data_manager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/Yeah114/Fatalder/defines"
)

const (
	interruptConfigFileSuffix = ".json"
)

func (dm *DataManager) ListInterruptConfigs() (map[string]defines.InterruptConfig, error) {
	dm.interruptConfigMu.RLock()
	defer dm.interruptConfigMu.RUnlock()

	interruptConfigs := make(map[string]defines.InterruptConfig)
	files, err := os.ReadDir(dm.interruptConfigsDirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return interruptConfigs, nil
		}
		return nil, err
	}

	for _, file := range files {
		if !file.IsDir() {
			if configName, ok := strings.CutSuffix(file.Name(), interruptConfigFileSuffix); ok {
				content, err := os.ReadFile(filepath.Join(dm.interruptConfigsDirPath, file.Name()))
				if err != nil {
					return nil, err
				}

				var config defines.InterruptConfig
				if err := json.Unmarshal(content, &config); err != nil {
					return nil, err
				}

				_, found, _ := dm.GetTasksConfig(config.Name)
				if !found {
					continue
				}

				interruptConfigs[configName] = config
			}
		}
	}

	return interruptConfigs, nil
}

func (dm *DataManager) GetInterruptConfig(configName string) (defines.InterruptConfig, bool, error) {
	dm.interruptConfigMu.RLock()
	defer dm.interruptConfigMu.RUnlock()

	filePath := filepath.Join(dm.interruptConfigsDirPath, configName+interruptConfigFileSuffix)
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return defines.InterruptConfig{}, false, nil
		}
		return defines.InterruptConfig{}, false, err
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return defines.InterruptConfig{}, false, err
	}

	var config defines.InterruptConfig
	if err := json.Unmarshal(content, &config); err != nil {
		return defines.InterruptConfig{}, false, err
	}

	_, found, _ := dm.GetTasksConfig(config.Name)
	if !found {
		return defines.InterruptConfig{}, false, nil
	}

	return config, true, nil
}

func (dm *DataManager) SaveInterruptConfig(configName string, config defines.InterruptConfig) error {
	dm.interruptConfigMu.Lock()
	defer dm.interruptConfigMu.Unlock()

	if err := os.MkdirAll(dm.interruptConfigsDirPath, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(dm.interruptConfigsDirPath, configName+interruptConfigFileSuffix)

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	tempFile := filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return err
	}

	return os.Rename(tempFile, filePath)
}

func (dm *DataManager) DeleteInterruptConfig(configName string) error {
	dm.interruptConfigMu.Lock()
	defer dm.interruptConfigMu.Unlock()

	filePath := filepath.Join(dm.interruptConfigsDirPath, configName+interruptConfigFileSuffix)
	return os.Remove(filePath)
}
