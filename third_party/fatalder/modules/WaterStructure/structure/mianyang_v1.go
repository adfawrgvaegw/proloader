package structure

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"strings"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/blocks"
	blocksnbt "github.com/Yeah114/blocks/snbt"
)

type MianYangV1 struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset

	namespaces   []string
	blocks       []mianyangBlock
	paletteCache map[paletteCacheKey]uint32

	nonAirBlocks int
}

type mianyangBlock struct {
	LocalX    int
	LocalY    int
	LocalZ    int
	RuntimeID uint32
	NBT       map[string]any
}

type paletteCacheKey struct {
	Index int
	Data  uint16
}

type rawMianYangFile struct {
	ChunkedBlocks []rawMianYangChunk `json:"chunkedBlocks"`
	Namespaces    []string           `json:"namespaces"`
}

type rawMianYangChunk struct {
	StartX int     `json:"startX"`
	StartZ int     `json:"startZ"`
	Blocks [][]any `json:"blocks"`
}

type worldBlock struct {
	X         int
	Y         int
	Z         int
	RuntimeID uint32
	NBT       map[string]any
}

func (m *MianYangV1) ID() uint8 {
	return IDMianYangV1
}

func (m *MianYangV1) Name() string {
	return NameMianYangV1
}

func (m *MianYangV1) FromFile(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	decoder := json.NewDecoder(file)
	decoder.UseNumber()

	var data rawMianYangFile
	if err := decoder.Decode(&data); err != nil {
		return fmt.Errorf("解析 MianYang V1 的 JSON 失败: %w", err)
	}

	m.file = file
	return m.populateFromData(data)
}

func (m *MianYangV1) populateFromData(data rawMianYangFile) error {
	if len(data.ChunkedBlocks) == 0 || len(data.Namespaces) == 0 {
		return ErrInvalidFile
	}

	m.size = &define.Size{}
	m.originalSize = &define.Size{}
	m.offsetPos = define.Offset{}
	m.namespaces = data.Namespaces
	m.paletteCache = make(map[paletteCacheKey]uint32)
	m.blocks = nil
	m.nonAirBlocks = 0

	worldBlocks := make([]worldBlock, 0)

	minX, minY, minZ := math.MaxInt, math.MaxInt, math.MaxInt
	maxX, maxY, maxZ := math.MinInt, math.MinInt, math.MinInt

	for _, chunkData := range data.ChunkedBlocks {
		for _, entry := range chunkData.Blocks {
			if len(entry) < 5 {
				return fmt.Errorf("方块条目长度无效: %d", len(entry))
			}

			blockIndex, err := extractInt(entry[0], "方块索引")
			if err != nil {
				return err
			}
			if blockIndex < 0 || blockIndex >= len(m.namespaces) {
				return fmt.Errorf("方块索引 %d 越界", blockIndex)
			}

			dataValue, err := extractInt(entry[1], "方块数据")
			if err != nil {
				return err
			}
			if dataValue < 0 || dataValue > math.MaxUint16 {
				return fmt.Errorf("方块数据 %d 超出 uint16 范围", dataValue)
			}

			localX, err := extractInt(entry[2], "局部 x")
			if err != nil {
				return err
			}
			localY, err := extractInt(entry[3], "局部 y")
			if err != nil {
				return err
			}
			localZ, err := extractInt(entry[4], "局部 z")
			if err != nil {
				return err
			}

			worldX := chunkData.StartX + localX
			worldY := localY
			worldZ := chunkData.StartZ + localZ

			runtimeID := m.runtimeIDFor(blockIndex, dataValue)

			var nbt map[string]any
			if len(entry) >= 6 {
				rawNBT, ok := entry[5].(string)
				if !ok {
					return fmt.Errorf("NBT 负载必须为字符串, 实际为 %T", entry[5])
				}
				rawNBT = strings.TrimSpace(rawNBT)
				if rawNBT != "" {
					parsedNBT, err := parseMianYangNBT(rawNBT)
					if err != nil {
						return fmt.Errorf("解析坐标 (%d,%d,%d) 的 NBT 负载失败: %w", worldX, worldY, worldZ, err)
					}
					nbt = parsedNBT
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

			worldBlocks = append(worldBlocks, worldBlock{
				X:         worldX,
				Y:         worldY,
				Z:         worldZ,
				RuntimeID: runtimeID,
				NBT:       nbt,
			})

			if runtimeID != block.AirRuntimeID {
				m.nonAirBlocks++
			}
		}
	}

	if len(worldBlocks) == 0 {
		return ErrInvalidFile
	}

	width := maxX - minX + 1
	height := maxY - minY + 1
	length := maxZ - minZ + 1

	m.originalSize.Width = width
	m.originalSize.Height = height
	m.originalSize.Length = length

	m.size.Width = width
	m.size.Height = height
	m.size.Length = length

	m.blocks = make([]mianyangBlock, len(worldBlocks))
	for i, wb := range worldBlocks {
		m.blocks[i] = mianyangBlock{
			LocalX:    wb.X - minX,
			LocalY:    wb.Y - minY,
			LocalZ:    wb.Z - minZ,
			RuntimeID: wb.RuntimeID,
			NBT:       wb.NBT,
		}
	}

	// 检查是不是这个文件
	if len(m.namespaces) == 0 {
		return ErrInvalidFile
	}

	return nil
}

func (m *MianYangV1) runtimeIDFor(index int, dataValue int) uint32 {
	key := paletteCacheKey{Index: index, Data: uint16(dataValue)}
	if runtimeID, ok := m.paletteCache[key]; ok {
		return runtimeID
	}

	name := m.namespaces[index]
	runtimeID, found := blocks.LegacyBlockToRuntimeID(name, uint16(dataValue))
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
	m.paletteCache[key] = runtimeID
	return runtimeID
}

func (m *MianYangV1) GetOffsetPos() define.Offset {
	return m.offsetPos
}

func (m *MianYangV1) SetOffsetPos(offset define.Offset) {
	m.offsetPos = offset
	m.size.Width = m.originalSize.Width + int(math.Abs(float64(offset.X())))
	m.size.Length = m.originalSize.Length + int(math.Abs(float64(offset.Z())))
	m.size.Height = m.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (m *MianYangV1) GetSize() define.Size {
	return *m.size
}

func (m *MianYangV1) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	result := make(map[define.ChunkPos]*chunk.Chunk, len(posList))
	height := m.size.Height
	if height <= 0 {
		height = 1
	}

	for _, pos := range posList {
		if _, exists := result[pos]; !exists {
			result[pos] = chunk.NewChunk(block.AirRuntimeID, MCWorldOverworldRange)
		}
	}

	if len(result) == 0 {
		return result, nil
	}

	offsetX := int(m.offsetPos.X())
	offsetY := int(m.offsetPos.Y())
	offsetZ := int(m.offsetPos.Z())

	for _, blockEntry := range m.blocks {
		newX := blockEntry.LocalX + offsetX
		newY := blockEntry.LocalY + offsetY
		newZ := blockEntry.LocalZ + offsetZ

		chunkX := floorDiv(newX, 16)
		chunkZ := floorDiv(newZ, 16)
		chunkPos := define.ChunkPos{int32(chunkX), int32(chunkZ)}

		c, exists := result[chunkPos]
		if !exists {
			continue
		}

		localX := newX - chunkX*16
		localZ := newZ - chunkZ*16
		c.SetBlock(uint8(localX), int16(newY)-64, uint8(localZ), 0, blockEntry.RuntimeID)
	}

	return result, nil
}

func (m *MianYangV1) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any, len(posList))

	for _, pos := range posList {
		if _, exists := result[pos]; !exists {
			result[pos] = make(map[define.BlockPos]map[string]any)
		}
	}

	if len(result) == 0 {
		return result, nil
	}

	offsetX := int(m.offsetPos.X())
	offsetY := int(m.offsetPos.Y())
	offsetZ := int(m.offsetPos.Z())

	for _, blockEntry := range m.blocks {
		if blockEntry.NBT == nil {
			continue
		}

		newX := blockEntry.LocalX + offsetX
		newY := blockEntry.LocalY + offsetY
		newZ := blockEntry.LocalZ + offsetZ

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
		chunkNBT[blockPos] = blockEntry.NBT
	}

	return result, nil
}

func (m *MianYangV1) CountNonAirBlocks() (int, error) {
	return m.nonAirBlocks, nil
}

func (m *MianYangV1) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
) error {
	return convertReaderToMCWorld(m, bedrockWorld, startSubChunkPos, startCallback, progressCallback)
}

func (m *MianYangV1) Close() error {
	return nil
}

func extractInt(value any, field string) (int, error) {
	switch v := value.(type) {
	case json.Number:
		i64, err := v.Int64()
		if err != nil {
			return 0, fmt.Errorf("解析 %s 失败: %w", field, err)
		}
		return int(i64), nil
	case float64:
		return int(v), nil
	case int:
		return v, nil
	default:
		return 0, fmt.Errorf("字段 %s 的类型异常: %T", field, value)
	}
}

func parseMianYangNBT(raw string) (map[string]any, error) {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return nil, nil
	}

	// Attempt to parse as JSON wrapper first.
	var jsonCandidate map[string]any
	if err := json.Unmarshal([]byte(candidate), &jsonCandidate); err == nil {
		if len(jsonCandidate) > 0 {
			if encodedComplete, ok := jsonCandidate["blockCompleteNBT"].(string); ok {
				compound, err := decodeMianYangSNBT(encodedComplete)
				if err != nil {
					return nil, err
				}
				return compound, nil
			}
			// Fallback: Return the JSON payload directly when it doesn't contain encoded SNBT.
			return cloneMap(jsonCandidate), nil
		}
	}

	// If not JSON, treat it as raw SNBT (with optional encoded content).
	compound, err := decodeMianYangSNBT(candidate)
	if err != nil {
		return nil, err
	}
	return compound, nil
}

func fixCommand(rawStr string) (string, error) {
	// 1. 定位目标片段的前后边界
	preFix := `Command:"`      // 前边界: Command:" 之后
	sufFix := `",CustomName:"` // 后边界: ",CustomName:" 之前

	// 2. 找到前边界在原字符串中的结束位置
	preEnd := strings.Index(rawStr, preFix)
	if preEnd == -1 {
		return rawStr, fmt.Errorf("未找到前边界 %q", preFix)
	}
	preEnd += len(preFix) // 移动到 "Command:\"" 后面, 即中间片段的起始位置

	// 3. 找到后边界在原字符串中的起始位置
	sufStart := strings.Index(rawStr[preEnd:], sufFix)
	if sufStart == -1 {
		return rawStr, fmt.Errorf("未找到后边界 %q", sufFix)
	}
	sufStart += preEnd // 转换为原字符串中的实际索引

	// 4. 提取中间被修改过的「未编码/编码异常」内容
	modifiedMiddle := rawStr[preEnd:sufStart]

	// 5. 对提取的内容进行「原始JSON编码」, 恢复转义格式
	originalEncoded, err := json.Marshal(modifiedMiddle)
	if err != nil {
		return rawStr, fmt.Errorf("JSON编码恢复失败: %w", err)
	}
	// 6. 将恢复编码后的内容放回原字符串, 完成还原
	result := rawStr[:preEnd-1] + string(originalEncoded) + rawStr[sufStart+1:]
	return result, nil
}

func decodeMianYangSNBT(encoded string) (map[string]any, error) {
	decoded, err := url.QueryUnescape(encoded)
	if err != nil {
		return nil, fmt.Errorf("URL 解码 NBT 失败: %w", err)
	}
	decoded = strings.TrimSpace(decoded)
	if decoded == "" {
		return nil, nil
	}
	decoded, _ = fixCommand(decoded)
	if strings.HasPrefix(decoded, "\"\"") {
		decoded = "{" + decoded + "}"
	}
	if strings.HasPrefix(decoded, "{") || strings.HasPrefix(decoded, "[") {
		val, err := blocksnbt.SNBToNbt(decoded)
		if err != nil {
			return nil, fmt.Errorf("解析 SNBT 失败: %w", err)
		}
		compound, ok := val.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("SNBT 根类型异常: %T", val)
		}
		if inner, exists := compound[""]; exists {
			if innerMap, ok := inner.(map[string]any); ok {
				return cloneMap(innerMap), nil
			}
		}
		return cloneMap(compound), nil
	}

	// Not a structured SNBT; treat as plain string map with special key.
	return map[string]any{"": decoded}, nil
}

func cloneMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dup := make(map[string]any, len(src))
	for k, v := range src {
		dup[k] = v
	}
	return dup
}
