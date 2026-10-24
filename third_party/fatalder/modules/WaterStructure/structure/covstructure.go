package structure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/WaterStructure/utils"
)

type CovStructure struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset

	palette      map[int]covPaletteEntry
	nonAirBlocks int
}

type covPaletteEntry struct {
	name string
	data *int
}

func (c *CovStructure) ID() uint8 {
	return IDCovStructure
}

func (c *CovStructure) Name() string {
	return NameCovStructure
}

func (c *CovStructure) FromFile(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	raw, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("读取 covstructure 失败: %w", err)
	}
	if len(raw) == 0 {
		return ErrInvalidFile
	}

	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("解析 covstructure JSON 失败: %w", err)
	}

	structure, _ := data["structure"].(map[string]any)
	sizeAny, ok := data["size"]
	if !ok {
		sizeAny = data["dimensions"]
	}
	sizeList := covToIntSlice(sizeAny)
	if len(sizeList) < 3 {
		return ErrInvalidFile
	}
	sx, sy, sz := sizeList[0], sizeList[1], sizeList[2]
	if sx <= 0 || sy <= 0 || sz <= 0 {
		return ErrInvalidFile
	}

	paletteRaw := any(nil)
	if structure != nil {
		paletteRaw = structure["palette"]
	}
	if paletteRaw == nil {
		paletteRaw = data["palette"]
	}

	entries := findCovPaletteEntries(paletteRaw)
	c.palette = buildCovPaletteIndex(entries)

	c.file = file
	c.offsetPos = define.Offset{}
	c.size = &define.Size{Width: sx, Height: sy, Length: sz}
	c.originalSize = &define.Size{Width: sx, Height: sy, Length: sz}
	c.nonAirBlocks = -1

	return nil
}

func covToIntSlice(v any) []int {
	switch t := v.(type) {
	case []any:
		out := make([]int, 0, len(t))
		for _, it := range t {
			if i, ok := covToInt(it); ok {
				out = append(out, i)
			}
		}
		return out
	default:
		return nil
	}
}

func covToInt(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int8:
		return int(t), true
	case int16:
		return int(t), true
	case int32:
		return int(t), true
	case int64:
		return int(t), true
	case uint:
		return int(t), true
	case uint8:
		return int(t), true
	case uint16:
		return int(t), true
	case uint32:
		return int(t), true
	case uint64:
		return int(t), true
	case float32:
		return int(t), true
	case float64:
		return int(t), true
	case json.Number:
		if i64, err := t.Int64(); err == nil {
			return int(i64), true
		}
		if f64, err := t.Float64(); err == nil {
			return int(f64), true
		}
		return 0, false
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
			return i, true
		}
		return 0, false
	default:
		return 0, false
	}
}

func findCovPaletteEntries(paletteRaw any) []map[string]any {
	if paletteRaw == nil {
		return nil
	}
	if arr, ok := paletteRaw.([]any); ok {
		out := make([]map[string]any, 0, len(arr))
		for _, v := range arr {
			if m, ok := v.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	if m, ok := paletteRaw.(map[string]any); ok {
		for _, v := range m {
			if vm, ok := v.(map[string]any); ok {
				if bp, ok := vm["block_palette"].([]any); ok {
					out := make([]map[string]any, 0, len(bp))
					for _, item := range bp {
						if im, ok := item.(map[string]any); ok {
							out = append(out, im)
						}
					}
					return out
				}
			}
		}
		entries := make([]map[string]any, 0)
		for _, v := range m {
			vm, ok := v.(map[string]any)
			if !ok {
				continue
			}
			if hasAnyKey(vm, "name", "states", "properties", "id", "block", "data", "meta") {
				entries = append(entries, vm)
			}
		}
		if len(entries) > 0 {
			return entries
		}
	}
	return nil
}

func hasAnyKey(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

func buildCovPaletteIndex(entries []map[string]any) map[int]covPaletteEntry {
	idxMap := make(map[int]covPaletteEntry)
	for i, entry := range entries {
		val := i
		if v, ok := covToInt(entry["val"]); ok {
			val = v
		}
		name := ""
		for _, key := range []string{"name", "block", "id"} {
			if s, ok := entry[key].(string); ok && s != "" {
				name = s
				break
			}
		}
		if name == "" {
			name = "minecraft:air"
		}

		var dataPtr *int
		for _, key := range []string{"data", "meta", "damage", "value"} {
			if v, ok := covToInt(entry[key]); ok {
				dataPtr = new(int)
				*dataPtr = v
				break
			}
		}

		idxMap[val] = covPaletteEntry{name: name, data: dataPtr}
	}
	return idxMap
}

func (c *CovStructure) GetOffsetPos() define.Offset {
	return c.offsetPos
}

func (c *CovStructure) SetOffsetPos(offset define.Offset) {
	c.offsetPos = offset
	c.size.Width = c.originalSize.Width + int(math.Abs(float64(offset.X())))
	c.size.Length = c.originalSize.Length + int(math.Abs(float64(offset.Z())))
	c.size.Height = c.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (c *CovStructure) GetSize() define.Size {
	return *c.size
}

func (c *CovStructure) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	chunks := make(map[define.ChunkPos]*chunk.Chunk, len(posList))
	for _, pos := range posList {
		if _, exists := chunks[pos]; !exists {
			chunks[pos] = chunk.NewChunk(block.AirRuntimeID, MCWorldOverworldRange)
		}
	}
	if len(chunks) == 0 {
		return chunks, nil
	}
	if c.file == nil {
		return nil, fmt.Errorf("CovStructure 文件未初始化")
	}

	file, err := os.Open(c.file.Name())
	if err != nil {
		return nil, fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("读取 covstructure 失败: %w", err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("解析 covstructure JSON 失败: %w", err)
	}
	structure, _ := data["structure"].(map[string]any)

	blockIndices := any(nil)
	if structure != nil {
		blockIndices = structure["block_indices"]
		if blockIndices == nil {
			blockIndices = structure["blocks"]
		}
	}
	if blockIndices == nil {
		return chunks, nil
	}

	sx, sy, sz := c.originalSize.Width, c.originalSize.Height, c.originalSize.Length
	if sx <= 0 || sy <= 0 || sz <= 0 {
		return chunks, nil
	}

	offsetX := int(c.offsetPos.X())
	offsetY := int(c.offsetPos.Y())
	offsetZ := int(c.offsetPos.Z())

	flat := make([]any, 0)
	flattenAny(blockIndices, &flat)
	total := sx * sy * sz
	if total > 0 && len(flat) > total {
		flat = flat[:total]
	}

	for idx, palIdx := range flat {
		if palIdx == nil {
			continue
		}
		if v, ok := covToInt(palIdx); ok && v == -1 {
			continue
		}

		x := 0
		y := 0
		z := idx
		if sx > 0 && sy > 0 && sz > 0 {
			x = idx % sx
			z = (idx / sx) % sz
			y = idx / (sx * sz)
		}

		entry, ok := resolveCovPaletteValue(palIdx, c.palette)
		if !ok {
			continue
		}
		name := strings.TrimSpace(entry.name)
		if name == "" || strings.EqualFold(name, "minecraft:air") {
			continue
		}

		runtimeID := UnknownBlockRuntimeID
		if entry.data != nil {
			runtimeID = legacyBlockToBedrockRuntimeID(name, uint16(*entry.data))
		} else {
			runtimeID = runtimeIDForBlock(name, nil)
		}

		newX := x + offsetX
		newY := y + offsetY
		newZ := z + offsetZ

		chunkX := floorDiv(newX, 16)
		chunkZ := floorDiv(newZ, 16)
		chunkPos := define.ChunkPos{int32(chunkX), int32(chunkZ)}
		target, exists := chunks[chunkPos]
		if !exists {
			continue
		}
		localX := newX - chunkX*16
		localZ := newZ - chunkZ*16
		target.SetBlock(uint8(localX), int16(newY)-64, uint8(localZ), 0, runtimeID)
	}

	return chunks, nil
}

func resolveCovPaletteValue(palIdx any, idxMap map[int]covPaletteEntry) (covPaletteEntry, bool) {
	if palIdx == nil {
		return covPaletteEntry{}, false
	}
	if m, ok := palIdx.(map[string]any); ok {
		name := ""
		for _, key := range []string{"name", "id", "block"} {
			if s, ok := m[key].(string); ok && s != "" {
				name = s
				break
			}
		}
		var dataPtr *int
		for _, key := range []string{"data", "meta", "damage", "value"} {
			if v, ok := covToInt(m[key]); ok {
				dataPtr = new(int)
				*dataPtr = v
				break
			}
		}
		if name == "" {
			name = "minecraft:air"
		}
		return covPaletteEntry{name: name, data: dataPtr}, true
	}
	if i, ok := covToInt(palIdx); ok {
		entry, ok := idxMap[i]
		return entry, ok
	}
	return covPaletteEntry{}, false
}

func flattenAny(v any, out *[]any) {
	switch t := v.(type) {
	case []any:
		for _, it := range t {
			flattenAny(it, out)
		}
	default:
		*out = append(*out, t)
	}
}

func (c *CovStructure) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any, len(posList))
	for _, pos := range posList {
		if _, exists := result[pos]; !exists {
			result[pos] = make(map[define.BlockPos]map[string]any)
		}
	}
	return result, nil
}

func (c *CovStructure) CountNonAirBlocks() (int, error) {
	if c.nonAirBlocks >= 0 {
		return c.nonAirBlocks, nil
	}
	if c.file == nil {
		return 0, fmt.Errorf("CovStructure 文件未初始化")
	}

	file, err := os.Open(c.file.Name())
	if err != nil {
		return 0, fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		return 0, fmt.Errorf("读取 covstructure 失败: %w", err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return 0, fmt.Errorf("解析 covstructure JSON 失败: %w", err)
	}
	structure, _ := data["structure"].(map[string]any)

	blockIndices := any(nil)
	if structure != nil {
		blockIndices = structure["block_indices"]
		if blockIndices == nil {
			blockIndices = structure["blocks"]
		}
	}
	if blockIndices == nil {
		c.nonAirBlocks = 0
		return 0, nil
	}

	flat := make([]any, 0)
	flattenAny(blockIndices, &flat)

	nonAirBlocks := 0
	for _, palIdx := range flat {
		entry, ok := resolveCovPaletteValue(palIdx, c.palette)
		if !ok {
			continue
		}
		name := strings.TrimSpace(entry.name)
		if name == "" || strings.EqualFold(name, "minecraft:air") {
			continue
		}
		nonAirBlocks++
	}

	c.nonAirBlocks = nonAirBlocks
	return nonAirBlocks, nil
}

func (c *CovStructure) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
) error {
	if bedrockWorld == nil {
		return fmt.Errorf("bedrock 世界为 nil")
	}
	if c.file == nil {
		return fmt.Errorf("CovStructure 文件未初始化")
	}

	startX := startSubChunkPos.X() * 16
	startY := startSubChunkPos.Y() * 16
	startZ := startSubChunkPos.Z() * 16

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	mcworld, err := utils.NewMCWorld(bedrockWorld, ctx)
	if err != nil {
		return err
	}
	mcworld.AutoFlush(time.Second)

	totalProgress := 100
	if startCallback != nil {
		startCallback(totalProgress)
	}

	file, err := os.Open(c.file.Name())
	if err != nil {
		return fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("读取 covstructure 失败: %w", err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("解析 covstructure JSON 失败: %w", err)
	}
	structure, _ := data["structure"].(map[string]any)
	blockIndices := any(nil)
	if structure != nil {
		blockIndices = structure["block_indices"]
		if blockIndices == nil {
			blockIndices = structure["blocks"]
		}
	}
	if blockIndices == nil {
		return nil
	}

	sx, sy, sz := c.originalSize.Width, c.originalSize.Height, c.originalSize.Length
	totalItems := sx * sy * sz
	if totalItems <= 0 {
		totalItems = 1
	}

	offsetX := int(c.offsetPos.X())
	offsetY := int(c.offsetPos.Y())
	offsetZ := int(c.offsetPos.Z())

	flat := make([]any, 0)
	flattenAny(blockIndices, &flat)
	if total := sx * sy * sz; total > 0 && len(flat) > total {
		flat = flat[:total]
	}

	currentItem := 0
	lastReportedProgress := -1
	for idx, palIdx := range flat {
		entry, ok := resolveCovPaletteValue(palIdx, c.palette)
		if ok {
			name := strings.TrimSpace(entry.name)
			if name != "" && !strings.EqualFold(name, "minecraft:air") {
				x := idx % sx
				z := (idx / sx) % sz
				y := idx / (sx * sz)

				runtimeID := UnknownBlockRuntimeID
				if entry.data != nil {
					runtimeID = legacyBlockToBedrockRuntimeID(name, uint16(*entry.data))
				} else {
					runtimeID = runtimeIDForBlock(name, nil)
				}

				wx := x + offsetX
				wy := y + offsetY
				wz := z + offsetZ

				ax := startX + int32(wx)
				ay := int16(int(startY) + wy)
				az := startZ + int32(wz)
				if err := mcworld.SetBlock(ax, ay, az, runtimeID); err != nil {
					return err
				}
			}
		}

		currentItem++
		currentProgress := (currentItem * totalProgress) / totalItems
		if progressCallback != nil && currentProgress > lastReportedProgress {
			for j := lastReportedProgress + 1; j <= currentProgress; j++ {
				progressCallback()
			}
			lastReportedProgress = currentProgress
		}
	}

	mcworld.Flush()
	if progressCallback != nil && lastReportedProgress < totalProgress {
		for j := lastReportedProgress + 1; j <= totalProgress; j++ {
			progressCallback()
		}
	}

	return nil
}

func (c *CovStructure) Close() error {
	return nil
}
