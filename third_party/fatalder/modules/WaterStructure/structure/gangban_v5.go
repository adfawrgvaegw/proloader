package structure

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/blocks"
)

type GangBanV5 struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset
	origin       define.Origin

	palette      []string
	blocks       []gangBanBlock
	paletteCache map[paletteCacheKey]uint32

	nonAirBlocks int
}

func (g *GangBanV5) FromFile(file *os.File) error {
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	decoder := json.NewDecoder(file)
	decoder.UseNumber()

	var entries []any
	if err := decoder.Decode(&entries); err != nil {
		return fmt.Errorf("解析 GangBan V5 的 JSON 失败: %w", err)
	}
	if len(entries) < 2 {
		return ErrInvalidFile
	}

	areaEntry := entries[len(entries)-2]
	paletteEntry := entries[len(entries)-1]
	stream := entries[:len(entries)-2]

	areaMap, ok := areaEntry.(map[string]any)
	if !ok {
		return fmt.Errorf("区域条目无效")
	}
	epAny, ok := areaMap["ep"].([]any)
	if !ok || len(epAny) != 3 {
		return fmt.Errorf("区域边界条目无效")
	}
	ep := make([]int, 3)
	for i := 0; i < 3; i++ {
		val, err := toInt(epAny[i])
		if err != nil {
			return fmt.Errorf("解析 area 值 %d 失败: %w", i, err)
		}
		ep[i] = val
	}

	paletteAny, ok := paletteEntry.([]any)
	if !ok || len(paletteAny) == 0 {
		return ErrInvalidFile
	}
	palette := make([]string, len(paletteAny))
	for i, raw := range paletteAny {
		name, ok := raw.(string)
		if !ok {
			return fmt.Errorf("调色板条目 %d 不是字符串", i)
		}
		palette[i] = name
	}

	g.file = file
	return g.populateFromComponents(stream, palette, ep)
}

func (g *GangBanV5) populateFromComponents(stream []any, palette []string, ep []int) error {
	if len(ep) != 3 {
		return fmt.Errorf("区域信息无效")
	}

	g.palette = palette
	g.paletteCache = make(map[paletteCacheKey]uint32)
	g.offsetPos = define.Offset{}
	g.blocks = nil
	g.nonAirBlocks = 0

	accum := make(map[[3]int]*gangBanV4Accum)

	minX, minY, minZ := math.MaxInt, math.MaxInt, math.MaxInt
	maxX, maxY, maxZ := math.MinInt, math.MinInt, math.MinInt

	idx := 0
	for idx < len(stream) {
		if idx+6 > len(stream) {
			return fmt.Errorf("索引 %d 存在悬空的方块条目", idx)
		}

		base := [6]int{}
		for i := 0; i < 6; i++ {
			val, err := toInt(stream[idx+i])
			if err != nil {
				return fmt.Errorf("解析索引 %d 的方块值 %d 失败: %w", idx+i, i, err)
			}
			base[i] = val
		}
		idx += 6

		var payload any
		if idx < len(stream) {
			switch stream[idx].(type) {
			case []any, map[string]any:
				payload = stream[idx]
				idx++
			}
		}

		typeFlag := base[0]
		x := base[1]
		y := base[2]
		z := base[3]
		primary := base[4]
		secondary := base[5]

		if x < minX {
			minX = x
		}
		if y < minY {
			minY = y
		}
		if z < minZ {
			minZ = z
		}
		if x > maxX {
			maxX = x
		}
		if y > maxY {
			maxY = y
		}
		if z > maxZ {
			maxZ = z
		}

		var runtimeID uint32
		var nbt map[string]any

		if payloadMap, ok := payload.(map[string]any); ok {
			runtimeID, nbt = g.buildCommandBlock(primary, secondary, payloadMap)
		} else {
			if primary < 0 || primary >= len(g.palette) {
				return fmt.Errorf("调色板索引 %d 越界", primary)
			}
			runtimeID = g.runtimeIDFor(primary, secondary)

			if payloadList, ok := payload.([]any); ok {
				nbt = buildGangBanV5ContainerNBT(g.palette[primary], payloadList)
			}
		}

		key := [3]int{x, y, z}
		rec, exists := accum[key]
		if !exists {
			rec = &gangBanV4Accum{
				WorldX:    x,
				WorldY:    y,
				WorldZ:    z,
				RuntimeID: runtimeID,
				NBT:       nbt,
			}
			accum[key] = rec
		} else {
			rec.RuntimeID = runtimeID
			if rec.NBT == nil && nbt != nil {
				rec.NBT = nbt
			}
		}

		_ = typeFlag // currently unused but retained for potential future handling
	}

	if len(accum) == 0 {
		return ErrInvalidFile
	}

	width := maxX - minX + 1
	height := maxY - minY + 1
	length := maxZ - minZ + 1

	g.origin = define.Origin{int32(minX), int32(minY), int32(minZ)}
	g.size = &define.Size{Width: width, Height: height, Length: length}
	g.originalSize = &define.Size{Width: width, Height: height, Length: length}

	keys := make([][3]int, 0, len(accum))
	for key := range accum {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][1] != keys[j][1] {
			return keys[i][1] < keys[j][1]
		}
		if keys[i][2] != keys[j][2] {
			return keys[i][2] < keys[j][2]
		}
		return keys[i][0] < keys[j][0]
	})

	g.blocks = make([]gangBanBlock, 0, len(accum))
	g.nonAirBlocks = 0

	for _, key := range keys {
		rec := accum[key]
		localX := rec.WorldX - minX
		localY := rec.WorldY - minY
		localZ := rec.WorldZ - minZ

		g.blocks = append(g.blocks, gangBanBlock{
			LocalX:    localX,
			LocalY:    localY,
			LocalZ:    localZ,
			RuntimeID: rec.RuntimeID,
			NBT:       rec.NBT,
		})

		if rec.RuntimeID != block.AirRuntimeID {
			g.nonAirBlocks++
		}
	}

	// 检查是不是这个文件
	if len(g.palette) == 0 {
		return ErrInvalidFile
	}

	return nil
}

func (g *GangBanV5) buildCommandBlock(primary, variant int, payload map[string]any) (uint32, map[string]any) {
	blockName := "minecraft:command_block"
	if variant >= 0 && variant < len(CommandBlockNames) {
		blockName = "minecraft:" + CommandBlockNames[variant]
	}

	runtimeID, found := blocks.LegacyBlockToRuntimeID(blockName, uint16(primary))
	if !found {
		runtimeID = UnknownBlockRuntimeID
	}

	nbt := make(map[string]any)
	nbt["id"] = "CommandBlock"

	if cmd, ok := payload["cmd"].(string); ok {
		nbt["Command"] = cmd
	} else {
		nbt["Command"] = ""
	}

	if name, ok := payload["name"].(string); ok {
		nbt["CustomName"] = name
	}

	if delayVal, ok := payload["delay"]; ok {
		if delay, err := toInt(delayVal); err == nil {
			nbt["TickDelay"] = int32(delay)
		}
	}

	auto := false
	if v, ok := payload["auto"].(bool); ok {
		auto = v
	}
	condition := false
	if v, ok := payload["condition"].(bool); ok {
		condition = v
	}

	nbt["ExecuteOnFirstTick"] = boolToByte(auto)
	nbt["TrackOutput"] = byte(0)
	nbt["conditionalMode"] = boolToByte(condition)
	nbt["auto"] = boolToByte(auto)
	nbt["Powered"] = byte(0)
	nbt["LPCommandMode"] = int32(variant)
	nbt["LastOutput"] = ""

	return runtimeID, nbt
}

func buildGangBanV5ContainerNBT(blockName string, payload []any) map[string]any {
	if len(payload) == 0 {
		if id := chestBlockNameToID(blockName); id != "" {
			nbt := make(map[string]any)
			nbt["id"] = id
			return nbt
		}
		return nil
	}

	items := make([]map[string]any, 0, len(payload))
	for _, raw := range payload {
		itemMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		name, _ := itemMap["ns"].(string)
		aux := 0
		if val, ok := itemMap["aux"]; ok {
			if parsed, err := toInt(val); err == nil {
				aux = parsed
			}
		}
		count := 0
		if val, ok := itemMap["num"]; ok {
			if parsed, err := toInt(val); err == nil {
				count = parsed
			}
		}
		slot := 0
		if val, ok := itemMap["slot"]; ok {
			if parsed, err := toInt(val); err == nil {
				slot = parsed
			}
		}

		item := map[string]any{
			"Slot":   byte(slot),
			"Name":   name,
			"Count":  byte(count),
			"Damage": int16(aux),
		}
		items = append(items, item)
	}

	id := chestBlockNameToID(blockName)
	if id == "" && len(items) == 0 {
		return nil
	}

	nbt := make(map[string]any)
	if id != "" {
		nbt["id"] = id
	}
	if len(items) > 0 {
		nbt["Items"] = items
	}
	return nbt
}

func (g *GangBanV5) runtimeIDFor(index int, aux int) uint32 {
	if g.paletteCache == nil {
		g.paletteCache = make(map[paletteCacheKey]uint32)
	}
	key := paletteCacheKey{Index: index, Data: uint16(aux)}
	if runtimeID, ok := g.paletteCache[key]; ok {
		return runtimeID
	}

	name := g.palette[index]
	runtimeID, found := blocks.LegacyBlockToRuntimeID(name, uint16(aux))
	if !found {
		runtimeID = UnknownBlockRuntimeID
	}
	baseName, properties, found := blocks.RuntimeIDToState(runtimeID)
	if !found {
		runtimeID = UnknownBlockRuntimeID
	} else {
		runtimeID, found = block.StateToRuntimeID(baseName, properties)
		if !found {
			runtimeID = UnknownBlockRuntimeID
		}
	}
	g.paletteCache[key] = runtimeID
	return runtimeID
}

func (g *GangBanV5) GetOffsetPos() define.Offset {
	return g.offsetPos
}

func (g *GangBanV5) SetOffsetPos(offset define.Offset) {
	g.offsetPos = offset
	g.size.Width = g.originalSize.Width + int(math.Abs(float64(offset.X())))
	g.size.Length = g.originalSize.Length + int(math.Abs(float64(offset.Z())))
	g.size.Height = g.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (g *GangBanV5) GetSize() define.Size {
	return *g.size
}

func (g *GangBanV5) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	chunks := make(map[define.ChunkPos]*chunk.Chunk, len(posList))
	height := g.size.Height
	if height <= 0 {
		height = 1
	}
	for _, pos := range posList {
		if _, exists := chunks[pos]; !exists {
			chunks[pos] = chunk.NewChunk(block.AirRuntimeID, MCWorldOverworldRange)
		}
	}

	if len(chunks) == 0 {
		return chunks, nil
	}

	offsetX := int(g.offsetPos.X())
	offsetY := int(g.offsetPos.Y())
	offsetZ := int(g.offsetPos.Z())

	for _, blk := range g.blocks {
		newX := blk.LocalX + offsetX
		newY := blk.LocalY + offsetY
		newZ := blk.LocalZ + offsetZ

		chunkX := floorDiv(newX, 16)
		chunkZ := floorDiv(newZ, 16)
		chunkPos := define.ChunkPos{int32(chunkX), int32(chunkZ)}

		c, exists := chunks[chunkPos]
		if !exists {
			continue
		}

		localX := newX - chunkX*16
		localZ := newZ - chunkZ*16
		c.SetBlock(uint8(localX), int16(newY)-64, uint8(localZ), 0, blk.RuntimeID)
	}

	return chunks, nil
}

func (g *GangBanV5) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any, len(posList))
	for _, pos := range posList {
		if _, exists := result[pos]; !exists {
			result[pos] = make(map[define.BlockPos]map[string]any)
		}
	}

	if len(result) == 0 {
		return result, nil
	}

	offsetX := int(g.offsetPos.X())
	offsetY := int(g.offsetPos.Y())
	offsetZ := int(g.offsetPos.Z())

	for _, blk := range g.blocks {
		if blk.NBT == nil {
			continue
		}

		newX := blk.LocalX + offsetX
		newY := blk.LocalY + offsetY
		newZ := blk.LocalZ + offsetZ

		chunkX := floorDiv(newX, 16)
		chunkZ := floorDiv(newZ, 16)
		chunkPos := define.ChunkPos{int32(chunkX), int32(chunkZ)}

		chunkNBT, exists := result[chunkPos]
		if !exists {
			continue
		}

		localX := newX - chunkX*16
		localZ := newZ - chunkZ*16
		blockPos := define.BlockPos{int32(localX), chunkLocalYFromWorld(newY), int32(localZ)}
		chunkNBT[blockPos] = blk.NBT
	}

	return result, nil
}

func (g *GangBanV5) CountNonAirBlocks() (int, error) {
	return g.nonAirBlocks, nil
}

func (g *GangBanV5) ID() uint8 {
	return IDGangBanV5
}

func (g *GangBanV5) Name() string {
	return NameGangBanV5
}

func (g *GangBanV5) Close() error {
	return nil
}
