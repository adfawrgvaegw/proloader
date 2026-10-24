package structure

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/blocks"
	"github.com/Yeah114/blocks/snbt"
)

type FuHongV2 struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset
	origin       define.Origin

	paletteCache map[string]uint32
	blocks       []fuHongV2Block

	nonAirBlocks int
}

type fuHongV2Block struct {
	LocalX    int
	LocalY    int
	LocalZ    int
	RuntimeID uint32
	NBT       map[string]any
}

type fuHongV2Root struct {
	BuildInfo             map[string]any   `json:"Build_Info"`
	FuHongBuildFinalArray []map[string]any `json:"FuHongBuild_FinalFormat"`
}

func (f *FuHongV2) ID() uint8 {
	return IDFuHongV2
}

func (f *FuHongV2) Name() string {
	return NameFuHongV2
}

func (f *FuHongV2) FromFile(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	var root fuHongV2Root
	if err := json.NewDecoder(file).Decode(&root); err != nil {
		return fmt.Errorf("解析 FuHong V2 的 JSON 失败: %w", err)
	}

	if len(root.FuHongBuildFinalArray) == 0 {
		return ErrInvalidFile
	}

	f.file = file
	return f.populate(root.FuHongBuildFinalArray)
}

func (f *FuHongV2) populate(chunksData []map[string]any) error {
	f.paletteCache = make(map[string]uint32)
	f.blocks = nil
	f.nonAirBlocks = 0

	accum := make(map[[3]int]*fuHongV2Block)
	minX, minY, minZ := math.MaxInt, math.MaxInt, math.MaxInt
	maxX, maxY, maxZ := math.MinInt, math.MinInt, math.MinInt

	for chunkIdx, chunkData := range chunksData {
		entries, ok := chunkData["block"].([]any)
		if !ok {
			return fmt.Errorf("区块 %d: 缺少方块列表", chunkIdx)
		}

		for entryIdx, entryRaw := range entries {
			entryMap, ok := entryRaw.(map[string]any)
			if !ok {
				continue
			}

			if nameRaw, exists := entryMap["n"]; exists {
				// block group
				name := strings.TrimSpace(fmt.Sprint(nameRaw))
				if name == "" {
					return fmt.Errorf("区块 %d 条目 %d: 方块名称为空", chunkIdx, entryIdx)
				}

				xs, err := toIntSlice(entryMap["x"])
				if err != nil {
					return fmt.Errorf("区块 %d 条目 %d: x 坐标: %w", chunkIdx, entryIdx, err)
				}
				ys, err := toIntSlice(entryMap["y"])
				if err != nil {
					return fmt.Errorf("区块 %d 条目 %d: y 坐标: %w", chunkIdx, entryIdx, err)
				}
				zs, err := toIntSlice(entryMap["z"])
				if err != nil {
					return fmt.Errorf("区块 %d 条目 %d: z 坐标: %w", chunkIdx, entryIdx, err)
				}

				if len(xs) != len(ys) || len(xs) != len(zs) {
					return fmt.Errorf("区块 %d 条目 %d: 坐标长度不匹配", chunkIdx, entryIdx)
				}
				auxValues := expandAuxValues(entryMap["a"], len(xs))

				props := parseStateMap(entryMap["state"])

				commandPayloads := buildFuHongV2CommandPayload(entryMap["c"], len(xs))
				containerPayloads := buildFuHongV2ContainerPayload(name, entryMap["d"], len(xs))

				for i := 0; i < len(xs); i++ {
					x, y, z := xs[i], ys[i], zs[i]
					runtimeID := f.runtimeIDFor(name, auxValues[i], props)
					key := [3]int{x, y, z}

					existing, exists := accum[key]
					if !exists {
						existing = &fuHongV2Block{
							LocalX:    x,
							LocalY:    y,
							LocalZ:    z,
							RuntimeID: runtimeID,
						}
						accum[key] = existing
					} else {
						existing.RuntimeID = runtimeID
					}

					nbt := mergeNBT(commandPayloads, containerPayloads, props, i)
					if nbt != nil {
						existing.NBT = nbt
					}

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
				}
			}
		}
	}

	if len(accum) == 0 {
		return ErrInvalidFile
	}

	width := maxX - minX + 1
	height := maxY - minY + 1
	length := maxZ - minZ + 1

	f.origin = define.Origin{int32(minX), int32(minY), int32(minZ)}
	f.size = &define.Size{Width: width, Height: height, Length: length}
	f.originalSize = &define.Size{Width: width, Height: height, Length: length}

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

	f.blocks = make([]fuHongV2Block, 0, len(accum))
	for _, key := range keys {
		rec := accum[key]
		blk := fuHongV2Block{
			LocalX:    rec.LocalX - int(f.origin.X()),
			LocalY:    rec.LocalY - int(f.origin.Y()),
			LocalZ:    rec.LocalZ - int(f.origin.Z()),
			RuntimeID: rec.RuntimeID,
			NBT:       rec.NBT,
		}
		f.blocks = append(f.blocks, blk)
		if blk.RuntimeID != block.AirRuntimeID {
			f.nonAirBlocks++
		}
	}

	if len(f.paletteCache) == 0 {
		return ErrInvalidFile
	}

	return nil
}

func expandAuxValues(raw any, count int) []int {
	result := make([]int, count)
	if raw == nil {
		return result
	}

	if val, err := toInt(raw); err == nil {
		for i := range result {
			result[i] = val
		}
		return result
	}

	values, err := toIntSlice(raw)
	if err != nil || len(values) == 0 {
		return result
	}
	for i := 0; i < count && i < len(values); i++ {
		result[i] = values[i]
	}
	if len(values) > 0 {
		last := values[len(values)-1]
		for i := len(values); i < count; i++ {
			result[i] = last
		}
	}
	return result
}

func parseStateMap(raw any) map[string]string {
	state := make(map[string]string)
	stateArray, ok := raw.([]any)
	if !ok {
		return state
	}
	for _, entry := range stateArray {
		pair, ok := entry.(string)
		if !ok {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.Trim(parts[0], " \"")
		val := strings.Trim(parts[1], " \"")
		state[key] = val
	}
	return state
}

func buildFuHongV2CommandPayload(raw any, count int) []map[string]any {
	result := make([]map[string]any, count)
	cMap, ok := raw.(map[string]any)
	if !ok {
		return result
	}

	commands, _ := cMap["c"].([]any)
	ticks, _ := cMap["t"].([]any)
	autos, _ := cMap["a"].([]any)
	names, _ := cMap["n"].([]any)

	for i := 0; i < count; i++ {
		payload := make([]any, 0, 4)
		payload = append(payload, extractIndexed(commands, i))
		payload = append(payload, extractIndexed(ticks, i))
		payload = append(payload, extractIndexed(autos, i))
		payload = append(payload, extractIndexed(names, i))

		nbt := buildFuHongCommandNBT(payload)
		if nbt != nil {
			result[i] = nbt
		}
	}
	return result
}

func extractIndexed(list []any, index int) any {
	if index < len(list) {
		return list[index]
	}
	if len(list) == 0 {
		return nil
	}
	return list[len(list)-1]
}

func buildFuHongV2ContainerPayload(blockName string, raw any, count int) []map[string]any {
	result := make([]map[string]any, count)
	payloads, ok := raw.([]any)
	if !ok {
		return result
	}

	for idx, entry := range payloads {
		if idx >= count {
			break
		}
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}

		if snbtRaw, ok := entryMap["e"].(string); ok {
			parsed, err := snbt.SNBToNbt(snbtRaw)
			if err != nil {
				continue
			}
			if compound, ok := convertToMap(parsed).(map[string]any); ok {
				result[idx] = compound
			}
			continue
		}

		if items, ok := entryMap["d"].([]any); ok {
			nbt := buildFuHongV2ItemsNBT(blockName, items)
			if nbt != nil {
				result[idx] = nbt
			}
		}
	}

	return result
}

func buildFuHongV2ItemsNBT(blockName string, raw []any) map[string]any {
	if len(raw) == 0 {
		return nil
	}

	items := make([]map[string]any, 0, len(raw))
	for _, itemRaw := range raw {
		itemMap, ok := itemRaw.(map[string]any)
		if !ok {
			continue
		}
		name := fmt.Sprint(firstNotNil(itemMap["name"], itemMap["Name"]))
		if name != "" && !strings.Contains(name, ":") {
			name = "minecraft:" + name
		}
		damage, _ := toInt(firstNotNil(itemMap["damage"], itemMap["Damage"]))
		count, _ := toInt(firstNotNil(itemMap["count"], itemMap["Count"]))
		slot, _ := toInt(firstNotNil(itemMap["slot"], itemMap["Slot"]))

		item := map[string]any{
			"Name":   name,
			"Damage": int16(damage),
			"Count":  byte(count),
			"Slot":   byte(slot),
		}
		items = append(items, item)
	}

	if len(items) == 0 {
		return nil
	}

	nbt := make(map[string]any)
	if id := chestBlockNameToID(blockName); id != "" {
		nbt["id"] = id
	}
	nbt["Items"] = items
	return nbt
}

func mergeNBT(commandPayloads, containerPayloads []map[string]any, props map[string]string, index int) map[string]any {
	var nbt map[string]any
	if index < len(commandPayloads) && commandPayloads[index] != nil {
		nbt = copyMap(commandPayloads[index])
	}
	if index < len(containerPayloads) && containerPayloads[index] != nil {
		if nbt == nil {
			nbt = copyMap(containerPayloads[index])
		} else {
			for k, v := range containerPayloads[index] {
				nbt[k] = v
			}
		}
	}
	if nbt == nil {
		return nil
	}

	if conditional, ok := props["conditional_bit"]; ok {
		nbt["conditionalMode"] = boolToByte(strings.EqualFold(conditional, "true"))
	}

	if custom, ok := nbt["CustomName"].(string); ok && custom == "" {
		delete(nbt, "CustomName")
	}

	return nbt
}

func copyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func convertToMap(value any) any {
	switch v := value.(type) {
	case map[string]interface{}:
		result := make(map[string]any, len(v))
		for key, val := range v {
			result[key] = convertToMap(val)
		}
		return result
	case []interface{}:
		arr := make([]any, len(v))
		for i, item := range v {
			arr[i] = convertToMap(item)
		}
		return arr
	default:
		return v
	}
}

func firstNotNil(values ...any) any {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

func (f *FuHongV2) runtimeIDFor(name string, aux int, state map[string]string) uint32 {
	cacheKey := fmt.Sprintf("%s|%d", name, aux)
	if len(state) > 0 {
		keys := make([]string, 0, len(state))
		for k := range state {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		builder := strings.Builder{}
		builder.WriteString(name)
		builder.WriteString("|")
		for idx, k := range keys {
			if idx > 0 {
				builder.WriteByte(',')
			}
			builder.WriteString(k)
			builder.WriteByte('=')
			builder.WriteString(state[k])
		}
		cacheKey = builder.String()
	}

	if runtimeID, ok := f.paletteCache[cacheKey]; ok {
		return runtimeID
	}

	var runtimeID uint32
	var found bool

	if len(state) > 0 {
		stateMap := make(map[string]any, len(state))
		for k, v := range state {
			stateMap[k] = v
		}
		runtimeID, found = blocks.BlockNameAndStateToRuntimeID(name, stateMap)
	}

	if !found {
		runtimeID, found = blocks.LegacyBlockToRuntimeID(name, uint16(aux))
	}
	if !found {
		runtimeID, found = blocks.BlockStrToRuntimeID(name)
	}
	if !found && !strings.Contains(name, ":") {
		prefixed := "minecraft:" + name
		runtimeID, found = blocks.LegacyBlockToRuntimeID(prefixed, uint16(aux))
		if !found {
			runtimeID, found = blocks.BlockStrToRuntimeID(prefixed)
		}
	}
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

	f.paletteCache[cacheKey] = runtimeID
	return runtimeID
}

func (f *FuHongV2) GetOffsetPos() define.Offset {
	return f.offsetPos
}

func (f *FuHongV2) SetOffsetPos(offset define.Offset) {
	f.offsetPos = offset
	f.size.Width = f.originalSize.Width + int(math.Abs(float64(offset.X())))
	f.size.Length = f.originalSize.Length + int(math.Abs(float64(offset.Z())))
	f.size.Height = f.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (f *FuHongV2) GetSize() define.Size {
	return *f.size
}

func (f *FuHongV2) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	chunks := make(map[define.ChunkPos]*chunk.Chunk, len(posList))
	height := f.size.Height
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

	offsetX := int(f.offsetPos.X())
	offsetY := int(f.offsetPos.Y())
	offsetZ := int(f.offsetPos.Z())

	for _, blk := range f.blocks {
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

func (f *FuHongV2) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any, len(posList))
	for _, pos := range posList {
		if _, exists := result[pos]; !exists {
			result[pos] = make(map[define.BlockPos]map[string]any)
		}
	}

	if len(result) == 0 {
		return result, nil
	}

	offsetX := int(f.offsetPos.X())
	offsetY := int(f.offsetPos.Y())
	offsetZ := int(f.offsetPos.Z())

	for _, blk := range f.blocks {
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

func (f *FuHongV2) CountNonAirBlocks() (int, error) {
	return f.nonAirBlocks, nil
}

func (f *FuHongV2) Close() error {
	return nil
}
