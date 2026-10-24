package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Yeah114/Fatalder/cmd/cli/ui"
	l10n "github.com/Yeah114/Fatalder/utils/localization"
)

// modifyTaskGroup 修改任务组
func modifyTaskGroup(taskFilePath string, tasks *[]json.RawMessage) {
	fmt.Printf("\n=== %s ===\n", l10n.T(l10n.KeyTaskModify))

	if len(*tasks) == 0 {
		fmt.Println(l10n.T(l10n.KeyTaskGroupEmpty))
		return
	}

	// 显示任务列表供选择
	options := make([]string, len(*tasks)+2)
	for i, taskData := range *tasks {
		var taskInfo map[string]interface{}
		json.Unmarshal(taskData, &taskInfo)
		taskType, _ := taskInfo["type"].(string)
		filePath, _ := taskInfo["file_path"].(string)
		options[i] = fmt.Sprintf("[%s] %s", taskType, filePath)
	}
	options[len(*tasks)] = l10n.T(l10n.KeyTaskAddNew)
	options[len(*tasks)+1] = l10n.T(l10n.KeyReturn)

	index, selected := ui.Select(l10n.T(l10n.KeySelectToModify), options)

	if selected == l10n.T(l10n.KeyReturn) {
		return
	}

	if selected == l10n.T(l10n.KeyTaskAddNew) {
		addNewTaskToGroup(taskFilePath, tasks)
		return
	}

	if index >= 0 && index < len(*tasks) {
		// 删除选中的任务
		if ui.Confirm(fmt.Sprintf(l10n.T(l10n.KeyConfirmDelete), selected)) {
			*tasks = append((*tasks)[:index], (*tasks)[index+1:]...)

			// 保存修改
			data, _ := json.MarshalIndent(*tasks, "", "  ")
			os.WriteFile(taskFilePath, data, 0644)
			fmt.Println(l10n.T(l10n.KeyTaskDeleted))
		}
	}
}

// renameTaskGroup 重命名任务组
func renameTaskGroup(taskFilePath string) {
	oldName := strings.TrimSuffix(filepath.Base(taskFilePath), ".json")
	newName := ui.Input(fmt.Sprintf("%s (%s: %s): ", l10n.T(l10n.KeyEnterNewName), l10n.T(l10n.KeyCurrentName), oldName))

	if newName == "" || newName == oldName {
		fmt.Println(l10n.T(l10n.KeyNameUnchanged))
		return
	}

	newFilePath := filepath.Join(filepath.Dir(taskFilePath), newName+".json")

	if err := os.Rename(taskFilePath, newFilePath); err != nil {
		fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
	} else {
		fmt.Printf("%s\n", fmt.Sprintf(l10n.T(l10n.KeyTaskGroupRenamed), newName))
	}
}

// deleteTaskGroup 删除任务组
func deleteTaskGroup(taskFilePath string) bool {
	taskGroupName := strings.TrimSuffix(filepath.Base(taskFilePath), ".json")

	if ui.Confirm(fmt.Sprintf(l10n.T(l10n.KeyConfirmDelete), taskGroupName)) {
		if err := os.Remove(taskFilePath); err != nil {
			fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
			return false
		} else {
			fmt.Printf("%s\n", fmt.Sprintf(l10n.T(l10n.KeyTaskGroupDeleted), taskGroupName))
			return true
		}
	}
	return false
}

// addNewTaskToGroup 向任务组添加新任务
func addNewTaskToGroup(taskFilePath string, tasks *[]json.RawMessage) {
	fmt.Printf("\n=== %s ===\n", l10n.T(l10n.KeyTaskAddNew))

	// 选择任务类型
	options := []string{
		l10n.T(l10n.KeyTaskAddBuild),
		l10n.T(l10n.KeyTaskAddExport),
		l10n.T(l10n.KeyCancel),
	}

	index, _ := ui.Select(l10n.T(l10n.KeySelectTaskType), options)

	switch index {
	case 0:
		addBuildTaskToGroup(taskFilePath, tasks)
	case 1:
		addExportTaskToGroup(taskFilePath, tasks)
	case 2:
		return
	}
}

// addBuildTaskToGroup 添加导入任务到任务组
func addBuildTaskToGroup(taskFilePath string, tasks *[]json.RawMessage) {
	// 创建构建任务
	buildTask, err := createSingleBuildTask()
	if err != nil {
		fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
		return
	}

	// 将任务转换为 JSON
	taskData, err := json.Marshal(&buildTask)
	if err != nil {
		fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
		return
	}

	// 添加到任务列表
	*tasks = append(*tasks, json.RawMessage(taskData))

	// 保存任务组
	data, _ := json.MarshalIndent(*tasks, "", "  ")
	os.WriteFile(taskFilePath, data, 0644)

	fmt.Println(l10n.T(l10n.KeyTaskAdded))
}

// addExportTaskToGroup 添加导出任务到任务组
func addExportTaskToGroup(taskFilePath string, tasks *[]json.RawMessage) {
	// 创建导出任务
	exportTask, err := createSingleExportTask()
	if err != nil {
		fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
		return
	}

	// 将任务转换为 JSON
	taskData, err := json.Marshal(&exportTask)
	if err != nil {
		fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
		return
	}

	// 添加到任务列表
	*tasks = append(*tasks, json.RawMessage(taskData))

	// 保存任务组
	data, _ := json.MarshalIndent(*tasks, "", "  ")
	os.WriteFile(taskFilePath, data, 0644)

	fmt.Println(l10n.T(l10n.KeyTaskAdded))
}
