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

type GangBanV4 struct {
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

type gangBanV4Header struct {
	Dx int `json:"xcha"`
	Dy int `json:"ycha"`
	Dz int `json:"zcha"`
}

type gangBanV4Accum struct {
	WorldX    int
	WorldY    int
	WorldZ    int
	RuntimeID uint32
	NBT       map[string]any
}

func (g *GangBanV4) ID() uint8 {
	return IDGangBanV4
}

func (g *GangBanV4) Name() string {
	return NameGangBanV4
}

func (g *GangBanV4) FromFile(file *os.File) error {
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	decoder := json.NewDecoder(file)
	decoder.UseNumber()

	var entries []json.RawMessage
	if err := decoder.Decode(&entries); err != nil {
		return fmt.Errorf("解析 GangBan V4 的 JSON 失败: %w", err)
	}
	if len(entries) < 3 {
		return ErrInvalidFile
	}

	var header gangBanV4Header
	if err := json.Unmarshal(entries[0], &header); err != nil {
		return fmt.Errorf("解析头部失败: %w", err)
	}

	var palette []string
	if err := json.Unmarshal(entries[1], &palette); err != nil {
		return fmt.Errorf("解析调色板失败: %w", err)
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

func (g *GangBanV4) populateFromComponents(_ gangBanV4Header, palette []string, chunks []gangBanV3Chunk) error {
	g.palette = palette
	g.paletteCache = make(map[paletteCacheKey]uint32)
	g.offsetPos = define.Offset{}
	g.nonAirBlocks = 0

	accum := make(map[[3]int]*gangBanV4Accum)

	minX, minY, minZ := math.MaxInt, math.MaxInt, math.MaxInt
	maxX, maxY, maxZ := math.MinInt, math.MinInt, math.MinInt

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

			key := [3]int{worldX, worldY, worldZ}
			rec, exists := accum[key]
			if !exists {
				rec = &gangBanV4Accum{
					WorldX:    worldX,
					WorldY:    worldY,
					WorldZ:    worldZ,
					RuntimeID: g.runtimeIDFor(paletteIndex, aux),
				}
				accum[key] = rec
			} else {
				rec.RuntimeID = g.runtimeIDFor(paletteIndex, aux)
			}

			if len(entry) >= 7 {
				if marker, ok := entry[5].(string); ok && marker == "nbt" {
					if snbtStr, ok := entry[6].(string); ok {
						if nbt := parseGangBanV3NBT(snbtStr); nbt != nil {
							rec.NBT = nbt
						}
					}
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

	if len(accum) == 0 {
		return ErrInvalidFile
	}

	width := maxX - minX + 1
	height := maxY - minY + 1
	length := maxZ - minZ + 1

	if width <= 0 || height <= 0 || length <= 0 {
		return ErrInvalidFile
	}

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

func (g *GangBanV4) runtimeIDFor(index int, aux int) uint32 {
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

func (g *GangBanV4) GetOffsetPos() define.Offset {
	return g.offsetPos
}

func (g *GangBanV4) SetOffsetPos(offset define.Offset) {
	g.offsetPos = offset
	g.size.Width = g.originalSize.Width + int(math.Abs(float64(offset.X())))
	g.size.Length = g.originalSize.Length + int(math.Abs(float64(offset.Z())))
	g.size.Height = g.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (g *GangBanV4) GetSize() define.Size {
	return *g.size
}

func (g *GangBanV4) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
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

func (g *GangBanV4) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
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

func (g *GangBanV4) CountNonAirBlocks() (int, error) {
	return g.nonAirBlocks, nil
}

func (g *GangBanV4) Close() error {
	return nil
}
