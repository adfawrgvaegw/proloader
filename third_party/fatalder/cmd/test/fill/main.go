package main

// 测试合并 fill 速度

import (
	"fmt"
	"os"
	"time"

	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/Yeah114/Fatalder/defines"
	"github.com/Yeah114/Fatalder/utils/chunk_fill"
	"github.com/Yeah114/WaterStructure/structure"
	wsDefine "github.com/Yeah114/WaterStructure/define"
)

func main() {
	structurePath := "/root/Fatalder/超凡脱俗的森林.mcworld"

	file, err := os.Open(structurePath)
	if err != nil {
		fmt.Printf("打开文件失败: %v\n", err)
		return
	}
	defer file.Close()

	structure, err := structure.StructureFromFile(file)
	if err != nil {
		fmt.Printf("解析结构失败: %v\n", err)
		return
	}

	defer structure.Close()

	size := structure.GetSize()
	chunkXCount := size.GetChunkXCount()
	chunkZCount := size.GetChunkZCount()
	totalChunks := chunkXCount * chunkZCount

	if totalChunks == 0 {
		fmt.Println("结构体不包含任何区块")
		return
	}

	fmt.Printf("总区块数量: %d (X: %d, Z: %d)\n", totalChunks, chunkXCount, chunkZCount)

	chunkPositions := make([]wsDefine.ChunkPos, 0, totalChunks)
	for x := 0; x < chunkXCount; x++ {
		for z := 0; z < chunkZCount; z++ {
			chunkPositions = append(chunkPositions, wsDefine.ChunkPos{int32(x), int32(z)})
		}
	}

	fmt.Println("开始加载全部区块数据...")
	loadStart := time.Now()
	rawChunkMap, err := structure.GetChunks(chunkPositions)
	if err != nil {
		fmt.Printf("获取区块失败: %v\n", err)
		return
	}
	chunkMap := make(map[defines.ChunkPos]*chunk.Chunk, len(rawChunkMap))
	for pos, c := range rawChunkMap {
		chunkMap[defines.ChunkPos{pos.X(), pos.Z()}] = c
	}
	fmt.Printf("区块加载完成，用时: %v，可用区块: %d\n", time.Since(loadStart), len(chunkMap))

	processedChunks := 0
	var totalCommands int64
	totalStart := time.Now()
	lastLog := time.Now()

	for _, pos := range chunkPositions {
		mapPos := defines.ChunkPos{pos.X(), pos.Z()}
		chunkData := chunkMap[mapPos]
		processedChunks++

		if chunkData == nil {
			if processedChunks%64 == 0 || processedChunks == totalChunks {
				fmt.Printf("区块 %d/%d 无数据\n", processedChunks, totalChunks)
			}
			continue
		}

		chunkStart := time.Now()
		basePos := defines.BlockPos{
			pos.X() * 16,
			0,
			pos.Z() * 16,
		}

		var chunkCommands int64
		for range chunk_fill.GenerateChunkCommand(chunkData, basePos) {
			chunkCommands++
		}
		totalCommands += chunkCommands

		if processedChunks%32 == 0 || processedChunks == totalChunks || time.Since(lastLog) > 5*time.Second {
			fmt.Printf(
				"区块 %d/%d => 命令数: %d, 用时: %v\n",
				processedChunks,
				totalChunks,
				chunkCommands,
				time.Since(chunkStart),
			)
			lastLog = time.Now()
		}
	}

	totalDuration := time.Since(totalStart)
	var avgDuration time.Duration
	if processedChunks > 0 {
		avgDuration = totalDuration / time.Duration(processedChunks)
	}

	fmt.Printf(
		"全部区块处理完成，总命令数: %d，总耗时: %v，平均每区块耗时: %v\n",
		totalCommands,
		totalDuration,
		avgDuration,
	)

	fmt.Println("开始执行全量合并 fill 测试...")
	mergeStart := time.Now()
	var mergedCommands int64
	var minChunkX, minChunkZ int32
	minSet := false
	for pos := range chunkMap {
		if !minSet {
			minChunkX = pos.X()
			minChunkZ = pos.Z()
			minSet = true
			continue
		}
		if pos.X() < minChunkX {
			minChunkX = pos.X()
		}
		if pos.Z() < minChunkZ {
			minChunkZ = pos.Z()
		}
	}
	startPos := defines.BlockPos{minChunkX * 16, 0, minChunkZ * 16}
	for range chunk_fill.GenerateChunksCommand(chunkMap, startPos) {
		mergedCommands++
	}
	fmt.Printf(
		"全量合并 fill 完成，命令数: %d，耗时: %v\n",
		mergedCommands,
		time.Since(mergeStart),
	)
}
