package structure

import (
	"compress/gzip"
	"fmt"
	"io"
	"math"
	"os"
	"slices"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/WaterStructure/utils/nbt"
	"github.com/Yeah114/blocks"
)

type Schematic struct {
	BaseReader
	file                *os.File
	size                *define.Size // 当前尺寸（原始尺寸+偏移扩展部分）
	originalSize        *define.Size // 原始建筑尺寸
	Origin              *define.Origin
	Offset              *define.Offset
	offsetPos           define.Offset // 建筑在新尺寸中的偏移量（相对于原始位置）
	Materials           string
	EntityNBT           []map[string]any
	BlockNBT            []map[string]any
	BlocksTagGzipOffset int64
	DataTagGzipOffset   int64
}

func (s *Schematic) ID() uint8 {
	return IDSchematic
}

func (s *Schematic) Name() string {
	return NameSchematic
}

func (s *Schematic) FromFile(file *os.File) error {
	s.file = file
	s.size = &define.Size{}
	s.originalSize = &define.Size{}
	s.Origin = &define.Origin{}
	s.Offset = &define.Offset{}
	s.Materials = "Alpha"

	gzipReader, err := gzip.NewReader(s.file)
	if err != nil {
		return fmt.Errorf("创建 gzip 读取器失败: %w", err)
	}
	defer gzipReader.Close()

	tagReader := nbt.NewTagReader(nbt.BigEndian)
	offsetReader := nbt.NewOffsetReader(gzipReader)

	rootTagType, rootTagName, err := tagReader.ReadTag(offsetReader)
	if err != nil {
		return fmt.Errorf("读取根标签失败: %w", err)
	}

	if rootTagType != nbt.TagStruct {
		return ErrInvalidRootTagType
	}

	if rootTagName != "Schematic" {
		return ErrInvalidRootTagName
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
		case "Width":
			if tagType != nbt.TagInt16 {
				return fmt.Errorf("期望 Width 为 TAG_Short, 实际为 %s", tagType)
			}
			width, err := tagReader.ReadTagInt16(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 Width 失败: %w", err)
			}
			s.size.Width = int(width)
			s.originalSize.Width = int(width)

		case "Height":
			if tagType != nbt.TagInt16 {
				return fmt.Errorf("期望 Height 为 TAG_Short, 实际为 %s", tagType)
			}
			height, err := tagReader.ReadTagInt16(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 Height 失败: %w", err)
			}
			s.size.Height = int(height)
			s.originalSize.Height = int(height)

		case "Length":
			if tagType != nbt.TagInt16 {
				return fmt.Errorf("期望 Length 为 TAG_Short, 实际为 %s", tagType)
			}
			length, err := tagReader.ReadTagInt16(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 Length 失败: %w", err)
			}
			s.size.Length = int(length)
			s.originalSize.Length = int(length)

		case "WEOriginX":
			if tagType != nbt.TagInt32 {
				return fmt.Errorf("期望 WEOriginX 为 TAG_Int, 实际为 %s", tagType)
			}
			x, err := tagReader.ReadTagInt32(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 WEOriginX 失败: %w", err)
			}
			s.Origin[0] = x

		case "WEOriginY":
			if tagType != nbt.TagInt32 {
				return fmt.Errorf("期望 WEOriginY 为 TAG_Int, 实际为 %s", tagType)
			}
			y, err := tagReader.ReadTagInt32(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 WEOriginY 失败: %w", err)
			}
			s.Origin[1] = y

		case "WEOriginZ":
			if tagType != nbt.TagInt32 {
				return fmt.Errorf("期望 WEOriginZ 为 TAG_Int, 实际为 %s", tagType)
			}
			z, err := tagReader.ReadTagInt32(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 WEOriginZ 失败: %w", err)
			}
			s.Origin[2] = z

		case "WEOffsetX":
			if tagType != nbt.TagInt32 {
				return fmt.Errorf("期望 WEOffsetX 为 TAG_Int, 实际为 %s", tagType)
			}
			x, err := tagReader.ReadTagInt32(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 WEOffsetX 失败: %w", err)
			}
			s.Offset[0] = x

		case "WEOffsetY":
			if tagType != nbt.TagInt32 {
				return fmt.Errorf("期望 WEOffsetY 为 TAG_Int, 实际为 %s", tagType)
			}
			y, err := tagReader.ReadTagInt32(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 WEOffsetY 失败: %w", err)
			}
			s.Offset[1] = y

		case "WEOffsetZ":
			if tagType != nbt.TagInt32 {
				return fmt.Errorf("期望 WEOffsetZ 为 TAG_Int, 实际为 %s", tagType)
			}
			z, err := tagReader.ReadTagInt32(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 WEOffsetZ 失败: %w", err)
			}
			s.Offset[2] = z

		case "Materials":
			if tagType != nbt.TagString {
				return fmt.Errorf("期望 Materials 为 TAG_String, 实际为 %s", tagType)
			}
			materials, err := tagReader.ReadTagString(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 Materials 失败: %w", err)
			}
			s.Materials = materials

		case "Entities":
			if tagType != nbt.TagSlice {
				return fmt.Errorf("期望 Entities 为 TAG_List, 实际为 %s", tagType)
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
			s.EntityNBT = entities

		case "TileEntities":
			if tagType != nbt.TagSlice {
				return fmt.Errorf("期望 TileEntities 为 TAG_List, 实际为 %s", tagType)
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
			s.BlockNBT = blocks

		case "Blocks":
			s.BlocksTagGzipOffset = offsetReader.GetOffset()
			err = tagReader.SkipTagValue(offsetReader, tagType)
			if err != nil {
				return fmt.Errorf("跳过标签 %s 失败: %w", tagName, err)
			}

		case "Data":
			s.DataTagGzipOffset = offsetReader.GetOffset()
			err = tagReader.SkipTagValue(offsetReader, tagType)
			if err != nil {
				return fmt.Errorf("跳过标签 %s 失败: %w", tagName, err)
			}

		default:
			err = tagReader.SkipTagValue(offsetReader, tagType)
			if err != nil {
				return fmt.Errorf("跳过标签 %s 失败: %w", tagName, err)
			}
		}
	}

	// 验证是不是真正的 Schematic 文件 查看必要数据是否获取成功
	if s.BlocksTagGzipOffset == 0 || s.DataTagGzipOffset == 0 {
		return ErrInvalidFile
	}

	return nil
}

func (s *Schematic) GetOffsetPos() define.Offset {
	return s.offsetPos
}

// SetOffsetPos 调整偏移并扩展尺寸, 使原始建筑偏移后保留, 周围填充空气
// 偏移量会使尺寸扩大, 以包含原始建筑和偏移产生的空气区域
func (s *Schematic) SetOffsetPos(offset define.Offset) {
	// 保存新的偏移位置
	s.offsetPos = offset

	// 计算需要扩展的尺寸: 原始尺寸 + 偏移量的绝对值（确保包含所有区域）
	// 例如: 原始宽16, 偏移X=16 → 新宽=16+16=32
	s.size.Width = s.originalSize.Width + int(math.Abs(float64(offset.X())))
	s.size.Length = s.originalSize.Length + int(math.Abs(float64(offset.Z())))
	s.size.Height = s.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

// GetSize 返回当前尺寸（原始尺寸+偏移扩展部分）
func (s *Schematic) GetSize() define.Size {
	return *s.size
}

func (s *Schematic) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	chunks := make(map[define.ChunkPos]*chunk.Chunk)
	// 初始化所有请求的区块为空气
	for _, pos := range posList {
		chunks[pos] = chunk.NewChunk(block.AirRuntimeID, MCWorldOverworldRange)
	}

	// 原始建筑的尺寸
	origWidth := s.originalSize.Width
	origLength := s.originalSize.Length
	origHeight := s.originalSize.Height

	// 偏移量（建筑在新尺寸中的位置）
	offsetX := int(s.offsetPos.X())
	offsetY := int(s.offsetPos.Y())
	offsetZ := int(s.offsetPos.Z())

	// 收集需要读取的原始建筑方块索引
	allIndices := []int{}
	for _, pos := range posList {
		// 计算当前区块在全局的坐标范围
		chunkMinX := int(pos.X()) * 16
		chunkMaxX := chunkMinX + 16
		chunkMinZ := int(pos.Z()) * 16
		chunkMaxZ := chunkMinZ + 16

		// 遍历区块内可能包含原始建筑的位置（考虑偏移后的位置）
		for y := 0; y < origHeight; y++ {
			// 建筑在新范围中的Y坐标 = 原始Y + 偏移Y
			newY := y + offsetY
			if newY < 0 || newY >= s.size.Height {
				continue
			}

			for z := 0; z < origLength; z++ {
				// 建筑在新范围中的Z坐标 = 原始Z + 偏移Z
				newZ := z + offsetZ
				if newZ < chunkMinZ || newZ >= chunkMaxZ {
					continue // 不在当前区块的Z范围内
				}

				for x := 0; x < origWidth; x++ {
					// 建筑在新范围中的X坐标 = 原始X + 偏移X
					newX := x + offsetX
					if newX < chunkMinX || newX >= chunkMaxX {
						continue // 不在当前区块的X范围内
					}

					// 计算原始建筑中的索引（用于读取NBT数据）
					index := (y*origLength+z)*origWidth + x
					allIndices = append(allIndices, index)
				}
			}
		}
	}

	if len(allIndices) == 0 {
		return chunks, nil // 没有需要读取的建筑方块, 返回全空气区块
	}

	// 排序索引, 优化读取效率
	slices.Sort(allIndices)

	// 读取方块数据
	file, err := os.Open(s.file.Name())
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
	datas := make(map[int]byte)

	// 处理数据标签和方块标签
	if s.DataTagGzipOffset < s.BlocksTagGzipOffset {
		// 先读Data标签
		if _, err := io.CopyN(io.Discard, offsetReader, s.DataTagGzipOffset); err != nil {
			return nil, fmt.Errorf("定位到 data 标签失败: %w", err)
		}

		lastIndex := 0
		for _, index := range allIndices {
			if _, err := io.CopyN(io.Discard, offsetReader, int64(index-lastIndex)); err != nil {
				return nil, fmt.Errorf("定位数据索引 %d 失败: %w", index, err)
			}
			b := make([]byte, 1)
			if _, err := io.ReadFull(offsetReader, b); err != nil {
				return nil, fmt.Errorf("读取索引 %d 的数据失败: %w", index, err)
			}
			datas[index] = b[0]
			lastIndex = index + 1
		}

		// 再读Blocks标签
		if _, err := io.CopyN(io.Discard, offsetReader, s.BlocksTagGzipOffset-offsetReader.GetOffset()); err != nil {
			return nil, fmt.Errorf("定位到 blocks 标签失败: %w", err)
		}

		lastIndex = 0
		for _, index := range allIndices {
			if _, err := io.CopyN(io.Discard, offsetReader, int64(index-lastIndex)); err != nil {
				return nil, fmt.Errorf("定位方块索引 %d 失败: %w", index, err)
			}
			b := make([]byte, 1)
			if _, err := io.ReadFull(offsetReader, b); err != nil {
				return nil, fmt.Errorf("读取索引 %d 的方块失败: %w", index, err)
			}

			// 从原始索引反推原始坐标
			x := index % origWidth
			remaining := index / origWidth
			z := remaining % origLength
			y := remaining / origLength

			// 计算在新范围中的坐标（原始坐标 + 偏移）
			newX := x + offsetX
			newY := y + offsetY
			newZ := z + offsetZ

			// 计算在区块内的局部坐标
			chunkX := int32(newX / 16)
			chunkZ := int32(newZ / 16)
			localX := uint8(newX % 16)
			localZ := uint8(newZ % 16)
			localY := int16(newY)

			// 获取当前区块
			c, ok := chunks[define.ChunkPos{chunkX, chunkZ}]
			if !ok {
				lastIndex = index + 1
				continue
			}

			// 转换方块ID
			blockIndex := uint8(b[0])
			dataValue := uint8(datas[index])
			runtimeID := blocks.SchematicToRuntimeID(blockIndex, dataValue)
			baseName, properties, found := blocks.RuntimeIDToState(runtimeID)
			blockRuntimeID := block.AirRuntimeID // 默认空气
			if found {
				if rtid, found := block.StateToRuntimeID("minecraft:"+baseName, properties); found {
					blockRuntimeID = rtid
				}
			}

			// 设置方块到新位置
			c.SetBlock(localX, localY - 64, localZ, 0, blockRuntimeID)
			lastIndex = index + 1
		}
	} else if s.DataTagGzipOffset > s.BlocksTagGzipOffset {
		// 先读Blocks标签
		blockIndices := make(map[int]byte)
		if _, err := io.CopyN(io.Discard, offsetReader, s.BlocksTagGzipOffset); err != nil {
			return nil, fmt.Errorf("定位到 blocks 标签失败: %w", err)
		}

		lastIndex := 0
		for _, index := range allIndices {
			if _, err := io.CopyN(io.Discard, offsetReader, int64(index-lastIndex)); err != nil {
				return nil, fmt.Errorf("定位方块索引 %d 失败: %w", index, err)
			}
			b := make([]byte, 1)
			if _, err := io.ReadFull(offsetReader, b); err != nil {
				return nil, fmt.Errorf("读取索引 %d 的方块失败: %w", index, err)
			}
			blockIndices[index] = b[0]
			lastIndex = index + 1
		}

		// 重新打开gzip读取Data标签
		gzipReader.Close()
		s.file.Seek(0, io.SeekStart)
		gzipReader, err = gzip.NewReader(s.file)
		if err != nil {
			return nil, fmt.Errorf("创建第二个 gzip 读取器失败: %w", err)
		}
		offsetReader = nbt.NewOffsetReader(gzipReader)

		if _, err := io.CopyN(io.Discard, offsetReader, s.DataTagGzipOffset); err != nil {
			return nil, fmt.Errorf("定位到 data 标签失败: %w", err)
		}

		lastIndex = 0
		for _, index := range allIndices {
			if _, err := io.CopyN(io.Discard, offsetReader, int64(index-lastIndex)); err != nil {
				return nil, fmt.Errorf("定位数据索引 %d 失败: %w", index, err)
			}
			b := make([]byte, 1)
			if _, err := io.ReadFull(offsetReader, b); err != nil {
				return nil, fmt.Errorf("读取索引 %d 的数据失败: %w", index, err)
			}
			datas[index] = b[0]
			lastIndex = index + 1
		}

		// 处理方块放置
		for _, index := range allIndices {
			// 从原始索引反推原始坐标
			x := index % origWidth
			remaining := index / origWidth
			z := remaining % origLength
			y := remaining / origLength

			// 计算在新范围中的坐标（原始坐标 + 偏移）
			newX := x + offsetX
			newY := y + offsetY
			newZ := z + offsetZ

			// 计算在区块内的局部坐标
			chunkX := int32(newX / 16)
			chunkZ := int32(newZ / 16)
			localX := uint8(newX % 16)
			localZ := uint8(newZ % 16)
			localY := int16(newY)

			// 获取当前区块
			c, ok := chunks[define.ChunkPos{chunkX, chunkZ}]
			if !ok {
				continue
			}

			// 转换方块ID
			blockIndex := uint8(blockIndices[index])
			dataValue := uint8(datas[index])
			runtimeID := blocks.SchematicToRuntimeID(blockIndex, dataValue)
			baseName, properties, found := blocks.RuntimeIDToState(runtimeID)
			blockRuntimeID := block.AirRuntimeID // 默认空气
			if found {
				if rtid, found := block.StateToRuntimeID("minecraft:"+baseName, properties); found {
					blockRuntimeID = rtid
				}
			}

			// 设置方块到新位置
			c.SetBlock(localX, localY - 64, localZ, 0, blockRuntimeID)
		}
	}

	return chunks, nil
}

// GetChunksNBT 获取指定chunk位置的NBT数据
func (s *Schematic) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	return nil, nil
}

func (s *Schematic) CountNonAirBlocks() (int, error) {
	volume := s.originalSize.GetVolume()
	nonAirBlocks := 0

	file, err := os.Open(s.file.Name())
	if err != nil {
		return 0, fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return 0, fmt.Errorf("创建 gzip 读取器失败: %w", err)
	}
	defer gzipReader.Close()

	// 直接读Block
	if _, err := io.CopyN(io.Discard, gzipReader, s.BlocksTagGzipOffset); err != nil {
		return 0, fmt.Errorf("定位到 blocks 标签失败: %w", err)
	}

	// 1MB缓冲区（1024*1024字节）
	bufSize := 1024 * 1024
	buf := make([]byte, bufSize)
	totalRead := 0

	for totalRead < volume {
		// 计算本次实际要读的字节数（避免最后一次读取超出volume）
		readLen := bufSize
		if remaining := volume - totalRead; remaining < bufSize {
			readLen = remaining
		}

		// 批量读取数据到缓冲区（仅使用前readLen字节）
		n, err := io.ReadFull(gzipReader, buf[:readLen])
		if err != nil {
			return 0, fmt.Errorf("读取方块数据失败: %w", err)
		}
		totalRead += n

		// 遍历缓冲区, 统计非0字节数
		for _, b := range buf[:n] {
			if b != 0 {
				nonAirBlocks++
			}
		}
	}

	return nonAirBlocks, nil
}

func (s *Schematic) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(num int),
	progressCallback func(),
) error {
	length := s.originalSize.GetLength()
	width := s.originalSize.GetWidth()
	height := s.originalSize.GetHeight()
	chunkCount := s.originalSize.GetChunkCount()
	totalVolume := length * width * height // 总方块数据量, 用于控制读取边界

	// 1. 初始化文件读取器
	dataTagFile, err := os.Open(s.file.Name())
	if err != nil {
		return fmt.Errorf("重新打开 data 标签文件失败: %w", err)
	}
	defer dataTagFile.Close()
	dataTagGzipReader, err := gzip.NewReader(dataTagFile)
	if err != nil {
		return fmt.Errorf("创建 data 标签 gzip 读取器失败: %w", err)
	}
	defer dataTagGzipReader.Close()

	blocksTagFile, err := os.Open(s.file.Name())
	if err != nil {
		return fmt.Errorf("重新打开 blocks 标签文件失败: %w", err)
	}
	defer blocksTagFile.Close()
	blocksTagGzipReader, err := gzip.NewReader(blocksTagFile)
	if err != nil {
		return fmt.Errorf("创建 blocks 标签 gzip 读取器失败: %w", err)
	}
	defer blocksTagGzipReader.Close()

	// 2. 定位到目标标签位置
	if _, err := io.CopyN(io.Discard, dataTagGzipReader, s.DataTagGzipOffset); err != nil {
		return fmt.Errorf("定位到 data 标签失败: %w", err)
	}
	if _, err := io.CopyN(io.Discard, blocksTagGzipReader, s.BlocksTagGzipOffset); err != nil {
		return fmt.Errorf("定位到 blocks 标签失败: %w", err)
	}

	// 3. 设置缓存（1MB）
	const bufSize = 1024 * 1024
	blockBuf := make([]byte, bufSize) // 存储blockID的缓存
	dataBuf := make([]byte, bufSize)  // 存储blockData的缓存
	bufReadOffset := 0                // 当前缓冲区已消费的字节数
	bufDataLen := 0                   // 当前缓冲区内有效数据长度
	processedBlocks := 0              // 已处理的方块数量, 用于控制读取进度

	// 4. 按子区块维度处理方块数据
	subChunkYNum := (height + 15) / 16
	if startCallback != nil {
		startCallback(subChunkYNum)
	}
	for subChunkYIndex := range subChunkYNum {
		subChunks := make([]*chunk.SubChunk, chunkCount)
		currentSubChunkHeight := min(16, height-subChunkYIndex*16) // 当前子区块实际高度（避免最后一个子区块超出总高度）

		// 遍历当前子区块的所有方块（Y→Z→X顺序, 匹配原逻辑）
		for y := range currentSubChunkHeight {
			for z := range length {
				for x := range width {
					if bufReadOffset >= bufDataLen {
						remaining := totalVolume - processedBlocks
						if remaining == 0 {
							break
						}
						readLen := min(remaining, bufSize)
						if _, err := io.ReadFull(blocksTagGzipReader, blockBuf[:readLen]); err != nil {
							return fmt.Errorf("读取 blockID 缓冲区失败: %w", err)
						}
						if _, err := io.ReadFull(dataTagGzipReader, dataBuf[:readLen]); err != nil {
							return fmt.Errorf("读取 blockData 缓冲区失败: %w", err)
						}
						bufDataLen = readLen
						bufReadOffset = 0
					}

					blockID := blockBuf[bufReadOffset]
					blockData := dataBuf[bufReadOffset]
					bufReadOffset++
					processedBlocks++

					// 跳过空气方块（0为空气ID, 无需处理）
					if blockID == 0 {
						continue
					}

					// 计算方块对应的区块、子区块位置及本地坐标
					blockRuntimeID := SchematicBlockMapping[blockID][blockData&0xF]
					chunkX := x / 16
					chunkZ := z / 16
					subChunkIndex := chunkZ*((width+15)/16) + chunkX
					localX := byte(x % 16)
					localY := byte(y % 16)
					localZ := byte(z % 16)

					// 初始化子区块（若未创建）
					if subChunks[subChunkIndex] == nil {
						subChunks[subChunkIndex] = chunk.NewSubChunk(block.AirRuntimeID)
					}
					// 设置方块到子区块
					subChunks[subChunkIndex].SetBlock(localX, localY, localZ, 0, blockRuntimeID)
				}
			}
		}

		// 5. 保存当前子区块到世界
		XChunkCount := (width + 15) / 16
		for index, subChunk := range subChunks {
			if subChunk == nil {
				continue // 跳过空的子区块, 减少无效IO
			}
			chunkX := index % XChunkCount
			chunkZ := index / XChunkCount
			subChunkPos := bwo_define.SubChunkPos{
				int32(chunkX) + startSubChunkPos.X(),
				int32(subChunkYIndex) + startSubChunkPos.Y(),
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

func (s *Schematic) FromMCWorld(
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
	startBlockPosX := startBlockPos.X()
	startBlockPosY := startBlockPos.Y()
	startBlockPosZ := startBlockPos.Z()

	endBlockPos := define.BlockPos{
		max(point1BlockPos.X(), point2BlockPos.X()),
		max(point1BlockPos.Y(), point2BlockPos.Y()),
		max(point1BlockPos.Z(), point2BlockPos.Z()),
	}
	endBlockPosX := endBlockPos.X()
	endBlockPosY := endBlockPos.Y()
	endBlockPosZ := endBlockPos.Z()

	width := endBlockPosX - startBlockPosX + 1
	height := endBlockPosY - startBlockPosY + 1
	length := endBlockPosZ - startBlockPosZ + 1
	volume := width * height * length

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
	subChunkXNum := endSubChunkPos.X() - startSubChunkPosX + 1
	subChunkYNum := endSubChunkPos.Y() - startSubChunkPosY + 1
	subChunkZNum := endSubChunkPos.Z() - startSubChunkPosZ + 1
	chunkCount := subChunkXNum * subChunkZNum
	if startCallback != nil {
		startCallback(int(subChunkYNum) * 2)
	}

	gzipWriter, err := gzip.NewWriterLevel(target, gzip.BestSpeed)
	if err != nil {
		return err
	}
	defer gzipWriter.Close()
	tagWriter := nbt.NewTagWriter(nbt.BigEndian)
	offsetWriter := nbt.NewOffsetWriter(gzipWriter)
	err = tagWriter.WriteTag(offsetWriter, nbt.TagStruct, "Schematic")
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagInt16, "Length")
	if err != nil {
		return err
	}
	err = tagWriter.WriteTagInt16(offsetWriter, int16(length))
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagInt16, "Height")
	if err != nil {
		return err
	}
	err = tagWriter.WriteTagInt16(offsetWriter, int16(height))
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagInt16, "Width")
	if err != nil {
		return err
	}
	err = tagWriter.WriteTagInt16(offsetWriter, int16(width))
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagString, "Materials")
	if err != nil {
		return err
	}
	err = tagWriter.WriteTagString(offsetWriter, "Alpha")
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagByteArray, "Blocks")
	if err != nil {
		return err
	}

	err = tagWriter.WriteTagInt32(offsetWriter, volume)
	if err != nil {
		return err
	}
	for subChunkY := range subChunkYNum {
		worldSubChunkPosY := startSubChunkPosY + subChunkY
		subChunkWorldYStart := worldSubChunkPosY * 16
		subChunkWorldYEnd := subChunkWorldYStart + 15
		effectiveWorldYStart := max(subChunkWorldYStart, startBlockPosY)
		effectiveWorldYEnd := min(subChunkWorldYEnd, endBlockPosY)
		if effectiveWorldYStart > effectiveWorldYEnd {
			if progressCallback != nil {
				progressCallback()
			}
			continue
		}
		subChunks := make(map[bwo_define.SubChunkPos]*chunk.SubChunk, chunkCount)

		for localY := byte(effectiveWorldYStart - subChunkWorldYStart); localY <= byte(effectiveWorldYEnd-subChunkWorldYStart); localY++ {
			for subChunkZ := range subChunkZNum {
				worldSubChunkPosZ := startSubChunkPosZ + subChunkZ
				subChunkWorldZStart := worldSubChunkPosZ * 16
				subChunkWorldZEnd := subChunkWorldZStart + 15
				effectiveWorldZStart := max(subChunkWorldZStart, startBlockPosZ)
				effectiveWorldZEnd := min(subChunkWorldZEnd, endBlockPosZ)
				if effectiveWorldZStart > effectiveWorldZEnd {
					continue
				}
				for localZ := byte(effectiveWorldZStart - subChunkWorldZStart); localZ <= byte(effectiveWorldZEnd-subChunkWorldZStart); localZ++ {
					for subChunkX := range subChunkXNum {
						worldSubChunkPosX := startSubChunkPosX + subChunkX
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
						for localX := byte(effectiveWorldXStart - subChunkWorldXStart); localX <= byte(effectiveWorldXEnd-subChunkWorldXStart); localX++ {
							subChunk, ok := subChunks[worldSubChunkPos]
							if !ok {
								subChunk = world.LoadSubChunk(bwo_define.DimensionIDOverworld, worldSubChunkPos)
								if subChunk == nil {
									subChunk = chunk.NewSubChunk(block.AirRuntimeID)
								}
								subChunks[worldSubChunkPos] = subChunk
							}
							blockRuntimeID := subChunk.Block(byte(localX), byte(localY), byte(localZ), 0)
							name, properties, _ := block.RuntimeIDToState(blockRuntimeID)
							runtimeID, _ := blocks.BlockNameAndStateToRuntimeID(name, properties)
							block, _, _ := blocks.RuntimeIDToSchematic(runtimeID)
							_, err := offsetWriter.Write([]byte{block})
							if err != nil {
								return err
							}
						}
					}
				}
			}
		}
		if progressCallback != nil {
			progressCallback()
		}
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagByteArray, "Data")
	if err != nil {
		return err
	}

	err = tagWriter.WriteTagInt32(offsetWriter, volume)
	if err != nil {
		return err
	}

	for subChunkY := range subChunkYNum {
		worldSubChunkPosY := startSubChunkPosY + subChunkY
		subChunkWorldYStart := worldSubChunkPosY * 16
		subChunkWorldYEnd := subChunkWorldYStart + 15
		effectiveWorldYStart := max(subChunkWorldYStart, startBlockPosY)
		effectiveWorldYEnd := min(subChunkWorldYEnd, endBlockPosY)
		if effectiveWorldYStart > effectiveWorldYEnd {
			if progressCallback != nil {
				progressCallback()
			}
			continue
		}
		subChunks := make(map[bwo_define.SubChunkPos]*chunk.SubChunk, chunkCount)

		for localY := byte(effectiveWorldYStart - subChunkWorldYStart); localY <= byte(effectiveWorldYEnd-subChunkWorldYStart); localY++ {
			for subChunkZ := range subChunkZNum {
				worldSubChunkPosZ := startSubChunkPosZ + subChunkZ
				subChunkWorldZStart := worldSubChunkPosZ * 16
				subChunkWorldZEnd := subChunkWorldZStart + 15
				effectiveWorldZStart := max(subChunkWorldZStart, startBlockPosZ)
				effectiveWorldZEnd := min(subChunkWorldZEnd, endBlockPosZ)
				if effectiveWorldZStart > effectiveWorldZEnd {
					continue
				}
				for localZ := byte(effectiveWorldZStart - subChunkWorldZStart); localZ <= byte(effectiveWorldZEnd-subChunkWorldZStart); localZ++ {
					for subChunkX := range subChunkXNum {
						worldSubChunkPosX := startSubChunkPosX + subChunkX
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
						for localX := byte(effectiveWorldXStart - subChunkWorldXStart); localX <= byte(effectiveWorldXEnd-subChunkWorldXStart); localX++ {
							subChunk, ok := subChunks[worldSubChunkPos]
							if !ok {
								subChunk = world.LoadSubChunk(bwo_define.DimensionIDOverworld, worldSubChunkPos)
								if subChunk == nil {
									subChunk = chunk.NewSubChunk(block.AirRuntimeID)
								}
								subChunks[worldSubChunkPos] = subChunk
							}
							blockRuntimeID := subChunk.Block(byte(localX), byte(localY), byte(localZ), 0)
							name, properties, _ := block.RuntimeIDToState(blockRuntimeID)
							runtimeID, _ := blocks.BlockNameAndStateToRuntimeID(name, properties)
							_, value, _ := blocks.RuntimeIDToSchematic(runtimeID)
							_, err := offsetWriter.Write([]byte{value})
							if err != nil {
								return err
							}
						}
					}
				}
			}
		}
		if progressCallback != nil {
			progressCallback()
		}
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagEnd, "")
	if err != nil {
		return err
	}
	return nil
}

func (s *Schematic) Close() error {
	return nil
}
