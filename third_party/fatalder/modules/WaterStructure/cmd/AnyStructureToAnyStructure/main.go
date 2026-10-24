package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/schollz/progressbar/v3"

	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/WaterStructure/structure"
)

func main() {
	start := time.Now()
	defer func() {
		fmt.Printf("转换耗时: %v\n", time.Since(start))
	}()

	var names []string
	for name := range structure.StructureNamePool {
		names = append(names, name)
	}
	fmt.Printf("支持的结构文件格式: %s\n", strings.Join(names, " "))

	reader := bufio.NewReader(os.Stdin)
	srcPath := readLine(reader, "请输入源结构文件路径: ")
	if srcPath == "" {
		panic("未提供源文件路径")
	}

	srcFile, err := os.Open(srcPath)
	if err != nil {
		panic(fmt.Sprintf("打开源文件失败: %v", err))
	}
	defer srcFile.Close()

	srcStruct, err := structure.StructureFromFile(srcFile)
	if err != nil {
		panic(fmt.Sprintf("解析源文件失败: %v", err))
	}
	fmt.Printf("检测到源文件类型: %s\n", srcStruct.Name())

	targetFormat := readLine(reader, "请输入目标结构类型名称: ")
	targetFactory, ok := structure.StructureNamePool[targetFormat]
	if !ok {
		panic(fmt.Sprintf("不支持的目标结构类型: %s", targetFormat))
	}

	targetPath := readLine(reader, "请输入目标文件输出路径: ")
	if targetPath == "" {
		panic("未提供目标文件路径")
	}
	_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
	targetFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		panic(fmt.Sprintf("创建目标文件失败: %v", err))
	}
	defer targetFile.Close()

	start = time.Now()
	// 使用临时世界作为中转
	worldPath := "world"
	fmt.Println("打开/创建临时世界中...")
	bedrockWorld, err := world.Open(worldPath, nil)
	if err != nil {
		panic(fmt.Sprintf("打开世界失败: %v", err))
	}
	defer func() {
		_ = bedrockWorld.CloseWorld()
		_ = os.RemoveAll(worldPath)
	}()

	startSubChunkPos := define.SubChunkPos{0, -4, 0}
	var bar *progressbar.ProgressBar

	fmt.Println("正在将源结构写入世界...")
	if err := srcStruct.ToMCWorld(
		bedrockWorld,
		startSubChunkPos,
		func(total int) {
			bar = buildBar(total, "写入世界")
		},
		func() {
			if bar != nil {
				_ = bar.Add(1)
			}
		},
	); err != nil {
		panic(fmt.Sprintf("写入世界失败: %v", err))
	}
	if bar != nil {
		_ = bar.Finish()
	}

	size := srcStruct.GetSize()
	startBlockPos := define.BlockPos{
		startSubChunkPos.X() * 16,
		startSubChunkPos.Y() * 16,
		startSubChunkPos.Z() * 16,
	}
	endBlockPos := define.BlockPos{
		startBlockPos.X() + int32(size.Width) - 1,
		startBlockPos.Y() + int32(size.Height) - 1,
		startBlockPos.Z() + int32(size.Length) - 1,
	}

	fmt.Println("正在从世界导出到目标结构...")
	bar = nil
	targetStruct := targetFactory()
	if err := targetStruct.FromMCWorld(
		bedrockWorld,
		targetFile,
		startBlockPos,
		endBlockPos,
		func(total int) {
			bar = buildBar(total, "导出结构")
		},
		func() {
			if bar != nil {
				_ = bar.Add(1)
			}
		},
	); err != nil {
		panic(fmt.Sprintf("导出目标结构失败: %v", err))
	}
	if bar != nil {
		_ = bar.Finish()
	}

	fmt.Printf("转换完成，输出文件: %s\n", targetPath)
}

func readLine(r *bufio.Reader, prompt string) string {
	fmt.Print(prompt)
	text, _ := r.ReadString('\n')
	return strings.TrimSpace(text)
}

func buildBar(total int, desc string) *progressbar.ProgressBar {
	if total <= 0 {
		return nil
	}
	return progressbar.NewOptions(
		total,
		progressbar.OptionSetWriter(os.Stdout),
		progressbar.OptionSetDescription(desc),
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
}
