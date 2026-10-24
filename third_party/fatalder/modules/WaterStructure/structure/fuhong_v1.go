// //go:build goexperiment.jsonv2
package structure

import (
	"encoding/json"
	"fmt"
	"io"
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

type FuHongV1 struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset
	origin       define.Origin

	paletteCache map[string]uint32
	blocks       []fuHongV1Block

	nonAirBlocks int
}

type fuHongV1Block struct {
	LocalX    int
	LocalY    int
	LocalZ    int
	RuntimeID uint32
}

type fuHongV1Raw struct {
	Name string `json:"name"`
	Aux  any    `json:"aux"`
	X    any    `json:"x"`
	Y    any    `json:"y"`
	Z    any    `json:"z"`
}

func (f *FuHongV1) ID() uint8 {
	return IDFuHongV1
}

func (f *FuHongV1) Name() string {
	return NameFuHongV1
}

func (f *FuHongV1) FromFile(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	var rawBlocks []fuHongV1Raw
	if err := json.NewDecoder(file).Decode(&rawBlocks); err != nil {
		return fmt.Errorf("解析 FuHong V1 的 JSON 失败: %w", err)
	}

	f.file = file
	return f.populate(rawBlocks)
}

func (f *FuHongV1) populate(rawBlocks []fuHongV1Raw) error {
	f.paletteCache = make(map[string]uint32)
	f.blocks = nil
	f.nonAirBlocks = 0

	if len(rawBlocks) == 0 {
		return ErrInvalidFile
	}

	minX, minY, minZ := math.MaxInt, math.MaxInt, math.MaxInt
	maxX, maxY, maxZ := math.MinInt, math.MinInt, math.MinInt

	accum := make(map[[3]int]*fuHongV1Block)

	for idx, raw := range rawBlocks {
		name := strings.TrimSpace(raw.Name)
		if name == "" {
			return fmt.Errorf("方块 %d: 缺少名称", idx)
		}

		aux := 0
		if raw.Aux != nil {
			val, err := toInt(raw.Aux)
			if err != nil {
				return fmt.Errorf("方块 %d: aux 无效: %w", idx, err)
			}
			aux = val
		}

		x, err := firstCoordinate(raw.X)
		if err != nil {
			return fmt.Errorf("方块 %d: x 坐标: %w", idx, err)
		}
		y, err := firstCoordinate(raw.Y)
		if err != nil {
			return fmt.Errorf("方块 %d: y 坐标: %w", idx, err)
		}
		z, err := firstCoordinate(raw.Z)
		if err != nil {
			return fmt.Errorf("方块 %d: z 坐标: %w", idx, err)
		}

		runtimeID := f.runtimeIDFor(name, aux)
		key := [3]int{x, y, z}
		accum[key] = &fuHongV1Block{
			LocalX:    x,
			LocalY:    y,
			LocalZ:    z,
			RuntimeID: runtimeID,
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

	f.blocks = make([]fuHongV1Block, 0, len(accum))
	for _, key := range keys {
		rec := accum[key]
		blk := fuHongV1Block{
			LocalX:    rec.LocalX - minX,
			LocalY:    rec.LocalY - minY,
			LocalZ:    rec.LocalZ - minZ,
			RuntimeID: rec.RuntimeID,
		}
		f.blocks = append(f.blocks, blk)
		if blk.RuntimeID != block.AirRuntimeID {
			f.nonAirBlocks++
		}
	}

	if len(f.paletteCache) == 0 {
		return ErrInvalidFile
	}

	return nil
}

func firstCoordinate(value any) (int, error) {
	switch v := value.(type) {
	case []any:
		if len(v) == 0 {
			return 0, fmt.Errorf("坐标数组为空")
		}
		return toInt(v[0])
	case []int:
		if len(v) == 0 {
			return 0, fmt.Errorf("坐标数组为空")
		}
		return v[0], nil
	case []float64:
		if len(v) == 0 {
			return 0, fmt.Errorf("坐标数组为空")
		}
		return int(v[0]), nil
	default:
		return toInt(v)
	}
}

func (f *FuHongV1) runtimeIDFor(name string, aux int) uint32 {
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

func (f *FuHongV1) GetOffsetPos() define.Offset {
	return f.offsetPos
}

func (f *FuHongV1) SetOffsetPos(offset define.Offset) {
	f.offsetPos = offset
	f.size.Width = f.originalSize.Width + int(math.Abs(float64(offset.X())))
	f.size.Length = f.originalSize.Length + int(math.Abs(float64(offset.Z())))
	f.size.Height = f.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (f *FuHongV1) GetSize() define.Size {
	return *f.size
}

func (f *FuHongV1) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
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
		c.SetBlock(uint8(localX), int16(newY) - 64, uint8(localZ), 0, blk.RuntimeID)
	}

	return chunks, nil
}

func (f *FuHongV1) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any, len(posList))
	for _, pos := range posList {
		if _, exists := result[pos]; !exists {
			result[pos] = make(map[define.BlockPos]map[string]any)
		}
	}
	return result, nil
}

func (f *FuHongV1) CountNonAirBlocks() (int, error) {
	return f.nonAirBlocks, nil
}

func (f *FuHongV1) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
) error {
	return convertReaderToMCWorld(f, bedrockWorld, startSubChunkPos, startCallback, progressCallback)
}

func (f *FuHongV1) Close() error {
	return nil
}
