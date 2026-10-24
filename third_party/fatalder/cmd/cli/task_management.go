package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Yeah114/Fatalder/cmd/cli/ui"
	"github.com/Yeah114/Fatalder/control/task"
	"github.com/Yeah114/Fatalder/defines"
	"github.com/Yeah114/Fatalder/utils"
	l10n "github.com/Yeah114/Fatalder/utils/localization"
)

// handleConfigTasks 处理配置任务功能
func handleConfigTasks() {
	fmt.Printf("\n=== %s ===\n", l10n.T(l10n.KeyMenuConfigTasks))

	storagePath := utils.GetStoragePath()
	tasksDir := filepath.Join(storagePath, "tasks")

	for {
		// 列出所有任务组文件
		files, err := os.ReadDir(tasksDir)
		if err != nil {
			fmt.Printf("%s: %v\n", l10n.T(l10n.KeyErrorReadTaskDir), err)
			return
		}

		var taskFiles []string
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
				taskFiles = append(taskFiles, file.Name())
			}
		}

		// 添加新建和返回选项
		options := make([]string, len(taskFiles)+2)
		copy(options, taskFiles)
		options[len(taskFiles)] = l10n.T(l10n.KeyTaskGroupCreate)
		options[len(taskFiles)+1] = l10n.T(l10n.KeyReturn)

		if len(taskFiles) == 0 {
			fmt.Println(l10n.T(l10n.KeyTaskGroupNoFiles))
			options = []string{l10n.T(l10n.KeyTaskGroupCreate), l10n.T(l10n.KeyReturn)}
		}

		index, selected := ui.Select(l10n.T(l10n.KeyTaskGroupManage), options)
		if index < 0 {
			return
		}

		if selected == l10n.T(l10n.KeyReturn) {
			return
		}

		if selected == l10n.T(l10n.KeyTaskGroupCreate) {
			createNewTaskGroup()
			continue
		}

		// 管理选中的任务组
		manageTaskGroup(filepath.Join(tasksDir, selected))
	}
}

// saveTaskGroup 保存任务组（简单的任务列表）
func saveTaskGroup(tasks []defines.Task) error {
	storagePath := utils.GetStoragePath()
	tasksDir := filepath.Join(storagePath, "tasks")

	// 输入任务组名称（作为文件名）
	defaultName := time.Now().Format("2006-01-02_15-04-05")
	groupName := ui.InputWithDefault(l10n.T(l10n.KeyTaskGroupName), defaultName)

	// 保存任务列表
	fileName := groupName + ".json"
	taskPath := filepath.Join(tasksDir, fileName)

	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(taskPath, data, 0644); err != nil {
		return err
	}

	fmt.Printf("%s\n", fmt.Sprintf(l10n.T(l10n.KeyOperationSuccess), fileName))
	return nil
}

// createNewTaskGroup 创建新任务组
func createNewTaskGroup() {
	fmt.Printf("\n=== %s ===\n", l10n.T(l10n.KeyTaskGroupCreate))

	// 选择任务类型
	options := []string{
		l10n.T(l10n.KeyTaskGroupCreateBuild),
		l10n.T(l10n.KeyTaskGroupCreateExport),
		l10n.T(l10n.KeyTaskGroupCreateMixed),
	}

	index, _ := ui.Select(l10n.T(l10n.KeyTaskGroupSelectType), options)

	var buildTasks []task.BuildTask
	var exportTasks []task.ExportTask
	var err error

	switch index {
	case 0:
		buildTasks, err = buildBuildTasks()
	case 1:
		exportTasks, err = buildExportTasks()
	case 2:
		// 混合任务组
		if ui.Confirm(l10n.T(l10n.KeyTaskAddBuildConfirm)) {
			buildTasks, err = buildBuildTasks()
			if err != nil {
				fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
				return
			}
		}
		if ui.Confirm(l10n.T(l10n.KeyTaskAddExportConfirm)) {
			exportTasks, err = buildExportTasks()
		}
	default:
		return
	}

	if err != nil {
		fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
		return
	}

	// 保存任务组
	if ui.Confirm(l10n.T(l10n.KeyTaskSaveGroup)) {
		tasks := convertMixedTasksToDefinesTask(buildTasks, exportTasks)
		saveTaskGroup(tasks)
	}
}

// manageTaskGroup 管理任务组
func manageTaskGroup(taskFilePath string) {
	fileName := filepath.Base(taskFilePath)
	taskGroupName := strings.TrimSuffix(fileName, ".json")

	fmt.Printf("\n=== %s ===\n", fmt.Sprintf(l10n.T(l10n.KeyTaskGroupManage), taskGroupName))

	// 读取任务组文件
	data, err := os.ReadFile(taskFilePath)
	if err != nil {
		fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
		return
	}

	// 解析任务列表
	var tasks []json.RawMessage
	if err := json.Unmarshal(data, &tasks); err != nil {
		fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
		return
	}

	for {
		// 显示任务列表
		fmt.Printf("\n"+l10n.T(l10n.KeyTaskGroupInfo)+"\n", taskGroupName, len(tasks))
		for i, taskData := range tasks {
			displayTaskSummary(i+1, taskData)
		}

		// 操作选项
		options := []string{
			l10n.T(l10n.KeyTaskGroupRunOption),
			l10n.T(l10n.KeyTaskModify),
			l10n.T(l10n.KeyTaskRename),
			l10n.T(l10n.KeyTaskDelete),
			l10n.T(l10n.KeyReturn),
		}

		index, _ := ui.Select(l10n.T(l10n.KeySelectOperation), options)

		switch index {
		case 0:
			executeTaskGroup(taskFilePath)
		case 1:
			modifyTaskGroup(taskFilePath, &tasks)
		case 2:
			renameTaskGroup(taskFilePath)
			return
		case 3:
			if deleteTaskGroup(taskFilePath) {
				return
			}
		case 4:
			return
		}
	}
}

// displayTaskSummary 显示任务摘要
func displayTaskSummary(index int, taskData json.RawMessage) {
	var taskInfo map[string]interface{}
	if err := json.Unmarshal(taskData, &taskInfo); err != nil {
		fmt.Printf("  %d. [%s] %v\n", index, l10n.T(l10n.KeyErrorParseTask), err)
		return
	}

	taskType, _ := taskInfo["type"].(string)
	filePath, _ := taskInfo["file_path"].(string)

	switch taskType {
	case "build":
		speed, _ := taskInfo["speed"].(float64)
		fmt.Printf("  %d. [%s] %s (%s: %.0f)\n", index, l10n.T(l10n.KeyTaskTypeBuild), filePath, l10n.T(l10n.KeyTaskSpeedLabel), speed)
	case "export":
		fileType, _ := taskInfo["file_type"].(string)
		fmt.Printf("  %d. [%s] %s (%s: %s)\n", index, l10n.T(l10n.KeyTaskTypeExport), filePath, l10n.T(l10n.KeyTaskTypeLabel), fileType)
	default:
		fmt.Printf("  %d. [%s] %s\n", index, l10n.T(l10n.KeyTaskTypeUnknown), filePath)
	}
}

// executeTaskGroup 执行任务组
func executeTaskGroup(taskFilePath string) {
	fileName := filepath.Base(taskFilePath)
	taskGroupName := strings.TrimSuffix(fileName, ".json")

	fmt.Printf("\n=== %s ===\n", fmt.Sprintf(l10n.T(l10n.KeyTaskGroupRunTitle), taskGroupName))

	// 选择服务器配置
	serverConfig, err := selectServerConfig()
	if err != nil {
		fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
		return
	}

	// 读取任务组文件
	data, err := os.ReadFile(taskFilePath)
	if err != nil {
		fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
		return
	}

	// 解析任务列表
	var rawTasks []json.RawMessage
	if err := json.Unmarshal(data, &rawTasks); err != nil {
		fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
		return
	}

	// 分离构建任务和导出任务
	var buildTasks []task.BuildTask
	var exportTasks []task.ExportTask

	for _, rawTask := range rawTasks {
		var taskInfo map[string]interface{}
		if err := json.Unmarshal(rawTask, &taskInfo); err != nil {
			fmt.Printf(l10n.T(l10n.KeyTaskGroupSkipInvalid)+"\n", err)
			continue
		}

		taskType, _ := taskInfo["type"].(string)
		switch taskType {
		case "build":
			var buildTask task.BuildTask
			if err := json.Unmarshal(rawTask, &buildTask); err != nil {
				fmt.Printf(l10n.T(l10n.KeyTaskGroupSkipInvalidBuild)+"\n", err)
				continue
			}
			buildTasks = append(buildTasks, buildTask)
		case "export":
			var exportTask task.ExportTask
			if err := json.Unmarshal(rawTask, &exportTask); err != nil {
				fmt.Printf(l10n.T(l10n.KeyTaskGroupSkipInvalidExport)+"\n", err)
				continue
			}
			exportTasks = append(exportTasks, exportTask)
		}
	}

	// 确认执行
	totalTasks := len(buildTasks) + len(exportTasks)
	if totalTasks == 0 {
		fmt.Println(l10n.T(l10n.KeyTaskGroupEmpty))
		return
	}

	fmt.Printf(l10n.T(l10n.KeyTaskGroupSummary)+"\n", totalTasks, len(buildTasks), len(exportTasks))
	if !ui.Confirm(l10n.T(l10n.KeyTaskGroupExecuteConfirm)) {
		fmt.Println(l10n.T(l10n.KeyTaskExecutionCancelled))
		return
	}

	// 执行构建任务
	if len(buildTasks) > 0 {
		if err := executeBuildTasks(serverConfig, buildTasks, "", true); err != nil {
			fmt.Printf(l10n.T(l10n.KeyBuildTaskExecuteFailed)+"\n", err)
			return
		}
	}

	// 执行导出任务
	if len(exportTasks) > 0 {
		if err := executeExportTasks(serverConfig, exportTasks); err != nil {
			fmt.Printf(l10n.T(l10n.KeyExportTaskExecuteFailed)+"\n", err)
			return
		}
	}

	fmt.Printf("%s\n", l10n.T(l10n.KeyTaskExecuteComplete))
}
