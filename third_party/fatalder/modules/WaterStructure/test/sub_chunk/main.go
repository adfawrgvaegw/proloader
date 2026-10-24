package main

import (
	"fmt"
	"runtime"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
)

const subChunkCount = 1_000_000

func main() {
	runtime.GC()
	before := readMemStats()
	fmt.Printf("Before allocation: %s\n", formatMemStats(before))

	subChunks := make([]*chunk.SubChunk, subChunkCount)
	for i := 0; i < subChunkCount; i++ {
		subChunks[i] = chunk.NewSubChunk(block.AirRuntimeID)
	}

	after := readMemStats()
	fmt.Printf("After allocation: %s\n", formatMemStats(after))
	fmt.Printf("Delta alloc: %.2f MB for %d sub chunks\n", bytesToMB(after.Alloc-before.Alloc), subChunkCount)

	runtime.KeepAlive(subChunks)
}

func readMemStats() runtime.MemStats {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats
}

func formatMemStats(stats runtime.MemStats) string {
	return fmt.Sprintf("Alloc=%.2f MB TotalAlloc=%.2f MB Sys=%.2f MB NumGC=%d",
		bytesToMB(stats.Alloc), bytesToMB(stats.TotalAlloc), bytesToMB(stats.Sys), stats.NumGC)
}

func bytesToMB(b uint64) float64 {
	return float64(b) / 1024.0 / 1024.0
}
