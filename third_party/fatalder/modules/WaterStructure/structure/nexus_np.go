package structure

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/WaterStructure/utils"
	"github.com/vmihailenco/msgpack/v5"
)

// NexusNP format (np.py):
// msgpack([block_data, block_actor_data])
// block_data: [[name, x, y, z, data], ...]
// block_actor_data: unknown, usually []
type NexusNP struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset
	origin       define.Origin

	nonAirBlocks int
}

func (n *NexusNP) ID() uint8 {
	return IDNexusNP
}

func (n *NexusNP) Name() string {
	return NameNexusNP
}

func (n *NexusNP) FromFile(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	dec := msgpack.NewDecoder(file)
	topLen, err := dec.DecodeArrayLen()
	if err != nil {
		return fmt.Errorf("读取 NP 顶层数组失败: %w", err)
	}
	if topLen < 2 {
		return ErrInvalidFile
	}

	blocksLen, err := dec.DecodeArrayLen()
	if err != nil {
		return fmt.Errorf("读取 NP block_data 失败: %w", err)
	}
	if blocksLen <= 0 {
		return ErrInvalidFile
	}

	minX, minY, minZ := math.MaxInt, math.MaxInt, math.MaxInt
	maxX, maxY, maxZ := math.MinInt, math.MinInt, math.MinInt

	for i := 0; i < blocksLen; i++ {
		blkLen, err := dec.DecodeArrayLen()
		if err != nil {
			return fmt.Errorf("读取 NP 方块 %d 失败: %w", i, err)
		}
		if blkLen < 5 {
			return ErrInvalidFile
		}
		name, err := dec.DecodeString()
		if err != nil {
			return fmt.Errorf("读取 NP 方块 %d 名称失败: %w", i, err)
		}
		x, err := decodeMsgpackInt(dec)
		if err != nil {
			return fmt.Errorf("读取 NP 方块 %d X 失败: %w", i, err)
		}
		y, err := decodeMsgpackInt(dec)
		if err != nil {
			return fmt.Errorf("读取 NP 方块 %d Y 失败: %w", i, err)
		}
		z, err := decodeMsgpackInt(dec)
		if err != nil {
			return fmt.Errorf("读取 NP 方块 %d Z 失败: %w", i, err)
		}
		if _, err := decodeMsgpackInt(dec); err != nil { // data
			return fmt.Errorf("读取 NP 方块 %d data 失败: %w", i, err)
		}
		for extra := 5; extra < blkLen; extra++ {
			if _, err := dec.DecodeInterface(); err != nil {
				return fmt.Errorf("读取 NP 方块 %d 扩展字段失败: %w", i, err)
			}
		}

		if strings.TrimSpace(name) == "" || strings.EqualFold(name, "minecraft:air") {
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

	// skip block_actor_data
	if _, err := dec.DecodeInterface(); err != nil {
		return fmt.Errorf("读取 NP block_actor_data 失败: %w", err)
	}

	if minX == math.MaxInt || minY == math.MaxInt || minZ == math.MaxInt {
		return ErrInvalidFile
	}

	width := maxX - minX + 1
	height := maxY - minY + 1
	length := maxZ - minZ + 1

	n.file = file
	n.offsetPos = define.Offset{}
	n.origin = define.Origin{int32(minX), int32(minY), int32(minZ)}
	n.size = &define.Size{Width: width, Height: height, Length: length}
	n.originalSize = &define.Size{Width: width, Height: height, Length: length}
	n.nonAirBlocks = -1
	return nil
}

func (n *NexusNP) GetOffsetPos() define.Offset {
	return n.offsetPos
}

func (n *NexusNP) SetOffsetPos(offset define.Offset) {
	n.offsetPos = offset
	n.size.Width = n.originalSize.Width + int(math.Abs(float64(offset.X())))
	n.size.Length = n.originalSize.Length + int(math.Abs(float64(offset.Z())))
	n.size.Height = n.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (n *NexusNP) GetSize() define.Size {
	return *n.size
}

func (n *NexusNP) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	chunks := make(map[define.ChunkPos]*chunk.Chunk, len(posList))
	for _, pos := range posList {
		if _, exists := chunks[pos]; !exists {
			chunks[pos] = chunk.NewChunk(block.AirRuntimeID, MCWorldOverworldRange)
		}
	}
	if len(chunks) == 0 {
		return chunks, nil
	}
	if n.file == nil {
		return nil, fmt.Errorf("NexusNP 文件未初始化")
	}

	file, err := os.Open(n.file.Name())
	if err != nil {
		return nil, fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	dec := msgpack.NewDecoder(file)
	topLen, err := dec.DecodeArrayLen()
	if err != nil || topLen < 2 {
		return nil, ErrInvalidFile
	}
	blocksLen, err := dec.DecodeArrayLen()
	if err != nil {
		return nil, ErrInvalidFile
	}

	offsetX := int(n.offsetPos.X())
	offsetY := int(n.offsetPos.Y())
	offsetZ := int(n.offsetPos.Z())
	originX := int(n.origin.X())
	originY := int(n.origin.Y())
	originZ := int(n.origin.Z())

	for i := 0; i < blocksLen; i++ {
		blkLen, err := dec.DecodeArrayLen()
		if err != nil || blkLen < 5 {
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
		data, err := decodeMsgpackInt(dec)
		if err != nil {
			return nil, ErrInvalidFile
		}
		for extra := 5; extra < blkLen; extra++ {
			if _, err := dec.DecodeInterface(); err != nil {
				return nil, ErrInvalidFile
			}
		}

		if strings.TrimSpace(name) == "" || strings.EqualFold(name, "minecraft:air") {
			continue
		}

		runtimeID := legacyBlockToBedrockRuntimeID(name, uint16(data))
		localX := x - originX
		localY := y - originY
		localZ := z - originZ

		newX := localX + offsetX
		newY := localY + offsetY
		newZ := localZ + offsetZ

		chunkX := floorDiv(newX, 16)
		chunkZ := floorDiv(newZ, 16)
		chunkPos := define.ChunkPos{int32(chunkX), int32(chunkZ)}
		target, ok := chunks[chunkPos]
		if !ok {
			continue
		}
		localXInChunk := newX - chunkX*16
		localZInChunk := newZ - chunkZ*16
		target.SetBlock(uint8(localXInChunk), int16(newY)-64, uint8(localZInChunk), 0, runtimeID)
	}

	// skip block_actor_data
	_, _ = dec.DecodeInterface()

	return chunks, nil
}

func (n *NexusNP) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any, len(posList))
	for _, pos := range posList {
		if _, exists := result[pos]; !exists {
			result[pos] = make(map[define.BlockPos]map[string]any)
		}
	}
	return result, nil
}

func (n *NexusNP) CountNonAirBlocks() (int, error) {
	if n.nonAirBlocks >= 0 {
		return n.nonAirBlocks, nil
	}
	if n.file == nil {
		return 0, fmt.Errorf("NexusNP 文件未初始化")
	}

	file, err := os.Open(n.file.Name())
	if err != nil {
		return 0, fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	dec := msgpack.NewDecoder(file)
	topLen, err := dec.DecodeArrayLen()
	if err != nil || topLen < 2 {
		return 0, ErrInvalidFile
	}
	blocksLen, err := dec.DecodeArrayLen()
	if err != nil {
		return 0, ErrInvalidFile
	}

	nonAir := 0
	for i := 0; i < blocksLen; i++ {
		blkLen, err := dec.DecodeArrayLen()
		if err != nil || blkLen < 5 {
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
		for extra := 5; extra < blkLen; extra++ {
			if _, err := dec.DecodeInterface(); err != nil {
				return 0, ErrInvalidFile
			}
		}
		if strings.TrimSpace(name) == "" || strings.EqualFold(name, "minecraft:air") {
			continue
		}
		nonAir++
	}

	n.nonAirBlocks = nonAir
	return nonAir, nil
}

func (n *NexusNP) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
) error {
	if bedrockWorld == nil {
		return fmt.Errorf("bedrock 世界为 nil")
	}
	if n.file == nil {
		return fmt.Errorf("NexusNP 文件未初始化")
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

	file, err := os.Open(n.file.Name())
	if err != nil {
		return fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	dec := msgpack.NewDecoder(file)
	topLen, err := dec.DecodeArrayLen()
	if err != nil || topLen < 2 {
		return ErrInvalidFile
	}
	blocksLen, err := dec.DecodeArrayLen()
	if err != nil {
		return ErrInvalidFile
	}

	offsetX := int(n.offsetPos.X())
	offsetY := int(n.offsetPos.Y())
	offsetZ := int(n.offsetPos.Z())
	originX := int(n.origin.X())
	originY := int(n.origin.Y())
	originZ := int(n.origin.Z())

	totalItems := blocksLen
	if totalItems <= 0 {
		totalItems = 1
	}
	currentItem := 0
	lastReportedProgress := -1

	for i := 0; i < blocksLen; i++ {
		blkLen, err := dec.DecodeArrayLen()
		if err != nil || blkLen < 5 {
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
		data, err := decodeMsgpackInt(dec)
		if err != nil {
			return ErrInvalidFile
		}
		for extra := 5; extra < blkLen; extra++ {
			if _, err := dec.DecodeInterface(); err != nil {
				return ErrInvalidFile
			}
		}

		if strings.TrimSpace(name) != "" && !strings.EqualFold(name, "minecraft:air") {
			runtimeID := legacyBlockToBedrockRuntimeID(name, uint16(data))
			localX := x - originX
			localY := y - originY
			localZ := z - originZ

			wx := localX + offsetX
			wy := localY + offsetY
			wz := localZ + offsetZ

			ax := startX + int32(wx)
			ay := int16(int(startY) + wy)
			az := startZ + int32(wz)
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

	// skip block_actor_data
	_, _ = dec.DecodeInterface()

	mcworld.Flush()
	if progressCallback != nil && lastReportedProgress < totalProgress {
		for j := lastReportedProgress + 1; j <= totalProgress; j++ {
			progressCallback()
		}
	}
	return nil
}

func (n *NexusNP) Close() error {
	return nil
}
