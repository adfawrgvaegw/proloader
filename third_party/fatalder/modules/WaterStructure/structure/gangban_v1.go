package structure

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/blocks"
)

type GangBanV1 struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset
	origin       define.Origin

	namespaces   []string
	blocks       []gangBanBlock
	paletteCache map[paletteCacheKey]uint32

	nonAirBlocks int
}

type gangBanBlock struct {
	LocalX    int
	LocalY    int
	LocalZ    int
	RuntimeID uint32
	NBT       map[string]any
}

type rawGangBanRange struct {
	Start []int `json:"start"`
	End   []int `json:"end"`
}

type rawGangBanPalette struct {
	List []string `json:"list"`
}

type rawGangBanBlock struct {
	ID       int                     `json:"id"`
	Aux      *int                    `json:"aux,omitempty"`
	Position []int                   `json:"p"`
	Cmds     *rawGangBanCommandBlock `json:"cmds,omitempty"`
}

type rawGangBanCommandBlock struct {
	Mode      string `json:"mode"`
	Auto      bool   `json:"auto"`
	Condition bool   `json:"condition"`
	Cmd       string `json:"cmd"`
	Last      string `json:"last"`
	Name      string `json:"name"`
	Tick      int    `json:"tick"`
	Should    bool   `json:"should"`
	On        bool   `json:"on"`
}

func (g *GangBanV1) ID() uint8 {
	return IDGangBanV1
}

func (g *GangBanV1) Name() string {
	return NameGangBanV1
}

func (g *GangBanV1) FromFile(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	decoder := json.NewDecoder(file)
	decoder.UseNumber()

	var entries []json.RawMessage
	if err := decoder.Decode(&entries); err != nil {
		return fmt.Errorf("解析 GangBan V1 的 JSON 失败: %w", err)
	}
	if len(entries) < 2 {
		return ErrInvalidFile
	}

	var palette rawGangBanPalette
	if err := json.Unmarshal(entries[len(entries)-1], &palette); err != nil {
		return fmt.Errorf("解析调色板失败: %w", err)
	}
	if len(palette.List) == 0 {
		return ErrInvalidFile
	}

	var rng rawGangBanRange
	if err := json.Unmarshal(entries[len(entries)-2], &rng); err != nil {
		return fmt.Errorf("解析范围失败: %w", err)
	}
	if len(rng.Start) != 3 || len(rng.End) != 3 {
		return ErrInvalidFile
	}

	g.file = file
	return g.populateFromComponents(entries[:len(entries)-2], rng, palette)
}

func (g *GangBanV1) populateFromComponents(blockEntries []json.RawMessage, rng rawGangBanRange, palette rawGangBanPalette) error {
	originX := minInt(rng.Start[0], rng.End[0])
	originY := minInt(rng.Start[1], rng.End[1])
	originZ := minInt(rng.Start[2], rng.End[2])
	endX := maxInt(rng.Start[0], rng.End[0])
	endY := maxInt(rng.Start[1], rng.End[1])
	endZ := maxInt(rng.Start[2], rng.End[2])

	width := endX - originX + 1
	height := endY - originY + 1
	length := endZ - originZ + 1

	if width <= 0 || height <= 0 || length <= 0 {
		return ErrInvalidFile
	}

	g.size = &define.Size{Width: width, Height: height, Length: length}
	g.originalSize = &define.Size{Width: width, Height: height, Length: length}
	g.offsetPos = define.Offset{}
	g.origin = define.Origin{int32(originX), int32(originY), int32(originZ)}
	g.namespaces = palette.List
	g.paletteCache = make(map[paletteCacheKey]uint32)
	g.blocks = make([]gangBanBlock, 0, len(blockEntries))
	g.nonAirBlocks = 0

	for _, rawBlock := range blockEntries {
		var entry rawGangBanBlock
		if err := json.Unmarshal(rawBlock, &entry); err != nil {
			return fmt.Errorf("解析方块条目失败: %w", err)
		}

		if entry.ID < 0 || entry.ID >= len(g.namespaces) {
			return fmt.Errorf("方块调色板索引 %d 越界", entry.ID)
		}

		aux := 0
		if entry.Aux != nil {
			aux = *entry.Aux
		}
		if aux < 0 || aux > math.MaxUint16 {
			return fmt.Errorf("方块 aux %d 超出 uint16 范围", aux)
		}

		if len(entry.Position) != 3 {
			return fmt.Errorf("方块 %d 的位置长度无效", entry.ID)
		}

		worldX := entry.Position[0]
		worldY := entry.Position[1]
		worldZ := entry.Position[2]

		localX := worldX - originX
		localY := worldY - originY
		localZ := worldZ - originZ
		if localX < 0 || localY < 0 || localZ < 0 || localX >= width || localY >= height || localZ >= length {
			return fmt.Errorf("方块位置 [%d,%d,%d] 超出声明的范围", worldX, worldY, worldZ)
		}

		runtimeID := g.runtimeIDFor(entry.ID, aux)

		var nbt map[string]any
		if entry.Cmds != nil {
			nbt = buildGangBanCommandNBT(entry.Cmds)
		}

		g.blocks = append(g.blocks, gangBanBlock{
			LocalX:    localX,
			LocalY:    localY,
			LocalZ:    localZ,
			RuntimeID: runtimeID,
			NBT:       nbt,
		})

		if runtimeID != block.AirRuntimeID {
			g.nonAirBlocks++
		}
	}

	// 检查是不是这个文件
	if len(g.namespaces) == 0 {
		return ErrInvalidFile
	}

	return nil
}

func (g *GangBanV1) runtimeIDFor(index int, aux int) uint32 {
	if g.paletteCache == nil {
		g.paletteCache = make(map[paletteCacheKey]uint32)
	}
	key := paletteCacheKey{Index: index, Data: uint16(aux)}
	if runtimeID, ok := g.paletteCache[key]; ok {
		return runtimeID
	}

	name := g.namespaces[index]
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

func (g *GangBanV1) GetOffsetPos() define.Offset {
	return g.offsetPos
}

func (g *GangBanV1) SetOffsetPos(offset define.Offset) {
	g.offsetPos = offset
	g.size.Width = g.originalSize.Width + int(math.Abs(float64(offset.X())))
	g.size.Length = g.originalSize.Length + int(math.Abs(float64(offset.Z())))
	g.size.Height = g.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (g *GangBanV1) GetSize() define.Size {
	return *g.size
}

func (g *GangBanV1) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
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

func (g *GangBanV1) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
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

func (g *GangBanV1) CountNonAirBlocks() (int, error) {
	return g.nonAirBlocks, nil
}

func (g *GangBanV1) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
) error {
	return convertReaderToMCWorld(g, bedrockWorld, startSubChunkPos, startCallback, progressCallback)
}

func (g *GangBanV1) Close() error {
	return nil
}

func buildGangBanCommandNBT(data *rawGangBanCommandBlock) map[string]any {
	if data == nil {
		return nil
	}

	nbt := make(map[string]any)
	nbt["id"] = "CommandBlock"
	nbt["Command"] = data.Cmd
	nbt["CustomName"] = data.Name
	nbt["LastOutput"] = data.Last
	nbt["TickDelay"] = int32(data.Tick)
	nbt["ExecuteOnFirstTick"] = boolToByte(data.Auto)
	nbt["TrackOutput"] = boolToByte(data.Should)
	nbt["conditionalMode"] = boolToByte(data.Condition)
	nbt["auto"] = boolToByte(data.Auto)
	nbt["Powered"] = boolToByte(data.On)

	if mode, ok := gangBanModeValue(data.Mode); ok {
		nbt["LPCommandMode"] = int32(mode)
	}

	return nbt
}

func gangBanModeValue(mode string) (int32, bool) {
	switch strings.ToLower(mode) {
	case "tick", "impulse", "normal":
		return 0, true
	case "repeating", "repeat":
		return 1, true
	case "chain":
		return 2, true
	default:
		return 0, false
	}
}

func boolToByte(v bool) byte {
	if v {
		return 1
	}
	return 0
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
