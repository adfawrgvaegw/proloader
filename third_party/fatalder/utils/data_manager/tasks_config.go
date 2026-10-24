package data_manager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Yeah114/Fatalder/control/task"
	"github.com/Yeah114/Fatalder/defines"
)

const (
	tasksConfigFileSuffix = ".json"
)

type TaskType struct {
	Type task.TaskType `json:"type"`
}

func (dm *DataManager) ListTasksConfigs() (map[string][]defines.Task, error) {
	dm.tasksConfigMu.RLock()
	defer dm.tasksConfigMu.RUnlock()

	tasksConfigs := make(map[string][]defines.Task)
	files, err := os.ReadDir(dm.tasksConfigsDirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return tasksConfigs, nil
		}
		return nil, err
	}

	for _, file := range files {
		if !file.IsDir() {
			if configName, ok := strings.CutSuffix(file.Name(), tasksConfigFileSuffix); ok {
				content, err := os.ReadFile(filepath.Join(dm.tasksConfigsDirPath, file.Name()))
				if err != nil {
					return nil, err
				}

				var rawTasks []json.RawMessage
				if err := json.Unmarshal(content, &rawTasks); err != nil {
					return nil, err
				}

				tasks := make([]defines.Task, 0, len(rawTasks))
				for _, rawTask := range rawTasks {
					taskInstance, err := dm.decodeTask(rawTask)
					if err != nil {
						return nil, fmt.Errorf("parse task in %s: %w", file.Name(), err)
					}
					if taskInstance != nil {
						tasks = append(tasks, taskInstance)
					}
				}

				if len(tasks) != 0 {
					tasksConfigs[configName] = tasks
				}
			}
		}
	}

	return tasksConfigs, nil
}

func (dm *DataManager) GetTasksConfig(configName string) ([]defines.Task, bool, error) {
	dm.tasksConfigMu.RLock()
	defer dm.tasksConfigMu.RUnlock()

	filePath := filepath.Join(dm.tasksConfigsDirPath, configName+tasksConfigFileSuffix)
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, false, err
	}

	var rawTasks []json.RawMessage
	if err := json.Unmarshal(content, &rawTasks); err != nil {
		return nil, false, err
	}

	tasks := make([]defines.Task, 0, len(rawTasks))
	for _, raw := range rawTasks {
		t, err := dm.decodeTask(raw)
		if err != nil {
			return nil, false, err
		}
		if t != nil {
			tasks = append(tasks, t)
		}
	}

	return tasks, true, nil
}

func (dm *DataManager) SaveTasksConfig(configName string, tasks []defines.Task) error {
	dm.tasksConfigMu.Lock()
	defer dm.tasksConfigMu.Unlock()

	if err := os.MkdirAll(dm.tasksConfigsDirPath, 0755); err != nil {
		return err
	}

	rawTasks := make([]json.RawMessage, 0, len(tasks))
	for i, t := range tasks {
		if t == nil {
			continue
		}
		jsonStr, err := t.ToJSON()
		if err != nil {
			return fmt.Errorf("serialize task %d: %w", i, err)
		}
		rawTasks = append(rawTasks, json.RawMessage([]byte(jsonStr)))
	}

	data, err := json.MarshalIndent(rawTasks, "", "  ")
	if err != nil {
		return err
	}

	filePath := filepath.Join(dm.tasksConfigsDirPath, configName+tasksConfigFileSuffix)
	tempFile := filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return err
	}

	return os.Rename(tempFile, filePath)
}

func (dm *DataManager) DeleteTasksConfig(configName string) error {
	dm.tasksConfigMu.Lock()
	defer dm.tasksConfigMu.Unlock()

	filePath := filepath.Join(dm.tasksConfigsDirPath, configName+tasksConfigFileSuffix)
	return os.Remove(filePath)
}

func (dm *DataManager) decodeTask(raw json.RawMessage) (defines.Task, error) {
	var t TaskType
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, err
	}

	taskInstance := t.Type.GetTask()
	if taskInstance == nil {
		return nil, nil
	}

	if err := taskInstance.FromJSON(string(raw)); err != nil {
		return nil, err
	}
	return taskInstance, nil
}
