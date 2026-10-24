package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	structure_define "github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/WaterStructure/structure"
	"github.com/schollz/progressbar/v3"

	"github.com/Yeah114/Fatalder/cmd/cli/ui"
	"github.com/Yeah114/Fatalder/control"
	"github.com/Yeah114/Fatalder/control/task"
	"github.com/Yeah114/Fatalder/defines"
	"github.com/Yeah114/Fatalder/utils"
	l10n "github.com/Yeah114/Fatalder/utils/localization"
)

// 全局变量用于存储外部InterruptJSON文件路径
var externalInterruptJSONPath string

// Coordinates 坐标（用于输入）
type Coordinates struct {
	X int `json:"x"`
	Y int `json:"y"`
	Z int `json:"z"`
}

// handleStartExport 处理启动导出功能
func handleStartExport() {
	fmt.Printf("\n=== %s ===\n", l10n.T(l10n.KeyExportTitle))

	serverConfig, err := selectServerConfig()
	if err != nil {
		fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
		return
	}

	exportTasks, err := buildExportTasks()
	if err != nil {
		fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
		return
	}

	if len(exportTasks) == 0 {
		fmt.Println(l10n.T(l10n.KeyOperationCancel))
		return
	}

	if ui.Confirm(l10n.T(l10n.KeyTaskSaveGroup)) {
		tasks := convertTasksToDefinesTask(exportTasks)
		if err := saveTaskGroup(tasks); err != nil {
			fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
		}
	}

	if !ui.ConfirmWithDefault(l10n.T(l10n.KeyTaskExecuteNow), true) {
		return
	}

	if err := executeExportTasks(serverConfig, exportTasks); err != nil {
		fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
		return
	}

	fmt.Printf("%s\n", l10n.T(l10n.KeyTaskExecuteComplete))
}

// handleStartBuild 处理启动构建功能
func handleStartBuild() {
	fmt.Printf("\n=== %s ===\n", l10n.T(l10n.KeyBuildTitle))

	if !ensureValidUserCenterAPIKey() {
		return
	}

	serverConfig, err := selectServerConfig()
	if err != nil {
		fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
		return
	}

	interruptData, interruptFile, err := handleBuildInterrupt()
	if err != nil {
		fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
		return
	}

	var buildTasks []task.BuildTask
	if interruptData != nil {
		if ui.Confirm(l10n.T(l10n.KeyBuildAppendNewTask)) {
			buildTasks, err = buildBuildTasks()
			if err != nil {
				fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
				return
			}
		}
	} else {
		buildTasks, err = buildBuildTasks()
		if err != nil {
			fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
			return
		}
	}

	hasResume := interruptData != nil
	hasNewTasks := len(buildTasks) > 0

	if !hasResume && !hasNewTasks {
		fmt.Println(l10n.T(l10n.KeyOperationCancel))
		return
	}

	if hasNewTasks && ui.Confirm(l10n.T(l10n.KeyTaskSaveGroup)) {
		tasks := convertTasksToDefinesTask(buildTasks)
		saveTaskGroup(tasks)
	}

	if !ui.ConfirmWithDefault(l10n.T(l10n.KeyTaskExecuteNow), true) {
		return
	}

	if hasResume {
		if err := resumeBuildFromInterrupt(*interruptData, serverConfig, interruptFile, buildTasks, false); err != nil {
			fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
			return
		}
	} else {
		if err := executeBuildTasks(serverConfig, buildTasks, "", true); err != nil {
			fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
			return
		}
	}

	fmt.Printf("%s\n", l10n.T(l10n.KeyTaskExecuteComplete))
}

// buildExportTasks 构建导出任务
func buildExportTasks() ([]task.ExportTask, error) {
	// 选择任务类型
	options := []string{
		l10n.T(l10n.KeyModeSingleExport),
		l10n.T(l10n.KeyModeMultipleExport),
	}

	index, _ := ui.Select(l10n.T(l10n.KeyExportModePrompt), options)

	var tasks []task.ExportTask

	switch index {
	case 0:
		// 单导出
		task, err := createSingleExportTask()
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	case 1:
		// 多导出
		for {
			task, err := createSingleExportTask()
			if err != nil {
				return nil, err
			}
			tasks = append(tasks, task)

			if !ui.Confirm(l10n.T(l10n.KeyExportAddAnotherTask)) {
				break
			}
		}
	default:
		return nil, fmt.Errorf("operation cancelled")
	}

	return tasks, nil
}

// createSingleExportTask 创建单个导出任务
func createSingleExportTask() (task.ExportTask, error) {
	exportTask := task.ExportTask{
		TaskType: task.TaskTypeExport,
	}

	// 输入文件名
	fileName := ui.Input(l10n.T(l10n.KeyExportFilePrompt) + ": ")
	if fileName == "" {
		return exportTask, fmt.Errorf("file name is required")
	}
	exportTask.FilePath = fileName

	// 文件类型（目前只支持 mcworld）
	exportTask.FileType = "mcworld"

	// 输入起始坐标
	startCoord, err := inputCoordinates(l10n.T(l10n.KeyTaskStartCoord))
	if err != nil {
		return exportTask, err
	}
	exportTask.StartPos = defines.BlockPos{int32(startCoord.X), int32(startCoord.Y), int32(startCoord.Z)}

	// 输入终止坐标
	endCoord, err := inputCoordinates(l10n.T(l10n.KeyTaskEndCoord))
	if err != nil {
		return exportTask, err
	}
	exportTask.EndPos = defines.BlockPos{int32(endCoord.X), int32(endCoord.Y), int32(endCoord.Z)}

	// 输入维度名
	dimensionName := ui.InputWithDefault(l10n.T(l10n.KeyTaskDimensionName)+": ", "overworld")
	exportTask.DimensionName = dimensionName

	return exportTask, nil
}

// inputCoordinates 输入坐标
func inputCoordinates(prompt string) (Coordinates, error) {
	fmt.Printf("%s:\n", prompt)

	x := ui.InputInt("X: ")
	y := ui.InputInt("Y: ")
	z := ui.InputInt("Z: ")

	return Coordinates{X: x, Y: y, Z: z}, nil
}

// buildBuildTasks 构建构建任务
func buildBuildTasks() ([]task.BuildTask, error) {
	// 选择任务类型
	options := []string{
		l10n.T(l10n.KeyModeSingleBuild),
		l10n.T(l10n.KeyModeMultipleBuild),
	}

	index, _ := ui.Select(l10n.T(l10n.KeyBuildModePrompt), options)

	var tasks []task.BuildTask

	switch index {
	case 0:
		// 单任务
		buildTask, err := createSingleBuildTask()
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, buildTask)
	case 1:
		// 多任务
		for {
			buildTask, err := createSingleBuildTask()
			if err != nil {
				return nil, err
			}
			tasks = append(tasks, buildTask)

			if !ui.Confirm(l10n.T(l10n.KeyBuildAddAnotherTask)) {
				break
			}
		}
	}

	return tasks, nil
}

// createSingleBuildTask 创建单个构建任务
func createSingleBuildTask() (task.BuildTask, error) {
	buildTask := task.NewBuildTask()

	// 选择要构建的文件
	filePath, err := selectBuildFile()
	if err != nil {
		return *buildTask, err
	}
	buildTask.FilePath = filePath

	// 输入起始坐标
	startCoord, err := inputCoordinates(l10n.T(l10n.KeyTaskStartCoord))
	if err != nil {
		return *buildTask, err
	}
	buildTask.StartPos = defines.BlockPos{int32(startCoord.X), int32(startCoord.Y), int32(startCoord.Z)}

	// 输入维度名
	dimensionName := ui.InputWithDefault(l10n.T(l10n.KeyTaskDimensionName)+": ", "overworld")
	buildTask.DimensionName = dimensionName

	// 输入速度
	speed := ui.InputIntWithDefault(l10n.T(l10n.KeyTaskSpeed), 3000)
	buildTask.Speed = speed

	chunkGroupSide := ui.InputIntWithDefault(l10n.T(l10n.KeyTaskChunkGroupSide), 1)
	if chunkGroupSide < 1 {
		chunkGroupSide = 1
	}
	buildTask.ChunkGroupSide = chunkGroupSide

	// 是否自动清除原方块
	buildTask.AutoCleanBlock = ui.ConfirmWithDefault(l10n.T(l10n.KeyTaskClearBlocks), true)

	// 是否自动清除掉落物
	buildTask.AutoCleanItem = ui.ConfirmWithDefault(l10n.T(l10n.KeyTaskClearDrops), true)

	// 自动铺设 deny 方块（抬高建筑一格，并在底部铺 deny）
	buildTask.AutoPlaceDenyBlock = ui.ConfirmWithDefault(l10n.T(l10n.KeyTaskAutoPlaceDenyBlock), false)

	// 自动铺设 border_block 边框（X/Z 外扩一格，让长宽各增加 2）
	buildTask.AutoPlaceBorderBlock = ui.ConfirmWithDefault(l10n.T(l10n.KeyTaskAutoPlaceBorderBlock), false)

	// 自动关闭命令方块启用的配置
	buildTask.AutoDisableCommand = ui.ConfirmWithDefault(l10n.T(l10n.KeyTaskAutoDisableCommand), true)

	// 启用增量导入的配置
	buildTask.EnableDeltaMode = ui.ConfirmWithDefault(l10n.T(l10n.KeyTaskEnableDeltaMode), true)

	// 是否忽略命令方块
	buildTask.IgnoreCommandBlock = ui.ConfirmWithDefault(l10n.T(l10n.KeyTaskIgnoreCommandBlock), false)

	// 是否忽略其他NBT方块
	buildTask.IgnoreOtherNBTBlock = ui.ConfirmWithDefault(l10n.T(l10n.KeyTaskIgnoreOtherNBTBlock), false)

	// 自动升级命令方块命令
	buildTask.AutoUpgradeCommand = ui.ConfirmWithDefault(l10n.T(l10n.KeyTaskAutoUpgradeCommand), true)

	// 是否等待区块加载
	buildTask.WaitChunkLoad = ui.ConfirmWithDefault(l10n.T(l10n.KeyTaskWaitChunkLoad), true)

	// 是否在导入前转换为 MCWorld
	buildTask.ConvertToMCWorld = ui.ConfirmWithDefault(l10n.T(l10n.KeyTaskConvertToMCWorld), false)

	// 控制台坐标配置
	// 默认 -50 0 -50
	buildTask.ConsoleWorldPos = defines.BlockPos{-50, 0, -50}
	if ui.ConfirmWithDefault(l10n.T(l10n.KeyTaskSetConsolePos), false) {
		consoleCoord, err := inputCoordinates(l10n.T(l10n.KeyTaskConsoleCoordinates))
		if err != nil {
			return *buildTask, err
		}
		buildTask.ConsoleWorldPos = defines.BlockPos{int32(consoleCoord.X), int32(consoleCoord.Y), int32(consoleCoord.Z)}
	}

	// NBT导入进度显示配置
	buildTask.ShowNBTProgress = ui.ConfirmWithDefault(l10n.T(l10n.KeyTaskShowNBTProgress), true)

	// 是否在游戏中显示进度条（actionbar）
	buildTask.ShowProgressInGame = ui.ConfirmWithDefault(l10n.T(l10n.KeyTaskShowProgressInGame), false)

	// 验证间隔
	verifyInterval := ui.InputIntWithDefault(l10n.T(l10n.KeyTaskVerifyInterval), 1)
	buildTask.VerifyAfterChunk = verifyInterval

	// 验证等级
	verifyOptions := []string{
		l10n.T(l10n.KeyVerifyNone),
		l10n.T(l10n.KeyVerifyAir),
		l10n.T(l10n.KeyVerifySimilar),
		l10n.T(l10n.KeyVerifyComplete),
	}
	verifyIndex, _ := ui.Select(l10n.T(l10n.KeyTaskVerifyLevel), verifyOptions)
	buildTask.VerifyChunkLevel = verifyIndex + 1

	// 修补模式配置
	buildTask.AutoEnterFixMode = ui.ConfirmWithDefault(l10n.T(l10n.KeyTaskAutoEnterFixMode), false)
	buildTask.EnterFixModeDirectly = ui.ConfirmWithDefault(l10n.T(l10n.KeyTaskEnterFixModeDirectly), false)

	// 起始进度
	progress := ui.InputIntWithDefault(l10n.T(l10n.KeyTaskProgress), 0)
	buildTask.Progress = progress

	return *buildTask, nil
}

// selectBuildFile 选择要构建的文件
func selectBuildFile() (string, error) {
	// 列出当前目录的文件
	files, err := os.ReadDir(".")
	if err != nil {
		return "", err
	}

	var fileOptions []string
	for _, file := range files {
		if !file.IsDir() {
			fileOptions = append(fileOptions, file.Name())
		}
	}

	// 添加自定义路径选项
	fileOptions = append(fileOptions, l10n.T(l10n.KeyFileCustomPath))

	index, selected := ui.Select(l10n.T(l10n.KeyBuildFilePrompt), fileOptions)
	if index < 0 {
		return "", fmt.Errorf("operation cancelled")
	}

	// 如果选择自定义路径
	if index == len(fileOptions)-1 {
		for {
			customPath := ui.Input(l10n.T(l10n.KeyFileEnterPath) + ": ")
			if customPath == "" {
				continue
			}

			if _, err := os.Stat(customPath); err != nil {
				fmt.Println(l10n.T(l10n.KeyFileNotFound))
				continue
			}

			return customPath, nil
		}
	}

	// 验证文件存在
	if _, err := os.Stat(selected); err != nil {
		return "", fmt.Errorf("%s: %s", l10n.T(l10n.KeyFileNotFound), selected)
	}

	return selected, nil
}

// handleBuildInterrupt 处理构建中断，返回中断数据和文件路径
func handleBuildInterrupt() (*task.BuildTaskInterruptData, string, error) {
	storagePath := utils.GetStoragePath()

	interruptFile := filepath.Join(storagePath, "build_interrupt.json")

	if _, err := os.Stat(interruptFile); err != nil {
		if os.IsNotExist(err) {
			if ensureErr := ensureInterruptFile(interruptFile); ensureErr != nil {
				return nil, "", ensureErr
			}
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("failed to access interrupt file: %w", err)
	}

	fmt.Printf("\n%s\n", l10n.T(l10n.KeyBuildInterruptFound))

	data, err := os.ReadFile(interruptFile)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read interrupt file: %w", err)
	}

	var interruptData task.BuildTaskInterruptData
	if err := json.Unmarshal(data, &interruptData); err != nil {
		return nil, "", fmt.Errorf("failed to parse interrupt file: %w", err)
	}

	fmt.Printf("%s\n", fmt.Sprintf(l10n.T(l10n.KeyBuildInterruptFile), filepath.Base(interruptFile)))
	fmt.Printf("%s\n", fmt.Sprintf(l10n.T(l10n.KeyBuildInterruptProgress),
		interruptData.CurrentChunkIndex,
		interruptData.TotalChunks,
		float64(interruptData.CurrentChunkIndex)/float64(interruptData.TotalChunks)*100))

	options := []string{
		l10n.T(l10n.KeyBuildInterruptResume),
		l10n.T(l10n.KeyBuildInterruptNew),
		l10n.T(l10n.KeyBuildInterruptDelete),
	}

	index, _ := ui.Select(l10n.T(l10n.KeyUISelectPrompt), options)

	switch index {
	case 0:
		fmt.Printf("%s\n", l10n.T(l10n.KeyBuildInterruptRestored))
		return &interruptData, interruptFile, nil
	case 1:
		return nil, "", nil
	case 2:
		if err := os.Remove(interruptFile); err != nil {
			fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
			return nil, "", nil
		}
		fmt.Printf("%s\n", l10n.T(l10n.KeyBuildInterruptDeleted))
		return nil, "", nil
	default:
		return nil, "", nil
	}
}

func loadInterruptDataFromFile(path string) (*task.BuildTaskInterruptData, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("interrupt file path is empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var interruptData task.BuildTaskInterruptData
	if err := json.Unmarshal(data, &interruptData); err != nil {
		return nil, err
	}

	return &interruptData, nil
}

func cloneBuildTasks(tasks []task.BuildTask) []task.BuildTask {
	if len(tasks) == 0 {
		return nil
	}

	cloned := make([]task.BuildTask, len(tasks))
	copy(cloned, tasks)
	return cloned
}

func isUserAbortError(err error) bool {
	if err == nil {
		return false
	}

	return strings.Contains(err.Error(), l10n.T(l10n.KeyOperationAborted))
}

// executeBuildTasks 执行构建任务
func executeBuildTasks(serverConfig defines.ControlConfig, buildTasks []task.BuildTask, interruptFileOverride string, allowAutoResume bool) error {
	fmt.Printf("\n=== %s ===\n", l10n.T(l10n.KeyBuildTitle))

	// 设置外部InterruptJSON文件路径（保存在全局存储目录中）
	storagePath := utils.GetStoragePath()

	if interruptFileOverride != "" {
		externalInterruptJSONPath = interruptFileOverride
		if err := os.MkdirAll(filepath.Dir(externalInterruptJSONPath), 0755); err != nil {
			return fmt.Errorf("failed to prepare interrupt file directory: %w", err)
		}
	} else {
		externalInterruptJSONPath = filepath.Join(storagePath, "build_interrupt.json")
	}

	if externalInterruptJSONPath != "" {
		if err := ensureInterruptFile(externalInterruptJSONPath); err != nil {
			return fmt.Errorf("failed to prepare interrupt file: %w", err)
		}
	}

	// 创建控制实例
	ctrl := control.NewControl(serverConfig)

	// 连接到服务器
	fmt.Printf("%s...\n", l10n.T(l10n.KeyServerConnecting))
	for {
		if err := ctrl.EnterRentalServer(); err != nil {
			fmt.Printf(l10n.T(l10n.KeyServerConnectFailed)+"\n", err)
			continue
		}
		break
	}
	defer ctrl.LeaveRentalServer()

	fmt.Printf("%s\n", l10n.T(l10n.KeyServerConnected))

	// 顺序执行，每个任务可交互控制
	for i := range buildTasks {
		buildTasks[i].SetControl(ctrl)
		setupBuildTaskCallbacks(&buildTasks[i], i+1, len(buildTasks))
		fmt.Printf("\n"+l10n.T(l10n.KeyBuildTaskPreparingIndexed)+"\n", i+1, len(buildTasks), buildTasks[i].FilePath)
		if err := runBuildTaskInteractive(&buildTasks[i]); err != nil {
			if isUserAbortError(err) {
				return err
			}

			fmt.Println(err)

			if allowAutoResume && externalInterruptJSONPath != "" {
				interruptData, loadErr := loadInterruptDataFromFile(externalInterruptJSONPath)
				if loadErr != nil {
					fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), loadErr)
					return err
				}

				savedPath := strings.TrimSpace(interruptData.TaskConfig.FilePath)
				currentPath := strings.TrimSpace(buildTasks[i].FilePath)
				if savedPath == "" || interruptData.TotalChunks <= 0 || !strings.EqualFold(savedPath, currentPath) {
					return err
				}

				remainingTasks := append([]task.BuildTask{}, buildTasks[i+1:]...)
				return resumeBuildFromInterrupt(*interruptData, serverConfig, externalInterruptJSONPath, remainingTasks, true)
			}

			return err
		}
	}

	// 构建任务完成后自动删除中断文件
	if externalInterruptJSONPath != "" {
		if err := os.Remove(externalInterruptJSONPath); err != nil {
			// 删除失败不是致命错误，只记录警告
			fmt.Printf(l10n.T(l10n.KeyBuildInterruptRemoveWarn)+"\n", err)
		} else {
			fmt.Printf(l10n.T(l10n.KeyBuildInterruptCleaned)+"\n", filepath.Base(externalInterruptJSONPath))
		}
	}

	return nil
}

func executeExportTasks(serverConfig defines.ControlConfig, exportTasks []task.ExportTask) error {
	if len(exportTasks) == 0 {
		return nil
	}

	fmt.Printf("\n=== %s ===\n", l10n.T(l10n.KeyExportTitle))

	ctrl := control.NewControl(serverConfig)

	fmt.Printf("%s...\n", l10n.T(l10n.KeyServerConnecting))
	for {
		if err := ctrl.EnterRentalServer(); err != nil {
			fmt.Printf(l10n.T(l10n.KeyServerConnectFailed)+"\n", err)
			continue
		}
		break
	}
	defer ctrl.LeaveRentalServer()

	fmt.Printf("%s\n", l10n.T(l10n.KeyServerConnected))

	for i := range exportTasks {
		exportTasks[i].SetControl(ctrl)
		setupExportTaskCallbacks(&exportTasks[i], i+1, len(exportTasks))
		fmt.Printf("\n"+l10n.T(l10n.KeyExportTaskPreparingIndexed)+"\n", i+1, len(exportTasks), exportTasks[i].FilePath)
		if err := runExportTask(&exportTasks[i]); err != nil {
			return err
		}
	}

	return nil
}

func setupExportTaskCallbacks(exportTask *task.ExportTask, index, total int) {
	var (
		chunkBar    *progressbar.ProgressBar
		totalChunks int
	)

	startPos := exportTask.StartPos
	endPos := exportTask.EndPos

	exportTask.SetStartCallback(func(size structure_define.Size) {
		totalChunks = calculateChunkTotal(startPos, endPos)
		fmt.Printf(l10n.T(l10n.KeyExportTaskStartSummary)+"\n",
			index,
			total,
			size.Width+1,
			size.Height+1,
			size.Length+1,
		)
		fmt.Printf(l10n.T(l10n.KeyExportEstimatedTotalChunks)+"\n", totalChunks)
		if totalChunks > 0 {
			chunkBar = progressbar.NewOptions(totalChunks,
				progressbar.OptionSetDescription(l10n.T(l10n.KeyExportProgressBarDesc)),
				progressbar.OptionShowCount(),
				progressbar.OptionThrottle(65*time.Millisecond),
				progressbar.OptionShowElapsedTimeOnFinish(),
				progressbar.OptionFullWidth(),
				progressbar.OptionSetRenderBlankState(true),
			)
		}
	})

	exportTask.SetEveryChunkExportCallback(func(pos bwo_define.ChunkPos) {
		if chunkBar != nil && totalChunks > 0 {
			_ = chunkBar.Add(1)
		}
	})

	exportTask.SetEverySubChunkDecodeNBTFailedCallback(func(pos bwo_define.SubChunkPos, err error) {
		fmt.Printf(l10n.T(l10n.KeyExportSubChunkDecodeFailed)+"\n", pos.X(), pos.Y(), pos.Z(), err)
	})

	exportTask.SetEverySubChunkSaveFailedCallback(func(pos bwo_define.SubChunkPos, err error) {
		fmt.Printf(l10n.T(l10n.KeyExportSubChunkSaveFailed)+"\n", pos.X(), pos.Y(), pos.Z(), err)
	})

	exportTask.SetEveryChunkNBTSaveFailedCallback(func(pos bwo_define.ChunkPos, err error) {
		fmt.Printf(l10n.T(l10n.KeyExportChunkSaveFailed)+"\n", pos.X(), pos.Z(), err)
	})

	exportTask.SetFinishedCallback(func(path string) {
		if chunkBar != nil {
			chunkBar.Finish()
		}
		fmt.Printf(l10n.T(l10n.KeyExportFinishedPath)+"\n", path)
	})
}

func runExportTask(exportTask *task.ExportTask) error {
	fmt.Println(l10n.T(l10n.KeyExportTaskRunning))
	ctx := context.Background()
	if err := exportTask.Start(ctx); err != nil {
		return fmt.Errorf(l10n.T(l10n.KeyExportTaskFailed), err)
	}
	fmt.Println(l10n.T(l10n.KeyExportTaskCompleted))
	return nil
}

func calculateChunkTotal(start, end defines.BlockPos) int {
	width := int(math.Abs(float64(end.X()-start.X()))) + 1
	length := int(math.Abs(float64(end.Z()-start.Z()))) + 1
	return ((width + 15) / 16) * ((length + 15) / 16)
}

// runBuildTaskInteractive 以交互方式运行单个构建任务
func runBuildTaskInteractive(b *task.BuildTask) error {
	fmt.Println(l10n.T(l10n.KeyBuildControlHint))
	ctx := context.Background()
	done := make(chan error, 1)
	go func() {
		done <- b.Start(ctx)
	}()
	cmdCh := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			line, _ := reader.ReadString('\n')
			cmd := strings.TrimSpace(line)
			if cmd != "" {
				cmdCh <- cmd
			}
		}
	}()
	for {
		select {
		case err := <-done:
			if err != nil {
				return fmt.Errorf(l10n.T(l10n.KeyBuildTaskFailed), err)
			}
			fmt.Println(l10n.T(l10n.KeyBuildTaskCompleted))
			return nil
		case cmd := <-cmdCh:
			switch strings.ToLower(cmd) {
			case "p":
				if err := b.Pause(); err != nil {
					fmt.Printf(l10n.T(l10n.KeyBuildPauseFailed)+"\n", err)
				} else {
					fmt.Println(l10n.T(l10n.KeyBuildPaused))
				}
			case "r":
				if err := b.Resume(); err != nil {
					fmt.Printf(l10n.T(l10n.KeyBuildContinueFailed)+"\n", err)
				} else {
					fmt.Println(l10n.T(l10n.KeyBuildResumed))
				}
			case "s":
				_ = b.Stop()
				fmt.Println(l10n.T(l10n.KeyBuildStopSuccess))
			case "n":
				_ = b.Stop()
				fmt.Println(l10n.T(l10n.KeyBuildSkipSuccess))
				return nil
			case "q":
				_ = b.Stop()
				return fmt.Errorf("%s", l10n.T(l10n.KeyOperationAborted))
			default:
				fmt.Println(l10n.T(l10n.KeyBuildUnknownCommand))
			}
		}
	}
}

// setupBuildTaskCallbacks 设置构建任务的回调函数
func setupBuildTaskCallbacks(buildTask *task.BuildTask, taskIndex, totalTasks int) {
	// 声明进度条变量
	var chunkBar *progressbar.ProgressBar
	var totalChunks int
	var convertBar *progressbar.ProgressBar

	buildTask.SetStartCallback(func() {
		if totalTasks > 1 {
			fmt.Printf("\n[%d/%d] %s\n", taskIndex, totalTasks, l10n.T(l10n.KeyBuildTaskStart))
		} else {
			fmt.Printf("\n%s\n", l10n.T(l10n.KeyBuildTaskStart))
		}
	})

	buildTask.SetGotTypeCallback(func(t structure.Structure) {
		fmt.Printf("%s: %s\n", l10n.T(l10n.KeyBuildStructureType), t.Name())
	})

	buildTask.SetParsingCallback(func() {
		fmt.Printf("%s...\n", l10n.T(l10n.KeyBuildParsingFile))
	})

	buildTask.SetParsedCallback(func(r structure.Structure) {
		fmt.Printf("%s\n", l10n.T(l10n.KeyBuildFileParsed))
		size := r.GetSize()
		fmt.Printf("%s: %s=%d, %s=%d, %s=%d\n",
			l10n.T(l10n.KeyBuildStructureSize),
			l10n.T(l10n.KeyBuildWidth), size.Width,
			l10n.T(l10n.KeyBuildHeight), size.Height,
			l10n.T(l10n.KeyBuildLength), size.Length)

		// 解析完成后获取总区块数，初始化进度条
		totalChunks = size.GetChunkCount()
		fmt.Printf("%s: %d\n", l10n.T(l10n.KeyBuildTotalChunks), totalChunks)
	})

	buildTask.SetStartBuildCallback(func() {
		fmt.Printf("%s\n", l10n.T(l10n.KeyBuildStartImport))

		// 创建进度条
		chunkBar = progressbar.NewOptions(totalChunks,
			progressbar.OptionSetDescription(l10n.T(l10n.KeyBuildProgressBarDesc)),
			progressbar.OptionShowCount(),
			progressbar.OptionThrottle(65*time.Millisecond),
			progressbar.OptionShowElapsedTimeOnFinish(),
			progressbar.OptionFullWidth(),
			progressbar.OptionSetRenderBlankState(true),
			progressbar.OptionOnCompletion(func() {
				fmt.Println()
			}),
		)
		_ = chunkBar.Add(buildTask.GetProgress()*buildTask.ChunkGroupSide*buildTask.ChunkGroupSide)

		if err := saveExternalInterruptJSON(buildTask); err != nil {
			fmt.Printf("Warning: failed to save external interrupt JSON: %v\n", err)
		}
	})

	buildTask.SetEveryChunkBuiltCallback(func(chunkPos defines.ChunkPos, c *chunk.Chunk) {
		// 每完成一个区块，进度条+1
		if chunkBar != nil && totalChunks > 0 {
			_ = chunkBar.Add(1)
		}

		// 每构建一个区块后保存外部InterruptJSON
		if err := saveExternalInterruptJSON(buildTask); err != nil {
			fmt.Printf("Warning: failed to save external interrupt JSON: %v\n", err)
		}
	})

	buildTask.SetEveryChunkAutoFixCallback(func(chunkPos defines.ChunkPos) {
		fmt.Printf("%s: (%d, %d)\n",
			l10n.T(l10n.KeyBuildAutoFixingChunk),
			chunkPos.X(), chunkPos.Z())
	})

	buildTask.SetChunkFixExceededLimitCallback(func(chunkPos defines.ChunkPos, attempts int) {
		fmt.Printf(l10n.T(l10n.KeyBuildChunkFixExceeded)+"\n",
			chunkPos.X(), chunkPos.Z(), attempts)
	})

	buildTask.SetEnterFixModeCallback(func() {
		fmt.Println(l10n.T(l10n.KeyBuildEnterFixMode))
		fmt.Println(l10n.T(l10n.KeyBuildFixModeInstructions))
	})

	buildTask.SetExitFixModeCallback(func() {
		fmt.Println(l10n.T(l10n.KeyBuildExitFixMode))
	})

	buildTask.SetEveryChunkFixStartCallback(func(chunkPos defines.ChunkPos, playerName string) {
		fmt.Printf(l10n.T(l10n.KeyBuildFixChunkStart)+"\n",
			chunkPos.X(), chunkPos.Z(), playerName)
	})

	buildTask.SetEveryChunkFixEndCallback(func(chunkPos defines.ChunkPos, playerName string) {
		fmt.Printf(l10n.T(l10n.KeyBuildFixChunkEnd)+"\n",
			chunkPos.X(), chunkPos.Z(), playerName)
	})

	buildTask.SetConvertStartCallback(func(srcPath, destPath string) {
		if convertBar != nil {
			_ = convertBar.Finish()
			convertBar = nil
		}
		fmt.Println(l10n.T(l10n.KeyBuildConvertToMCWorldStart))
	})

	buildTask.SetConvertExistsCallback(func(path string) {
		fmt.Printf(l10n.T(l10n.KeyBuildConvertToMCWorldExists)+"\n", path)
	})

	buildTask.SetConvertSuccessCallback(func(path string) {
		if convertBar != nil {
			_ = convertBar.Finish()
			convertBar = nil
		}
		fmt.Printf(l10n.T(l10n.KeyBuildConvertToMCWorldSuccess)+"\n", path)
	})

	buildTask.SetConvertProgressInitCallback(func(total int) {
		if total <= 0 {
			convertBar = nil
			return
		}
		convertBar = progressbar.NewOptions(total,
			progressbar.OptionSetDescription(l10n.T(l10n.KeyBuildConvertToMCWorldProgress)),
			progressbar.OptionShowCount(),
			progressbar.OptionThrottle(65*time.Millisecond),
			progressbar.OptionShowElapsedTimeOnFinish(),
			progressbar.OptionFullWidth(),
			progressbar.OptionSetRenderBlankState(true),
			progressbar.OptionOnCompletion(func() {
				fmt.Println()
			}),
		)
	})

	buildTask.SetConvertProgressCallback(func() {
		if convertBar != nil {
			_ = convertBar.Add(1)
		}
	})

	// 设置NBT导入进度显示回调
	buildTask.SetEveryNBTBlockBuildingCallback(func(blockPos defines.BlockPos, blockRuntimeID uint32, nbt map[string]any) {
		if buildTask.ShowNBTProgress {
			fmt.Printf(l10n.T(l10n.KeyTaskNBTImportProgress)+"\n",
				blockPos.X(), blockPos.Y(), blockPos.Z(), blockRuntimeID)
		}
	})

	// 设置扣费相关回调
	buildTask.SetChargeCostSuccessCallback(func(deducted int, balance string, blockInfo task.BlockCountInfo) {
		fmt.Printf(l10n.T(l10n.KeyCostBlockStats)+"\n",
			blockInfo.BlockCount, blockInfo.CommandBlockCount, blockInfo.OtherNBTCount)
		fmt.Printf(l10n.T(l10n.KeyCostChargeSuccess)+"\n", deducted, balance)
	})

	buildTask.SetChargeCostDuplicateCallback(func(blockInfo task.BlockCountInfo) {
		fmt.Printf(l10n.T(l10n.KeyCostBlockStats)+"\n",
			blockInfo.BlockCount, blockInfo.CommandBlockCount, blockInfo.OtherNBTCount)
		fmt.Printf("%s\n", l10n.T(l10n.KeyCostChargeDuplicate))
	})

	buildTask.SetChargeCostFailedCallback(func(message string, blockInfo task.BlockCountInfo) {
		fmt.Printf(l10n.T(l10n.KeyCostBlockStats)+"\n",
			blockInfo.BlockCount, blockInfo.CommandBlockCount, blockInfo.OtherNBTCount)
		fmt.Printf(l10n.T(l10n.KeyCostChargeFailed)+"\n", message)
	})

	buildTask.SetUserCenterNotInitCallback(func() {
		fmt.Printf("%s\n", l10n.T(l10n.KeyUserCenterNotInit))
	})
}

// ensureInterruptFile 确保断点文件存在，不存在时创建默认文件
func ensureInterruptFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("interrupt file path is empty")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create interrupt directory: %w", err)
	}

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			defaultTask := task.NewBuildTask()
			defaultData := task.BuildTaskInterruptData{
				CurrentChunkIndex: 0,
				TotalChunks:       1,
				TaskConfig:        *defaultTask,
			}

			data, marshalErr := json.Marshal(defaultData)
			if marshalErr != nil {
				return fmt.Errorf("failed to marshal default interrupt data: %w", marshalErr)
			}

			if writeErr := os.WriteFile(path, data, 0644); writeErr != nil {
				return fmt.Errorf("failed to create interrupt file: %w", writeErr)
			}
		} else {
			return fmt.Errorf("failed to access interrupt file: %w", err)
		}
	}

	return nil
}

// saveExternalInterruptJSON 保存外部InterruptJSON文件
func saveExternalInterruptJSON(buildTask *task.BuildTask) error {
	if externalInterruptJSONPath == "" {
		return nil // 没有设置路径时不保存
	}

	// 生成中断JSON
	interruptJSON, err := buildTask.MakeInterruptJSON()
	if err != nil {
		return err
	}

	// 写入外部文件
	return os.WriteFile(externalInterruptJSONPath, []byte(interruptJSON), 0644)
}

// resumeBuildFromInterrupt 从中断点恢复并执行构建任务
func resumeBuildFromInterrupt(interruptData task.BuildTaskInterruptData, serverConfig defines.ControlConfig, interruptFile string, additionalTasks []task.BuildTask, autoResume bool) error {
	fmt.Printf("\n=== %s ===\n", l10n.T(l10n.KeyBuildResumeTitle))

	if !ensureValidUserCenterAPIKey() {
		return fmt.Errorf("API Key is not valid")
	}

	additionalTemplate := cloneBuildTasks(additionalTasks)
	baseData := interruptData
	firstDisplay := true

	for {
		buildTask := task.NewBuildTask()
		jsonData, _ := json.Marshal(baseData)
		if err := buildTask.RestoreInterruptJSON(string(jsonData)); err != nil {
			return fmt.Errorf("failed to restore build task from interrupt data: %w", err)
		}
		buildTask.DisableCounting()

		if firstDisplay {
			fmt.Printf("%s\n", l10n.T(l10n.KeyBuildResumeInfo))
			fmt.Printf(l10n.T(l10n.KeyBuildResumeFile)+"\n", buildTask.FilePath)
			fmt.Printf(l10n.T(l10n.KeyBuildResumeStartPos)+"\n", buildTask.StartPos.X(), buildTask.StartPos.Y(), buildTask.StartPos.Z())
			fmt.Printf(l10n.T(l10n.KeyBuildResumeSpeed)+"\n", buildTask.Speed)
			fmt.Printf(l10n.T(l10n.KeyBuildProgressCurrent)+"\n",
				baseData.CurrentChunkIndex,
				baseData.TotalChunks,
				float64(baseData.CurrentChunkIndex)/float64(baseData.TotalChunks)*100)
			if !autoResume && !ui.ConfirmWithDefault(l10n.T(l10n.KeyBuildInterruptResume), true) {
				return fmt.Errorf("%s", l10n.T(l10n.KeyBuildResumeCancelled))
			}
			firstDisplay = false
		}

		combinedTasks := append([]task.BuildTask{*buildTask}, cloneBuildTasks(additionalTemplate)...)

		fmt.Printf("%s\n", l10n.T(l10n.KeyBuildResumeInProgress))
		if err := executeBuildTasks(serverConfig, combinedTasks, interruptFile, false); err != nil {
			if isUserAbortError(err) {
				return err
			}

			fmt.Println(err)

			if strings.TrimSpace(interruptFile) != "" {
				if updatedData, loadErr := loadInterruptDataFromFile(interruptFile); loadErr == nil {
					baseData = *updatedData
				}
			}

			time.Sleep(2 * time.Second)
			continue
		}

		return nil
	}
}
