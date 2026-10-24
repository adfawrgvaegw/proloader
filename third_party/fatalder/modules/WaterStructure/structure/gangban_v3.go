package structure

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/blocks"
	blocksnbt "github.com/Yeah114/blocks/snbt"
)

type GangBanV3 struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset
	origin       define.Origin

	palette      []string
	blocks       []gangBanBlock
	paletteCache map[paletteCacheKey]uint32
	seenBlocks   map[[3]int]int

	nonAirBlocks int
}

type gangBanV3Header struct {
	X  int `json:"x"`
	Y  int `json:"y"`
	Z  int `json:"z"`
	Dx int `json:"xcha"`
	Dy int `json:"ycha"`
	Dz int `json:"zcha"`
}

type gangBanV3Chunk struct {
	ID    int                  `json:"id"`
	Grids gangBanV3ChunkBounds `json:"grids"`
	Data  [][]any              `json:"data"`
}

type gangBanV3ChunkBounds struct {
	X  int `json:"x"`
	Z  int `json:"z"`
	X1 int `json:"x1"`
	Z1 int `json:"z1"`
}

var palettePattern = regexp.MustCompile(`\[(-?[0-9]+)\](.*?)\[(-?[0-9]+)\]`)

func (g *GangBanV3) FromFile(file *os.File) error {
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	decoder := json.NewDecoder(file)
	decoder.UseNumber()

	var entries []json.RawMessage
	if err := decoder.Decode(&entries); err != nil {
		return fmt.Errorf("解析 GangBan V3 的 JSON 失败: %w", err)
	}
	if len(entries) < 3 {
		return ErrInvalidFile
	}

	var header gangBanV3Header
	if err := json.Unmarshal(entries[0], &header); err != nil {
		return fmt.Errorf("解析头部失败: %w", err)
	}

	var paletteString string
	if err := json.Unmarshal(entries[1], &paletteString); err != nil {
		return fmt.Errorf("解析调色板字符串失败: %w", err)
	}

	palette, err := parseGangBanV3Palette(paletteString)
	if err != nil {
		return err
	}
	if len(palette) == 0 {
		return ErrInvalidFile
	}

	chunks := make([]gangBanV3Chunk, len(entries)-2)
	for i := range chunks {
		if err := json.Unmarshal(entries[i+2], &chunks[i]); err != nil {
			return fmt.Errorf("解析区块 %d 失败: %w", i, err)
		}
	}

	g.file = file
	return g.populateFromComponents(header, palette, chunks)
}

func (g *GangBanV3) populateFromComponents(header gangBanV3Header, palette []string, chunks []gangBanV3Chunk) error {
	width := header.Dx
	height := header.Dy
	length := header.Dz
	if width <= 0 || height <= 0 || length <= 0 {
		return ErrInvalidFile
	}

	g.size = &define.Size{Width: width, Height: height, Length: length}
	g.originalSize = &define.Size{Width: width, Height: height, Length: length}
	g.offsetPos = define.Offset{}
	g.origin = define.Origin{int32(header.X), int32(header.Y), int32(header.Z)}
	g.palette = palette
	g.paletteCache = make(map[paletteCacheKey]uint32)
	g.blocks = make([]gangBanBlock, 0)
	g.nonAirBlocks = 0
	g.seenBlocks = make(map[[3]int]int)

	for _, chunk := range chunks {
		chunkOriginX := chunk.Grids.X
		chunkOriginZ := chunk.Grids.Z

		for _, entry := range chunk.Data {
			if len(entry) < 5 {
				return fmt.Errorf("方块条目长度无效: %d", len(entry))
			}

			paletteIndex, err := toInt(entry[0])
			if err != nil || paletteIndex < 0 || paletteIndex >= len(g.palette) {
				return fmt.Errorf("调色板索引无效: %v", entry[0])
			}

			aux, err := toInt(entry[1])
			if err != nil || aux < 0 || aux > math.MaxUint16 {
				return fmt.Errorf("辅助值无效: %v", entry[1])
			}

			localX, err := toInt(entry[2])
			if err != nil {
				return fmt.Errorf("局部 x 无效: %v", entry[2])
			}

			y, err := toInt(entry[3])
			if err != nil {
				return fmt.Errorf("y 无效: %v", entry[3])
			}

			localZ, err := toInt(entry[4])
			if err != nil {
				return fmt.Errorf("局部 z 无效: %v", entry[4])
			}

			worldX := chunkOriginX + localX
			worldY := y
			worldZ := chunkOriginZ + localZ

			localXInStructure := worldX - int(g.origin.X())
			localYInStructure := worldY - int(g.origin.Y())
			localZInStructure := worldZ - int(g.origin.Z())

			/*
				if localXInStructure < 0 || localYInStructure < 0 || localZInStructure < 0 || localXInStructure >= g.size.Width || localYInStructure >= g.size.Height || localZInStructure >= g.size.Length {
					return fmt.Errorf("block position outside structure bounds: [%d,%d,%d]", worldX, worldY, worldZ)
				}
			*/

			runtimeID := g.runtimeIDFor(paletteIndex, aux)

			var nbt map[string]any
			if len(entry) >= 7 {
				marker, _ := entry[5].(string)
				if marker == "nbt" {
					if snbtStr, ok := entry[6].(string); ok {
						nbt = parseGangBanV3NBT(snbtStr)
					}
				}
			}

			key := [3]int{localXInStructure, localYInStructure, localZInStructure}
			if idx, exists := g.seenBlocks[key]; exists {
				blk := &g.blocks[idx]
				blk.RuntimeID = runtimeID
				if blk.NBT == nil && nbt != nil {
					blk.NBT = nbt
				}
				continue
			}

			g.blocks = append(g.blocks, gangBanBlock{
				LocalX:    localXInStructure,
				LocalY:    localYInStructure,
				LocalZ:    localZInStructure,
				RuntimeID: runtimeID,
				NBT:       nbt,
			})
			g.seenBlocks[key] = len(g.blocks) - 1

			if runtimeID != block.AirRuntimeID {
				g.nonAirBlocks++
			}
		}
	}

	// 检查是不是这个文件
	if len(g.palette) == 0 {
		return ErrInvalidFile
	}

	return nil
}

func (g *GangBanV3) runtimeIDFor(index int, aux int) uint32 {
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

func (g *GangBanV3) GetOffsetPos() define.Offset {
	return g.offsetPos
}

func (g *GangBanV3) SetOffsetPos(offset define.Offset) {
	g.offsetPos = offset
	g.size.Width = g.originalSize.Width + int(math.Abs(float64(offset.X())))
	g.size.Length = g.originalSize.Length + int(math.Abs(float64(offset.Z())))
	g.size.Height = g.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (g *GangBanV3) GetSize() define.Size {
	return *g.size
}

func (g *GangBanV3) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
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

func (g *GangBanV3) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
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

func (g *GangBanV3) CountNonAirBlocks() (int, error) {
	return g.nonAirBlocks, nil
}

func (g *GangBanV3) ID() uint8 {
	return IDGangBanV3
}

func (g *GangBanV3) Name() string {
	return NameGangBanV3
}

func (g *GangBanV3) Close() error {
	return nil
}

func parseGangBanV3Palette(palette string) ([]string, error) {
	matches := palettePattern.FindAllStringSubmatch(palette, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("解析调色板字符串失败")
	}

	indexMap := make(map[int]string)
	maxIndex := 0
	for _, match := range matches {
		if len(match) != 4 || match[1] != match[3] {
			continue
		}
		idx, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, fmt.Errorf("调色板索引无效: %s", match[1])
		}
		indexMap[idx] = match[2]
		if idx > maxIndex {
			maxIndex = idx
		}
	}

	paletteList := make([]string, maxIndex+1)
	for i := 0; i <= maxIndex; i++ {
		name, ok := indexMap[i]
		if !ok {
			return nil, fmt.Errorf("调色板缺少索引 %d", i)
		}
		paletteList[i] = name
	}

	return paletteList, nil
}

func parseGangBanV3NBT(snbt string) map[string]any {
	snbt = strings.TrimSpace(snbt)
	if snbt == "" {
		return nil
	}

	if val, err := blocksnbt.SNBToNbt(snbt); err == nil {
		if compound, ok := val.(map[string]any); ok {
			return cloneMap(compound)
		}
	}

	return nil
}

func toInt(value any) (int, error) {
	switch v := value.(type) {
	case json.Number:
		i64, err := v.Int64()
		return int(i64), err
	case float64:
		return int(v), nil
	case float32:
		return int(v), nil
	case int:
		return v, nil
	case int8:
		return int(v), nil
	case int16:
		return int(v), nil
	case int32:
		return int(v), nil
	case int64:
		return int(v), nil
	case uint8:
		return int(v), nil
	case uint16:
		return int(v), nil
	case uint32:
		return int(v), nil
	case uint64:
		return int(v), nil
	case string:
		return strconv.Atoi(v)
	default:
		return 0, fmt.Errorf("不支持的类型 %T", value)
	}
}
