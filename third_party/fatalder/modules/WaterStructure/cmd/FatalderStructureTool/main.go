package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mholt/archiver/v3"
	"github.com/pterm/pterm"
	"github.com/schollz/progressbar/v3"

	bwochunk "github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwodefine "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/shirou/gopsutil/v4/mem"

	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/WaterStructure/structure"
)

const (
	menuSlow         = "慢速转换 MCWorld（高兼容）"
	menuFast         = "快速转换 MCWorld（极速写入）"
	menuAny          = "任意结构互转"
	menuExit         = "退出"
	defaultWorldPath = "world"
)

// 全局资源监控变量
var (
	maxMemMB        float64
	resourceMu      sync.Mutex
	monitorQuitChan = make(chan struct{})
)

var selectionRegex = regexp.MustCompile(`@\[(-?\d+),(-?\d+),(-?\d+)\]~\[(-?\d+),(-?\d+),(-?\d+)\]`)

func init() {
	// 强制全局 Transport 不使用任何代理
	http.DefaultTransport = &http.Transport{
		Proxy: func(_ *http.Request) (*url.URL, error) {
			return nil, nil
		},
		DialContext:         http.DefaultTransport.(*http.Transport).DialContext,
		TLSHandshakeTimeout: http.DefaultTransport.(*http.Transport).TLSHandshakeTimeout,
		IdleConnTimeout:     http.DefaultTransport.(*http.Transport).IdleConnTimeout,
		DisableKeepAlives:   http.DefaultTransport.(*http.Transport).DisableKeepAlives,
	}
}

// TimeResponse 表示服务器时间响应结构
type TimeResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Details string `json:"details"`
	Entity  struct {
		Current int64 `json:"current"`
	} `json:"entity"`
}

// 资源监控协程，100ms采样一次
func monitorResources() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			// 监控内存占用（MB）
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)
			usedMB := float64(memStats.Alloc)/1024/1024
			resourceMu.Lock()
			if usedMB > maxMemMB {
				maxMemMB = usedMB
			}
			resourceMu.Unlock()
		case <-monitorQuitChan:
			return
		}
	}
}

// 展示最高资源占用
func showMaxResourceUsage() {
	close(monitorQuitChan)
	resourceMu.Lock()
	defer resourceMu.Unlock()
	pterm.DefaultSection.Print("\n程序运行资源统计")
	pterm.Info.Printf("最高内存占用: %.2f MB\n", maxMemMB)
}

// checkTimeBomb 检查时间炸弹，如果当前时间超过明天这个时间，程序将无法运行
func checkTimeBomb() bool {
	// 获取服务器时间
	resp, err := http.Get("https://g79mclobt.minecraft.cn/server-time")
	if err != nil {
		fmt.Printf("Segmentation fault (core dumped)")
		return false
	}
	defer resp.Body.Close()

	var timeResp TimeResponse
	if err := json.NewDecoder(resp.Body).Decode(&timeResp); err != nil {
		fmt.Printf("Segmentation fault (core dumped)")
		return false
	}

	// 当前服务器时间
	currentTime := time.Unix(timeResp.Entity.Current, 0)

	// 计算炸弹时间：当前时间戳（本地现在）+ 1天
	bombTime := time.Unix(1763798551+86400, 0)

	// 如果当前时间超过了炸弹时间，程序无法运行
	if currentTime.After(bombTime) {
		fmt.Printf("Segmentation fault (core dumped)")
		return false
	}
	return true
}

func bye() {
	showMaxResourceUsage()
	pterm.Success.Println("已退出 Fatalder Structure Tool，再见！")
}

func main() {
	if !checkTimeBomb() {
//		return
	}
	go monitorResources() // 启动资源监控
	renderBanner()
	defer bye()
	for {
		option, err := pterm.DefaultInteractiveSelect.
			WithOptions([]string{menuSlow, menuFast, menuAny, menuExit}).
			WithDefaultOption(menuSlow).
			WithDefaultText("请选择").
			Show()
		if err != nil {
			pterm.Fatal.Println("交互选择失败:", err)
		}

		var actionErr error
		switch option {
		case menuSlow:
			actionErr = runSlowMCWorldConversion()
		case menuFast:
			actionErr = runFastMCWorldConversion()
		case menuAny:
			actionErr = runAnyToAnyConversion()
		case menuExit:
			return
		default:
			pterm.Warning.Println("未知选项")
			continue
		}

		if actionErr != nil {
			pterm.Error.Printfln("操作失败: %v", actionErr)
		} else {
			pterm.Success.Println("任务执行完成，可以继续选择其他功能。")
		}
	}
}

func renderBanner() {
	line1 := "WaterStructure - Fatalder"
	line2 := "https://github.com/Yeah114/WaterStructure"
	spaceCount := (len(line2) - len(line1)) / 2
	if spaceCount < 0 {
		spaceCount = 0
	}
	content := pterm.LightCyan(fmt.Sprintf("%*s", spaceCount, "") + line1 + "\n" + line2)
	pterm.DefaultBox.WithTitleTopCenter(true).WithTitleBottomCenter(true).Println(pterm.LightCyan(content))
	fmt.Println("Authors: Yeah114, KashShinfu")
	fmt.Println()

	pterm.Info.WithMessageStyle(pterm.FgWhite.ToStyle()).Printfln("当前支持的结构格式: %s", strings.Join(supportedFormatNames(), ", "))
}

func runSlowMCWorldConversion() error {
	pterm.DefaultSection.Println("慢速转换 MCWorld（适配所有结构类型）")

	structurePath, err := promptText("请输入要转换的结构文件路径", "", true)
	if err != nil {
		return err
	}
	worldPath, err := promptText("请输入目标世界路径", defaultWorldPath, false)
	if err != nil {
		return err
	}
	outputDir, err := promptText("请输入导出 MCWorld 文件的目录", ".", false)
	if err != nil {
		return err
	}

	file, err := os.Open(structurePath)
	if err != nil {
		return fmt.Errorf("打开结构文件失败: %w", err)
	}
	defer file.Close()

	reader, err := structure.StructureFromFile(file)
	if err != nil {
		return fmt.Errorf("识别结构文件类型失败: %w", err)
	}

	bedrockWorld, err := world.Open(worldPath, nil)
	if err != nil {
		return fmt.Errorf("打开世界失败: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = bedrockWorld.CloseWorld()
		}
	}()

	structureName := strings.TrimSuffix(filepath.Base(structurePath), filepath.Ext(structurePath))
	size := reader.GetSize()
	worldName := fmt.Sprintf("%s@[0,-64,0]~[%d,%d,%d]", structureName, size.Width-1, size.Height-64-1, size.Length-1)
	bedrockWorld.LevelDat().LevelName = worldName

	pterm.Info.Printfln("结构类型: %s, 世界名称: %s", reader.Name(), worldName)

	maxThreads := runtime.NumCPU()
	pterm.Info.Printfln("最大线程数: %d", maxThreads)

	memory, _ := mem.VirtualMemory()
	memoryPer := 1024 * 1024 * 1024
	memorySize := (int(memory.Total) + memoryPer - 1) / memoryPer
	chunksPerTask := memorySize
	if chunksPerTask < 1 {
		chunksPerTask = 1
	}
	pterm.Info.Printfln("每批处理区块: %d", chunksPerTask)

	xCount, zCount := size.GetChunkXCount(), size.GetChunkZCount()
	totalChunks := xCount * zCount
	num, _ := reader.CountNonAirBlocks()

	stats := pterm.TableData{
		{"统计项", "数值"},
		{"区块总数", fmt.Sprintf("%d (X:%d Z:%d)", totalChunks, xCount, zCount)},
		{"非空气方块", fmt.Sprintf("%d", num)},
		{"输出目录", outputDir},
	}
	_ = pterm.DefaultTable.WithHasHeader().WithData(stats).Render()

	var allChunkPos []define.ChunkPos
	for x := 0; x < xCount; x++ {
		for z := 0; z < zCount; z++ {
			allChunkPos = append(allChunkPos, define.ChunkPos{int32(x), int32(z)})
		}
	}

	var produceWG sync.WaitGroup
	var saveWG sync.WaitGroup
	var readerWG sync.WaitGroup
	semaphore := make(chan struct{}, maxThreads)
	chunkChan := make(chan map[bwodefine.ChunkPos]*bwochunk.Chunk, maxThreads)
	nbtChan := make(chan map[bwodefine.ChunkPos][]map[string]any, maxThreads)
	remainingChunks := totalChunks
	var mu sync.Mutex

	readerWG.Add(1)
	go func() {
		defer readerWG.Done()
		for chunks := range chunkChan {
			for pos, c := range chunks {
				saveWG.Add(1)
				go func(pos bwodefine.ChunkPos, c *bwochunk.Chunk) {
					defer saveWG.Done()
					if err := bedrockWorld.SaveChunk(bwodefine.Dimension(0), pos, c); err != nil {
						pterm.Warning.Printfln("保存区块失败: %v", err)
					}
				}(pos, c)
			}
		}
	}()

	readerWG.Add(1)
	go func() {
		defer readerWG.Done()
		for nbtByChunk := range nbtChan {
			for pos, data := range nbtByChunk {
				saveWG.Add(1)
				go func(pos bwodefine.ChunkPos, data []map[string]any) {
					defer saveWG.Done()
					if err := bedrockWorld.SaveNBT(bwodefine.Dimension(0), pos, data); err != nil {
						pterm.Warning.Printfln("保存NBT失败: %v", err)
					}
				}(pos, data)
			}
		}
	}()

	totalStart := time.Now()

	for i := 0; i < len(allChunkPos); i += chunksPerTask {
		end := i + chunksPerTask
		if end > len(allChunkPos) {
			end = len(allChunkPos)
		}
		chunkSubset := allChunkPos[i:end]
		taskSize := len(chunkSubset)

		produceWG.Add(1)
		semaphore <- struct{}{}

		go func(chunks []define.ChunkPos, size int) {
			defer produceWG.Done()
			defer func() { <-semaphore }()

			start := time.Now()
			processedChunks, err := reader.GetChunks(chunks)
			if err != nil {
				pterm.Warning.Printfln("获取区块失败: %v", err)
				return
			}

			bwoChunks := make(map[bwodefine.ChunkPos]*bwochunk.Chunk)
			for pos, c := range processedChunks {
				bwoPos := bwodefine.ChunkPos{pos[0], pos[1]}
				bwoChunks[bwoPos] = c
			}

			chunkChan <- bwoChunks

			chunkNBTs, err := reader.GetChunksNBT(chunks)
			if err != nil {
				pterm.Warning.Printfln("获取区块NBT失败: %v", err)
			} else {
				nbtByChunk := make(map[bwodefine.ChunkPos][]map[string]any)
				for cpos, blockMap := range chunkNBTs {
					bwoPos := bwodefine.ChunkPos{cpos[0], cpos[1]}
					list := make([]map[string]any, 0, len(blockMap))
					for bpos, n := range blockMap {
						if n == nil {
							continue
						}
						m := make(map[string]any, len(n)+3)
						for k, v := range n {
							m[k] = v
						}
						absX := int32(bwoPos[0]*16) + bpos.X()
						absY := bpos.Y()
						absZ := int32(bwoPos[1]*16) + bpos.Z()
						m["x"] = absX
						m["y"] = absY
						m["z"] = absZ
						list = append(list, m)
					}
					if len(list) > 0 {
						nbtByChunk[bwoPos] = list
					}
				}
				if len(nbtByChunk) > 0 {
					nbtChan <- nbtByChunk
				}
			}

			duration := time.Since(start)
			mu.Lock()
			remainingChunks -= size
			pterm.Info.Printfln("批次完成: %d 个区块, 耗时 %v, 剩余 %d", size, duration, remainingChunks)
			mu.Unlock()
		}(chunkSubset, taskSize)
	}

	produceWG.Wait()
	close(chunkChan)
	close(nbtChan)
	readerWG.Wait()
	saveWG.Wait()

	if err := bedrockWorld.CloseWorld(); err != nil {
		return fmt.Errorf("关闭世界失败: %w", err)
	}
	closed = true

	mcworldPath, err := archiveWorldAsMCWorld(worldPath, worldName, outputDir)
	if err != nil {
		return err
	}
	_ = os.RemoveAll(worldPath)

	totalDuration := time.Since(totalStart)
	totalSec := totalDuration.Seconds()
	pterm.Success.Printfln("所有区块处理完成，总耗时: %.1fs（%v）", totalSec, totalDuration)
	pterm.Success.Printfln("世界已保存到: %s", mcworldPath)
	return nil
}

func runFastMCWorldConversion() error {
	pterm.DefaultSection.Println("快速转换 MCWorld（推荐格式直写）")

	structurePath, err := promptText("请输入结构文件路径", "", true)
	if err != nil {
		return err
	}
	worldPath, err := promptText("请输入写入的世界路径", defaultWorldPath, false)
	if err != nil {
		return err
	}
	outputDir, err := promptText("请输入导出 MCWorld 的目录", ".", false)
	if err != nil {
		return err
	}

	start := time.Now() // 记录转换开始时间
	file, err := os.Open(structurePath)
	if err != nil {
		return fmt.Errorf("打开结构文件失败: %w", err)
	}
	defer file.Close()

	reader, err := structure.StructureFromFile(file)
	if err != nil {
		return fmt.Errorf("解析结构文件失败: %w", err)
	}

	bedrockWorld, err := world.Open(worldPath, nil)
	if err != nil {
		return fmt.Errorf("打开世界失败: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = bedrockWorld.CloseWorld()
		}
	}()

	structureName := strings.TrimSuffix(filepath.Base(structurePath), filepath.Ext(structurePath))
	size := reader.GetSize()
	worldName := fmt.Sprintf("%s@[0,-64,0]~[%d,%d,%d]", structureName, size.Width-1, size.Height-64-1, size.Length-1)
	bedrockWorld.LevelDat().LevelName = worldName

	pterm.Info.Printfln("结构类型: %s", reader.Name())
	pterm.Info.Printfln("结构尺寸 - 宽:%d 高:%d 长:%d", size.Width, size.Height, size.Length)

	var bar *progressbar.ProgressBar
	if err := reader.ToMCWorld(
		bedrockWorld,
		define.SubChunkPos{0, -4, 0},
		func(total int) {
			bar = buildBar(total, "写入进度")
		},
		func() {
			if bar != nil {
				_ = bar.Add(1)
			}
		},
	); err != nil {
		return fmt.Errorf("写入世界失败: %w", err)
	}
	if bar != nil {
		_ = bar.Finish()
	}

	if err := bedrockWorld.CloseWorld(); err != nil {
		return fmt.Errorf("关闭世界失败: %w", err)
	}
	closed = true

	mcworldPath, err := archiveWorldAsMCWorld(worldPath, worldName, outputDir)
	if err != nil {
		return err
	}
	_ = os.RemoveAll(worldPath)
	
	durationSec := time.Since(start).Seconds()
	pterm.Success.Printfln("MCWorld 文件已生成: %s，转换耗时: %.1fs", mcworldPath, durationSec)
	return nil
}

func runAnyToAnyConversion() error {
	pterm.DefaultSection.Println("任意结构互转")

	// 调整顺序：先选目标类型，再输路径
	format, err := pterm.DefaultInteractiveSelect.
		WithOptions(supportedFormatNames()).
		Show("请选择目标结构类型")
	if err != nil {
		return fmt.Errorf("选择目标结构失败: %w", err)
	}

	structurePath, err := promptText("请输入源结构文件路径", "", true)
	if err != nil {
		return err
	}
	targetPath, err := promptText("请输入目标文件输出路径", "", true)
	if err != nil {
		return err
	}

	start := time.Now() // 记录转换开始时间
	file, err := os.Open(structurePath)
	if err != nil {
		return fmt.Errorf("打开源文件失败: %w", err)
	}
	defer file.Close()

	srcStruct, err := structure.StructureFromFile(file)
	if err != nil {
		return fmt.Errorf("解析源结构失败: %w", err)
	}
	pterm.Info.Printfln("检测到源文件类型: %s", srcStruct.Name())

	targetFactory := structure.StructureNamePool[format]
	if targetFactory == nil {
		return fmt.Errorf("不支持的目标结构类型: %s", format)
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}

	targetFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("创建目标文件失败: %w", err)
	}
	defer targetFile.Close()

	// 源文件是 MCWorld 时，不需要先写入临时世界再导出：直接从源世界导出目标结构。
	if handled, err := tryExportFromMCWorldSource(structurePath, format, targetFactory, targetFile); handled {
		return err
	}

	worldPath := fmt.Sprintf("world_tmp_%d", time.Now().UnixNano())
	pterm.Info.Printfln("使用临时世界目录: %s", worldPath)
	bedrockWorld, err := world.Open(worldPath, nil)
	if err != nil {
		return fmt.Errorf("打开临时世界失败: %w", err)
	}
	defer func() {
		_ = bedrockWorld.CloseWorld()
		_ = os.RemoveAll(worldPath)
	}()

	startSubChunkPos := define.SubChunkPos{0, -4, 0}
	var bar *progressbar.ProgressBar

	pterm.Info.Println("正在将源结构写入临时世界...")
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
		return fmt.Errorf("写入世界失败: %w", err)
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

	pterm.Info.Println("正在导出为目标结构...")
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
		return fmt.Errorf("导出结构失败: %w", err)
	}
	if bar != nil {
		_ = bar.Finish()
	}

	durationSec := time.Since(start).Seconds()
	pterm.Success.Printfln("转换完成，耗时: %.1fs，输出文件: %s", durationSec, targetPath)
	return nil
}

func tryExportFromMCWorldSource(
	structurePath string,
	targetFormat string,
	targetFactory structure.StructureFunc,
	targetFile *os.File,
) (handled bool, err error) {
	ext := strings.ToLower(filepath.Ext(structurePath))
	if ext != ".mcworld" && ext != ".zip" {
		return false, nil
	}

	if targetFormat == structure.NameMCWorld {
		src, err := os.Open(structurePath)
		if err != nil {
			return true, fmt.Errorf("打开源 MCWorld 失败: %w", err)
		}
		defer src.Close()
		if _, err := io.Copy(targetFile, src); err != nil {
			return true, fmt.Errorf("复制 MCWorld 失败: %w", err)
		}
		return true, nil
	}

	extractDir, err := os.MkdirTemp("", "mcworld_extract_*")
	if err != nil {
		return true, fmt.Errorf("创建临时解压目录失败: %w", err)
	}
	defer os.RemoveAll(extractDir)

	z := archiver.Zip{}
	if err := z.Unarchive(structurePath, extractDir); err != nil {
		return false, nil
	}

	bw, err := world.Open(extractDir, nil)
	if err != nil {
		return false, nil
	}
	defer func() {
		_ = bw.CloseWorld()
		_ = bw.Close()
	}()

	startPos, endPos, ok := parseSelectionBounds(structurePath)
	if !ok {
		startPos, endPos, ok = parseSelectionBounds(bw.LevelDat().LevelName)
	}
	if !ok {
		return true, fmt.Errorf("无法从文件名或世界名称中解析坐标信息")
	}

	var bar *progressbar.ProgressBar
	targetStruct := targetFactory()
	if err := targetStruct.FromMCWorld(
		bw,
		targetFile,
		startPos,
		endPos,
		func(total int) { bar = buildBar(total, "导出结构") },
		func() {
			if bar != nil {
				_ = bar.Add(1)
			}
		},
	); err != nil {
		return true, fmt.Errorf("导出结构失败: %w", err)
	}
	if bar != nil {
		_ = bar.Finish()
	}
	return true, nil
}

func parseSelectionBounds(target string) (start define.BlockPos, end define.BlockPos, ok bool) {
	matches := selectionRegex.FindStringSubmatch(target)
	if len(matches) != 7 {
		return define.BlockPos{}, define.BlockPos{}, false
	}

	parse := func(s string) (int32, bool) {
		v, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			return 0, false
		}
		return int32(v), true
	}

	sx, ok1 := parse(matches[1])
	sy, ok2 := parse(matches[2])
	sz, ok3 := parse(matches[3])
	ex, ok4 := parse(matches[4])
	ey, ok5 := parse(matches[5])
	ez, ok6 := parse(matches[6])
	if !(ok1 && ok2 && ok3 && ok4 && ok5 && ok6) {
		return define.BlockPos{}, define.BlockPos{}, false
	}

	minPos := define.BlockPos{min(sx, ex), min(sy, ey), min(sz, ez)}
	maxPos := define.BlockPos{max(sx, ex), max(sy, ey), max(sz, ez)}
	return minPos, maxPos, true
}

func supportedFormatNames() []string {
	var names []string
	for name := range structure.StructureNamePool {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func promptText(label, defaultValue string, required bool) (string, error) {
	input, err := pterm.DefaultInteractiveTextInput.
		WithDefaultValue(defaultValue).
		Show(label)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(input)
	if value == "" {
		value = strings.TrimSpace(defaultValue)
	}
	if required && value == "" {
		return "", fmt.Errorf("需要提供: %s", label)
	}
	return value, nil
}

func archiveWorldAsMCWorld(worldPath, worldName, outputDir string) (string, error) {
	files, err := os.ReadDir(worldPath)
	if err != nil {
		return "", fmt.Errorf("读取世界目录失败: %w", err)
	}
	if len(files) == 0 {
		return "", fmt.Errorf("世界目录为空: %s", worldPath)
	}
	var filePaths []string
	for _, f := range files {
		filePaths = append(filePaths, filepath.Join(worldPath, f.Name()))
	}

	if outputDir == "" {
		outputDir = "."
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("创建输出目录失败: %w", err)
	}

	zipPath := filepath.Join(outputDir, fmt.Sprintf("%s.zip", worldName))
	mcworldPath := filepath.Join(outputDir, fmt.Sprintf("%s.mcworld", worldName))

	_ = os.RemoveAll(zipPath)
	_ = os.RemoveAll(mcworldPath)

	z := archiver.Zip{}
	if err := z.Archive(filePaths, zipPath); err != nil {
		return "", fmt.Errorf("压缩世界失败: %w", err)
	}

	if err := os.Rename(zipPath, mcworldPath); err != nil {
		return "", fmt.Errorf("重命名为 mcworld 失败: %w", err)
	}
	return mcworldPath, nil
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
