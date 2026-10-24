package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	structure_define "github.com/Yeah114/WaterStructure/define"
	"github.com/schollz/progressbar/v3"

	"github.com/Yeah114/Fatalder/control"
	"github.com/Yeah114/Fatalder/control/task"
	"github.com/Yeah114/Fatalder/defines"
	"github.com/Yeah114/Fatalder/utils"
	"github.com/Yeah114/Fatalder/utils/user_center"
)

func main() {
	if err := utils.InitStorage(); err != nil {
		panic(err)
	}
	if err := utils.InitConfig(); err != nil {
		panic(err)
	}
	if err := user_center.InitGlobalFromConfig(); err != nil {
		panic(err)
	}

	savedOperator := user_center.GlobalOperator
	user_center.SetGlobalOperator(nil)
	defer user_center.SetGlobalOperator(savedOperator)

	ctrl := control.NewControl(defines.ControlConfig{
		RentalServerCode:     "48285363",
		RentalServerPasscode: "",
	})

	fmt.Println("正在连接租赁服...")
	if err := ctrl.EnterRentalServer(); err != nil {
		panic(err)
	}
	defer ctrl.LeaveRentalServer()

	exportTask := task.NewExportTask()
	exportTask.FilePath = "./export_result.mcworld"
	exportTask.StartPos = defines.BlockPos{-100, -64, -100}
	exportTask.EndPos = defines.BlockPos{100, 319, 100}

	if err := cleanupExportArtifacts(exportTask.FilePath); err != nil {
		panic(err)
	}

	attachExportCallbacks(exportTask)

	ctrl.AddTask(exportTask)

	fmt.Println("开始执行导出任务")
	if err := ctrl.RunAllTask(context.Background(), true); err != nil {
		panic(err)
	}

	fmt.Println("导出任务完成，等待 3 秒后退出")
	time.Sleep(3 * time.Second)
}

func attachExportCallbacks(exportTask *task.ExportTask) {
	startPos := exportTask.StartPos
	endPos := exportTask.EndPos

	var (
		chunkBar    *progressbar.ProgressBar
		totalChunks int
	)

	exportTask.SetStartCallback(func(size structure_define.Size) {
		totalChunks = calculateChunkTotal(startPos, endPos)
		fmt.Printf("导出范围: 宽=%d, 高=%d, 长=%d\n", size.Width+1, size.Height+1, size.Length+1)
		fmt.Printf("预计总区块数: %d\n", totalChunks)
		if totalChunks > 0 {
			chunkBar = progressbar.NewOptions(totalChunks,
				progressbar.OptionSetDescription("导出进度"),
				progressbar.OptionShowCount(),
				progressbar.OptionFullWidth(),
				progressbar.OptionThrottle(65*time.Millisecond),
				progressbar.OptionShowElapsedTimeOnFinish(),
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
		fmt.Printf("子区块NBT解码失败 (%d, %d, %d): %v\n", pos.X(), pos.Y(), pos.Z(), err)
	})

	exportTask.SetEverySubChunkSaveFailedCallback(func(pos bwo_define.SubChunkPos, err error) {
		fmt.Printf("子区块保存失败 (%d, %d, %d): %v\n", pos.X(), pos.Y(), pos.Z(), err)
	})

	exportTask.SetEveryChunkNBTSaveFailedCallback(func(pos bwo_define.ChunkPos, err error) {
		fmt.Printf("区块NBT保存失败 (%d, %d): %v\n", pos.X(), pos.Z(), err)
	})

	exportTask.SetFinishedCallback(func(path string) {
		if chunkBar != nil {
			chunkBar.Finish()
		}
		fmt.Printf("导出文件已创建: %s\n", path)
	})
}

func calculateChunkTotal(start, end defines.BlockPos) int {
	width := int(math.Abs(float64(end.X()-start.X()))) + 1
	length := int(math.Abs(float64(end.Z()-start.Z()))) + 1
	return ((width + 15) / 16) * ((length + 15) / 16)
}

func cleanupExportArtifacts(filePath string) error {
	worldDir := strings.TrimSuffix(filePath, filepath.Ext(filePath))
	if worldDir != "" {
		if err := os.RemoveAll(worldDir); err != nil {
			return err
		}
	}
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
