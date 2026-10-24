package structure

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/blocks"
)

type IBImport struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset
	origin       define.Origin

	paletteCache map[string]uint32
	blocks       []ibImportBlock

	nonAirBlocks int
}

type ibImportBlock struct {
	LocalX    int
	LocalY    int
	LocalZ    int
	RuntimeID uint32
	NBT       map[string]any
}

type ibImportBoolish bool

func (b *ibImportBoolish) UnmarshalJSON(data []byte) error {
	value := strings.TrimSpace(string(data))
	if value == "" || value == "null" {
		*b = false
		return nil
	}

	switch value {
	case "true", "1":
		*b = true
		return nil
	case "false", "0":
		*b = false
		return nil
	}

	if unquoted, err := strconv.Unquote(value); err == nil {
		switch strings.TrimSpace(unquoted) {
		case "true", "1":
			*b = true
			return nil
		case "false", "0":
			*b = false
			return nil
		}
	}

	return fmt.Errorf("无效的布尔值: %s", value)
}

func (b ibImportBoolish) Bool() bool {
	return bool(b)
}

type ibImportCommand struct {
	PosX           string `json:"posX"`
	PosY           string `json:"posY"`
	PosZ           string `json:"posZ"`
	CommandMessage string `json:"CommandMessage"`
	CommandTitle   string `json:"Commandtitle"`
	Mode           int    `json:"mode"`
	TickDelay      int    `json:"isTime"`
	Conditional    ibImportBoolish `json:"Conditional"`
	IsRedstone     ibImportBoolish `json:"isRedstone"`
}

type ibImportCommandExport struct {
	PosX           string `json:"posX"`
	PosY           string `json:"posY"`
	PosZ           string `json:"posZ"`
	CommandMessage string `json:"CommandMessage"`
	CommandTitle   string `json:"Commandtitle"`
	Mode           int    `json:"mode"`
	TickDelay      int    `json:"isTime"`
	Conditional    int    `json:"Conditional"`
	IsRedstone     *bool  `json:"isRedstone,omitempty"`
}

func decodeIBImportBase64(text string) (string, bool) {
	if strings.ContainsAny(text, " \t\r\n") {
		return "", false
	}

	if decoded, ok := decodeIBImportBase64WithEncoding(text, base64.StdEncoding); ok {
		return decoded, true
	}
	if decoded, ok := decodeIBImportBase64WithEncoding(text, base64.RawStdEncoding); ok {
		return decoded, true
	}
	return "", false
}

func decodeIBImportBase64WithEncoding(text string, enc *base64.Encoding) (string, bool) {
	decoded, err := enc.DecodeString(text)
	if err != nil {
		return "", false
	}
	if enc.EncodeToString(decoded) != text {
		return "", false
	}
	if !utf8.Valid(decoded) {
		return "", false
	}
	return string(decoded), true
}

func (i *IBImport) ID() uint8 {
	return IDIBImport
}

func (i *IBImport) Name() string {
	return NameIBImport
}

func (i *IBImport) FromFile(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	buf := bufio.NewReader(file)
	header := make([]byte, 9)
	if _, err := io.ReadFull(buf, header); err != nil {
		return fmt.Errorf("读取 IBImport 头部失败: %w", err)
	}
	if string(header) != "IBImport " {
		return ErrInvalidFile
	}

	segments, err := readIBImportSegments(buf)
	if err != nil {
		return err
	}
	if len(segments) == 0 {
		return ErrInvalidFile
	}

	script := string(segments[0])
	var commands []ibImportCommand
	if len(segments) >= 2 {
		if err := json.Unmarshal(segments[1], &commands); err != nil {
			return fmt.Errorf("解析 IBImport 命令 JSON 失败: %w", err)
		}
	}

	i.file = file
	return i.populate(script, commands)
}

func readIBImportSegments(r *bufio.Reader) ([][]byte, error) {
	var segments [][]byte
	for {
		if _, err := r.Peek(1); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("预读段失败: %w", err)
		}

		length, err := readIBImportVarInt(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("读取段长度失败: %w", err)
		}
		key, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("读取段密钥失败: %w", err)
		}

		data := make([]byte, length)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, fmt.Errorf("读取段数据失败: %w", err)
		}
		for idx := range data {
			data[idx] ^= key
		}
		segments = append(segments, data)
	}
	return segments, nil
}

func readIBImportVarInt(r *bufio.Reader) (int, error) {
	value := 0
	position := 0
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		value |= int(b&0x7F) << position
		if b&0x80 == 0 {
			break
		}
		position += 7
		if position >= 32 {
			return 0, fmt.Errorf("IBImport 变长整数过长")
		}
	}
	return value, nil
}

func (i *IBImport) populate(script string, commands []ibImportCommand) error {
	i.paletteCache = make(map[string]uint32)
	i.blocks = nil
	i.nonAirBlocks = 0

	accum := make(map[[3]int]*ibImportBlock)
	minX, minY, minZ := math.MaxInt, math.MaxInt, math.MaxInt
	maxX, maxY, maxZ := math.MinInt, math.MinInt, math.MinInt

	lines := strings.Split(script, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimSuffix(line, "\r")
		pos, name, props, err := parseIBImportSetblock(line)
		if err != nil {
			continue
		}

		runtimeID := i.runtimeIDFor(name, props)
		key := [3]int{pos[0], pos[1], pos[2]}
		if existing, ok := accum[key]; ok {
			existing.RuntimeID = runtimeID
		} else {
			accum[key] = &ibImportBlock{
				LocalX:    pos[0],
				LocalY:    pos[1],
				LocalZ:    pos[2],
				RuntimeID: runtimeID,
			}
		}

		if pos[0] < minX {
			minX = pos[0]
		}
		if pos[1] < minY {
			minY = pos[1]
		}
		if pos[2] < minZ {
			minZ = pos[2]
		}
		if pos[0] > maxX {
			maxX = pos[0]
		}
		if pos[1] > maxY {
			maxY = pos[1]
		}
		if pos[2] > maxZ {
			maxZ = pos[2]
		}
	}

	for _, cmd := range commands {
		x := parseIBImportRelative(cmd.PosX)
		y := parseIBImportRelative(cmd.PosY)
		z := parseIBImportRelative(cmd.PosZ)
		key := [3]int{x, y, z}

		blk, ok := accum[key]
		if !ok {
			continue
		}
		blk.NBT = buildIBImportCommandNBT(cmd)
	}

	if len(accum) == 0 {
		return ErrInvalidFile
	}

	width := maxX - minX + 1
	height := maxY - minY + 1
	length := maxZ - minZ + 1

	i.origin = define.Origin{int32(minX), int32(minY), int32(minZ)}
	i.size = &define.Size{Width: width, Height: height, Length: length}
	i.originalSize = &define.Size{Width: width, Height: height, Length: length}

	keys := make([][3]int, 0, len(accum))
	for key := range accum {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(a, b int) bool {
		if keys[a][1] != keys[b][1] {
			return keys[a][1] < keys[b][1]
		}
		if keys[a][2] != keys[b][2] {
			return keys[a][2] < keys[b][2]
		}
		return keys[a][0] < keys[b][0]
	})

	i.blocks = make([]ibImportBlock, 0, len(accum))
	for _, key := range keys {
		rec := accum[key]
		blk := ibImportBlock{
			LocalX:    rec.LocalX - minX,
			LocalY:    rec.LocalY - minY,
			LocalZ:    rec.LocalZ - minZ,
			RuntimeID: rec.RuntimeID,
			NBT:       rec.NBT,
		}
		i.blocks = append(i.blocks, blk)
		if blk.RuntimeID != block.AirRuntimeID {
			i.nonAirBlocks++
		}
	}

	// 检查是不是这个文件
	if len(i.paletteCache) == 0 {
		return ErrInvalidFile
	}

	return nil
}

func parseIBImportSetblock(line string) ([3]int, string, map[string]string, error) {
	if !strings.HasPrefix(line, "setblock ") {
		return [3]int{}, "", nil, fmt.Errorf("不支持的行")
	}
	parts := strings.Fields(line)
	if len(parts) < 5 {
		return [3]int{}, "", nil, fmt.Errorf("无效的 setblock 行")
	}

	x := parseIBImportRelative(parts[1])
	y := parseIBImportRelative(parts[2])
	z := parseIBImportRelative(parts[3])

	name := parts[4]
	if !strings.Contains(name, ":") {
		name = "minecraft:" + name
	}

	var props map[string]string
	if len(parts) >= 6 {
		props = parseIBImportStates(parts[5])
	}

	return [3]int{x, y, z}, name, props, nil
}

func parseIBImportRelative(token string) int {
	token = strings.TrimSpace(token)
	token = strings.TrimPrefix(token, "~")
	if token == "" {
		return 0
	}
	value, err := strconv.Atoi(token)
	if err != nil {
		return 0
	}
	return value
}

func parseIBImportStates(stateToken string) map[string]string {
	stateToken = strings.TrimSpace(stateToken)
	if stateToken == "" || stateToken == "[]" {
		return nil
	}
	stateToken = strings.TrimPrefix(stateToken, "[")
	stateToken = strings.TrimSuffix(stateToken, "]")
	stateToken = strings.ReplaceAll(stateToken, `\"`, `"`)
	if strings.TrimSpace(stateToken) == "" {
		return nil
	}

	props := make(map[string]string)
	for _, part := range strings.Split(stateToken, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.Trim(kv[0], " \"")
		val := strings.Trim(kv[1], " \"")
		props[key] = val
	}
	if len(props) == 0 {
		return nil
	}
	return props
}

func buildIBImportCommandNBT(cmd ibImportCommand) map[string]any {
	command := cmd.CommandMessage
	if decoded, ok := decodeIBImportBase64(cmd.CommandMessage); ok {
		command = decoded
	}
	custom := cmd.CommandTitle
	conditional := cmd.Conditional.Bool()
	auto := !cmd.IsRedstone.Bool()

	nbt := map[string]any{
		"id":                 "CommandBlock",
		"Command":            command,
		"CustomName":         custom,
		"ExecuteOnFirstTick": boolToByte(false),
		"TrackOutput":        boolToByte(false),
		"conditionalMode":    boolToByte(conditional),
		"auto":               boolToByte(auto),
		"TickDelay":          int32(cmd.TickDelay),
		"LastOutput":         "",
		"Version":           int32(38),
	}
	return nbt
}

func (i *IBImport) runtimeIDFor(name string, properties map[string]string) uint32 {
	key := name
	if len(properties) > 0 {
		keys := make([]string, 0, len(properties))
		for k := range properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		builder := strings.Builder{}
		builder.WriteString(name)
		builder.WriteString("|")
		for idx, propKey := range keys {
			if idx > 0 {
				builder.WriteByte(',')
			}
			builder.WriteString(propKey)
			builder.WriteByte('=')
			builder.WriteString(properties[propKey])
		}
		key = builder.String()
	}

	if runtimeID, ok := i.paletteCache[key]; ok {
		return runtimeID
	}

	runtimeID := UnknownBlockRuntimeID
	name = strings.TrimSpace(name)
	if name != "" {
		if len(properties) > 0 {
			stateMap := make(map[string]any, len(properties))
			for k, v := range properties {
				stateMap[k] = v
			}
			if id, ok := blocks.BlockNameAndStateToRuntimeID(name, stateMap); ok {
				runtimeID = id
			} else if id, ok := blocks.BlockStrToRuntimeID(name); ok {
				runtimeID = id
			}
		} else {
			if id, ok := blocks.BlockStrToRuntimeID(name); ok {
				runtimeID = id
			} else if id, ok := blocks.LegacyBlockToRuntimeID(name, 0); ok {
				runtimeID = id
			}
		}
	}
	baseName, props, found := blocks.RuntimeIDToState(runtimeID)
	if !found {
		runtimeID = UnknownBlockRuntimeID
	} else {
		runtimeID, found = block.StateToRuntimeID(baseName, props)
		if !found {
			runtimeID = UnknownBlockRuntimeID
		}
	}

	i.paletteCache[key] = runtimeID
	return runtimeID
}

func (i *IBImport) GetOffsetPos() define.Offset {
	return i.offsetPos
}

func (i *IBImport) SetOffsetPos(offset define.Offset) {
	i.offsetPos = offset
	i.size.Width = i.originalSize.Width + int(math.Abs(float64(offset.X())))
	i.size.Length = i.originalSize.Length + int(math.Abs(float64(offset.Z())))
	i.size.Height = i.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (i *IBImport) GetSize() define.Size {
	return *i.size
}

func (i *IBImport) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	chunks := make(map[define.ChunkPos]*chunk.Chunk, len(posList))
	height := i.size.Height
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

	offsetX := int(i.offsetPos.X())
	offsetY := int(i.offsetPos.Y())
	offsetZ := int(i.offsetPos.Z())

	for _, blk := range i.blocks {
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

func (i *IBImport) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any, len(posList))
	for _, pos := range posList {
		if _, exists := result[pos]; !exists {
			result[pos] = make(map[define.BlockPos]map[string]any)
		}
	}

	if len(result) == 0 {
		return result, nil
	}

	offsetX := int(i.offsetPos.X())
	offsetY := int(i.offsetPos.Y())
	offsetZ := int(i.offsetPos.Z())

	for _, blk := range i.blocks {
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

func (i *IBImport) CountNonAirBlocks() (int, error) {
	return i.nonAirBlocks, nil
}

func (i *IBImport) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
) error {
	return convertReaderToMCWorld(i, bedrockWorld, startSubChunkPos, startCallback, progressCallback)
}

func (i *IBImport) FromMCWorld(
	world *world.BedrockWorld,
	target *os.File,
	point1BlockPos define.BlockPos,
	point2BlockPos define.BlockPos,
	startCallback func(int),
	progressCallback func(),
) error {
	if world == nil {
		return fmt.Errorf("bedrock 世界为 nil")
	}
	if target == nil {
		return fmt.Errorf("目标文件为 nil")
	}

	startBlockPos := define.BlockPos{
		min(point1BlockPos.X(), point2BlockPos.X()),
		min(point1BlockPos.Y(), point2BlockPos.Y()),
		min(point1BlockPos.Z(), point2BlockPos.Z()),
	}
	endBlockPos := define.BlockPos{
		max(point1BlockPos.X(), point2BlockPos.X()),
		max(point1BlockPos.Y(), point2BlockPos.Y()),
		max(point1BlockPos.Z(), point2BlockPos.Z()),
	}

	startX := int(startBlockPos.X())
	startY := int(startBlockPos.Y())
	startZ := int(startBlockPos.Z())
	endX := int(endBlockPos.X())
	endY := int(endBlockPos.Y())
	endZ := int(endBlockPos.Z())

	minChunkX := floorDiv(startX, 16)
	maxChunkX := floorDiv(endX, 16)
	minChunkZ := floorDiv(startZ, 16)
	maxChunkZ := floorDiv(endZ, 16)
	minSubChunkY := floorDiv(startY, 16)
	maxSubChunkY := floorDiv(endY, 16)

	chunkXCount := maxChunkX - minChunkX + 1
	chunkZCount := maxChunkZ - minChunkZ + 1
	subChunkYCount := maxSubChunkY - minSubChunkY + 1
	if chunkXCount < 0 || chunkZCount < 0 || subChunkYCount < 0 {
		return fmt.Errorf("无效范围: %v ~ %v", startBlockPos, endBlockPos)
	}

	totalSubChunks := chunkXCount * chunkZCount * subChunkYCount
	if startCallback != nil {
		startCallback(totalSubChunks)
	}

	if _, err := target.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("重置目标文件指针失败: %w", err)
	}
	if err := target.Truncate(0); err != nil {
		return fmt.Errorf("清空目标文件失败: %w", err)
	}
	if _, err := target.Write([]byte("IBImport ")); err != nil {
		return fmt.Errorf("写入 IBImport 头部失败: %w", err)
	}

	const xorKey byte = 193

	scriptSeg, err := beginIBImportSegment(target, xorKey)
	if err != nil {
		return err
	}

	for subY := minSubChunkY; subY <= maxSubChunkY; subY++ {
		subChunkWorldYStart := subY * 16
		subChunkWorldYEnd := subChunkWorldYStart + 15
		effectiveWorldYStart := max(subChunkWorldYStart, startY)
		effectiveWorldYEnd := min(subChunkWorldYEnd, endY)
		if effectiveWorldYStart > effectiveWorldYEnd {
			for cz := minChunkZ; cz <= maxChunkZ; cz++ {
				for cx := minChunkX; cx <= maxChunkX; cx++ {
					if progressCallback != nil {
						progressCallback()
					}
				}
			}
			continue
		}

		for cz := minChunkZ; cz <= maxChunkZ; cz++ {
			chunkWorldZStart := cz * 16
			chunkWorldZEnd := chunkWorldZStart + 15
			effectiveWorldZStart := max(chunkWorldZStart, startZ)
			effectiveWorldZEnd := min(chunkWorldZEnd, endZ)
			if effectiveWorldZStart > effectiveWorldZEnd {
				for cx := minChunkX; cx <= maxChunkX; cx++ {
					if progressCallback != nil {
						progressCallback()
					}
				}
				continue
			}

			for cx := minChunkX; cx <= maxChunkX; cx++ {
				chunkWorldXStart := cx * 16
				chunkWorldXEnd := chunkWorldXStart + 15
				effectiveWorldXStart := max(chunkWorldXStart, startX)
				effectiveWorldXEnd := min(chunkWorldXEnd, endX)
				if effectiveWorldXStart > effectiveWorldXEnd {
					if progressCallback != nil {
						progressCallback()
					}
					continue
				}

				worldSubChunkPos := bwo_define.SubChunkPos{int32(cx), int32(subY), int32(cz)}
				subChunk := world.LoadSubChunk(bwo_define.DimensionIDOverworld, worldSubChunkPos)
				if subChunk == nil {
					if progressCallback != nil {
						progressCallback()
					}
					continue
				}

				for wy := effectiveWorldYStart; wy <= effectiveWorldYEnd; wy++ {
					localY := byte(wy - subChunkWorldYStart)
					for wz := effectiveWorldZStart; wz <= effectiveWorldZEnd; wz++ {
						localZ := byte(wz - chunkWorldZStart)
						for wx := effectiveWorldXStart; wx <= effectiveWorldXEnd; wx++ {
							localX := byte(wx - chunkWorldXStart)
							runtimeID := subChunk.Block(localX, localY, localZ, 0)
							if runtimeID == block.AirRuntimeID {
								continue
							}
							blockName, statesStr := ibImportRuntimeIDToSetblock(runtimeID)
							line := fmt.Sprintf(
								"setblock ~%d ~%d ~%d %s %s\r\n",
								wx-startX,
								wy-startY,
								wz-startZ,
								blockName,
								statesStr,
							)
							if _, err := scriptSeg.WriteString(line); err != nil {
								return fmt.Errorf("写入 IBImport 脚本失败: %w", err)
							}
						}
					}
				}

				if progressCallback != nil {
					progressCallback()
				}
			}
		}
	}

	if err := scriptSeg.Close(); err != nil {
		return err
	}

	jsonSeg, err := beginIBImportSegment(target, xorKey)
	if err != nil {
		return err
	}
	if err := ibImportWriteCommandBlocksJSON(jsonSeg, world, startX, startY, startZ, endX, endY, endZ); err != nil {
		return err
	}
	if err := jsonSeg.Close(); err != nil {
		return err
	}

	return nil
}

func ibImportRuntimeIDToSetblock(runtimeID uint32) (blockName string, states string) {
	name, properties, found := block.RuntimeIDToState(runtimeID)
	if !found || name == "" {
		return "unknown", "[]"
	}
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "minecraft:")
	if name == "" {
		name = "unknown"
	}
	return name, formatIBImportStatesFromNBT(properties)
}

func formatIBImportStatesFromNBT(properties map[string]any) string {
	if len(properties) == 0 {
		return "[]"
	}

	keys := make([]string, 0, len(properties))
	for k := range properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteByte('[')
	for idx, k := range keys {
		if idx > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Quote(k))
		b.WriteByte('=')
		b.WriteString(formatIBImportStateValue(properties[k]))
	}
	b.WriteByte(']')
	return b.String()
}

func formatIBImportStateValue(v any) string {
	switch t := v.(type) {
	case bool:
		if t {
			return "true"
		}
		return "false"
	case int8, int16, int32, int64, int:
		return fmt.Sprintf("%d", t)
	case uint8, uint16, uint32, uint64, uint:
		return fmt.Sprintf("%d", t)
	case float32:
		if float32(int64(t)) == t {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%v", t)
	case float64:
		if float64(int64(t)) == t {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%v", t)
	case string:
		return strconv.Quote(t)
	default:
		return strconv.Quote(fmt.Sprint(v))
	}
}

func ibImportWriteCommandBlocksJSON(
	w io.Writer,
	world *world.BedrockWorld,
	startX, startY, startZ int,
	endX, endY, endZ int,
) error {
	if w == nil {
		return fmt.Errorf("JSON writer 为 nil")
	}
	if world == nil {
		return fmt.Errorf("bedrock 世界为 nil")
	}

	minChunkX := floorDiv(startX, 16)
	maxChunkX := floorDiv(endX, 16)
	minChunkZ := floorDiv(startZ, 16)
	maxChunkZ := floorDiv(endZ, 16)

	subChunkCache := make(map[bwo_define.SubChunkPos]*chunk.SubChunk)

	lookupRuntimeID := func(x, y, z int) uint32 {
		cx := floorDiv(x, 16)
		cz := floorDiv(z, 16)
		sy := floorDiv(y, 16)
		pos := bwo_define.SubChunkPos{int32(cx), int32(sy), int32(cz)}
		if sc, ok := subChunkCache[pos]; ok {
			if sc == nil {
				return block.AirRuntimeID
			}
			lx := byte(x - cx*16)
			ly := byte(y - sy*16)
			lz := byte(z - cz*16)
			return sc.Block(lx, ly, lz, 0)
		}
		sc := world.LoadSubChunk(bwo_define.DimensionIDOverworld, pos)
		subChunkCache[pos] = sc
		if sc == nil {
			return block.AirRuntimeID
		}
		lx := byte(x - cx*16)
		ly := byte(y - sy*16)
		lz := byte(z - cz*16)
		return sc.Block(lx, ly, lz, 0)
	}

	modeFromRuntimeID := func(runtimeID uint32) int {
		name, _, ok := block.RuntimeIDToState(runtimeID)
		if !ok {
			return 0
		}
		name = strings.TrimPrefix(name, "minecraft:")
		switch name {
		case "chain_command_block":
			return 1
		case "repeating_command_block":
			return 2
		default:
			return 0
		}
	}

	if _, err := w.Write([]byte{'[', '\n'}); err != nil {
		return fmt.Errorf("写入 JSON 头失败: %w", err)
	}
	wroteAny := false

	for cz := minChunkZ; cz <= maxChunkZ; cz++ {
		for cx := minChunkX; cx <= maxChunkX; cx++ {
			chunkPos := bwo_define.ChunkPos{int32(cx), int32(cz)}
			nbts, err := world.LoadNBT(bwo_define.DimensionIDOverworld, chunkPos)
			if err != nil {
				return fmt.Errorf("读取区块 NBT 失败 (%v): %w", chunkPos, err)
			}
			if len(nbts) == 0 {
				continue
			}
			for _, nbt := range nbts {
				idVal, _ := nbt["id"].(string)
				if idVal != "CommandBlock" {
					continue
				}

				xv, okX := nbt["x"].(int32)
				yv, okY := nbt["y"].(int32)
				zv, okZ := nbt["z"].(int32)
				if !okX || !okY || !okZ {
					continue
				}
				x := int(xv)
				y := int(yv)
				z := int(zv)

				if x < startX || x > endX || y < startY || y > endY || z < startZ || z > endZ {
					continue
				}

				cmdStr, _ := nbt["Command"].(string)
				title, _ := nbt["CustomName"].(string)
				encodedCmd := base64.StdEncoding.EncodeToString([]byte(cmdStr))

				tickDelay := 0
				switch tv := nbt["TickDelay"].(type) {
				case int32:
					tickDelay = int(tv)
				case int64:
					tickDelay = int(tv)
				case int:
					tickDelay = tv
				case uint8:
					tickDelay = int(tv)
				case uint16:
					tickDelay = int(tv)
				case uint32:
					tickDelay = int(tv)
				}

				conditional := 0
				switch cv := nbt["conditionalMode"].(type) {
				case uint8:
					if cv != 0 {
						conditional = 1
					}
				case int8:
					if cv != 0 {
						conditional = 1
					}
				case int32:
					if cv != 0 {
						conditional = 1
					}
				case int:
					if cv != 0 {
						conditional = 1
					}
				}

				var isRedstone *bool
				switch av := nbt["auto"].(type) {
				case uint8:
					redstone := av == 0
					if redstone {
						isRedstone = &redstone
					}
				case int8:
					redstone := av == 0
					if redstone {
						isRedstone = &redstone
					}
				case int32:
					redstone := av == 0
					if redstone {
						isRedstone = &redstone
					}
				case int:
					redstone := av == 0
					if redstone {
						isRedstone = &redstone
					}
				}

				rtid := lookupRuntimeID(x, y, z)
				mode := modeFromRuntimeID(rtid)

				item := ibImportCommandExport{
					PosX:           fmt.Sprintf("~%d", x-startX),
					PosY:           fmt.Sprintf("~%d", y-startY),
					PosZ:           fmt.Sprintf("~%d", z-startZ),
					CommandMessage: encodedCmd,
					CommandTitle:   title,
					Mode:           mode,
					TickDelay:      tickDelay,
					Conditional:    conditional,
					IsRedstone:     isRedstone,
				}

				encoded, err := json.Marshal(item)
				if err != nil {
					return fmt.Errorf("序列化命令 JSON 失败: %w", err)
				}

				if wroteAny {
					if _, err := w.Write([]byte{',', '\n'}); err != nil {
						return fmt.Errorf("写入 JSON 分隔符失败: %w", err)
					}
				}
				if _, err := w.Write([]byte("    ")); err != nil {
					return fmt.Errorf("写入 JSON 缩进失败: %w", err)
				}
				if _, err := w.Write(encoded); err != nil {
					return fmt.Errorf("写入 JSON 对象失败: %w", err)
				}
				wroteAny = true
			}
		}
	}

	if wroteAny {
		if _, err := w.Write([]byte{'\n'}); err != nil {
			return fmt.Errorf("写入 JSON 结尾换行失败: %w", err)
		}
	}
	if _, err := w.Write([]byte{']', '\n'}); err != nil {
		return fmt.Errorf("写入 JSON 尾失败: %w", err)
	}

	return nil
}

func (i *IBImport) Close() error {
	return nil
}

type ibImportSegmentWriter struct {
	file        *os.File
	lengthPos   int64
	payload     *ibImportXORCountingWriter
	payloadSink *bufio.Writer
}

func beginIBImportSegment(file *os.File, key byte) (*ibImportSegmentWriter, error) {
	if file == nil {
		return nil, fmt.Errorf("目标文件为 nil")
	}
	lengthPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, fmt.Errorf("获取段起始位置失败: %w", err)
	}

	// 预留 5 字节 varint，后续回填真实长度（允许非最短 varint）。
	placeholder := [5]byte{0x80, 0x80, 0x80, 0x80, 0x00}
	if _, err := file.Write(placeholder[:]); err != nil {
		return nil, fmt.Errorf("写入段长度占位失败: %w", err)
	}
	if _, err := file.Write([]byte{key}); err != nil {
		return nil, fmt.Errorf("写入段密钥失败: %w", err)
	}

	payload := &ibImportXORCountingWriter{w: file, key: key}
	// 默认缓冲即可；外部还会再包一层 bufio.Writer 来减少 fmt 的 syscall。
	payloadSink := bufio.NewWriterSize(payload, 256*1024)
	return &ibImportSegmentWriter{
		file:        file,
		lengthPos:   lengthPos,
		payload:     payload,
		payloadSink: payloadSink,
	}, nil
}

func (s *ibImportSegmentWriter) Write(p []byte) (int, error) {
	if s == nil || s.payloadSink == nil {
		return 0, fmt.Errorf("segment writer 未初始化")
	}
	return s.payloadSink.Write(p)
}

func (s *ibImportSegmentWriter) WriteString(str string) (int, error) {
	if s == nil || s.payloadSink == nil {
		return 0, fmt.Errorf("segment writer 未初始化")
	}
	return s.payloadSink.WriteString(str)
}

func (s *ibImportSegmentWriter) Close() error {
	if s == nil || s.file == nil || s.payload == nil || s.payloadSink == nil {
		return fmt.Errorf("segment writer 未初始化")
	}
	if err := s.payloadSink.Flush(); err != nil {
		return fmt.Errorf("刷新段数据失败: %w", err)
	}

	payloadLen := s.payload.n
	if payloadLen < 0 {
		return fmt.Errorf("段长度为负数: %d", payloadLen)
	}

	endPos, err := s.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("获取段结束位置失败: %w", err)
	}

	if _, err := s.file.Seek(s.lengthPos, io.SeekStart); err != nil {
		return fmt.Errorf("回填段长度失败: %w", err)
	}
	encoded, err := encodeIBImportVarIntFixed5(payloadLen)
	if err != nil {
		return err
	}
	if _, err := s.file.Write(encoded[:]); err != nil {
		return fmt.Errorf("写入段长度失败: %w", err)
	}
	if _, err := s.file.Seek(endPos, io.SeekStart); err != nil {
		return fmt.Errorf("恢复文件指针失败: %w", err)
	}
	return nil
}

type ibImportXORCountingWriter struct {
	w   io.Writer
	key byte
	n   int
	buf []byte
}

func (x *ibImportXORCountingWriter) Write(p []byte) (int, error) {
	if x == nil || x.w == nil {
		return 0, fmt.Errorf("XOR writer 未初始化")
	}
	if len(p) == 0 {
		return 0, nil
	}
	if cap(x.buf) < len(p) {
		x.buf = make([]byte, len(p))
	} else {
		x.buf = x.buf[:len(p)]
	}
	copy(x.buf, p)
	for i := range x.buf {
		x.buf[i] ^= x.key
	}
	n, err := x.w.Write(x.buf)
	x.n += n
	return n, err
}

func encodeIBImportVarIntFixed5(value int) ([5]byte, error) {
	if value < 0 {
		return [5]byte{}, fmt.Errorf("IBImport 段长度为负数: %d", value)
	}
	v := uint64(value)
	if v > (1<<35)-1 {
		return [5]byte{}, fmt.Errorf("IBImport 段长度过大: %d", value)
	}
	var out [5]byte
	for i := 0; i < 5; i++ {
		out[i] = byte(v & 0x7F)
		v >>= 7
		if i < 4 {
			out[i] |= 0x80
		}
	}
	// 说明：此处故意使用固定 5 字节 varint（非最短编码），以支持“先写后回填”且不需要移动文件内容。
	return out, nil
}
