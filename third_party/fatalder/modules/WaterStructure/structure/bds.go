package structure

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/WaterStructure/utils"
	"github.com/vmihailenco/msgpack/v5"
)

type BDS struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset
	origin       define.Origin

	blockCount   int
	nonAirBlocks int
}

func (b *BDS) ID() uint8 {
	return IDBDS
}

func (b *BDS) Name() string {
	return NameBDS
}

func (b *BDS) FromFile(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	minX, minY, minZ := math.MaxInt, math.MaxInt, math.MaxInt
	maxX, maxY, maxZ := math.MinInt, math.MinInt, math.MinInt

	dec := msgpack.NewDecoder(file)
	topLen, err := dec.DecodeArrayLen()
	if err != nil {
		return fmt.Errorf("读取 BDS 顶层数组失败: %w", err)
	}
	if topLen < 1 {
		return ErrInvalidFile
	}

	blocksLen, err := dec.DecodeArrayLen()
	if err != nil {
		return fmt.Errorf("读取 BDS 方块列表失败: %w", err)
	}
	if blocksLen <= 0 {
		return ErrInvalidFile
	}
	b.blockCount = blocksLen

	for i := 0; i < blocksLen; i++ {
		blkLen, err := dec.DecodeArrayLen()
		if err != nil {
			return fmt.Errorf("读取 BDS 方块 %d 失败: %w", i, err)
		}
		if blkLen < 6 {
			return ErrInvalidFile
		}

		name, err := dec.DecodeString()
		if err != nil {
			return fmt.Errorf("读取 BDS 方块 %d 名称失败: %w", i, err)
		}
		x, err := decodeMsgpackInt(dec)
		if err != nil {
			return fmt.Errorf("读取 BDS 方块 %d X 失败: %w", i, err)
		}
		y, err := decodeMsgpackInt(dec)
		if err != nil {
			return fmt.Errorf("读取 BDS 方块 %d Y 失败: %w", i, err)
		}
		z, err := decodeMsgpackInt(dec)
		if err != nil {
			return fmt.Errorf("读取 BDS 方块 %d Z 失败: %w", i, err)
		}

		if _, err := dec.DecodeInterface(); err != nil {
			return fmt.Errorf("读取 BDS 方块 %d 数据失败: %w", i, err)
		}
		isAir, err := dec.DecodeBool()
		if err != nil {
			return fmt.Errorf("读取 BDS 方块 %d 空气标记失败: %w", i, err)
		}

		for extra := 6; extra < blkLen; extra++ {
			if _, err := dec.DecodeInterface(); err != nil {
				return fmt.Errorf("读取 BDS 方块 %d 扩展字段失败: %w", i, err)
			}
		}

		if isAir || strings.TrimSpace(name) == "" || strings.EqualFold(name, "minecraft:air") {
			continue
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

	if minX == math.MaxInt || minY == math.MaxInt || minZ == math.MaxInt {
		return ErrInvalidFile
	}

	width := maxX - minX + 1
	height := maxY - minY + 1
	length := maxZ - minZ + 1

	b.file = file
	b.offsetPos = define.Offset{}
	b.origin = define.Origin{int32(minX), int32(minY), int32(minZ)}
	b.size = &define.Size{Width: width, Height: height, Length: length}
	b.originalSize = &define.Size{Width: width, Height: height, Length: length}
	b.nonAirBlocks = -1

	return nil
}

func decodeMsgpackInt(dec *msgpack.Decoder) (int, error) {
	if v, err := dec.DecodeInt64(); err == nil {
		return int(v), nil
	}
	u, err := dec.DecodeUint64()
	if err != nil {
		return 0, err
	}
	return int(u), nil
}

func (b *BDS) GetOffsetPos() define.Offset {
	return b.offsetPos
}

func (b *BDS) SetOffsetPos(offset define.Offset) {
	b.offsetPos = offset
	b.size.Width = b.originalSize.Width + int(math.Abs(float64(offset.X())))
	b.size.Length = b.originalSize.Length + int(math.Abs(float64(offset.Z())))
	b.size.Height = b.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (b *BDS) GetSize() define.Size {
	return *b.size
}

func (b *BDS) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	chunks := make(map[define.ChunkPos]*chunk.Chunk, len(posList))
	for _, pos := range posList {
		if _, exists := chunks[pos]; !exists {
			chunks[pos] = chunk.NewChunk(block.AirRuntimeID, MCWorldOverworldRange)
		}
	}
	if len(chunks) == 0 {
		return chunks, nil
	}
	if b.file == nil {
		return nil, fmt.Errorf("BDS 文件未初始化")
	}

	file, err := os.Open(b.file.Name())
	if err != nil {
		return nil, fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	dec := msgpack.NewDecoder(file)
	topLen, err := dec.DecodeArrayLen()
	if err != nil || topLen < 1 {
		return nil, ErrInvalidFile
	}
	blocksLen, err := dec.DecodeArrayLen()
	if err != nil {
		return nil, ErrInvalidFile
	}

	offsetX := int(b.offsetPos.X())
	offsetY := int(b.offsetPos.Y())
	offsetZ := int(b.offsetPos.Z())
	originX := int(b.origin.X())
	originY := int(b.origin.Y())
	originZ := int(b.origin.Z())

	for i := 0; i < blocksLen; i++ {
		blkLen, err := dec.DecodeArrayLen()
		if err != nil || blkLen < 6 {
			return nil, ErrInvalidFile
		}

		name, err := dec.DecodeString()
		if err != nil {
			return nil, ErrInvalidFile
		}
		x, err := decodeMsgpackInt(dec)
		if err != nil {
			return nil, ErrInvalidFile
		}
		y, err := decodeMsgpackInt(dec)
		if err != nil {
			return nil, ErrInvalidFile
		}
		z, err := decodeMsgpackInt(dec)
		if err != nil {
			return nil, ErrInvalidFile
		}
		dataAny, err := dec.DecodeInterface()
		if err != nil {
			return nil, ErrInvalidFile
		}
		isAir, err := dec.DecodeBool()
		if err != nil {
			return nil, ErrInvalidFile
		}
		for extra := 6; extra < blkLen; extra++ {
			if _, err := dec.DecodeInterface(); err != nil {
				return nil, ErrInvalidFile
			}
		}

		if isAir || strings.TrimSpace(name) == "" || strings.EqualFold(name, "minecraft:air") {
			continue
		}

		runtimeID := runtimeIDFromBDS(name, dataAny)

		localX := x - originX
		localY := y - originY
		localZ := z - originZ

		newX := localX + offsetX
		newY := localY + offsetY
		newZ := localZ + offsetZ

		chunkX := floorDiv(newX, 16)
		chunkZ := floorDiv(newZ, 16)
		chunkPos := define.ChunkPos{int32(chunkX), int32(chunkZ)}
		c, ok := chunks[chunkPos]
		if !ok {
			continue
		}

		localXInChunk := newX - chunkX*16
		localZInChunk := newZ - chunkZ*16
		c.SetBlock(uint8(localXInChunk), int16(newY)-64, uint8(localZInChunk), 0, runtimeID)
	}

	return chunks, nil
}

func runtimeIDFromBDS(name string, data any) uint32 {
	switch v := data.(type) {
	case int:
		return legacyBlockToBedrockRuntimeID(name, uint16(v))
	case int8:
		return legacyBlockToBedrockRuntimeID(name, uint16(v))
	case int16:
		return legacyBlockToBedrockRuntimeID(name, uint16(v))
	case int32:
		return legacyBlockToBedrockRuntimeID(name, uint16(v))
	case int64:
		return legacyBlockToBedrockRuntimeID(name, uint16(v))
	case uint:
		return legacyBlockToBedrockRuntimeID(name, uint16(v))
	case uint8:
		return legacyBlockToBedrockRuntimeID(name, uint16(v))
	case uint16:
		return legacyBlockToBedrockRuntimeID(name, v)
	case uint32:
		return legacyBlockToBedrockRuntimeID(name, uint16(v))
	case uint64:
		return legacyBlockToBedrockRuntimeID(name, uint16(v))
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return runtimeIDForBlock(name, nil)
		}
		if i, err := strconv.Atoi(s); err == nil {
			return legacyBlockToBedrockRuntimeID(name, uint16(i))
		}
		if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
			if states, err := parseMCFunctionStates(s); err == nil {
				return runtimeIDForBlock(name, states)
			}
		}
		return runtimeIDForBlock(name, nil)
	default:
		return runtimeIDForBlock(name, nil)
	}
}

func (b *BDS) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any, len(posList))
	for _, pos := range posList {
		if _, exists := result[pos]; !exists {
			result[pos] = make(map[define.BlockPos]map[string]any)
		}
	}
	return result, nil
}

func (b *BDS) CountNonAirBlocks() (int, error) {
	if b.nonAirBlocks >= 0 {
		return b.nonAirBlocks, nil
	}
	if b.file == nil {
		return 0, fmt.Errorf("BDS 文件未初始化")
	}

	file, err := os.Open(b.file.Name())
	if err != nil {
		return 0, fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	dec := msgpack.NewDecoder(file)
	topLen, err := dec.DecodeArrayLen()
	if err != nil || topLen < 1 {
		return 0, ErrInvalidFile
	}
	blocksLen, err := dec.DecodeArrayLen()
	if err != nil {
		return 0, ErrInvalidFile
	}

	nonAirBlocks := 0
	for i := 0; i < blocksLen; i++ {
		blkLen, err := dec.DecodeArrayLen()
		if err != nil || blkLen < 6 {
			return 0, ErrInvalidFile
		}
		name, err := dec.DecodeString()
		if err != nil {
			return 0, ErrInvalidFile
		}
		for j := 0; j < 4; j++ {
			if _, err := dec.DecodeInterface(); err != nil {
				return 0, ErrInvalidFile
			}
		}
		isAir, err := dec.DecodeBool()
		if err != nil {
			return 0, ErrInvalidFile
		}
		for extra := 6; extra < blkLen; extra++ {
			if _, err := dec.DecodeInterface(); err != nil {
				return 0, ErrInvalidFile
			}
		}
		if isAir || strings.TrimSpace(name) == "" || strings.EqualFold(name, "minecraft:air") {
			continue
		}
		nonAirBlocks++
	}

	b.nonAirBlocks = nonAirBlocks
	return nonAirBlocks, nil
}

func (b *BDS) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
) error {
	if bedrockWorld == nil {
		return fmt.Errorf("bedrock 世界为 nil")
	}
	if b.file == nil {
		return fmt.Errorf("BDS 文件未初始化")
	}

	startX := startSubChunkPos.X() * 16
	startY := startSubChunkPos.Y() * 16
	startZ := startSubChunkPos.Z() * 16

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	mcworld, err := utils.NewMCWorld(bedrockWorld, ctx)
	if err != nil {
		return err
	}
	mcworld.AutoFlush(time.Second)

	totalProgress := 100
	if startCallback != nil {
		startCallback(totalProgress)
	}

	file, err := os.Open(b.file.Name())
	if err != nil {
		return fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	dec := msgpack.NewDecoder(file)
	topLen, err := dec.DecodeArrayLen()
	if err != nil || topLen < 1 {
		return ErrInvalidFile
	}
	blocksLen, err := dec.DecodeArrayLen()
	if err != nil {
		return ErrInvalidFile
	}

	offsetX := int(b.offsetPos.X())
	offsetY := int(b.offsetPos.Y())
	offsetZ := int(b.offsetPos.Z())
	originX := int(b.origin.X())
	originY := int(b.origin.Y())
	originZ := int(b.origin.Z())

	totalItems := blocksLen
	if totalItems <= 0 {
		totalItems = 1
	}
	currentItem := 0
	lastReportedProgress := -1

	for i := 0; i < blocksLen; i++ {
		blkLen, err := dec.DecodeArrayLen()
		if err != nil || blkLen < 6 {
			return ErrInvalidFile
		}

		name, err := dec.DecodeString()
		if err != nil {
			return ErrInvalidFile
		}
		x, err := decodeMsgpackInt(dec)
		if err != nil {
			return ErrInvalidFile
		}
		y, err := decodeMsgpackInt(dec)
		if err != nil {
			return ErrInvalidFile
		}
		z, err := decodeMsgpackInt(dec)
		if err != nil {
			return ErrInvalidFile
		}
		dataAny, err := dec.DecodeInterface()
		if err != nil {
			return ErrInvalidFile
		}
		isAir, err := dec.DecodeBool()
		if err != nil {
			return ErrInvalidFile
		}
		for extra := 6; extra < blkLen; extra++ {
			if _, err := dec.DecodeInterface(); err != nil {
				return ErrInvalidFile
			}
		}

		if !isAir && strings.TrimSpace(name) != "" && !strings.EqualFold(name, "minecraft:air") {
			runtimeID := runtimeIDFromBDS(name, dataAny)

			localX := x - originX
			localY := y - originY
			localZ := z - originZ

			worldX := localX + offsetX
			worldY := localY + offsetY
			worldZ := localZ + offsetZ

			ax := startX + int32(worldX)
			ay := int16(int(startY) + worldY)
			az := startZ + int32(worldZ)
			if err := mcworld.SetBlock(ax, ay, az, runtimeID); err != nil {
				return err
			}
		}

		currentItem++
		currentProgress := (currentItem * totalProgress) / totalItems
		if progressCallback != nil && currentProgress > lastReportedProgress {
			for j := lastReportedProgress + 1; j <= currentProgress; j++ {
				progressCallback()
			}
			lastReportedProgress = currentProgress
		}
	}

	mcworld.Flush()
	if progressCallback != nil && lastReportedProgress < totalProgress {
		for j := lastReportedProgress + 1; j <= totalProgress; j++ {
			progressCallback()
		}
	}

	return nil
}

func (b *BDS) Close() error {
	return nil
}
