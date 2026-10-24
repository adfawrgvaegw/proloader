package structure

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/blocks"
)

type MCFunction struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset

	blocks       []mcFunctionBlock
	nonAirBlocks int
}

type mcFunctionBlock struct {
	LocalX    int
	LocalY    int
	LocalZ    int
	RuntimeID uint32
}

func (m *MCFunction) ID() uint8 {
	return IDMCFunction
}

func (m *MCFunction) Name() string {
	return NameMCFunction
}

func (m *MCFunction) FromFile(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	scanner := bufio.NewScanner(file)
	blockMap := make(map[[3]int]uint32)
	minX, minY, minZ := math.MaxInt, math.MaxInt, math.MaxInt
	maxX, maxY, maxZ := math.MinInt, math.MinInt, math.MinInt

	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		cmdLower := strings.ToLower(line)
		if !strings.HasPrefix(cmdLower, "fill ") && !strings.HasPrefix(cmdLower, "setblock ") {
			continue
		}

		statePart := ""
		if idx := strings.Index(line, "["); idx != -1 {
			if end := strings.LastIndex(line, "]"); end > idx {
				statePart = line[idx : end+1]
				line = strings.TrimSpace(line[:idx] + line[end+1:])
			}
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		switch strings.ToLower(fields[0]) {
		case "fill":
			if len(fields) < 8 {
				continue
			}

			x1, err1 := parseMCFunctionCoord(fields[1])
			y1, err2 := parseMCFunctionCoord(fields[2])
			z1, err3 := parseMCFunctionCoord(fields[3])
			x2, err4 := parseMCFunctionCoord(fields[4])
			y2, err5 := parseMCFunctionCoord(fields[5])
			z2, err6 := parseMCFunctionCoord(fields[6])
			if err := firstError(err1, err2, err3, err4, err5, err6); err != nil {
				return fmt.Errorf("第 %d 行: %w", lineNumber, err)
			}

			blockName := fields[7]
			states, err := parseMCFunctionStates(statePart)
			if err != nil {
				return fmt.Errorf("第 %d 行: %w", lineNumber, err)
			}

			runtimeID := runtimeIDForBlock(blockName, states)

			xMin, xMax := min(x1, x2), max(x1, x2)
			yMin, yMax := min(y1, y2), max(y1, y2)
			zMin, zMax := min(z1, z2), max(z1, z2)

			for x := xMin; x <= xMax; x++ {
				for y := yMin; y <= yMax; y++ {
					for z := zMin; z <= zMax; z++ {
						blockMap[[3]int{x, y, z}] = runtimeID
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

		case "setblock":
			if len(fields) < 5 {
				continue
			}

			x, err1 := parseMCFunctionCoord(fields[1])
			y, err2 := parseMCFunctionCoord(fields[2])
			z, err3 := parseMCFunctionCoord(fields[3])
			if err := firstError(err1, err2, err3); err != nil {
				return fmt.Errorf("第 %d 行: %w", lineNumber, err)
			}

			blockName := fields[4]
			states, err := parseMCFunctionStates(statePart)
			if err != nil {
				return fmt.Errorf("第 %d 行: %w", lineNumber, err)
			}

			runtimeID := runtimeIDForBlock(blockName, states)

			blockMap[[3]int{x, y, z}] = runtimeID
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

		default:
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取 mcfunction 失败: %w", err)
	}

	if len(blockMap) == 0 {
		return ErrInvalidFile
	}

	m.file = file
	width := maxX - minX + 1
	height := maxY - minY + 1
	length := maxZ - minZ + 1

	m.size = &define.Size{Width: width, Height: height, Length: length}
	m.originalSize = &define.Size{Width: width, Height: height, Length: length}
	m.offsetPos = define.Offset{}

	keys := make([][3]int, 0, len(blockMap))
	for pos := range blockMap {
		keys = append(keys, pos)
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

	m.blocks = make([]mcFunctionBlock, 0, len(keys))
	m.nonAirBlocks = 0
	for _, pos := range keys {
		runtimeID := blockMap[pos]
		local := mcFunctionBlock{
			LocalX:    pos[0] - minX,
			LocalY:    pos[1] - minY,
			LocalZ:    pos[2] - minZ,
			RuntimeID: runtimeID,
		}
		m.blocks = append(m.blocks, local)
		if runtimeID != block.AirRuntimeID {
			m.nonAirBlocks++
		}
	}

	return nil
}

func (m *MCFunction) GetOffsetPos() define.Offset {
	return m.offsetPos
}

func (m *MCFunction) SetOffsetPos(offset define.Offset) {
	m.offsetPos = offset
	m.size.Width = m.originalSize.Width + int(math.Abs(float64(offset.X())))
	m.size.Length = m.originalSize.Length + int(math.Abs(float64(offset.Z())))
	m.size.Height = m.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (m *MCFunction) GetSize() define.Size {
	return *m.size
}

func (m *MCFunction) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	chunks := make(map[define.ChunkPos]*chunk.Chunk, len(posList))
	height := m.size.Height
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

	offsetX := int(m.offsetPos.X())
	offsetY := int(m.offsetPos.Y())
	offsetZ := int(m.offsetPos.Z())

	for _, blk := range m.blocks {
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
		c.SetBlock(uint8(localX), int16(newY) - 64, uint8(localZ), 0, blk.RuntimeID)
	}

	return chunks, nil
}

func (m *MCFunction) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any, len(posList))
	for _, pos := range posList {
		if _, exists := result[pos]; !exists {
			result[pos] = make(map[define.BlockPos]map[string]any)
		}
	}
	return result, nil
}

func (m *MCFunction) CountNonAirBlocks() (int, error) {
	return m.nonAirBlocks, nil
}

func (m *MCFunction) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
) error {
	return convertReaderToMCWorld(m, bedrockWorld, startSubChunkPos, startCallback, progressCallback)
}

func (m *MCFunction) Close() error {
	return nil
}

func parseMCFunctionCoord(token string) (int, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, fmt.Errorf("缺少坐标")
	}
	if token[0] == '~' {
		value := token[1:]
		if value == "" {
			return 0, nil
		}
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return 0, fmt.Errorf("相对坐标无效: %q", token)
		}
		return parsed, nil
	}
	parsed, err := strconv.Atoi(token)
	if err != nil {
		return 0, fmt.Errorf("坐标无效: %q", token)
	}
	return parsed, nil
}

func parseMCFunctionStates(statePart string) (map[string]any, error) {
	statePart = strings.TrimSpace(statePart)
	if statePart == "" {
		return nil, nil
	}
	if !strings.HasPrefix(statePart, "[") || !strings.HasSuffix(statePart, "]") {
		return nil, fmt.Errorf("状态格式无效: %s", statePart)
	}
	content := strings.TrimSpace(statePart[1 : len(statePart)-1])
	if content == "" {
		return nil, nil
	}

	parts := splitProperties(content)
	result := make(map[string]any, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		equalIdx := strings.Index(part, "=")
		if equalIdx <= 0 {
			return nil, fmt.Errorf("状态条目无效: %s", part)
		}
		key := strings.TrimSpace(part[:equalIdx])
		value := strings.TrimSpace(part[equalIdx+1:])

		key = strings.Trim(key, "\"")
		parsedValue, err := parseStateValue(value)
		if err != nil {
			return nil, err
		}
		result[key] = parsedValue
	}

	return result, nil
}

func splitProperties(content string) []string {
	parts := []string{}
	start := 0
	inQuotes := false
	for i, r := range content {
		switch r {
		case '"':
			inQuotes = !inQuotes
		case ',':
			if !inQuotes {
				parts = append(parts, strings.TrimSpace(content[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(content[start:]))
	return parts
}

func parseStateValue(value string) (any, error) {
	value = strings.TrimSpace(value)
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		return strings.Trim(value, "\""), nil
	}
	if i, err := strconv.Atoi(value); err == nil {
		return int32(i), nil
	}
	return value, nil
}

func runtimeIDForBlock(name string, states map[string]any) uint32 {
	if !strings.Contains(name, ":") {
		name = "minecraft:" + name
	}
	if states == nil {
		states = map[string]any{}
	}
	rtid, found := blocks.BlockNameAndStateToRuntimeID(name, states)
	if !found {
		return UnknownBlockRuntimeID
	}
	baseName, states, found := blocks.RuntimeIDToState(rtid)
	if !found {
		return UnknownBlockRuntimeID
	}
	name = "minecraft:" + baseName
	if runtimeID, ok := block.StateToRuntimeID(name, states); ok {
		return runtimeID
	}
	return UnknownBlockRuntimeID
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
