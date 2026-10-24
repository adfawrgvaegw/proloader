package structure

import (
	"bytes"
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
	"github.com/Yeah114/WaterStructure/utils"
	"github.com/Yeah114/WaterStructure/utils/nbt"
	"github.com/Yeah114/blocks"
)

type SchemV2 struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size

	DataVersion int32
	Version     int32
	Metadata    map[string]any
	Offset      *define.Offset
	BlockNBT    []map[string]any

	palette           map[int32]uint32
	offsetPos         define.Offset
	dataTagGzipOffset int64
}

func (s *SchemV2) FromFile(file *os.File) (err error) {
	s.file = file
	s.size = &define.Size{}
	s.originalSize = &define.Size{}
	s.Offset = &define.Offset{}
	s.palette = make(map[int32]uint32)

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

	if rootTagName == "" {
		rootTagType, rootTagName, err = tagReader.ReadTag(offsetReader)
		if err != nil {
			return fmt.Errorf("读取根标签失败: %w", err)
		}
		if rootTagType != nbt.TagStruct {
			return ErrInvalidRootTagType
		}
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
		case "DataVersion":
			if tagType != nbt.TagInt32 {
				return fmt.Errorf("期望 DataVersion 为 TAG_Int, 实际为 %s", tagType)
			}
			s.DataVersion, err = tagReader.ReadTagInt32(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 DataVersion 失败: %w", err)
			}

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

		case "Version":
			if tagType != nbt.TagInt32 {
				return fmt.Errorf("期望 Version 为 TAG_Int, 实际为 %s", tagType)
			}
			s.Version, err = tagReader.ReadTagInt32(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 Version 失败: %w", err)
			}

		case "Metadata":
			if tagType != nbt.TagStruct {
				return fmt.Errorf("期望 Metadata 为 TAG_Compound, 实际为 %s", tagType)
			}
			metadata, err := tagReader.ReadTagCompound(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 Metadata 失败: %w", err)
			}
			s.Metadata = metadata

		case "Offset":
			if tagType != nbt.TagInt32Array {
				return fmt.Errorf("期望 Offset 为 TAG_Int32Array, 实际为 %s", tagType)
			}
			offset, err := tagReader.ReadTagInt32Array(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 Offset 失败: %w", err)
			}
			s.Offset[0] = offset[0]
			s.Offset[1] = offset[1]
			s.Offset[2] = offset[2]

		case "Blocks":
			if tagType != nbt.TagStruct {
				return fmt.Errorf("期望 Palette 为 TAG_Compound, 实际为 %s", tagType)
			}
			for {
				subTagType, subTagName, err := tagReader.ReadTag(offsetReader)
				if err != nil {
					return fmt.Errorf("读取 Blocks 标签失败: %w", err)
				}

				if subTagType == nbt.TagEnd {
					break
				}

				switch subTagName {
				case "Palette":
					if subTagType != nbt.TagStruct {
						return fmt.Errorf("期望 Palette 为 TAG_Compound, 实际为 %s", subTagType)
					}
					palette, err := tagReader.ReadTagCompound(offsetReader)
					if err != nil {
						return fmt.Errorf("读取 Palette 失败: %w", err)
					}
					for schemStateStr, i := range palette {
						index := i.(int32)
						s.palette[index] = UnknownBlockRuntimeID
						runtimeID, found := blocks.BlockStrToRuntimeID(schemStateStr)
						if !found {
							continue
						}
						baseName, properties, found := blocks.RuntimeIDToState(runtimeID)
						if !found {
							continue
						}
						blockRuntimeID, found := block.StateToRuntimeID("minecraft:"+baseName, properties)
						if !found {
							continue
						}
						s.palette[index] = blockRuntimeID
					}

				case "BlockEntities":
					if subTagType != nbt.TagSlice {
						return fmt.Errorf("期望 BlockEntities 为 TAG_List, 实际为 %s", subTagType)
					}
					blockEntities, err := tagReader.ReadTagList(offsetReader)
					if err != nil {
						return fmt.Errorf("读取 BlockEntities 失败: %w", err)
					}
					s.BlockNBT = make([]map[string]any, len(blockEntities))
					for i, blockEntity := range blockEntities {
						if blockEntityMap, ok := blockEntity.(map[string]any); ok {
							s.BlockNBT[i] = blockEntityMap
						}
					}

				case "Data":
					if subTagType != nbt.TagByteArray {
						return fmt.Errorf("期望 Data 为 TAG_ByteArray, 实际为 %s", subTagType)
					}
					// 记录实际varint数据开始的位置
					var lengthBytes [4]byte
					if _, err := io.ReadFull(offsetReader, lengthBytes[:]); err != nil {
						return fmt.Errorf("读取 Data 长度失败: %w", err)
					}
					s.dataTagGzipOffset = offsetReader.GetOffset()
					// 计算数据长度并跳过剩余数据
					length := int32(lengthBytes[0])<<24 | int32(lengthBytes[1])<<16 | int32(lengthBytes[2])<<8 | int32(lengthBytes[3])
					if _, err := io.CopyN(io.Discard, offsetReader, int64(length)); err != nil {
						return fmt.Errorf("跳过 Data 内容失败: %w", err)
					}

				default:
					err = tagReader.SkipTagValue(offsetReader, subTagType)
					if err != nil {
						return fmt.Errorf("跳过标签 %s 失败: %w", tagName, err)
					}
				}
			}
		default:
			err = tagReader.SkipTagValue(offsetReader, tagType)
			if err != nil {
				return fmt.Errorf("跳过标签 %s 失败: %w", tagName, err)
			}
		}
	}

	// 验证是不是真正的 SchemV2 文件 查看必要数据是否获取成功
	if s.dataTagGzipOffset == 0 {
		return ErrInvalidFile
	}

	return nil
}

func (s *SchemV2) GetPalette() map[int32]uint32 {
	return s.palette
}

func (s *SchemV2) GetOffsetPos() define.Offset {
	return s.offsetPos
}

func (s *SchemV2) SetOffsetPos(offset define.Offset) {
	s.offsetPos = offset
	s.size.Width = s.originalSize.Width + int(math.Abs(float64(offset.X())))
	s.size.Length = s.originalSize.Length + int(math.Abs(float64(offset.Z())))
	s.size.Height = s.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (s *SchemV2) GetSize() define.Size {
	return *s.size
}

func (s *SchemV2) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
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
	if _, err = io.CopyN(io.Discard, offsetReader, s.dataTagGzipOffset); err != nil {
		return nil, fmt.Errorf("定位到 data 标签失败: %w", err)
	}

	currentIndex := 0
	for _, targetIndex := range allIndices {
		// 跳过不需要的varint, 逐个解码来正确跳过变长编码
		for currentIndex < targetIndex {
			_, err := ReadVarintFromReader(offsetReader)
			if err != nil {
				return nil, fmt.Errorf("跳过索引 %d 的 varint 失败: %w", currentIndex, err)
			}
			currentIndex++
		}

		// 读取目标位置的 varint 编码的方块索引数据
		blockIndex, err := ReadVarintFromReader(offsetReader)
		if err != nil {
			return nil, fmt.Errorf("读取索引 %d 的 varint 数据失败: %w", targetIndex, err)
		}
		currentIndex++

		// 从原始索引反推原始坐标
		x := targetIndex % origWidth
		remaining := targetIndex / origWidth
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

		// 从s.palette获取方块ID
		blockRuntimeID, ok := s.palette[int32(blockIndex)]
		if !ok {
			blockRuntimeID = UnknownBlockRuntimeID
		}

		// 设置方块到新位置
		c.SetBlock(localX, localY - 64, localZ, 0, blockRuntimeID)
	}

	return chunks, nil
}

func (s *SchemV2) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	return nil, nil
}

func (s *SchemV2) CountNonAirBlocks() (int, error) {
	volume := s.originalSize.GetVolume()
	airIndex := int32(0)
	found := false
	for k, v := range s.palette {
		if v == block.AirRuntimeID {
			found = true
			airIndex = k
			break
		}
	}
	if !found {
		return volume, nil
	}
	nonAirBlocks := 0

	file, err := os.Open(s.file.Name())
	if err != nil {
		return nonAirBlocks, fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nonAirBlocks, fmt.Errorf("创建 gzip 读取器失败: %w", err)
	}
	defer gzipReader.Close()

	if _, err = io.CopyN(io.Discard, gzipReader, s.dataTagGzipOffset); err != nil {
		return nonAirBlocks, fmt.Errorf("定位到 block data 标签失败: %w", err)
	}

	for range volume {
		blockIndex, err := ReadVarintFromReader(gzipReader)
		if err != nil {
			return nonAirBlocks, fmt.Errorf("读取 varint 方块索引失败: %w", err)
		}

		if int32(blockIndex) == airIndex {
			continue
		}

		nonAirBlocks++
	}

	return nonAirBlocks, nil
}

func (s *SchemV2) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(num int),
	progressCallback func(),
) error {
	width := s.originalSize.GetWidth()
	length := s.originalSize.GetLength()
	height := s.originalSize.GetHeight()
	chunkCount := s.originalSize.GetChunkCount()
	totalVolume := width * length * height

	if totalVolume == 0 {
		if startCallback != nil {
			startCallback(0)
		}
		return nil
	}

	dataFile, err := os.Open(s.file.Name())
	if err != nil {
		return fmt.Errorf("重新打开数据文件失败: %w", err)
	}
	defer dataFile.Close()

	gzipReader, err := gzip.NewReader(dataFile)
	if err != nil {
		return fmt.Errorf("创建 gzip 读取器失败: %w", err)
	}
	defer gzipReader.Close()

	if _, err := io.CopyN(io.Discard, gzipReader, s.dataTagGzipOffset); err != nil {
		return fmt.Errorf("定位到 data 失败: %w", err)
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
					blockIndex, err := ReadVarintFromReader(gzipReader)
					if err != nil {
						return fmt.Errorf("读取方块索引失败: %w", err)
					}

					runtimeID, ok := s.palette[int32(blockIndex)]
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

func (s *SchemV2) FromMCWorld(
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
		startCallback(int(subChunkYNum))
	}

	gzipWriter, err := gzip.NewWriterLevel(target, gzip.BestSpeed)
	if err != nil {
		return err
	}
	defer gzipWriter.Close()
	tagWriter := nbt.NewTagWriter(nbt.BigEndian)
	offsetWriter := nbt.NewOffsetWriter(gzipWriter)
	palette := map[uint32]int{}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagStruct, "Schematic")
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagInt32, "DataVersion")
	if err != nil {
		return err
	}
	err = tagWriter.WriteTagInt32(offsetWriter, JavaDataVersion)
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagInt32, "Version")
	if err != nil {
		return err
	}
	err = tagWriter.WriteTagInt32(offsetWriter, 2)
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

	err = tagWriter.WriteTag(offsetWriter, nbt.TagInt16, "Height")
	if err != nil {
		return err
	}
	err = tagWriter.WriteTagInt16(offsetWriter, int16(height))
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

	err = tagWriter.WriteTag(offsetWriter, nbt.TagStruct, "Blocks")
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagByteArray, "Data")
	if err != nil {
		return err
	}

	blockDataBuffer := bytes.NewBuffer(nil)
	blockDataGzipWriter, err := gzip.NewWriterLevel(blockDataBuffer, gzip.BestSpeed)
	blockDataOffsetWriter := nbt.NewOffsetWriter(blockDataGzipWriter)
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
							index, ok := palette[blockRuntimeID]
							if !ok {
								index = len(palette)
								palette[blockRuntimeID] = index
							}
							err = WriteVarintToWriter(blockDataOffsetWriter, index)
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

	err = blockDataGzipWriter.Close()
	if err != nil {
		return err
	}
	blockDataLength := int32(blockDataOffsetWriter.GetOffset())
	err = tagWriter.WriteTagInt32(offsetWriter, blockDataLength)
	if err != nil {
		return err
	}
	blockDataBuffer = bytes.NewBuffer(blockDataBuffer.Bytes())
	blockDataGzipReader, err := gzip.NewReader(blockDataBuffer)
	if err != nil {
		return err
	}
	_, err = io.Copy(offsetWriter, blockDataGzipReader)
	if err != nil {
		return err
	}
	err = blockDataGzipReader.Close()
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagStruct, "Palette")
	if err != nil {
		return err
	}
	for blockRuntimeID, index := range palette {
		name, properties, _ := block.RuntimeIDToState(blockRuntimeID)
		javaBlockStr, found := blocks.BedrockBlockStrToJavaBlockStr(name + utils.PropertiesToStateStr(properties))
		if !found {
			javaBlockStr = "air"
		}
		javaBlockStr = "minecraft:" + javaBlockStr
		err = tagWriter.WriteTag(offsetWriter, nbt.TagInt32, javaBlockStr)
		if err != nil {
			return err
		}
		err = tagWriter.WriteTagInt32(offsetWriter, int32(index))
		if err != nil {
			return err
		}
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagEnd, "")
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagEnd, "")
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagEnd, "")
	if err != nil {
		return err
	}
	return nil
}

func (s *SchemV2) ID() uint8 {
	return IDSchemV2
}

func (s *SchemV2) Name() string {
	return NameSchemV2
}

func (s *SchemV2) Close() error {
	return nil
}
