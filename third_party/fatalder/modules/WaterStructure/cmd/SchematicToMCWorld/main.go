package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/Yeah114/WaterStructure/structure"
	"github.com/mholt/archiver/v3"
	"github.com/schollz/progressbar/v3"
)

func main() {
	startTime := time.Now()
	// 关键修改: 用Seconds()方法将耗时转为秒, 保留2位小数让输出更清晰
	defer func() {
		totalCostSec := time.Since(startTime).Seconds()
		// 格式化输出, %.2f 表示保留2位小数, 单位明确为s
		fmt.Printf("程序总耗时: %.2f s\n", totalCostSec)
	}()

	// 1. 打印支持的结构文件格式
	var names string
	for name := range structure.StructureNamePool {
		names += name + " "
	}
	fmt.Printf("支持的结构文件格式: %s\n", names)

	// 2. 打开世界与结构文件
	bedrockWorld, err := world.Open("world", nil)
	if err != nil {
		panic(fmt.Sprintf("打开世界失败: %v", err))
	}

	path := "test/测试文件/末至审判者.bp"
	//path := "KaioAC反作弊.mcstructure"
	file, err := os.Open(path)
	if err != nil {
		panic(fmt.Sprintf("打开结构文件失败: %v", err))
	}
	defer file.Close()

	// 3. 解析结构文件并打印核心信息
	// 判断文件类型
	structure, err := structure.StructureFromFile(file)
	if err != nil {
		panic(fmt.Sprintf("判断结构文件类型失败: %v", err))
	}
	fmt.Printf("当前结构文件类型: %s\n", structure.Name())

	reader := structure
	// 打印结构关键信息
	structureSize := reader.GetSize()
	fmt.Printf("结构尺寸 - 宽(X): %d, 高(Y): %d, 长(Z): %d\n", structureSize.Width, structureSize.Height, structureSize.Length)

	//nonAirCount, _ := reader.CountNonAirBlocks()
	//fmt.Printf("结构中非空方块总数: %d\n", nonAirCount)

	chunkXCount, chunkZCount := structureSize.GetChunkXCount(), structureSize.GetChunkZCount()
	totalChunks := chunkXCount * chunkZCount
	fmt.Printf("结构占用区块数 - 总: %d (X方向: %d, Z方向: %d)\n", totalChunks, chunkXCount, chunkZCount)

	// 4. 结构转MCWorld并保存
	fmt.Println("开始将结构写入世界...")
	startTime = time.Now()
	var bar *progressbar.ProgressBar
	if err := reader.ToMCWorld(
		bedrockWorld,
		define.SubChunkPos{0, -4, 0},
		func(total int) {
			fmt.Printf("需要处理的子区块总数: %d\n", total)
			if total <= 0 {
				bar = nil
				return
			}
			bar = progressbar.NewOptions(
				total,
				progressbar.OptionSetWriter(os.Stdout),
				progressbar.OptionSetDescription("写入进度"),
				progressbar.OptionSetWidth(40),
				progressbar.OptionSetTheme(progressbar.Theme{
					Saucer:        "#",
					SaucerHead:    ">",
					SaucerPadding: "-",
					BarStart:      "[",
					BarEnd:        "]",
				}),
				progressbar.OptionShowCount(),
				progressbar.OptionSetPredictTime(true),
				progressbar.OptionShowElapsedTimeOnFinish(),
				progressbar.OptionSetRenderBlankState(true),
				progressbar.OptionOnCompletion(func() {
					fmt.Println()
				}),
			)
		},
		func() {
			if bar != nil {
				_ = bar.Add(1)
			}
		},
	); err != nil {
		panic(fmt.Sprintf("结构写入世界失败: %v", err))
	}
	if bar != nil {
		_ = bar.Finish()
	}
	fmt.Println("结构写入世界完成, 开始打包MCWorld文件...")

	// 5. 打包为.mcworld格式
	// 生成世界名称（基于结构文件名和尺寸）
	structureName := filepath.Base(path)
	structureName = strings.TrimSuffix(structureName, filepath.Ext(structureName))
	worldName := fmt.Sprintf("%s@[0,-64,0]~[%d,%d,%d]", structureName, structureSize.Width-1, structureSize.Height-64-1, structureSize.Length-1)
	// 更新世界LevelName
	bedrockWorld.LevelDat().LevelName = worldName
	bedrockWorld.CloseWorld()

	// 压缩世界目录为ZIP, 再重命名为.mcworld
	zipPath := fmt.Sprintf("%s.zip", worldName)
	mcworldPath := fmt.Sprintf("%s.mcworld", worldName)

	// 读取世界目录下所有文件
	worldFiles, err := os.ReadDir("world")
	if err != nil {
		panic(fmt.Sprintf("读取世界目录文件失败: %v", err))
	}
	var filePaths []string
	for _, f := range worldFiles {
		filePaths = append(filePaths, filepath.Join("world", f.Name()))
	}

	// 删除已存在的压缩文件（避免冲突）
	_ = os.RemoveAll(zipPath)
	// 执行ZIP压缩
	z := archiver.Zip{}
	if err := z.Archive(filePaths, zipPath); err != nil {
		panic(fmt.Sprintf("压缩世界为ZIP失败: %v", err))
	}

	// 重命名为.mcworld并清理临时文件
	_ = os.RemoveAll(mcworldPath)
	if err := os.Rename(zipPath, mcworldPath); err != nil {
		panic(fmt.Sprintf("重命名ZIP为MCWorld失败: %v", err))
	}
	// 可选: 删除原始world目录（若不需要保留）
	_ = os.RemoveAll("world")

	// 6. 打印最终结果
	fmt.Printf("MCWorld文件打包完成！\n")
	fmt.Printf("文件保存路径: %s\n", mcworldPath)
}
