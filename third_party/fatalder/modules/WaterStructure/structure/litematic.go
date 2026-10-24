package structure

import (
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/WaterStructure/utils/nbt"
	"github.com/Yeah114/blocks"
)

type Litematic struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset

	Version     int32
	DataVersion int32
	SubVersion  int32
	Metadata    map[string]any
	Origin      define.Origin
	Size        define.Size
	EntityNBT   []map[string]any
	BlockNBT    []map[string]any

	palette           map[int32]uint32
	blockStatesOffset int64 // BlockStates 在 gzip 流中的偏移位置
}

func (l *Litematic) ID() uint8 {
	return IDLitematic
}

func (l *Litematic) Name() string {
	return NameLitematic
}

func (l *Litematic) FromFile(file *os.File) error {
	l.file = file
	l.size = &define.Size{}
	l.originalSize = &define.Size{}
	l.offsetPos = define.Offset{}
	l.palette = make(map[int32]uint32)

	gzipReader, err := gzip.NewReader(l.file)
	if err != nil {
		return fmt.Errorf("创建 gzip 读取器失败: %w", err)
	}
	defer gzipReader.Close()

	tagReader := nbt.NewTagReader(nbt.BigEndian)
	offsetReader := nbt.NewOffsetReader(gzipReader)

	rootTagType, _, err := tagReader.ReadTag(offsetReader)
	if err != nil {
		return fmt.Errorf("读取根标签失败: %w", err)
	}

	if rootTagType != nbt.TagStruct {
		return ErrInvalidRootTagType
	}

	for {
		tagType, tagName, err := tagReader.ReadTag(offsetReader)
		if err != nil {
			return fmt.Errorf("读取标签失败: %w", err)
		}

		if tagType == nbt.TagEnd {
			break
		}

		switch tagName {
		case "Version":
			if tagType != nbt.TagInt32 {
				return fmt.Errorf("期望 Version 为 TAG_Int, 实际为 %s", tagType)
			}
			l.Version, err = tagReader.ReadTagInt32(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 Version 失败: %w", err)
			}

		case "MinecraftDataVersion":
			if tagType != nbt.TagInt32 {
				return fmt.Errorf("期望 MinecraftDataVersion 为 TAG_Int, 实际为 %s", tagType)
			}
			l.DataVersion, err = tagReader.ReadTagInt32(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 MinecraftDataVersion 失败: %w", err)
			}

		case "SubVersion":
			if tagType != nbt.TagInt32 {
				return fmt.Errorf("期望 SubVersion 为 TAG_Int, 实际为 %s", tagType)
			}
			l.SubVersion, err = tagReader.ReadTagInt32(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 SubVersion 失败: %w", err)
			}

		case "Metadata":
			if tagType != nbt.TagStruct {
				return fmt.Errorf("期望 Metadata 为 TAG_Compound, 实际为 %s", tagType)
			}

			l.Metadata, err = tagReader.ReadTagCompound(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 Metadata 失败: %w", err)
			}

		case "Regions":
			if tagType != nbt.TagStruct {
				return fmt.Errorf("期望 Regions 为 TAG_Compound, 实际为 %s", tagType)
			}

			// 只取第1个Region
			regionsTagType, _, err := tagReader.ReadTag(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 Region 标签失败: %w", err)
			}

			// 没有Region
			if regionsTagType == nbt.TagEnd {
				return fmt.Errorf("未找到任何 Region")
			}

			if regionsTagType != nbt.TagStruct {
				return fmt.Errorf("期望 Region 为 TAG_Compound, 实际为 %s", regionsTagType)
			}

			// 读取Region
			for {
				regionTagType, regionTagName, err := tagReader.ReadTag(offsetReader)
				if err != nil {
					return fmt.Errorf("读取 Region 标签失败: %w", err)
				}

				if regionTagType == nbt.TagEnd {
					break
				}

				switch regionTagName {
				case "Position":
					if regionTagType != nbt.TagStruct {
						return fmt.Errorf("期望 Position 为 TAG_Compound, 实际为 %s", regionTagType)
					}

					position, err := tagReader.ReadTagCompound(offsetReader)
					if err != nil {
						return fmt.Errorf("读取 Position 标签失败: %w", err)
					}

					l.Origin[0] = position["x"].(int32)
					l.Origin[1] = position["y"].(int32)
					l.Origin[2] = position["z"].(int32)

				case "Size":
					if regionTagType != nbt.TagStruct {
						return fmt.Errorf("期望 Size 为 TAG_Compound, 实际为 %s", regionTagType)
					}

					size, err := tagReader.ReadTagCompound(offsetReader)
					if err != nil {
						return fmt.Errorf("读取 Size 标签失败: %w", err)
					}

					width := int(size["x"].(int32))
					height := int(size["y"].(int32))
					length := int(size["z"].(int32))
					l.Size.Width = width
					l.Size.Length = length
					l.Size.Height = height
					width = int(math.Abs(float64(width)))
					height = int(math.Abs(float64(height)))
					length = int(math.Abs(float64(length)))
					l.size.Width = width
					l.size.Length = length
					l.size.Height = height
					l.originalSize.Width = width
					l.originalSize.Length = length
					l.originalSize.Height = height

				case "BlockStatePalette":
					if regionTagType != nbt.TagSlice {
						return fmt.Errorf("期望 BlockStatePalette 为 TAG_List, 实际为 %s", regionTagType)
					}
					blockStatePalette, err := tagReader.ReadTagList(offsetReader)
					if err != nil {
						return fmt.Errorf("读取 BlockStatePalette 失败: %w", err)
					}
					for i, blockState := range blockStatePalette {
						index := int32(i)
						b, ok := blockState.(map[string]any)
						if !ok {
							return fmt.Errorf("期望 blockState 为 map[string]any, 实际为 %T", blockState)
						}
						name, ok := b["Name"].(string)
						if !ok {
							return fmt.Errorf("期望 Name 为 string, 实际为 %T", b["Name"])
						}
						properties := make(map[string]any)
						if len(b) == 2 {
							properties = b["Properties"].(map[string]any)
						}
						runtimeID, found := blocks.BlockNameAndStateToRuntimeID(name, properties)
						if !found {
							l.palette[index] = UnknownBlockRuntimeID
							continue
						}
						baseName, properties, found := blocks.RuntimeIDToState(runtimeID)
						if !found {
							l.palette[index] = UnknownBlockRuntimeID
							continue
						}
						blockRuntimeID, found := block.StateToRuntimeID(baseName, properties)
						if !found {
							l.palette[index] = UnknownBlockRuntimeID
							continue
						}
						l.palette[index] = blockRuntimeID
					}

				case "Entities":
					if regionTagType != nbt.TagSlice {
						return fmt.Errorf("期望 Entities 为 TAG_List, 实际为 %s", regionTagType)
					}
					entityNBT, err := tagReader.ReadTagList(offsetReader)
					if err != nil {
						return fmt.Errorf("读取 Entities 失败: %w", err)
					}
					entities := make([]map[string]any, len(entityNBT))
					for i, entity := range entityNBT {
						if entityMap, ok := entity.(map[string]any); ok {
							entities[i] = entityMap
						} else {
							return fmt.Errorf("期望 entity 为 map[string]any, 实际为 %T", entity)
						}
					}
					l.EntityNBT = entities

				case "TileEntities":
					if regionTagType != nbt.TagSlice {
						return fmt.Errorf("期望 TileEntities 为 TAG_List, 实际为 %s", regionTagType)
					}
					blockNBT, err := tagReader.ReadTagList(offsetReader)
					if err != nil {
						return fmt.Errorf("读取 TileEntities 失败: %w", err)
					}
					blocks := make([]map[string]any, len(blockNBT))
					for i, block := range blockNBT {
						if blockMap, ok := block.(map[string]any); ok {
							blocks[i] = blockMap
						} else {
							return fmt.Errorf("期望 block 为 map[string]any, 实际为 %T", block)
						}
					}
					l.BlockNBT = blocks

				case "BlockStates":
					// 只记录 BlockStates 的偏移位置, 不读取数据
					if regionTagType != nbt.TagInt64Array {
						return fmt.Errorf("期望 BlockStates 为 TAG_LongArray, 实际为 %s", regionTagType)
					}
					l.blockStatesOffset = offsetReader.GetOffset()
					// 跳过 BlockStates 数据
					err = tagReader.SkipTagValue(offsetReader, regionTagType)
					if err != nil {
						return fmt.Errorf("跳过 BlockStates 失败: %w", err)
					}

				default:
					err = tagReader.SkipTagValue(offsetReader, regionTagType)
					if err != nil {
						return fmt.Errorf("跳过标签 %s 失败: %w", regionTagName, err)
					}
				}
			}

			// 跳过其他Region
			for {
				regionTagType, regionTagName, err := tagReader.ReadTag(offsetReader)
				if err != nil {
					return fmt.Errorf("读取 Region 标签失败: %w", err)
				}

				if regionTagType == nbt.TagEnd {
					break
				}

				err = tagReader.SkipTagValue(offsetReader, regionTagType)
				if err != nil {
					return fmt.Errorf("跳过标签 %s 失败: %w", regionTagName, err)
				}
			}

		default:
			err = tagReader.SkipTagValue(offsetReader, tagType)
			if err != nil {
				return fmt.Errorf("跳过标签 %s 失败: %w", tagName, err)
			}
		}
	}

	// 验证是不是真正的 Litematic 文件 查看必要数据是否获取成功
	if l.blockStatesOffset == 0 {
		return ErrInvalidFile
	}

	return nil
}

func (l *Litematic) GetPalette() map[int32]uint32 {
	return l.palette
}

func (l *Litematic) GetOffsetPos() define.Offset {
	return l.offsetPos
}

func (l *Litematic) SetOffsetPos(offset define.Offset) {
	l.offsetPos = offset
	l.size.Width = l.originalSize.Width + int(math.Abs(float64(offset.X())))
	l.size.Length = l.originalSize.Length + int(math.Abs(float64(offset.Z())))
	l.size.Height = l.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (l *Litematic) GetSize() define.Size {
	return *l.size
}

func (l *Litematic) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	chunks := make(map[define.ChunkPos]*chunk.Chunk)
	// 初始化所有请求的区块为空气
	for _, pos := range posList {
		chunks[pos] = chunk.NewChunk(block.AirRuntimeID, MCWorldOverworldRange)
	}

	// 如果没有记录 BlockStates 的偏移位置, 返回空区块
	if l.blockStatesOffset == 0 {
		return chunks, nil
	}

	// 重新打开文件进行流式读取
	file, err := os.Open(l.file.Name())
	if err != nil {
		return nil, fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("创建 gzip 读取器失败: %w", err)
	}
	defer gzipReader.Close()

	offsetReader := nbt.NewOffsetReader(gzipReader)

	// 跳过到 BlockStates 位置
	currentOffset := offsetReader.GetOffset()
	if currentOffset < l.blockStatesOffset {
		skipBytes := l.blockStatesOffset - currentOffset
		_, err = io.CopyN(io.Discard, offsetReader, skipBytes)
		if err != nil {
			return nil, fmt.Errorf("定位到 BlockStates 失败: %w", err)
		}
	}

	// 读取 LongArray 的长度（大端 int32）, 不把所有 long 读入内存
	var lenBuf [4]byte
	if _, err := io.ReadFull(offsetReader, lenBuf[:]); err != nil {
		return nil, fmt.Errorf("读取 BlockStates 长度失败: %w", err)
	}
	numLongs := int(int32(binary.BigEndian.Uint32(lenBuf[:])))
	if numLongs < 0 {
		return nil, fmt.Errorf("BlockStates 长度无效: %d", numLongs)
	}

	// 流式解析 BlockStates 数据（只读 numLongs 个 int64）
	if err := l.parseBlockStatesStreamFromReader(offsetReader, numLongs, chunks, posList); err != nil {
		return nil, fmt.Errorf("解析 BlockStates 失败: %w", err)
	}

	return chunks, nil
}

// streamingLSBBitReader 按大端读取 uint64, 并以 LSB 优先方式提供逐值位段
type streamingLSBBitReader struct {
	r        io.Reader
	remain   int // 剩余可读的 long 数
	curr     uint64
	bitsLeft uint // curr 中尚未消费的位数
}

func newStreamingLSBBitReader(r io.Reader, numLongs int) (*streamingLSBBitReader, error) {
	br := &streamingLSBBitReader{r: r, remain: numLongs}
	return br, nil
}

func (br *streamingLSBBitReader) readLong() error {
	if br.remain <= 0 {
		br.curr = 0
		br.bitsLeft = 0
		return io.EOF
	}
	var b [8]byte
	if _, err := io.ReadFull(br.r, b[:]); err != nil {
		return err
	}
	br.curr = binary.BigEndian.Uint64(b[:])
	br.bitsLeft = 64
	br.remain--
	return nil
}

func (br *streamingLSBBitReader) next(n uint) (uint64, error) {
	if n == 0 {
		return 0, nil
	}
	var val uint64
	var have uint
	for have < n {
		if br.bitsLeft == 0 {
			if err := br.readLong(); err != nil {
				return 0, err
			}
		}
		need := n - have
		if br.bitsLeft >= need {
			mask := (uint64(1) << need) - 1
			chunk := br.curr & mask
			val |= chunk << have
			br.curr >>= need
			br.bitsLeft -= need
			have += need
		} else {
			// consume all remaining bits
			mask := (uint64(1) << br.bitsLeft) - 1
			chunk := br.curr & mask
			val |= chunk << have
			have += br.bitsLeft
			br.curr = 0
			br.bitsLeft = 0
		}
	}
	return val, nil
}

// parseBlockStatesStreamFromReader 使用流式位读取器解析不加载全部 long 的 BlockStates
func (l *Litematic) parseBlockStatesStreamFromReader(r io.Reader, numLongs int, chunks map[define.ChunkPos]*chunk.Chunk, posList []define.ChunkPos) error {
	paletteSize := len(l.palette)
	if paletteSize == 0 {
		return fmt.Errorf("调色板为空")
	}
	absWidth := int(math.Abs(float64(l.Size.Width)))
	absHeight := int(math.Abs(float64(l.Size.Height)))
	absLength := int(math.Abs(float64(l.Size.Length)))
	numBlocks := absWidth * absHeight * absLength

	// 偏移量（外部期望的建筑放置偏移）
	offsetX := int(l.offsetPos.X())
	offsetY := int(l.offsetPos.Y())
	offsetZ := int(l.offsetPos.Z())

	// bitsPerBlock
	bitsPerBlock := int(math.Ceil(math.Log2(float64(paletteSize))))
	if bitsPerBlock < 2 {
		bitsPerBlock = 2
	}

	br, err := newStreamingLSBBitReader(r, numLongs)
	if err != nil {
		return err
	}

	requested := make(map[define.ChunkPos]bool)
	for _, p := range posList {
		requested[p] = true
	}

	layerSize := absWidth * absLength
	paletteLen := paletteSize

	for blockIndex := 0; blockIndex < numBlocks; blockIndex++ {
		// ind -> (x,y,z)
		y := blockIndex / layerSize
		rem := blockIndex % layerSize
		z := rem / absWidth
		x := rem % absWidth

		pi, err := br.next(uint(bitsPerBlock))
		if err != nil {
			break
		}
		if int(pi) >= paletteLen {
			continue
		}
		rtid, ok := l.palette[int32(pi)]
		if !ok || rtid == block.AirRuntimeID {
			continue
		}

		// 应用偏移后确定目标区块与局部坐标
		wx := x + offsetX
		wy := y + offsetY
		wz := z + offsetZ

		chunkX := int32(wx >> 4)
		chunkZ := int32(wz >> 4)
		cp := define.ChunkPos{chunkX, chunkZ}
		if !requested[cp] {
			continue
		}
		c := chunks[cp]
		c.SetBlock(uint8(wx%16), int16(wy)-64, uint8(wz%16), 0, rtid)
	}
	return nil
}

// LitematicaBitArray Litematic 位数组实现
type LitematicaBitArray struct {
	data         []uint64
	size         int
	bitsPerEntry int
	mask         uint64
}

// Get 从位数组中获取指定索引的值
func (ba *LitematicaBitArray) Get(index int) uint64 {
	if index < 0 || index >= ba.size {
		return 0
	}

	startOffset := index * ba.bitsPerEntry
	startArrayIndex := startOffset >> 6 // 除以64
	endArrayIndex := ((index+1)*ba.bitsPerEntry - 1) >> 6
	startBitOffset := uint(startOffset & 0x3F) // 模64

	if startArrayIndex == endArrayIndex {
		// 数据在同一个long中（逻辑位移）
		return (ba.data[startArrayIndex] >> startBitOffset) & ba.mask
	} else {
		// 数据跨越两个long
		endOffset := 64 - startBitOffset
		val := (ba.data[startArrayIndex] >> startBitOffset) | (ba.data[endArrayIndex] << endOffset)
		return val & ba.mask
	}
}

func (l *Litematic) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any)

	// 初始化结果映射
	for _, pos := range posList {
		result[pos] = make(map[define.BlockPos]map[string]any)
	}

	// 偏移量
	offsetX := int(l.offsetPos.X())
	offsetY := int(l.offsetPos.Y())
	offsetZ := int(l.offsetPos.Z())

	// 遍历所有 BlockNBT
	for _, blockNBT := range l.BlockNBT {
		// 获取方块位置
		x, xOK := blockNBT["x"].(int32)
		y, yOK := blockNBT["y"].(int32)
		z, zOK := blockNBT["z"].(int32)

		if !xOK || !yOK || !zOK {
			continue
		}

		// 应用偏移
		worldX := int(x) + offsetX
		worldY := int(y) + offsetY
		worldZ := int(z) + offsetZ

		// 计算区块坐标
		chunkX := int32(worldX / 16)
		chunkZ := int32(worldZ / 16)
		chunkPos := define.ChunkPos{chunkX, chunkZ}

		// 检查是否在请求的区块列表中
		if _, exists := result[chunkPos]; !exists {
			continue
		}

		// 使用区块内相对坐标
		localX := int32(worldX - int(chunkX)*16)
		localZ := int32(worldZ - int(chunkZ)*16)
		blockPos := define.BlockPos{localX, chunkLocalYFromWorld(worldY), localZ}
		result[chunkPos][blockPos] = blockNBT
	}

	return result, nil
}

func (l *Litematic) CountNonAirBlocks() (int, error) {
	volume := l.originalSize.GetVolume()
	airIndex := int32(0)
	found := false
	for k, v := range l.palette {
		if v == block.AirRuntimeID {
			found = true
			airIndex = k
		}
	}
	if !found {
		return volume, nil
	}

	// 重新打开文件进行流式读取
	file, err := os.Open(l.file.Name())
	if err != nil {
		return 0, fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return 0, fmt.Errorf("创建 gzip 读取器失败: %w", err)
	}
	defer gzipReader.Close()

	// 跳过到 BlockStates 位置
	_, err = io.CopyN(io.Discard, gzipReader, l.blockStatesOffset)
	if err != nil {
		return 0, fmt.Errorf("定位到 BlockStates 失败: %w", err)
	}

	// 读取 LongArray 的长度（大端 int32）, 不把所有 long 读入内存
	var lenBuf [4]byte
	if _, err := io.ReadFull(gzipReader, lenBuf[:]); err != nil {
		return 0, fmt.Errorf("读取 BlockStates 长度失败: %w", err)
	}
	numLongs := int(int32(binary.BigEndian.Uint32(lenBuf[:])))
	if numLongs < 0 {
		return 0, fmt.Errorf("BlockStates 长度无效: %d", numLongs)
	}

	// bitsPerBlock
	bitsPerBlock := int(math.Ceil(math.Log2(float64(len(l.palette)))))
	if bitsPerBlock < 2 {
		bitsPerBlock = 2
	}

	br, err := newStreamingLSBBitReader(gzipReader, numLongs)
	if err != nil {
		return 0, err
	}

	nonAirBlocks := 0
	for range volume {
		blockIndex, err := br.next(uint(bitsPerBlock))
		if err != nil {
			return 0, fmt.Errorf("读取方块索引失败: %w", err)
		}

		if int32(blockIndex) == airIndex {
			continue
		}

		nonAirBlocks++
	}

	return nonAirBlocks, nil
}

func (l *Litematic) FromMCWorld(
	world *world.BedrockWorld,
	target *os.File,
	point1BlockPos define.BlockPos,
	point2BlockPos define.BlockPos,
	startCallback func(int),
	progressCallback func(),
) error {
	startBlockPos := define.BlockPos{
		min(point1BlockPos.X(), point2BlockPos.X()),
		min(point1BlockPos.Y(), point2BlockPos.Y()),
		min(point1BlockPos.Z(), point2BlockPos.Z()),
	}
	endBlockPos := define.BlockPos{
		max(point1BlockPos.X(), point2BlockPos.X()),
		max(point1BlockPos.Y(), point2BlockPos.Y()),
		max(point1BlockPos.Z(), point2BlockPos.Z()),
	}

	startBlockPosX := startBlockPos.X()
	startBlockPosY := startBlockPos.Y()
	startBlockPosZ := startBlockPos.Z()
	endBlockPosX := endBlockPos.X()
	endBlockPosY := endBlockPos.Y()
	endBlockPosZ := endBlockPos.Z()

	width := int(endBlockPosX-startBlockPosX) + 1
	height := int(endBlockPosY-startBlockPosY) + 1
	length := int(endBlockPosZ-startBlockPosZ) + 1
	if width <= 0 || height <= 0 || length <= 0 {
		return fmt.Errorf("区域尺寸无效")
	}

	startSubChunkPos := define.SubChunkPos{
		(startBlockPosX - mod(startBlockPosX, 16)) / 16,
		(startBlockPosY - mod(startBlockPosY, 16)) / 16,
		(startBlockPosZ - mod(startBlockPosZ, 16)) / 16,
	}
	endSubChunkPos := define.SubChunkPos{
		(endBlockPosX + mod(endBlockPosX, 16) + 15) / 16,
		(endBlockPosY + mod(endBlockPosY, 16) + 15) / 16,
		(endBlockPosZ + mod(endBlockPosZ, 16) + 15) / 16,
	}
	startSubChunkPosX := startSubChunkPos.X()
	startSubChunkPosY := startSubChunkPos.Y()
	startSubChunkPosZ := startSubChunkPos.Z()
	endSubChunkPosX := endSubChunkPos.X()
	endSubChunkPosY := endSubChunkPos.Y()
	endSubChunkPosZ := endSubChunkPos.Z()

	subChunkXNum := int(endSubChunkPosX-startSubChunkPosX) + 1
	subChunkYNum := int(endSubChunkPosY-startSubChunkPosY) + 1
	subChunkZNum := int(endSubChunkPosZ-startSubChunkPosZ) + 1
	chunkCount := subChunkXNum * subChunkZNum
	if startCallback != nil {
		startCallback(subChunkYNum)
	}

	numBlocks := width * height * length
	palette := map[uint32]int32{
		block.AirRuntimeID: 0,
	}
	paletteOrder := []uint32{block.AirRuntimeID}

	iterator := newLitematicRegionIterator(
		world,
		startBlockPos,
		endBlockPos,
		startSubChunkPos,
		subChunkXNum,
		subChunkYNum,
		subChunkZNum,
		chunkCount,
	)

	// 第一次遍历: 仅建立调色板
	err := iterator.forEach(nil, func(runtimeID uint32) error {
		if _, exists := palette[runtimeID]; exists {
			return nil
		}
		index := int32(len(paletteOrder))
		palette[runtimeID] = index
		paletteOrder = append(paletteOrder, runtimeID)
		return nil
	})
	if err != nil {
		return err
	}

	paletteSize := len(paletteOrder)
	bitsPerBlock := int(math.Ceil(math.Log2(float64(paletteSize))))
	if bitsPerBlock < 2 {
		bitsPerBlock = 2
	}
	numLongs := (numBlocks*bitsPerBlock + 63) / 64

	gzipWriter, err := gzip.NewWriterLevel(target, gzip.BestSpeed)
	if err != nil {
		return fmt.Errorf("创建 gzip 写入器失败: %w", err)
	}
	defer gzipWriter.Close()
	tagWriter := nbt.NewTagWriter(nbt.BigEndian)
	offsetWriter := nbt.NewOffsetWriter(gzipWriter)

	if err := tagWriter.WriteTag(offsetWriter, nbt.TagStruct, "Litematic"); err != nil {
		return err
	}
	if err := tagWriter.WriteTag(offsetWriter, nbt.TagInt32, "Version"); err != nil {
		return err
	}
	if err := tagWriter.WriteTagInt32(offsetWriter, 6); err != nil {
		return err
	}
	if err := tagWriter.WriteTag(offsetWriter, nbt.TagInt32, "MinecraftDataVersion"); err != nil {
		return err
	}
	if err := tagWriter.WriteTagInt32(offsetWriter, JavaDataVersion); err != nil {
		return err
	}
	if err := tagWriter.WriteTag(offsetWriter, nbt.TagInt32, "SubVersion"); err != nil {
		return err
	}
	if err := tagWriter.WriteTagInt32(offsetWriter, 1); err != nil {
		return err
	}

	metadata := map[string]any{
		"Name":        "WaterStructure Export",
		"Author":      "WaterStructure",
		"Description": fmt.Sprintf("(%d,%d,%d) -> (%d,%d,%d)", startBlockPosX, startBlockPosY, startBlockPosZ, endBlockPosX, endBlockPosY, endBlockPosZ),
		"RegionCount": int32(1),
		"TotalVolume": int64(numBlocks),
		"EnclosingSize": map[string]any{
			"x": int32(width),
			"y": int32(height),
			"z": int32(length),
		},
	}
	if err := tagWriter.WriteTag(offsetWriter, nbt.TagStruct, "Metadata"); err != nil {
		return err
	}
	if err := tagWriter.WriteTagCompound(offsetWriter, metadata); err != nil {
		return err
	}

	if err := tagWriter.WriteTag(offsetWriter, nbt.TagStruct, "Regions"); err != nil {
		return err
	}
	if err := tagWriter.WriteTag(offsetWriter, nbt.TagStruct, "region"); err != nil {
		return err
	}

	position := map[string]any{
		"x": int32(0),
		"y": int32(0),
		"z": int32(0),
	}
	if err := tagWriter.WriteTag(offsetWriter, nbt.TagStruct, "Position"); err != nil {
		return err
	}
	if err := tagWriter.WriteTagCompound(offsetWriter, position); err != nil {
		return err
	}

	size := map[string]any{
		"x": int32(width),
		"y": int32(height),
		"z": int32(length),
	}
	if err := tagWriter.WriteTag(offsetWriter, nbt.TagStruct, "Size"); err != nil {
		return err
	}
	if err := tagWriter.WriteTagCompound(offsetWriter, size); err != nil {
		return err
	}

	paletteList := make([]interface{}, len(paletteOrder))
	for i, runtimeID := range paletteOrder {
		name, properties, _ := block.RuntimeIDToState(runtimeID)
		javaBlockName, javaBlockProperties, found := blocks.BedrockBlockNameAndStateToJavaBlock(name, properties)
		if !found {
			javaBlockName = "air"
		}
		javaBlockName = "minecraft:" + javaBlockName
		entry := map[string]any{
			"Name": javaBlockName,
		}
		if len(properties) > 0 {
			entry["Properties"] = javaBlockProperties
		}
		paletteList[i] = entry
	}
	if err := tagWriter.WriteTag(offsetWriter, nbt.TagSlice, "BlockStatePalette"); err != nil {
		return err
	}
	if err := tagWriter.WriteTagList(offsetWriter, paletteList); err != nil {
		return err
	}

	if err := tagWriter.WriteTag(offsetWriter, nbt.TagInt64Array, "BlockStates"); err != nil {
		return err
	}
	if err := nbt.BigEndian.WriteInt32(offsetWriter, int32(numLongs)); err != nil {
		return err
	}
	bitWriter := newLitematicBlockStateWriter(bitsPerBlock, func(value uint64) error {
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], value)
		_, err := offsetWriter.Write(buf[:])
		return err
	})
	layerProgress := func() {
		if progressCallback != nil {
			progressCallback()
		}
	}
	err = iterator.forEach(layerProgress, func(runtimeID uint32) error {
		index, ok := palette[runtimeID]
		if !ok {
			index = palette[block.AirRuntimeID]
		}
		return bitWriter.WriteIndex(index)
	})
	if err != nil {
		return err
	}
	if err := bitWriter.Finish(numLongs); err != nil {
		return err
	}

	if err := tagWriter.WriteTag(offsetWriter, nbt.TagSlice, "Entities"); err != nil {
		return err
	}
	if err := tagWriter.WriteTagList(offsetWriter, []interface{}{}); err != nil {
		return err
	}

	if err := tagWriter.WriteTag(offsetWriter, nbt.TagSlice, "TileEntities"); err != nil {
		return err
	}
	if err := tagWriter.WriteTagList(offsetWriter, []interface{}{}); err != nil {
		return err
	}

	if err := tagWriter.WriteTag(offsetWriter, nbt.TagEnd, ""); err != nil {
		return err
	}
	if err := tagWriter.WriteTag(offsetWriter, nbt.TagEnd, ""); err != nil {
		return err
	}
	if err := tagWriter.WriteTag(offsetWriter, nbt.TagEnd, ""); err != nil {
		return err
	}
	return nil
}

type litematicRegionIterator struct {
	world            *world.BedrockWorld
	startBlockPos    define.BlockPos
	endBlockPos      define.BlockPos
	startSubChunkPos define.SubChunkPos
	subChunkXNum     int
	subChunkYNum     int
	subChunkZNum     int
	chunkCount       int
}

func newLitematicRegionIterator(
	world *world.BedrockWorld,
	startBlockPos define.BlockPos,
	endBlockPos define.BlockPos,
	startSubChunkPos define.SubChunkPos,
	subChunkXNum int,
	subChunkYNum int,
	subChunkZNum int,
	chunkCount int,
) *litematicRegionIterator {
	return &litematicRegionIterator{
		world:            world,
		startBlockPos:    startBlockPos,
		endBlockPos:      endBlockPos,
		startSubChunkPos: startSubChunkPos,
		subChunkXNum:     subChunkXNum,
		subChunkYNum:     subChunkYNum,
		subChunkZNum:     subChunkZNum,
		chunkCount:       chunkCount,
	}
}

func (it *litematicRegionIterator) forEach(layerDone func(), process func(uint32) error) error {
	startBlockPosX := it.startBlockPos.X()
	startBlockPosY := it.startBlockPos.Y()
	startBlockPosZ := it.startBlockPos.Z()
	endBlockPosX := it.endBlockPos.X()
	endBlockPosY := it.endBlockPos.Y()
	endBlockPosZ := it.endBlockPos.Z()
	startSubChunkPosX := it.startSubChunkPos.X()
	startSubChunkPosY := it.startSubChunkPos.Y()
	startSubChunkPosZ := it.startSubChunkPos.Z()

	for subChunkY := 0; subChunkY < it.subChunkYNum; subChunkY++ {
		worldSubChunkPosY := startSubChunkPosY + int32(subChunkY)
		subChunkWorldYStart := worldSubChunkPosY * 16
		subChunkWorldYEnd := subChunkWorldYStart + 15
		effectiveWorldYStart := max(subChunkWorldYStart, startBlockPosY)
		effectiveWorldYEnd := min(subChunkWorldYEnd, endBlockPosY)
		if effectiveWorldYStart > effectiveWorldYEnd {
			if layerDone != nil {
				layerDone()
			}
			continue
		}
		subChunks := make(map[bwo_define.SubChunkPos]*chunk.SubChunk, it.chunkCount)
		for localY := byte(effectiveWorldYStart - subChunkWorldYStart); localY <= byte(effectiveWorldYEnd-subChunkWorldYStart); localY++ {
			for subChunkZ := 0; subChunkZ < it.subChunkZNum; subChunkZ++ {
				worldSubChunkPosZ := startSubChunkPosZ + int32(subChunkZ)
				subChunkWorldZStart := worldSubChunkPosZ * 16
				subChunkWorldZEnd := subChunkWorldZStart + 15
				effectiveWorldZStart := max(subChunkWorldZStart, startBlockPosZ)
				effectiveWorldZEnd := min(subChunkWorldZEnd, endBlockPosZ)
				if effectiveWorldZStart > effectiveWorldZEnd {
					continue
				}
				for localZ := byte(effectiveWorldZStart - subChunkWorldZStart); localZ <= byte(effectiveWorldZEnd-subChunkWorldZStart); localZ++ {
					for subChunkX := 0; subChunkX < it.subChunkXNum; subChunkX++ {
						worldSubChunkPosX := startSubChunkPosX + int32(subChunkX)
						subChunkWorldXStart := worldSubChunkPosX * 16
						subChunkWorldXEnd := subChunkWorldXStart + 15
						effectiveWorldXStart := max(subChunkWorldXStart, startBlockPosX)
						effectiveWorldXEnd := min(subChunkWorldXEnd, endBlockPosX)
						if effectiveWorldXStart > effectiveWorldXEnd {
							continue
						}
						worldSubChunkPos := bwo_define.SubChunkPos{
							worldSubChunkPosX,
							worldSubChunkPosY,
							worldSubChunkPosZ,
						}
						subChunk, ok := subChunks[worldSubChunkPos]
						if !ok {
							subChunk = it.world.LoadSubChunk(bwo_define.DimensionIDOverworld, worldSubChunkPos)
							if subChunk == nil {
								subChunk = chunk.NewSubChunk(block.AirRuntimeID)
							}
							subChunks[worldSubChunkPos] = subChunk
						}
						for localX := byte(effectiveWorldXStart - subChunkWorldXStart); localX <= byte(effectiveWorldXEnd-subChunkWorldXStart); localX++ {
							blockRuntimeID := subChunk.Block(localX, localY, localZ, 0)
							if err := process(blockRuntimeID); err != nil {
								return err
							}
						}
					}
				}
			}
		}
		if layerDone != nil {
			layerDone()
		}
	}
	return nil
}

type litematicBlockStateWriter struct {
	bitsPerBlock uint
	mask         uint64
	current      uint64
	bitsFilled   uint
	longCount    int
	writeLong    func(uint64) error
}

func newLitematicBlockStateWriter(bitsPerBlock int, writeLong func(uint64) error) *litematicBlockStateWriter {
	if bitsPerBlock < 1 {
		bitsPerBlock = 1
	}
	return &litematicBlockStateWriter{
		bitsPerBlock: uint(bitsPerBlock),
		mask:         (uint64(1) << bitsPerBlock) - 1,
		writeLong:    writeLong,
	}
}

func (w *litematicBlockStateWriter) WriteIndex(index int32) error {
	value := uint64(index) & w.mask
	remaining := w.bitsPerBlock
	for remaining > 0 {
		available := 64 - w.bitsFilled
		if available == 0 {
			if err := w.flush(); err != nil {
				return err
			}
			available = 64
		}
		if remaining <= available {
			chunk := value & ((uint64(1) << remaining) - 1)
			w.current |= chunk << w.bitsFilled
			w.bitsFilled += remaining
			remaining = 0
			if w.bitsFilled == 64 {
				if err := w.flush(); err != nil {
					return err
				}
			}
		} else {
			chunk := value & ((uint64(1) << available) - 1)
			w.current |= chunk << w.bitsFilled
			value >>= available
			remaining -= available
			if err := w.flush(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *litematicBlockStateWriter) flush() error {
	if w.bitsFilled == 0 && w.current == 0 {
		return nil
	}
	if err := w.writeLong(w.current); err != nil {
		return err
	}
	w.longCount++
	w.current = 0
	w.bitsFilled = 0
	return nil
}

func (w *litematicBlockStateWriter) Finish(expectedLongs int) error {
	if w.bitsFilled > 0 {
		if err := w.flush(); err != nil {
			return err
		}
	}
	if w.longCount != expectedLongs {
		return fmt.Errorf("BlockStates 长度不匹配: 期望 %d, 实际 %d", expectedLongs, w.longCount)
	}
	return nil
}

func (l *Litematic) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(num int),
	progressCallback func(),
) error {
	width := l.originalSize.GetWidth()
	length := l.originalSize.GetLength()
	height := l.originalSize.GetHeight()
	chunkCount := l.originalSize.GetChunkCount()
	totalVolume := width * length * height

	if totalVolume == 0 {
		if startCallback != nil {
			startCallback(0)
		}
		return nil
	}

	file, err := os.Open(l.file.Name())
	if err != nil {
		return fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("创建 gzip 读取器失败: %w", err)
	}
	defer gzipReader.Close()

	// 跳过到 BlockStates 位置
	_, err = io.CopyN(io.Discard, gzipReader, l.blockStatesOffset)
	if err != nil {
		return fmt.Errorf("定位到 BlockStates 失败: %w", err)
	}

	// 读取 LongArray 的长度（大端 int32）, 不把所有 long 读入内存
	var lenBuf [4]byte
	if _, err := io.ReadFull(gzipReader, lenBuf[:]); err != nil {
		return fmt.Errorf("读取 BlockStates 长度失败: %w", err)
	}
	numLongs := int(int32(binary.BigEndian.Uint32(lenBuf[:])))
	if numLongs < 0 {
		return fmt.Errorf("BlockStates 长度无效: %d", numLongs)
	}

	// bitsPerBlock
	bitsPerBlock := int(math.Ceil(math.Log2(float64(len(l.palette)))))
	if bitsPerBlock < 2 {
		bitsPerBlock = 2
	}

	br, err := newStreamingLSBBitReader(gzipReader, numLongs)
	if err != nil {
		return err
	}

	subChunkYNum := (height + 15) / 16
	chunkXNum := (width + 15) / 16
	if startCallback != nil {
		startCallback(subChunkYNum)
	}

	for subChunkY := range subChunkYNum {
		subChunks := make([]*chunk.SubChunk, chunkCount)
		currentSubChunkHeight := min(16, height-subChunkY*16)

		for localY := range currentSubChunkHeight {
			for z := range length {
				for x := range width {
					blockIndex, err := br.next(uint(bitsPerBlock))
					if err != nil {
						return fmt.Errorf("读取方块索引失败: %w", err)
					}

					runtimeID, ok := l.palette[int32(blockIndex)]
					if runtimeID == block.AirRuntimeID {
						continue
					}
					if !ok {
						runtimeID = UnknownBlockRuntimeID
					}

					chunkX := x / 16
					chunkZ := z / 16
					subChunkIndex := chunkZ*chunkXNum + chunkX
					localX := byte(x % 16)
					localZ := byte(z % 16)

					if subChunks[subChunkIndex] == nil {
						subChunks[subChunkIndex] = chunk.NewSubChunk(block.AirRuntimeID)
					}
					subChunks[subChunkIndex].SetBlock(localX, byte(localY), localZ, 0, runtimeID)
				}
			}
		}

		for index, subChunk := range subChunks {
			if subChunk == nil {
				continue
			}
			chunkX := index % chunkXNum
			chunkZ := index / chunkXNum
			subChunkPos := bwo_define.SubChunkPos{
				int32(chunkX) + startSubChunkPos.X(),
				int32(subChunkY) + startSubChunkPos.Y(),
				int32(chunkZ) + startSubChunkPos.Z(),
			}
			if err := bedrockWorld.SaveSubChunk(bwo_define.DimensionIDOverworld, subChunkPos, subChunk); err != nil {
				return fmt.Errorf("保存子区块 %v 失败: %w", subChunkPos, err)
			}
		}
		if progressCallback != nil {
			go progressCallback()
		}
	}
	return nil
}

func (l *Litematic) Close() error {
	return nil
}
