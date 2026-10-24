package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"runtime"

	"github.com/shirou/gopsutil/v4/mem"
	"github.com/mholt/archiver/v3"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/Yeah114/WaterStructure/structure"
	"github.com/Yeah114/WaterStructure/define"
)

func main() {
	names := ""
	for name := range structure.StructureNamePool {
		names += name + " "
	}
	fmt.Printf("支持的结构文件格式: %s\n", names)
	// 根据cpu核数设置最大线程
	maxThreads := runtime.NumCPU()
	fmt.Printf("最大线程: %d\n", maxThreads)
	// 根据存储内存大小除以1GB设置每次最大任务
	memory, _ := mem.VirtualMemory()
	// 向上取整
	memoryPer := 1024 * 1024 * 1024
	memorySize := (int(memory.Total) + memoryPer - 1) / memoryPer
	chunksPerTask := int(memorySize)
	fmt.Printf("每次最大任务: %d\n", chunksPerTask)
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("请输入要转换的结构文件路径: ")
	inputPath, err := reader.ReadString('\n')
	if err != nil {
		panic(fmt.Sprintf("读取输入失败: %v", err))
	}
	structurePath := strings.TrimSpace(inputPath)
	if structurePath == "" {
		panic("未提供结构文件路径")
	}
	// 取文件路径的最后一段和去掉后缀作为结构名称
	structureName := filepath.Base(structurePath)
	structureName = strings.TrimSuffix(structureName, filepath.Ext(structureName))

	fmt.Print("请输入要转换的世界路径(默认为world): ")
	worldPath, err := reader.ReadString('\n')
	if err != nil {
		panic(fmt.Sprintf("读取输入失败: %v", err))
	}
	worldPath = strings.TrimSpace(worldPath)
	if worldPath == "" {
		worldPath = "world"
	}

	bedrockWorld, err := world.Open(worldPath, nil)
	if err != nil {
		panic(fmt.Sprintf("打开世界失败: %v", err))
	}

	file, err := os.Open(structurePath)
	if err != nil {
		panic(fmt.Sprintf("打开文件失败: %v", err))
	}
	defer file.Close()

	fmt.Println("正在进行文件转换...")
	structure, err := structure.StructureFromFile(file)
	if err != nil {
		panic(fmt.Sprintf("判断文件类型失败: %v", err))
	}
	fmt.Printf("文件类型: %s\n", structure.Name())

	size := structure.GetSize()
	worldName := fmt.Sprintf("%s@[0,-64,0]~[%d,%d,%d]", structureName, size.Width-1, size.Height-64-1, size.Length-1)
	xCount, zCount := size.GetChunkXCount(), size.GetChunkZCount()
	totalChunks := xCount * zCount
	fmt.Printf("总区块数量: %d (X:%d, Z:%d)\n", totalChunks, xCount, zCount)
	num, _ := structure.CountNonAirBlocks()
	fmt.Printf("总非空方块数: %d\n", num)

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
	chunkChan := make(chan map[bwo_define.ChunkPos]*chunk.Chunk, maxThreads)
	nbtChan := make(chan map[bwo_define.ChunkPos][]map[string]any, maxThreads)
	remainingChunks := totalChunks // 剩余区块计数器
	var mu sync.Mutex              // 用于保护计数器的互斥锁

	// 保存区块的 goroutine
	readerWG.Add(1)
	go func() {
		defer readerWG.Done()
		for chunks := range chunkChan {
			for pos, c := range chunks {
				saveWG.Add(1)
				go func(pos bwo_define.ChunkPos, c *chunk.Chunk) {
					defer saveWG.Done()
					if err := bedrockWorld.SaveChunk(bwo_define.Dimension(0), pos, c); err != nil {
						fmt.Printf("保存区块失败: %v\n", err)
					}
				}(pos, c)
			}
		}
	}()

	// 保存 NBT 的 goroutine
	readerWG.Add(1)
	go func() {
		defer readerWG.Done()
		for nbtByChunk := range nbtChan {
			for pos, data := range nbtByChunk {
				saveWG.Add(1)
				go func(pos bwo_define.ChunkPos, data []map[string]any) {
					defer saveWG.Done()
					if err := bedrockWorld.SaveNBT(bwo_define.Dimension(0), pos, data); err != nil {
						fmt.Printf("保存NBT失败: %v\n", err)
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
			processedChunks, err := structure.GetChunks(chunks)
			if err != nil {
				fmt.Printf("获取区块失败: %v\n", err)
				return
			}

			bwoChunks := make(map[bwo_define.ChunkPos]*chunk.Chunk)
			for pos, c := range processedChunks {
				bwoPos := bwo_define.ChunkPos{pos[0], pos[1]}
				bwoChunks[bwoPos] = c
			}

			chunkChan <- bwoChunks

			// 处理并发送对应的区块 NBT
			chunkNBTs, err := structure.GetChunksNBT(chunks)
			if err != nil {
				fmt.Printf("获取区块NBT失败: %v\n", err)
			} else {
				nbtByChunk := make(map[bwo_define.ChunkPos][]map[string]any)
				for cpos, blockMap := range chunkNBTs {
					bwoPos := bwo_define.ChunkPos{cpos[0], cpos[1]}
					list := make([]map[string]any, 0, len(blockMap))
					for bpos, n := range blockMap {
						if n == nil {
							continue
						}
						m := make(map[string]any, len(n)+3)
						for k, v := range n {
							m[k] = v
						}
						// 计算绝对坐标并覆盖 x/y/z
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
			// 更新剩余区块数量并打印
			mu.Lock()
			remainingChunks -= size
			fmt.Printf("处理完成 %d 个区块，耗时: %v，剩余区块: %d\n", size, duration, remainingChunks)
			mu.Unlock()
		}(chunkSubset, taskSize)
	}

	// 等待生产者完成并关闭通道
	produceWG.Wait()
	close(chunkChan)
	close(nbtChan)
	// 等待读取者将通道完全耗尽并启动所有保存任务
	readerWG.Wait()
	// 等待所有保存任务完成
	saveWG.Wait()
	// 关闭世界
	bedrockWorld.LevelDat().LevelName = worldName
	bedrockWorld.CloseWorld()
	zipName := fmt.Sprintf("%s.zip", worldName)
	mcworldName := fmt.Sprintf("%s.mcworld", worldName)
	// 使用zip压缩世界所在目录里面内容
	files, err := os.ReadDir(worldPath)
	if err != nil {
		panic(err)
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, filepath.Join(worldPath, file.Name()))
	}
	// 删除一遍 防止文件存在
	_ = os.RemoveAll(zipName)
	z := archiver.Zip{}
	err = z.Archive(paths, zipName)
	if err != nil {
		panic(err)
	}

	os.RemoveAll(worldPath)
	// 修改后缀为mcworld
	os.Rename(zipName, mcworldName)

	totalDuration := time.Since(totalStart)
	fmt.Printf("所有区块处理完成，总耗时: %v\n", totalDuration)
	fmt.Printf("世界已保存到 %s\n", mcworldName)
}
