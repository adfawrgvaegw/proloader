package structure

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/blocks"
)

type FuHongV3 struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset
	origin       define.Origin

	palette      []string
	paletteCache map[string]uint32
	blocks       []fuHongBlock

	nonAirBlocks int
}

type fuHongBlock struct {
	LocalX    int
	LocalY    int
	LocalZ    int
	RuntimeID uint32
	NBT       map[string]any
}

func (f *FuHongV3) ID() uint8 {
	return IDFuHongV3
}

func (f *FuHongV3) Name() string {
	return NameFuHongV3
}

func (f *FuHongV3) FromFile(file *os.File) error {
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	var root struct {
		FuHongBuild         []map[string]any `json:"FuHongBuild"`
		BlocksList          []string         `json:"BlocksList"`
		BlockCalculationPos bool             `json:"BlockCalculationPos"`
	}

	if err := json.NewDecoder(file).Decode(&root); err != nil {
		return fmt.Errorf("解析 FuHong V3 的 JSON 失败: %w", err)
	}

	if len(root.BlocksList) == 0 {
		return ErrInvalidFile
	}

	f.file = file
	f.palette = root.BlocksList
	return f.populateFromBuild(root.FuHongBuild)
}

func (f *FuHongV3) populateFromBuild(chunks []map[string]any) error {
	f.paletteCache = make(map[string]uint32)
	f.blocks = nil
	f.nonAirBlocks = 0

	minX, minY, minZ := math.MaxInt, math.MaxInt, math.MaxInt
	maxX, maxY, maxZ := math.MinInt, math.MinInt, math.MinInt

	accum := make(map[[3]int]*fuHongBlock)

	for idx, chunk := range chunks {
		startX, err := extractIntField(chunk, "startX")
		if err != nil {
			return fmt.Errorf("chunk %d: %w", idx, err)
		}
		startZ, err := extractIntField(chunk, "startZ")
		if err != nil {
			return fmt.Errorf("chunk %d: %w", idx, err)
		}

		blockEntries, ok := chunk["block"].([]any)
		if !ok {
			return fmt.Errorf("区块 %d: 方块列表缺失或无效", idx)
		}

		for entryIdx, rawEntry := range blockEntries {
			tuple, ok := rawEntry.([]any)
			if !ok || len(tuple) < 5 {
				return fmt.Errorf("区块 %d 条目 %d: 元组无效", idx, entryIdx)
			}

			if _, isString := tuple[0].(string); isString {
				// entity record – currently ignored
				continue
			}

			paletteIndex, err := toInt(tuple[0])
			if err != nil {
				return fmt.Errorf("区块 %d 条目 %d: 调色板索引: %w", idx, entryIdx, err)
			}
			if paletteIndex < 0 || paletteIndex >= len(f.palette) {
				return fmt.Errorf("区块 %d 条目 %d: 调色板索引 %d 越界", idx, entryIdx, paletteIndex)
			}

			aux, err := toInt(tuple[1])
			if err != nil {
				return fmt.Errorf("区块 %d 条目 %d: aux: %w", idx, entryIdx, err)
			}

			xs, err := toIntSlice(tuple[2])
			if err != nil {
				return fmt.Errorf("区块 %d 条目 %d: xs: %w", idx, entryIdx, err)
			}
			ys, err := toIntSlice(tuple[3])
			if err != nil {
				return fmt.Errorf("区块 %d 条目 %d: ys: %w", idx, entryIdx, err)
			}
			zs, err := toIntSlice(tuple[4])
			if err != nil {
				return fmt.Errorf("区块 %d 条目 %d: zs: %w", idx, entryIdx, err)
			}

			if len(xs) != len(ys) || len(xs) != len(zs) {
				return fmt.Errorf("区块 %d 条目 %d: 坐标数组长度不匹配", idx, entryIdx)
			}

			var extras []any
			if len(tuple) >= 6 {
				if extraSlice, ok := tuple[5].([]any); ok {
					extras = extraSlice
				}
			}

			blockName := f.palette[paletteIndex]
			runtimeID := f.runtimeIDFor(blockName, aux)

			for i := 0; i < len(xs); i++ {
				worldX := startX + xs[i]
				worldY := ys[i]
				worldZ := startZ + zs[i]

				var nbt map[string]any
				if i < len(extras) {
					nbt = f.buildExtraData(blockName, extras[i])
				}

				key := [3]int{worldX, worldY, worldZ}
				if existing, ok := accum[key]; ok {
					existing.RuntimeID = runtimeID
					if existing.NBT == nil && nbt != nil {
						existing.NBT = nbt
					}
				} else {
					accum[key] = &fuHongBlock{
						LocalX:    worldX,
						LocalY:    worldY,
						LocalZ:    worldZ,
						RuntimeID: runtimeID,
						NBT:       nbt,
					}
				}

				if worldX < minX {
					minX = worldX
				}
				if worldY < minY {
					minY = worldY
				}
				if worldZ < minZ {
					minZ = worldZ
				}
				if worldX > maxX {
					maxX = worldX
				}
				if worldY > maxY {
					maxY = worldY
				}
				if worldZ > maxZ {
					maxZ = worldZ
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

	f.blocks = make([]fuHongBlock, 0, len(accum))
	f.nonAirBlocks = 0

	for _, key := range keys {
		rec := accum[key]
		blk := fuHongBlock{
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

	// 检查是不是这个文件
	if len(f.paletteCache) == 0 {
		return ErrInvalidFile
	}

	return nil
}

func (f *FuHongV3) buildExtraData(blockName string, payload any) map[string]any {
	if payload == nil {
		return nil
	}

	lowerName := strings.ToLower(blockName)
	if strings.Contains(lowerName, "command_block") {
		if arr, ok := payload.([]any); ok {
			return buildFuHongCommandNBT(arr)
		}
		return nil
	}

	if chestBlockNameToID(strings.TrimSpace(blockName)) != "" {
		return buildFuHongContainerNBT(blockName, payload)
	}

	if strings.Contains(lowerName, "sign") {
		return buildFuHongSignNBT(blockName, payload)
	}

	return nil
}

func buildFuHongCommandNBT(values []any) map[string]any {
	if len(values) == 0 {
		return nil
	}

	command := fmt.Sprint(values[0])

	tickDelay := 0
	if len(values) > 1 {
		if delay, err := toInt(values[1]); err == nil {
			tickDelay = delay
		}
	}

	auto := byte(0)
	if len(values) > 2 {
		auto = fuHongAutoByte(values[2])
	}

	customName := ""
	if len(values) > 3 {
		customName = fmt.Sprint(values[3])
	}

	version := int32(19)
	if kbdxExecuteRegex.MatchString(command) {
		version = 38
	}

	return map[string]any{
		"id":                 "CommandBlock",
		"Command":            command,
		"CustomName":         customName,
		"ExecuteOnFirstTick": byte(1),
		"auto":               auto,
		"TickDelay":          int32(tickDelay),
		"conditionalMode":    byte(0),
		"TrackOutput":        byte(1),
		"Version":            version,
	}
}

func fuHongAutoByte(v any) byte {
	switch t := v.(type) {
	case bool:
		return boolToByte(t)
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0
		}
		if strings.EqualFold(s, "true") {
			return 1
		}
		if strings.EqualFold(s, "false") {
			return 0
		}
		if i, err := toInt(s); err == nil {
			return boolToByte(i != 0)
		}
		return 0
	default:
		if i, err := toInt(v); err == nil {
			return boolToByte(i != 0)
		}
		return 0
	}
}

func buildFuHongContainerNBT(blockName string, payload any) map[string]any {
	id := chestBlockNameToID(strings.TrimSpace(blockName))
	if id == "" {
		return nil
	}

	var itemsRaw []any
	switch v := payload.(type) {
	case nil:
		itemsRaw = nil
	case []any:
		itemsRaw = v
	default:
		return map[string]any{
			"id":        id,
			"Findable":  byte(0),
			"IsOpened":  byte(0),
			"isMovable": byte(1),
			"Items":     []map[string]any{},
		}
	}

	items := make([]map[string]any, 0, len(itemsRaw))
	for _, raw := range itemsRaw {
		if tuple, ok := raw.([]any); ok && len(tuple) >= 4 {
			name := normalizeFuHongItemName(fmt.Sprint(tuple[0]))
			damage, _ := toInt(tuple[1])
			count, _ := toInt(tuple[2])
			slot, _ := toInt(tuple[3])
			items = append(items, map[string]any{
				"Name":   name,
				"Count":  byte(count),
				"Damage": int16(damage),
				"Slot":   byte(slot),
				"Block":  fuHongItemBlockNBT(name),
			})
			continue
		}

		itemMap, ok := raw.(map[string]any)
		if !ok || itemMap == nil {
			continue
		}

		name := normalizeFuHongItemName(fmt.Sprint(firstPresent(itemMap["Name"], itemMap["name"], itemMap["ns"])))
		damage, _ := toInt(firstPresent(itemMap["Damage"], itemMap["damage"], itemMap["aux"]))
		count, _ := toInt(firstPresent(itemMap["Count"], itemMap["count"], itemMap["num"]))
		slot, _ := toInt(firstPresent(itemMap["Slot"], itemMap["slot"]))
		if strings.TrimSpace(name) == "" {
			continue
		}
		items = append(items, map[string]any{
			"Name":   name,
			"Count":  byte(count),
			"Damage": int16(damage),
			"Slot":   byte(slot),
			"Block":  fuHongItemBlockNBT(name),
		})
	}

	return map[string]any{
		"id":        id,
		"Findable":  byte(0),
		"IsOpened":  byte(0),
		"isMovable": byte(1),
		"Items":     items,
	}
}

func normalizeFuHongItemName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if !strings.Contains(name, ":") {
		return "minecraft:" + name
	}
	return name
}

func fuHongItemBlockNBT(name string) map[string]any {
	return map[string]any{
		"name":    name,
		"states":  map[string]any{},
		"val":     int16(0),
		"version": int32(17959425),
	}
}

func buildFuHongSignNBT(blockName string, payload any) map[string]any {
	text := ""
	switch v := payload.(type) {
	case string:
		text = v
	case []any:
		builder := strings.Builder{}
		for i, line := range v {
			if i > 0 {
				builder.WriteByte('\n')
			}
			builder.WriteString(fmt.Sprint(line))
		}
		text = builder.String()
	default:
		text = fmt.Sprint(payload)
	}

	lower := strings.ToLower(strings.TrimSpace(blockName))
	id := "Sign"
	if strings.HasSuffix(lower, "hanging_sign") {
		id = "HangingSign"
	}

	return map[string]any{
		"id":        id,
		"IsWaxed":   byte(0),
		"isMovable": byte(1),
		"BackText": map[string]any{
			"FilteredText":       "",
			"HideGlowOutline":    byte(0),
			"IgnoreLighting":     byte(0),
			"PersistFormatting":  byte(1),
			"SignTextColor":      int32(-16777216),
			"Text":               "",
			"TextOwner":          "",
		},
		"FrontText": map[string]any{
			"FilteredText":       "",
			"HideGlowOutline":    byte(0),
			"IgnoreLighting":     byte(0),
			"PersistFormatting":  byte(1),
			"SignTextColor":      int32(-16777216),
			"Text":               text,
			"TextOwner":          "",
		},
	}
}

func extractIntField(obj map[string]any, key string) (int, error) {
	val, ok := obj[key]
	if !ok {
		return 0, fmt.Errorf("缺少字段 %s", key)
	}
	return toInt(val)
}

func toIntSlice(value any) ([]int, error) {
	switch v := value.(type) {
	case []any:
		result := make([]int, len(v))
		for i, item := range v {
			val, err := toInt(item)
			if err != nil {
				return nil, err
			}
			result[i] = val
		}
		return result, nil
	case []int:
		return append([]int(nil), v...), nil
	case []float64:
		result := make([]int, len(v))
		for i, item := range v {
			result[i] = int(item)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("坐标列表类型异常: %T", value)
	}
}

func (f *FuHongV3) runtimeIDFor(name string, aux int) uint32 {
	cacheKey := fmt.Sprintf("%s|%d", name, aux)
	if runtimeID, ok := f.paletteCache[cacheKey]; ok {
		return runtimeID
	}

	runtimeID, found := blocks.LegacyBlockToRuntimeID(name, uint16(aux))
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

func (f *FuHongV3) GetOffsetPos() define.Offset {
	return f.offsetPos
}

func (f *FuHongV3) SetOffsetPos(offset define.Offset) {
	f.offsetPos = offset
	f.size.Width = f.originalSize.Width + int(math.Abs(float64(offset.X())))
	f.size.Length = f.originalSize.Length + int(math.Abs(float64(offset.Z())))
	f.size.Height = f.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (f *FuHongV3) GetSize() define.Size {
	return *f.size
}

func (f *FuHongV3) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
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

func (f *FuHongV3) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
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

func (f *FuHongV3) CountNonAirBlocks() (int, error) {
	return f.nonAirBlocks, nil
}

func (f *FuHongV3) Close() error {
	return nil
}
