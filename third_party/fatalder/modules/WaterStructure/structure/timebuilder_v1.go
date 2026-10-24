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

type TimeBuilderV1 struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset
	origin       define.Origin

	paletteCache map[string]uint32
	blocks       []timeBuilderBlock

	nonAirBlocks int
}

type timeBuilderBlock struct {
	LocalX    int
	LocalY    int
	LocalZ    int
	RuntimeID uint32
}

type timeBuilderRoot struct {
	Version string                  `json:"version"`
	Blocks  []timeBuilderBlockEntry `json:"block"`
}

type timeBuilderBlockEntry struct {
	Name string  `json:"name"`
	Aux  int     `json:"aux"`
	Pos  [][]int `json:"pos"`
}

func (t *TimeBuilderV1) ID() uint8 {
	return IDTimeBuilderV1
}

func (t *TimeBuilderV1) Name() string {
	return NameTimeBuilderV1
}

func (t *TimeBuilderV1) FromFile(file *os.File) error {
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	var root timeBuilderRoot
	if err := json.NewDecoder(file).Decode(&root); err != nil {
		return fmt.Errorf("解析 timebuilder v1 的 JSON 失败: %w", err)
	}

	if strings.TrimSpace(root.Version) != "TimeBuilder" {
		return fmt.Errorf("不支持或未知的版本: %q", root.Version)
	}

	t.file = file
	return t.populateFromRoot(root)
}

func (t *TimeBuilderV1) populateFromRoot(root timeBuilderRoot) error {
	t.paletteCache = make(map[string]uint32)
	t.blocks = nil
	t.nonAirBlocks = 0

	minX, minY, minZ := math.MaxInt, math.MaxInt, math.MaxInt
	maxX, maxY, maxZ := math.MinInt, math.MinInt, math.MinInt

	accum := make(map[[3]int]uint32)

	for _, entry := range root.Blocks {
		runtimeID := t.runtimeIDFor(entry.Name, entry.Aux)

		for _, pos := range entry.Pos {
			if len(pos) < 3 {
				continue
			}

			x := pos[0]
			y := pos[1]
			z := pos[2]

			key := [3]int{x, y, z}
			accum[key] = runtimeID

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

	t.origin = define.Origin{int32(minX), int32(minY), int32(minZ)}
	t.size = &define.Size{Width: width, Height: height, Length: length}
	t.originalSize = &define.Size{Width: width, Height: height, Length: length}

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

	t.blocks = make([]timeBuilderBlock, 0, len(accum))
	t.nonAirBlocks = 0

	for _, key := range keys {
		x := key[0]
		y := key[1]
		z := key[2]
		runtimeID := accum[key]

		blk := timeBuilderBlock{
			LocalX:    x - minX,
			LocalY:    y - minY,
			LocalZ:    z - minZ,
			RuntimeID: runtimeID,
		}
		t.blocks = append(t.blocks, blk)

		if runtimeID != block.AirRuntimeID {
			t.nonAirBlocks++
		}
	}

	// 检查是不是这个文件
	if len(t.paletteCache) == 0 {
		return ErrInvalidFile
	}

	return nil
}

func (t *TimeBuilderV1) runtimeIDFor(name string, aux int) uint32 {
	name = strings.TrimSpace(name)
	cacheKey := fmt.Sprintf("%s|%d", name, aux)
	if runtimeID, ok := t.paletteCache[cacheKey]; ok {
		return runtimeID
	}

	runtimeID, found := blocks.LegacyBlockToRuntimeID(name, uint16(aux))
	if !found {
		runtimeID, found = blocks.BlockStrToRuntimeID(fmt.Sprintf("%s %d", name, aux))
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

	t.paletteCache[cacheKey] = runtimeID
	return runtimeID
}

func (t *TimeBuilderV1) GetOffsetPos() define.Offset {
	return t.offsetPos
}

func (t *TimeBuilderV1) SetOffsetPos(offset define.Offset) {
	t.offsetPos = offset
	t.size.Width = t.originalSize.Width + int(math.Abs(float64(offset.X())))
	t.size.Length = t.originalSize.Length + int(math.Abs(float64(offset.Z())))
	t.size.Height = t.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (t *TimeBuilderV1) GetSize() define.Size {
	return *t.size
}

func (t *TimeBuilderV1) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	chunks := make(map[define.ChunkPos]*chunk.Chunk, len(posList))
	height := t.size.Height
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

	offsetX := int(t.offsetPos.X())
	offsetY := int(t.offsetPos.Y())
	offsetZ := int(t.offsetPos.Z())

	for _, blk := range t.blocks {
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

func (t *TimeBuilderV1) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any, len(posList))
	for _, pos := range posList {
		if _, exists := result[pos]; !exists {
			result[pos] = make(map[define.BlockPos]map[string]any)
		}
	}

	return result, nil
}

func (t *TimeBuilderV1) CountNonAirBlocks() (int, error) {
	return t.nonAirBlocks, nil
}

func (t *TimeBuilderV1) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
) error {
	return convertReaderToMCWorld(t, bedrockWorld, startSubChunkPos, startCallback, progressCallback)
}

func (t *TimeBuilderV1) Close() error {
	return nil
}
