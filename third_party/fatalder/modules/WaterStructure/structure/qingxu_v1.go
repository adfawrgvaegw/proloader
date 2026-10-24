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
	"github.com/TriM-Organization/bedrock-world-operator/world"
	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/blocks"
)

type QingXuV1 struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset
	origin       define.Origin

	paletteCache map[string]uint32
	blocks       []qingXuBlock

	nonAirBlocks int
}

type qingXuBlock struct {
	LocalX    int
	LocalY    int
	LocalZ    int
	RuntimeID uint32
}

func (q *QingXuV1) ID() uint8 {
	return IDQingXuV1
}

func (q *QingXuV1) Name() string {
	return NameQingXuV1
}

func (q *QingXuV1) FromFile(file *os.File) error {
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	var root map[string]json.RawMessage
	if err := json.NewDecoder(file).Decode(&root); err != nil {
		return fmt.Errorf("解析 QingXu V1 的 JSON 失败: %w", err)
	}

	totalBlocksRaw, ok := root["totalBlocks"]
	if !ok {
		return ErrInvalidFile
	}

	var totalBlocks int
	if err := json.Unmarshal(totalBlocksRaw, &totalBlocks); err != nil {
		return fmt.Errorf("解析 totalBlocks 失败: %w", err)
	}

	q.file = file
	return q.populateFromRoot(root, totalBlocks)
}

func (q *QingXuV1) populateFromRoot(root map[string]json.RawMessage, totalBlocks int) error {
	q.paletteCache = make(map[string]uint32)
	q.blocks = nil
	q.nonAirBlocks = 0

	minX, minY, minZ := int(^uint(0)>>1), int(^uint(0)>>1), int(^uint(0)>>1)
	maxX, maxY, maxZ := -minX-1, -minY-1, -minZ-1

	accum := make(map[[3]int]uint32)

	for i := 0; i < totalBlocks; i++ {
		key := fmt.Sprintf("%d", i)
		chunkRaw, ok := root[key]
		if !ok {
			continue
		}

		var chunkStr string
		if err := json.Unmarshal(chunkRaw, &chunkStr); err != nil {
			return fmt.Errorf("解析第 %d 个区块字符串失败: %w", i, err)
		}
		if strings.TrimSpace(chunkStr) == "" {
			continue
		}

		var chunkMap map[string]json.RawMessage
		if err := json.Unmarshal([]byte(chunkStr), &chunkMap); err != nil {
			return fmt.Errorf("解析第 %d 个区块载荷失败: %w", i, err)
		}

		totalPointsRaw, ok := chunkMap["totalPoints"]
		if !ok {
			continue
		}
		var totalPoints int
		if err := json.Unmarshal(totalPointsRaw, &totalPoints); err != nil {
			return fmt.Errorf("解析第 %d 个区块的 totalPoints 失败: %w", i, err)
		}

		for j := 0; j < totalPoints; j++ {
			blockKey := fmt.Sprintf("%d", j)
			blockRaw, ok := chunkMap[blockKey]
			if !ok {
				continue
			}

			var blockStr string
			if err := json.Unmarshal(blockRaw, &blockStr); err != nil {
				return fmt.Errorf("解析第 %d 个区块中第 %d 个方块字符串失败: %w", i, j, err)
			}
			if strings.TrimSpace(blockStr) == "" {
				continue
			}

			var blockPayload map[string]any
			if err := json.Unmarshal([]byte(blockStr), &blockPayload); err != nil {
				return fmt.Errorf("解析第 %d 个区块中第 %d 个方块载荷失败: %w", i, j, err)
			}

			nameRaw, ok := blockPayload["Name"].(string)
			if !ok {
				return fmt.Errorf("第 %d 个区块中第 %d 个方块缺少 Name", i, j)
			}

			xVal, ok := blockPayload["X"]
			if !ok {
				return fmt.Errorf("第 %d 个区块中第 %d 个方块缺少 X", i, j)
			}
			yVal, ok := blockPayload["Y"]
			if !ok {
				return fmt.Errorf("第 %d 个区块中第 %d 个方块缺少 Y", i, j)
			}
			zVal, ok := blockPayload["Z"]
			if !ok {
				return fmt.Errorf("第 %d 个区块中第 %d 个方块缺少 Z", i, j)
			}

			x, err := toInt(xVal)
			if err != nil {
				return fmt.Errorf("解析第 %d 个区块中第 %d 个方块的 X 失败: %w", i, j, err)
			}
			y, err := toInt(yVal)
			if err != nil {
				return fmt.Errorf("解析第 %d 个区块中第 %d 个方块的 Y 失败: %w", i, j, err)
			}
			z, err := toInt(zVal)
			if err != nil {
				return fmt.Errorf("解析第 %d 个区块中第 %d 个方块的 Z 失败: %w", i, j, err)
			}

			runtimeID := q.runtimeIDFor(nameRaw)

			keyPos := [3]int{x, y, z}
			accum[keyPos] = runtimeID

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

	if len(accum) == 0 {
		return ErrInvalidFile
	}

	width := maxX - minX + 1
	height := maxY - minY + 1
	length := maxZ - minZ + 1

	q.origin = define.Origin{int32(minX), int32(minY), int32(minZ)}
	q.size = &define.Size{Width: width, Height: height, Length: length}
	q.originalSize = &define.Size{Width: width, Height: height, Length: length}

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

	q.blocks = make([]qingXuBlock, 0, len(accum))
	q.nonAirBlocks = 0

	for _, key := range keys {
		x := key[0]
		y := key[1]
		z := key[2]
		runtimeID := accum[key]

		blk := qingXuBlock{
			LocalX:    x - minX,
			LocalY:    y - minY,
			LocalZ:    z - minZ,
			RuntimeID: runtimeID,
		}
		q.blocks = append(q.blocks, blk)

		if runtimeID != block.AirRuntimeID {
			q.nonAirBlocks++
		}
	}

	// 检查是不是这个文件
	if len(q.paletteCache) == 0 {
		return ErrInvalidFile
	}

	return nil
}

func (q *QingXuV1) runtimeIDFor(name string) uint32 {
	name = strings.TrimSpace(name)
	name = reverseSplit(name)
	if name == "" {
		return UnknownBlockRuntimeID
	}

	if runtimeID, ok := q.paletteCache[name]; ok {
		return runtimeID
	}

	runtimeID, found := blocks.BlockStrToRuntimeID(name)
	if !found {
		runtimeID, found = blocks.LegacyBlockToRuntimeID(name, 0)
	}
	if !found && !strings.Contains(name, ":") {
		prefixed := "minecraft:" + name
		runtimeID, found = blocks.BlockStrToRuntimeID(prefixed)
		if !found {
			runtimeID, found = blocks.LegacyBlockToRuntimeID(prefixed, 0)
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

	q.paletteCache[name] = runtimeID
	return runtimeID
}

func reverseSplit(s string) string {
	// 找到第一个 '.' 的位置
	index := strings.Index(s, ".")
	if index == -1 {
		// 如果没有找到 '.', 直接返回原字符串
		return s
	}
	// 分割成前半段和后半段
	firstPart := s[:index]
	secondPart := s[index+1:]
	// 拼接成 "后半段_前半段" 并返回
	return fmt.Sprintf("%s_%s", secondPart, firstPart)
}

func (q *QingXuV1) GetOffsetPos() define.Offset {
	return q.offsetPos
}

func (q *QingXuV1) SetOffsetPos(offset define.Offset) {
	q.offsetPos = offset
	q.size.Width = q.originalSize.Width + int(math.Abs(float64(offset.X())))
	q.size.Length = q.originalSize.Length + int(math.Abs(float64(offset.Z())))
	q.size.Height = q.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (q *QingXuV1) GetSize() define.Size {
	return *q.size
}

func (q *QingXuV1) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	chunks := make(map[define.ChunkPos]*chunk.Chunk, len(posList))
	height := q.size.Height
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

	offsetX := int(q.offsetPos.X())
	offsetY := int(q.offsetPos.Y())
	offsetZ := int(q.offsetPos.Z())

	for _, blk := range q.blocks {
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

func (q *QingXuV1) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any, len(posList))
	for _, pos := range posList {
		if _, exists := result[pos]; !exists {
			result[pos] = make(map[define.BlockPos]map[string]any)
		}
	}

	return result, nil
}

func (q *QingXuV1) CountNonAirBlocks() (int, error) {
	return q.nonAirBlocks, nil
}

func (q *QingXuV1) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
) error {
	return convertReaderToMCWorld(q, bedrockWorld, startSubChunkPos, startCallback, progressCallback)
}

func (q *QingXuV1) Close() error {
	return nil
}
