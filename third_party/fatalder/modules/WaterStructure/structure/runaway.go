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

type RunAway struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset
	origin       define.Origin

	paletteCache map[string]uint32
	blocks       []runAwayBlock

	nonAirBlocks int
}

type runAwayBlock struct {
	LocalX    int
	LocalY    int
	LocalZ    int
	RuntimeID uint32
}

type runAwayEntry struct {
	Name string `json:"name"`
	Aux  int    `json:"aux"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
	Z    int    `json:"z"`
}

func (r *RunAway) ID() uint8 {
	return IDRunAway
}

func (r *RunAway) Name() string {
	return NameRunAway
}

func (r *RunAway) FromFile(file *os.File) error {
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	var entries []runAwayEntry
	if err := json.NewDecoder(file).Decode(&entries); err != nil {
		return fmt.Errorf("解析 RunAway 的 JSON 失败: %w", err)
	}

	r.file = file
	return r.populate(entries)
}

func (r *RunAway) populate(entries []runAwayEntry) error {
	r.paletteCache = make(map[string]uint32)
	r.blocks = nil
	r.nonAirBlocks = 0

	if len(entries) == 0 {
		return ErrInvalidFile
	}

	minX, minY, minZ := math.MaxInt, math.MaxInt, math.MaxInt
	maxX, maxY, maxZ := math.MinInt, math.MinInt, math.MinInt

	accum := make(map[[3]int]uint32, len(entries))

	for idx, entry := range entries {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			return fmt.Errorf("条目 %d: 缺少方块名称", idx)
		}

		runtimeID := r.runtimeIDFor(name, entry.Aux)

		x, y, z := entry.X, entry.Y, entry.Z
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

	if len(accum) == 0 {
		return ErrInvalidFile
	}

	width := maxX - minX + 1
	height := maxY - minY + 1
	length := maxZ - minZ + 1

	r.origin = define.Origin{int32(minX), int32(minY), int32(minZ)}
	r.size = &define.Size{Width: width, Height: height, Length: length}
	r.originalSize = &define.Size{Width: width, Height: height, Length: length}

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

	r.blocks = make([]runAwayBlock, 0, len(accum))
	r.nonAirBlocks = 0

	for _, key := range keys {
		x := key[0]
		y := key[1]
		z := key[2]
		runtimeID := accum[key]

		blk := runAwayBlock{
			LocalX:    x - minX,
			LocalY:    y - minY,
			LocalZ:    z - minZ,
			RuntimeID: runtimeID,
		}
		r.blocks = append(r.blocks, blk)
		if runtimeID != block.AirRuntimeID {
			r.nonAirBlocks++
		}
	}

	if len(r.paletteCache) == 0 {
		return ErrInvalidFile
	}

	return nil
}

func (r *RunAway) runtimeIDFor(name string, aux int) uint32 {
	cacheKey := fmt.Sprintf("%s|%d", name, aux)
	if runtimeID, ok := r.paletteCache[cacheKey]; ok {
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

	r.paletteCache[cacheKey] = runtimeID
	return runtimeID
}

func (r *RunAway) GetOffsetPos() define.Offset {
	return r.offsetPos
}

func (r *RunAway) SetOffsetPos(offset define.Offset) {
	r.offsetPos = offset
	r.size.Width = r.originalSize.Width + int(math.Abs(float64(offset.X())))
	r.size.Length = r.originalSize.Length + int(math.Abs(float64(offset.Z())))
	r.size.Height = r.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (r *RunAway) GetSize() define.Size {
	return *r.size
}

func (r *RunAway) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	chunks := make(map[define.ChunkPos]*chunk.Chunk, len(posList))
	height := r.size.Height
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

	offsetX := int(r.offsetPos.X())
	offsetY := int(r.offsetPos.Y())
	offsetZ := int(r.offsetPos.Z())

	for _, blk := range r.blocks {
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

func (r *RunAway) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any, len(posList))
	for _, pos := range posList {
		if _, exists := result[pos]; !exists {
			result[pos] = make(map[define.BlockPos]map[string]any)
		}
	}
	return result, nil
}

func (r *RunAway) CountNonAirBlocks() (int, error) {
	return r.nonAirBlocks, nil
}

func (r *RunAway) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
) error {
	return convertReaderToMCWorld(r, bedrockWorld, startSubChunkPos, startCallback, progressCallback)
}

func (r *RunAway) Close() error {
	return nil
}
