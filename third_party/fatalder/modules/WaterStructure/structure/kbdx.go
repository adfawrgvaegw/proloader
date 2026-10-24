package structure

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/blocks"
)

var kbdxExecuteRegex = regexp.MustCompile("[ ]*?/?[ ]*?execute[ ]*?(as|at|align|anchored|facing|in|positioned|rotated|if|unless|run)")

type KBDX struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset
	origin       define.Origin

	paletteCache map[string]uint32
	blocks       []kbdxBlock

	nonAirBlocks int
}

type kbdxBlock struct {
	LocalX    int
	LocalY    int
	LocalZ    int
	RuntimeID uint32
	NBT       map[string]any
}

type kbdxRawBlock struct {
	X     int32
	Y     int32
	Z     int32
	Index uint32
	Aux   uint32
}

func (k *KBDX) ID() uint8 {
	return IDKBDX
}

func (k *KBDX) Name() string {
	return NameKBDX
}

func (k *KBDX) FromFile(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	var blockCount uint32
	if err := binary.Read(file, binary.LittleEndian, &blockCount); err != nil {
		return fmt.Errorf("读取 KBDX 方块数量失败: %w", err)
	}
	if blockCount == 0 {
		return ErrInvalidFile
	}

	rawBlocks := make([]kbdxRawBlock, blockCount)
	for i := uint32(0); i < blockCount; i++ {
		if err := binary.Read(file, binary.LittleEndian, &rawBlocks[i]); err != nil {
			return fmt.Errorf("读取 KBDX 方块 %d 失败: %w", i, err)
		}
	}

	rest, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("读取 KBDX 元数据失败: %w", err)
	}
	if len(rest) == 0 {
		return fmt.Errorf("缺少 KBDX 元数据 JSON")
	}

	var metadata map[string]any
	if err := json.Unmarshal(rest, &metadata); err != nil {
		return fmt.Errorf("解析 KBDX 元数据失败: %w", err)
	}

	blockEntities := extractKBDXBlockEntities(metadata)
	palette := extractKBDXPalette(metadata)
	if len(palette) == 0 {
		return ErrInvalidFile
	}

	k.file = file
	return k.populate(rawBlocks, palette, blockEntities)
}

func (k *KBDX) populate(rawBlocks []kbdxRawBlock, palette map[int]string, blockEntities map[[3]int]map[string]any) error {
	k.paletteCache = make(map[string]uint32)
	k.blocks = nil
	k.nonAirBlocks = 0

	minX, minY, minZ := math.MaxInt, math.MaxInt, math.MaxInt
	maxX, maxY, maxZ := math.MinInt, math.MinInt, math.MinInt

	accum := make(map[[3]int]*kbdxBlock, len(rawBlocks))

	for _, rb := range rawBlocks {
		name, ok := palette[int(rb.Index)]
		if !ok {
			name = ""
		}
		runtimeID := k.runtimeIDFor(name, int(rb.Aux))

		x, y, z := int(rb.X), int(rb.Y), int(rb.Z)
		key := [3]int{x, y, z}

		blkNBT := blockEntities[key]

		if existing, exists := accum[key]; exists {
			existing.RuntimeID = runtimeID
			if existing.NBT == nil && blkNBT != nil {
				existing.NBT = blkNBT
			}
		} else {
			accum[key] = &kbdxBlock{
				LocalX:    x,
				LocalY:    y,
				LocalZ:    z,
				RuntimeID: runtimeID,
				NBT:       blkNBT,
			}
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

		if runtimeID == UnknownBlockRuntimeID {
			continue
		}
	}

	if len(accum) == 0 {
		return ErrInvalidFile
	}

	width := maxX - minX + 1
	height := maxY - minY + 1
	length := maxZ - minZ + 1

	k.origin = define.Origin{int32(minX), int32(minY), int32(minZ)}
	k.size = &define.Size{Width: width, Height: height, Length: length}
	k.originalSize = &define.Size{Width: width, Height: height, Length: length}

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

	k.blocks = make([]kbdxBlock, 0, len(accum))
	k.nonAirBlocks = 0

	for _, key := range keys {
		blk := accum[key]
		local := kbdxBlock{
			LocalX:    blk.LocalX - minX,
			LocalY:    blk.LocalY - minY,
			LocalZ:    blk.LocalZ - minZ,
			RuntimeID: blk.RuntimeID,
			NBT:       blk.NBT,
		}
		k.blocks = append(k.blocks, local)
		if local.RuntimeID != block.AirRuntimeID {
			k.nonAirBlocks++
		}
	}

	// 检查是不是这个文件
	if len(k.paletteCache) == 0 {
		return ErrInvalidFile
	}

	return nil
}

func (k *KBDX) runtimeIDFor(name string, aux int) uint32 {
	name = strings.TrimSpace(name)
	cacheKey := fmt.Sprintf("%s|%d", name, aux)
	if runtimeID, ok := k.paletteCache[cacheKey]; ok {
		return runtimeID
	}

	if name == "" {
		k.paletteCache[cacheKey] = UnknownBlockRuntimeID
		return UnknownBlockRuntimeID
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
	} else {
		baseName, properties, ok := blocks.RuntimeIDToState(runtimeID)
		if !ok {
			runtimeID = UnknownBlockRuntimeID
		} else {
			runtimeID, ok = block.StateToRuntimeID(baseName, properties)
			if !ok {
				runtimeID = UnknownBlockRuntimeID
			}
		}
	}

	k.paletteCache[cacheKey] = runtimeID
	return runtimeID
}

func (k *KBDX) GetOffsetPos() define.Offset {
	return k.offsetPos
}

func (k *KBDX) SetOffsetPos(offset define.Offset) {
	k.offsetPos = offset
	k.size.Width = k.originalSize.Width + int(math.Abs(float64(offset.X())))
	k.size.Length = k.originalSize.Length + int(math.Abs(float64(offset.Z())))
	k.size.Height = k.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (k *KBDX) GetSize() define.Size {
	return *k.size
}

func (k *KBDX) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	chunks := make(map[define.ChunkPos]*chunk.Chunk, len(posList))
	height := k.size.Height
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

	offsetX := int(k.offsetPos.X())
	offsetY := int(k.offsetPos.Y())
	offsetZ := int(k.offsetPos.Z())

	for _, blk := range k.blocks {
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

func (k *KBDX) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any, len(posList))
	for _, pos := range posList {
		if _, exists := result[pos]; !exists {
			result[pos] = make(map[define.BlockPos]map[string]any)
		}
	}

	if len(result) == 0 {
		return result, nil
	}

	offsetX := int(k.offsetPos.X())
	offsetY := int(k.offsetPos.Y())
	offsetZ := int(k.offsetPos.Z())

	for _, blk := range k.blocks {
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

func (k *KBDX) CountNonAirBlocks() (int, error) {
	return k.nonAirBlocks, nil
}

func (k *KBDX) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
) error {
	return convertReaderToMCWorld(k, bedrockWorld, startSubChunkPos, startCallback, progressCallback)
}

func (k *KBDX) Close() error {
	return nil
}

func extractKBDXBlockEntities(metadata map[string]any) map[[3]int]map[string]any {
	entities := make(map[[3]int]map[string]any)
	raw, ok := metadata["BlockEntityData"]
	if !ok {
		return entities
	}

	delete(metadata, "BlockEntityData")

	entries, ok := raw.([]any)
	if !ok {
		return entities
	}

	for _, entry := range entries {
		entityMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		x, xErr := toInt(entityMap["x"])
		y, yErr := toInt(entityMap["y"])
		z, zErr := toInt(entityMap["z"])
		if xErr != nil || yErr != nil || zErr != nil {
			continue
		}

		key := [3]int{x, y, z}
		if nbt := buildKBDXCommandNBT(entityMap); nbt != nil {
			entities[key] = nbt
			continue
		}
		if nbt := buildKBDXContainerNBT(entityMap); nbt != nil {
			entities[key] = nbt
			continue
		}
		if nbt := buildKBDXSignNBT(entityMap); nbt != nil {
			entities[key] = nbt
			continue
		}

		copyMap := make(map[string]any, len(entityMap))
		for k, v := range entityMap {
			if k == "x" || k == "y" || k == "z" {
				continue
			}
			copyMap[k] = v
		}
		if idValue, ok := copyMap["id"].(string); ok {
			if mapped := kbdxEntityIDToNBTID(idValue); mapped != "" {
				copyMap["id"] = mapped
			}
		}
		entities[key] = copyMap
	}

	return entities
}

func extractKBDXPalette(metadata map[string]any) map[int]string {
	palette := make(map[int]string)
	for name, value := range metadata {
		index, err := toInt(value)
		if err != nil {
			continue
		}
		palette[index] = name
	}
	return palette
}

func buildKBDXCommandNBT(data map[string]any) map[string]any {
	idValue, _ := data["id"].(string)
	if !strings.HasSuffix(idValue, "command_block") {
		return nil
	}

	command := fmt.Sprint(data["Command"])
	customName := fmt.Sprint(data["CustomName"])

	tickDelay, _ := toInt(data["TickDelay"])
	isConditional := toBool(data["isConditional"])
	redstone := toBool(data["redstone"])
	executeFirst := toBool(data["ExecuteOnFirstTick"])
	trackOutput := toBool(data["TrackOutput"])
	lastOutput := fmt.Sprint(data["LastOutput"])
	mode, _ := toInt(data["Mode"])

	version := int32(19)
	if kbdxExecuteRegex.MatchString(command) {
		version = 38
	}

	nbt := map[string]any{
		"id":                 "CommandBlock",
		"Command":            command,
		"CustomName":         customName,
		"ExecuteOnFirstTick": boolToByte(executeFirst),
		"TrackOutput":        boolToByte(trackOutput),
		"conditionalMode":    boolToByte(isConditional),
		"auto":               boolToByte(!redstone),
		"TickDelay":          int32(tickDelay),
		"Powered":            byte(0),
		"LPCommandMode":      int32(mode),
		"LastOutput":         lastOutput,
		"Version":            version,
	}

	return nbt
}

func buildKBDXContainerNBT(data map[string]any) map[string]any {
	idValue, _ := data["id"].(string)
	itemsRaw, hasItems := data["Items"].([]any)
	if idValue == "" && !hasItems {
		return nil
	}

	nbt := make(map[string]any)
	if mapped := kbdxEntityIDToNBTID(idValue); mapped != "" {
		nbt["id"] = mapped
	} else if idValue != "" {
		nbt["id"] = idValue
	}

	if hasItems {
		items := make([]map[string]any, 0, len(itemsRaw))
		for _, raw := range itemsRaw {
			itemMap, ok := raw.(map[string]any)
			if !ok {
				continue
			}

			name := fmt.Sprint(firstPresent(itemMap["Name"], itemMap["name"]))
			if name != "" && !strings.Contains(name, ":") {
				name = "minecraft:" + name
			}
			damage, _ := toInt(firstPresent(itemMap["Damage"], itemMap["damage"]))
			count, _ := toInt(firstPresent(itemMap["Count"], itemMap["count"]))
			slot, _ := toInt(firstPresent(itemMap["Slot"], itemMap["slot"]))

			item := map[string]any{
				"Name":   name,
				"Damage": int16(damage),
				"Count":  byte(count),
				"Slot":   byte(slot),
			}
			items = append(items, item)
		}
		if len(items) > 0 {
			nbt["Items"] = items
		}
	}

	if customName, ok := data["CustomName"].(string); ok && customName != "" {
		nbt["CustomName"] = customName
	}
	if lock, ok := data["Lock"].(string); ok && lock != "" {
		nbt["Lock"] = lock
	}

	if len(nbt) == 0 {
		return nil
	}
	return nbt
}

func buildKBDXSignNBT(data map[string]any) map[string]any {
	idValue := strings.ToLower(fmt.Sprint(data["id"]))
	if !strings.Contains(idValue, "sign") {
		return nil
	}

	text := extractKBDXSignText(data)
	nbt := map[string]any{"id": "Sign"}
	if text != "" {
		nbt["Text"] = text
	}
	if color, ok := data["Color"].(string); ok && color != "" {
		nbt["Color"] = color
	}
	if glowing, ok := data["GlowingText"].(bool); ok {
		nbt["GlowingText"] = boolToByte(glowing)
	}
	return nbt
}

func extractKBDXSignText(data map[string]any) string {
	if value, ok := data["Text"].(string); ok {
		return value
	}
	if lines, ok := data["Text"].([]any); ok {
		builder := strings.Builder{}
		for i, line := range lines {
			if i > 0 {
				builder.WriteByte('\n')
			}
			builder.WriteString(fmt.Sprint(line))
		}
		return builder.String()
	}
	if front, ok := data["FrontText"].(map[string]any); ok {
		if txt, ok := front["Text"].(string); ok {
			return txt
		}
	}
	return ""
}

func firstPresent(values ...any) any {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

func kbdxEntityIDToNBTID(id string) string {
	lower := strings.ToLower(id)
	switch lower {
	case "command_block", "repeating_command_block", "chain_command_block":
		return "CommandBlock"
	case "chest", "trapped_chest":
		return "Chest"
	case "barrel":
		return "Barrel"
	case "hopper":
		return "Hopper"
	case "dispenser":
		return "Dispenser"
	case "dropper":
		return "Dropper"
	case "blast_furnace":
		return "BlastFurnace"
	case "furnace":
		return "Furnace"
	case "smoker":
		return "Smoker"
	case "crafter":
		return "Crafter"
	}
	if strings.Contains(lower, "shulker_box") {
		return "ShulkerBox"
	}
	return ""
}

func toBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case int:
		return v != 0
	case int32:
		return v != 0
	case int64:
		return v != 0
	case uint32:
		return v != 0
	case uint64:
		return v != 0
	case float32:
		return v != 0
	case float64:
		return v != 0
	case string:
		lower := strings.ToLower(strings.TrimSpace(v))
		return lower == "true" || lower == "1"
	default:
		return false
	}
}
