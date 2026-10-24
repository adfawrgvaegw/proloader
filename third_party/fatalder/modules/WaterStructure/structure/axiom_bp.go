package structure

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/WaterStructure/utils/nbt"
	"github.com/Yeah114/blocks"
)

const AxiomBPMagic int32 = 0x0AE5BB36

type AxiomBP struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset

	DataVersion       int32
	blockDataStart    int64
	minRegionX        int32
	minRegionY        int32
	minRegionZ        int32
	maxRegionX        int32
	maxRegionY        int32
	maxRegionZ        int32
	regionsByChunkPos map[define.ChunkPos]map[int32]*AxiomRegion
	regions           []*AxiomRegion

	sizeComputed        bool
	nonAirBlocks        int
	nonAirCountComputed bool
}

type AxiomRegion struct {
	X          int32
	Y          int32
	Z          int32
	DataOffset int64
	Palette    map[int32]uint32
}

func (a *AxiomBP) ID() uint8 {
	return IDAxiomBP
}

func (a *AxiomBP) Name() string {
	return NameAxiomBP
}

func (a *AxiomBP) FromFile(file *os.File) error {
	a.file = file
	a.size = &define.Size{}
	a.originalSize = &define.Size{}
	a.offsetPos = define.Offset{}
	a.regions = make([]*AxiomRegion, 0)
	a.regionsByChunkPos = make(map[define.ChunkPos]map[int32]*AxiomRegion)
	a.minRegionX = math.MaxInt32
	a.minRegionY = math.MaxInt32
	a.minRegionZ = math.MaxInt32
	a.maxRegionX = math.MinInt32
	a.maxRegionY = math.MinInt32
	a.maxRegionZ = math.MinInt32
	a.sizeComputed = false
	a.nonAirCountComputed = false

	tagReader := nbt.NewTagReader(nbt.BigEndian)
	axiomOffsetReader := nbt.NewOffsetReader(a.file)
	magic, err := tagReader.ReadTagInt32(axiomOffsetReader)
	if err != nil {
		return fmt.Errorf("读取魔数失败: %w", err)
	}

	if magic != AxiomBPMagic {
		return fmt.Errorf("魔数无效: 期望 0x%x, 实际为 0x%x", AxiomBPMagic, magic)
	}

	_, err = tagReader.ReadTagByteArray(axiomOffsetReader)
	if err != nil {
		return fmt.Errorf("跳过头部失败: %w", err)
	}

	_, err = tagReader.ReadTagByteArray(axiomOffsetReader)
	if err != nil {
		return fmt.Errorf("跳过缩略图失败: %w", err)
	}

	_, err = tagReader.ReadTagInt32(axiomOffsetReader)
	if err != nil {
		return fmt.Errorf("跳过方块数据长度失败: %w", err)
	}

	a.blockDataStart, err = a.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("获取方块数据起始位置失败: %w", err)
	}

	gzipReader, err := gzip.NewReader(a.file)
	if err != nil {
		return fmt.Errorf("创建 gzip 读取器失败: %w", err)
	}
	defer gzipReader.Close()

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
		case "BlockRegion":
			if tagType != nbt.TagSlice {
				return fmt.Errorf("期望 BlockRegion 为 TAG_List, 实际为 %s", tagType)
			}
			blockRegionTagType, err := tagReader.ReadTagType(offsetReader)
			if err != nil {
				return fmt.Errorf("读取区域标签类型失败: %w", err)
			}

			if blockRegionTagType != nbt.TagStruct {
				return fmt.Errorf("期望区域为 TAG_Compound, 实际为 %s", blockRegionTagType)
			}

			regionsLength, err := tagReader.ReadTagInt32(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 BlockRegion 长度失败: %w", err)
			}

			if regionsLength == 0 {
				return fmt.Errorf("未找到任何区域")
			}

			for range regionsLength {
				region := &AxiomRegion{
					Palette: make(map[int32]uint32),
				}
				a.regions = append(a.regions, region)
				var x, y, z *int32
				for {
					regionTagType, regionTagName, err := tagReader.ReadTag(offsetReader)
					if err != nil {
						return fmt.Errorf("读取区域标签失败: %w", err)
					}

					if regionTagType == nbt.TagEnd {
						break
					}

					switch regionTagName {
					case "BlockStates":
						if regionTagType != nbt.TagStruct {
							return fmt.Errorf("期望区域 BlockStates 为 TAG_Compound, 实际为 %s", regionTagType)
						}
						for {
							blockStatesTagType, blockStatesTagName, err := tagReader.ReadTag(offsetReader)
							if err != nil {
								return fmt.Errorf("读取 block states 标签失败: %w", err)
							}

							if blockStatesTagType == nbt.TagEnd {
								break
							}

							switch blockStatesTagName {
							case "data":
								if blockStatesTagType != nbt.TagInt64Array {
									return fmt.Errorf("期望 block states data 为 TAG_LongArray, 实际为 %s", blockStatesTagType)
								}
								region.DataOffset = offsetReader.GetOffset()
								err = tagReader.SkipTagValue(offsetReader, blockStatesTagType)
								if err != nil {
									return fmt.Errorf("跳过 block states 数据失败: %w", err)
								}

							case "palette":
								if blockStatesTagType != nbt.TagSlice {
									return fmt.Errorf("期望 block states palette 为 TAG_List, 实际为 %s", blockStatesTagType)
								}
								blockStatesPalette, err := tagReader.ReadTagList(offsetReader)
								if err != nil {
									return fmt.Errorf("读取 block states palette 失败: %w", err)
								}
								for i, blockState := range blockStatesPalette {
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
									var blockRuntimeID uint32
									runtimeID, _ := blocks.BlockNameAndStateToRuntimeID(name, properties)
									baseName, properties, found := blocks.RuntimeIDToState(runtimeID)
									if !found {
										blockRuntimeID = UnknownBlockRuntimeID
									} else {
										blockRuntimeID, found = block.StateToRuntimeID(baseName, properties)
										if !found {
											blockRuntimeID = UnknownBlockRuntimeID
										}
									}
									region.Palette[index] = blockRuntimeID
								}
							default:
								err = tagReader.SkipTagValue(offsetReader, blockStatesTagType)
								if err != nil {
									return fmt.Errorf("跳过 block states 标签 %s 失败: %w", blockStatesTagName, err)
								}
							}
						}
					case "X":
						if regionTagType != nbt.TagInt32 {
							return fmt.Errorf("期望区域 X 为 TAG_Int, 实际为 %s", regionTagType)
						}
						valX, err := tagReader.ReadTagInt32(offsetReader)
						if err != nil {
							return fmt.Errorf("读取区域 X 标签 %s 失败: %w", regionTagName, err)
						}
						x = &valX
					case "Y":
						if regionTagType != nbt.TagInt32 {
							return fmt.Errorf("期望区域 Y 为 TAG_Int, 实际为 %s", regionTagType)
						}
						valY, err := tagReader.ReadTagInt32(offsetReader)
						if err != nil {
							return fmt.Errorf("读取区域 Y 标签 %s 失败: %w", regionTagName, err)
						}
						y = &valY
					case "Z":
						if regionTagType != nbt.TagInt32 {
							return fmt.Errorf("期望区域 Z 为 TAG_Int, 实际为 %s", regionTagType)
						}
						valZ, err := tagReader.ReadTagInt32(offsetReader)
						if err != nil {
							return fmt.Errorf("读取区域 Z 标签 %s 失败: %w", regionTagName, err)
						}
						z = &valZ
					default:
						err = tagReader.SkipTagValue(offsetReader, regionTagType)
						if err != nil {
							return fmt.Errorf("跳过区域标签 %s 失败: %w", regionTagName, err)
						}
					}
				}
				if x == nil || y == nil || z == nil {
					return fmt.Errorf("未找到区域坐标 XYZ")
				}
				regionX := *x
				regionY := *y
				regionZ := *z
				region.X = regionX
				region.Y = regionY
				region.Z = regionZ
				a.minRegionX = min(a.minRegionX, regionX)
				a.minRegionY = min(a.minRegionY, regionY)
				a.minRegionZ = min(a.minRegionZ, regionZ)
				a.maxRegionX = max(a.maxRegionX, regionX)
				a.maxRegionY = max(a.maxRegionY, regionY)
				a.maxRegionZ = max(a.maxRegionZ, regionZ)
			}
			for _, region := range a.regions {
				region.X = region.X - a.minRegionX
				region.Y = region.Y - a.minRegionY
				region.Z = region.Z - a.minRegionZ
				chunkPos := define.ChunkPos{region.X, region.Z}
				_, ok := a.regionsByChunkPos[chunkPos]
				if !ok {
					a.regionsByChunkPos[chunkPos] = make(map[int32]*AxiomRegion)
				}
				a.regionsByChunkPos[chunkPos][region.Y] = region
			}
			if a.minRegionX == math.MaxInt32 {
				a.minRegionX = 0
			}
			if a.minRegionY == math.MaxInt32 {
				a.minRegionY = 0
			}
			if a.minRegionZ == math.MaxInt32 {
				a.minRegionZ = 0
			}
			if a.maxRegionX == math.MinInt32 {
				a.maxRegionX = 0
			}
			if a.maxRegionY == math.MinInt32 {
				a.maxRegionY = 0
			}
			if a.maxRegionZ == math.MinInt32 {
				a.maxRegionZ = 0
			}
			width := int(a.maxRegionX-a.minRegionX+1) * 16
			height := int(a.maxRegionY-a.minRegionY+1) * 16
			length := int(a.maxRegionZ-a.minRegionZ+1) * 16
			if width < 0 {
				width = 0
			}
			if height < 0 {
				height = 0
			}
			if length < 0 {
				length = 0
			}
			a.size.Width = width
			a.size.Length = length
			a.size.Height = height
			a.originalSize.Width = width
			a.originalSize.Length = length
			a.originalSize.Height = height
		case "DataVersion":
			if tagType != nbt.TagInt32 {
				return fmt.Errorf("期望 DataVersion 为 TAG_Int, 实际为 %s", tagType)
			}
			a.DataVersion, err = tagReader.ReadTagInt32(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 DataVersion 失败: %w", err)
			}
		default:
			err = tagReader.SkipTagValue(offsetReader, tagType)
			if err != nil {
				return fmt.Errorf("跳过标签 %s 失败: %w", tagName, err)
			}
		}
	}
	return a.computeSizeAndNonAirBlocks()
}

func (a *AxiomBP) FromMCWorld(
	world *world.BedrockWorld,
	target *os.File,
	point1BlockPos define.BlockPos,
	point2BlockPos define.BlockPos,
	startCallback func(int),
	progressCallback func(),
) error {
	if world == nil {
		return fmt.Errorf("bedrock 世界为 nil")
	}
	if target == nil {
		return fmt.Errorf("目标文件为 nil")
	}

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

	width := int(endBlockPosX - startBlockPosX + 1)
	height := int(endBlockPosY - startBlockPosY + 1)
	length := int(endBlockPosZ - startBlockPosZ + 1)
	if width <= 0 || height <= 0 || length <= 0 {
		return fmt.Errorf("无效的导出范围: %dx%dx%d", width, height, length)
	}
	totalBlocks := int64(width) * int64(height) * int64(length)
	if totalBlocks <= 0 {
		return fmt.Errorf("导出体积过小")
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
	subChunkXNum := endSubChunkPos.X() - startSubChunkPosX + 1
	subChunkYNum := endSubChunkPos.Y() - startSubChunkPosY + 1
	subChunkZNum := endSubChunkPos.Z() - startSubChunkPosZ + 1

	if subChunkXNum <= 0 || subChunkYNum <= 0 || subChunkZNum <= 0 {
		return fmt.Errorf("子区块数量无效")
	}

	totalRegions := int64(subChunkXNum) * int64(subChunkYNum) * int64(subChunkZNum)
	if totalRegions <= 0 {
		return fmt.Errorf("需要导出的 Region 数量为 0")
	}
	if totalRegions > math.MaxInt32 {
		return fmt.Errorf("需要导出的 Region 数量过大: %d", totalRegions)
	}

	if startCallback != nil {
		startCallback(int(totalRegions))
	}

	blockDataBuffer := bytes.NewBuffer(nil)
	blockDataGzipWriter, err := gzip.NewWriterLevel(blockDataBuffer, gzip.BestSpeed)
	if err != nil {
		return fmt.Errorf("创建 gzip 写入器失败: %w", err)
	}
	blockDataWriter := nbt.NewOffsetWriter(blockDataGzipWriter)
	blockDataTagWriter := nbt.NewTagWriter(nbt.BigEndian)

	if err = blockDataTagWriter.WriteTag(blockDataWriter, nbt.TagStruct, ""); err != nil {
		return err
	}
	if err = blockDataTagWriter.WriteTag(blockDataWriter, nbt.TagSlice, "BlockRegion"); err != nil {
		return err
	}
	if err = blockDataTagWriter.WriteTagType(blockDataWriter, nbt.TagStruct); err != nil {
		return err
	}
	if err = blockDataTagWriter.WriteTagInt32(blockDataWriter, int32(totalRegions)); err != nil {
		return err
	}

	blockCount := int64(0)
	type paletteEntry struct {
		Name       string
		Properties map[string]any
	}

	for subChunkY := range subChunkYNum {
		worldSubChunkPosY := startSubChunkPosY + int32(subChunkY)
		subChunkWorldYStart := worldSubChunkPosY * 16
		subChunkWorldYEnd := subChunkWorldYStart + 15
		effectiveWorldYStart := max(subChunkWorldYStart, startBlockPosY)
		effectiveWorldYEnd := min(subChunkWorldYEnd, endBlockPosY)

		for subChunkZ := range subChunkZNum {
			worldSubChunkPosZ := startSubChunkPosZ + int32(subChunkZ)
			subChunkWorldZStart := worldSubChunkPosZ * 16
			subChunkWorldZEnd := subChunkWorldZStart + 15
			effectiveWorldZStart := max(subChunkWorldZStart, startBlockPosZ)
			effectiveWorldZEnd := min(subChunkWorldZEnd, endBlockPosZ)

			for subChunkX := range subChunkXNum {
				worldSubChunkPosX := startSubChunkPosX + int32(subChunkX)
				subChunkWorldXStart := worldSubChunkPosX * 16
				subChunkWorldXEnd := subChunkWorldXStart + 15
				effectiveWorldXStart := max(subChunkWorldXStart, startBlockPosX)
				effectiveWorldXEnd := min(subChunkWorldXEnd, endBlockPosX)

				blockIndices := make([]int32, 4096)
				paletteIndexes := make(map[uint32]int32)
				paletteEntries := make([]paletteEntry, 0, 16)
				addPaletteEntry := func(runtimeID uint32) int32 {
					if idx, ok := paletteIndexes[runtimeID]; ok {
						return idx
					}
					name, properties := convertRuntimeIDToAxiomPaletteState(runtimeID)
					index := int32(len(paletteEntries))
					paletteIndexes[runtimeID] = index
					paletteEntries = append(paletteEntries, paletteEntry{
						Name:       name,
						Properties: properties,
					})
					return index
				}
				addPaletteEntry(block.AirRuntimeID)

				worldSubChunkPos := bwo_define.SubChunkPos{
					worldSubChunkPosX,
					worldSubChunkPosY,
					worldSubChunkPosZ,
				}
				subChunk := world.LoadSubChunk(bwo_define.DimensionIDOverworld, worldSubChunkPos)

				if effectiveWorldXStart <= effectiveWorldXEnd &&
					effectiveWorldYStart <= effectiveWorldYEnd &&
					effectiveWorldZStart <= effectiveWorldZEnd &&
					subChunk != nil {
					startLocalY := byte(effectiveWorldYStart - subChunkWorldYStart)
					endLocalY := byte(effectiveWorldYEnd - subChunkWorldYStart)
					startLocalZ := byte(effectiveWorldZStart - subChunkWorldZStart)
					endLocalZ := byte(effectiveWorldZEnd - subChunkWorldZStart)
					startLocalX := byte(effectiveWorldXStart - subChunkWorldXStart)
					endLocalX := byte(effectiveWorldXEnd - subChunkWorldXStart)

					for localY := startLocalY; localY <= endLocalY; localY++ {
						for localZ := startLocalZ; localZ <= endLocalZ; localZ++ {
							for localX := startLocalX; localX <= endLocalX; localX++ {
								blockRuntimeID := subChunk.Block(localX, localY, localZ, 0)
								blockIdx := int(localY)*256 + int(localZ)*16 + int(localX)
								blockIndices[blockIdx] = addPaletteEntry(blockRuntimeID)
								if blockRuntimeID != block.AirRuntimeID {
									blockCount++
								}
							}
						}
					}
				}

				longArray, err := packBlockIndices(blockIndices, len(paletteEntries))
				if err != nil {
					return err
				}

				paletteList := make([]interface{}, len(paletteEntries))
				for i, entry := range paletteEntries {
					compound := map[string]any{
						"Name": entry.Name,
					}
					if len(entry.Properties) > 0 {
						compound["Properties"] = entry.Properties
					}
					paletteList[i] = compound
				}

				regionCompound := map[string]any{
					"BlockStates": map[string]any{
						"palette": paletteList,
						"data":    longArray,
					},
					"X": int32(subChunkX),
					"Y": int32(subChunkY),
					"Z": int32(subChunkZ),
				}

				if err = blockDataTagWriter.WriteTagCompound(blockDataWriter, regionCompound); err != nil {
					return err
				}

				if progressCallback != nil {
					progressCallback()
				}
			}
		}
	}

	if err = blockDataTagWriter.WriteTag(blockDataWriter, nbt.TagInt32, "DataVersion"); err != nil {
		return err
	}
	if err = blockDataTagWriter.WriteTagInt32(blockDataWriter, JavaDataVersion); err != nil {
		return err
	}
	if err = blockDataTagWriter.WriteTag(blockDataWriter, nbt.TagEnd, ""); err != nil {
		return err
	}

	if err = blockDataGzipWriter.Close(); err != nil {
		return err
	}

	if blockCount > math.MaxInt32 {
		return fmt.Errorf("非空气方块数量过大: %d", blockCount)
	}
	blockCountInt32 := int32(blockCount)
	containsAir := blockCount < totalBlocks

	structureName := filepath.Base(target.Name())
	if ext := filepath.Ext(structureName); ext != "" {
		structureName = strings.TrimSuffix(structureName, ext)
	}
	if structureName == "" {
		structureName = "AxiomStructure"
	}

	author := os.Getenv("USER")
	if author == "" {
		author = os.Getenv("USERNAME")
	}
	if author == "" {
		author = "WaterStructure"
	}

	metadataBytes, err := buildAxiomMetadata(structureName, author, blockCountInt32, containsAir)
	if err != nil {
		return err
	}

	blockDataBytes := blockDataBuffer.Bytes()
	if len(blockDataBytes) > math.MaxInt32 {
		return fmt.Errorf("方块数据过大: %d", len(blockDataBytes))
	}

	if _, err = target.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("定位目标文件失败: %w", err)
	}
	if err = target.Truncate(0); err != nil {
		return fmt.Errorf("清空目标文件失败: %w", err)
	}

	fileWriter := nbt.NewOffsetWriter(target)
	fileTagWriter := nbt.NewTagWriter(nbt.BigEndian)

	if err = fileTagWriter.WriteTagInt32(fileWriter, AxiomBPMagic); err != nil {
		return err
	}
	if err = fileTagWriter.WriteTagByteArray(fileWriter, metadataBytes); err != nil {
		return err
	}
	if err = fileTagWriter.WriteTagByteArray(fileWriter, nil); err != nil {
		return err
	}
	if err = fileTagWriter.WriteTagInt32(fileWriter, int32(len(blockDataBytes))); err != nil {
		return err
	}
	if _, err = fileWriter.Write(blockDataBytes); err != nil {
		return fmt.Errorf("写入方块数据失败: %w", err)
	}

	return nil
}

func (a *AxiomBP) GetOffsetPos() define.Offset {
	return a.offsetPos
}

func (a *AxiomBP) SetOffsetPos(offset define.Offset) {
	a.offsetPos = offset
	a.size.Width = a.originalSize.Width + int(math.Abs(float64(offset.X())))
	a.size.Length = a.originalSize.Length + int(math.Abs(float64(offset.Z())))
	a.size.Height = a.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (a *AxiomBP) GetSize() define.Size {
	if !a.sizeComputed {
		_ = a.computeSizeAndNonAirBlocks()
	}
	return *a.size
}

func (a *AxiomBP) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	chunks := make(map[define.ChunkPos]*chunk.Chunk)
	// 初始化所有请求的区块为空气
	for _, pos := range posList {
		chunks[pos] = chunk.NewChunk(block.AirRuntimeID, MCWorldOverworldRange)
	}

	// 若未记录区块数据起始位置, 直接返回空区块
	if a.blockDataStart == 0 || len(a.regions) == 0 {
		return chunks, nil
	}

	// 重新打开文件, 准备流式读取
	file, err := os.Open(a.file.Name())
	if err != nil {
		return nil, fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	// 定位到区块数据起始位置
	if _, err := file.Seek(a.blockDataStart, io.SeekStart); err != nil {
		return nil, fmt.Errorf("定位到方块数据失败: %w", err)
	}

	// 创建 gzip 读取器（Axiom BP 区块数据为 gzip 压缩）
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("创建 gzip 读取器失败: %w", err)
	}
	defer gzipReader.Close()

	// 筛选目标区域: 只处理与请求区块有重叠的 Region
	targetRegions := make([]*AxiomRegion, 0)
	requestedChunks := make(map[define.ChunkPos]bool)
	for _, pos := range posList {
		requestedChunks[pos] = true
		// 查找该区块下关联的所有 Y 层 Region
		if yRegions, ok := a.regionsByChunkPos[pos]; ok {
			for _, region := range yRegions {
				targetRegions = append(targetRegions, region)
			}
		}
	}

	// 若无目标区域, 直接返回初始化的空气区块
	if len(targetRegions) == 0 {
		return chunks, nil
	}

	// 按 Region 的数据偏移量排序, 确保顺序读取
	sort.Slice(targetRegions, func(i, j int) bool {
		return targetRegions[i].DataOffset < targetRegions[j].DataOffset
	})

	// 创建 NBT 读取器（大端序, Axiom BP 采用 Java 标准 NBT）
	offsetReader := nbt.NewOffsetReader(gzipReader)

	// 遍历目标 Region, 解析每个 Region 的方块数据
	for _, region := range targetRegions {
		// 定位到当前 Region 的 BlockStates.data 偏移位置
		currentOffset := offsetReader.GetOffset()
		if currentOffset < region.DataOffset {
			skipBytes := region.DataOffset - currentOffset
			_, err := io.CopyN(io.Discard, offsetReader, skipBytes)
			if err != nil {
				return nil, fmt.Errorf("定位到区域数据失败 (X:%d,Y:%d,Z:%d): %w", region.X, region.Y, region.Z, err)
			}
		}

		// 读取 LongArray 长度（大端 int32）
		var lenBuf [4]byte
		if _, err := io.ReadFull(offsetReader, lenBuf[:]); err != nil {
			return nil, fmt.Errorf("读取区域数据长度失败 (X:%d,Y:%d,Z:%d): %w", region.X, region.Y, region.Z, err)
		}
		numLongs := int(int32(binary.BigEndian.Uint32(lenBuf[:])))
		if numLongs < 0 {
			return nil, fmt.Errorf("区域数据长度无效 (X:%d,Y:%d,Z:%d): %d", region.X, region.Y, region.Z, numLongs)
		}

		// 读取 LongArray 数据（存储方块索引的位打包数据）
		longs := make([]uint64, numLongs)
		for i := 0; i < numLongs; i++ {
			var longBuf [8]byte
			if _, err := io.ReadFull(offsetReader, longBuf[:]); err != nil {
				return nil, fmt.Errorf("读取区域 long 数据失败 (X:%d,Y:%d,Z:%d, index:%d): %w", region.X, region.Y, region.Z, i, err)
			}
			// 转换为无符号 64 位整数（Java long -> Go uint64）
			longs[i] = binary.BigEndian.Uint64(longBuf[:])
		}

		// 解析位打包数据: 转换为方块调色板索引
		paletteSize := len(region.Palette)
		if paletteSize == 0 {
			continue // 空调色板, 跳过
		}

		// 计算每个方块索引占用的位数（Axiom 规则: 最小 4 位）
		bitsPerBlock := int(math.Ceil(math.Log2(float64(paletteSize))))
		if bitsPerBlock < 4 {
			bitsPerBlock = 4
		}
		valuesPerLong := 64 / bitsPerBlock
		mask := uint64((1 << bitsPerBlock) - 1)
		expectedEntries := 4096 // 16*16*16 子区块的方块总数

		// 遍历当前 Region 内的所有方块（16x16x16）
		for blockIndex := 0; blockIndex < expectedEntries; blockIndex++ {
			// 计算当前方块在 long 数组中的位置
			longIndex := blockIndex / valuesPerLong
			bitIndex := (blockIndex % valuesPerLong) * bitsPerBlock

			// 边界检查: 避免数组越界
			if longIndex >= len(longs) {
				continue
			}

			// 提取方块调色板索引（MSB 优先, Axiom 标准位读取顺序）
			paletteIdx := (longs[longIndex] >> bitIndex) & mask
			if int32(paletteIdx) >= int32(paletteSize) {
				continue // 索引超出调色板范围, 跳过
			}

			// 获取方块运行时 ID
			blockRuntimeID, ok := region.Palette[int32(paletteIdx)]
			if !ok || blockRuntimeID == block.AirRuntimeID {
				continue // 空气方块或未知方块, 跳过
			}

			// 计算方块在 Region 内的局部坐标（x,y,z: 0-15）
			localY := blockIndex / 256 // 256 = 16*16
			rem := blockIndex % 256
			localZ := rem / 16
			localX := rem % 16

			// 计算方块在世界中的绝对坐标（含偏移量）
			worldX := int(region.X)*16 + localX + int(a.offsetPos.X())
			worldY := int(region.Y)*16 + localY + int(a.offsetPos.Y())
			worldZ := int(region.Z)*16 + localZ + int(a.offsetPos.Z())

			// 计算方块所属的目标区块坐标
			chunkX := int32(worldX >> 4)
			chunkZ := int32(worldZ >> 4)
			targetChunkPos := define.ChunkPos{chunkX, chunkZ}

			// 检查目标区块是否在请求列表中
			if !requestedChunks[targetChunkPos] {
				continue
			}

			// 计算方块在目标区块内的局部坐标
			localChunkX := uint8(worldX % 16)
			localChunkY := int16(worldY)
			localChunkZ := uint8(worldZ % 16)

			// 边界检查: 确保 Y 坐标在区块高度范围内
			if localChunkY < 0 || localChunkY >= int16(a.size.Height) {
				continue
			}

			// 将方块设置到目标区块中
			targetChunk := chunks[targetChunkPos]
			targetChunk.SetBlock(localChunkX, localChunkY-64, localChunkZ, 0, blockRuntimeID)
		}
	}

	return chunks, nil
}

func (a *AxiomBP) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any)
	return result, nil
}

// CountNonAirBlocks 统计非空气方块数量
func (a *AxiomBP) CountNonAirBlocks() (int, error) {
	if !a.nonAirCountComputed || !a.sizeComputed {
		if err := a.computeSizeAndNonAirBlocks(); err != nil {
			return 0, err
		}
	}
	return a.nonAirBlocks, nil
}

func (a *AxiomBP) computeSizeAndNonAirBlocks() error {
	if a.sizeComputed && a.nonAirCountComputed {
		return nil
	}
	if len(a.regions) == 0 {
		a.size.Width = 0
		a.size.Height = 0
		a.size.Length = 0
		a.originalSize.Width = 0
		a.originalSize.Height = 0
		a.originalSize.Length = 0
		a.nonAirBlocks = 0
		a.sizeComputed = true
		a.nonAirCountComputed = true
		return nil
	}

	file, err := os.Open(a.file.Name())
	if err != nil {
		return fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	if _, err := file.Seek(a.blockDataStart, io.SeekStart); err != nil {
		return fmt.Errorf("定位到方块数据失败: %w", err)
	}

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("创建 gzip 读取器失败: %w", err)
	}
	defer gzipReader.Close()

	offsetReader := nbt.NewOffsetReader(gzipReader)

	targetRegions := make([]*AxiomRegion, len(a.regions))
	copy(targetRegions, a.regions)
	sort.Slice(targetRegions, func(i, j int) bool {
		return targetRegions[i].DataOffset < targetRegions[j].DataOffset
	})

	maxBlockX := -1
	maxBlockY := -1
	maxBlockZ := -1
	nonAirBlocks := 0

	for _, region := range targetRegions {
		currentOffset := offsetReader.GetOffset()
		if currentOffset < region.DataOffset {
			skipBytes := region.DataOffset - currentOffset
			if _, err := io.CopyN(io.Discard, offsetReader, skipBytes); err != nil {
				return fmt.Errorf("定位到区域数据失败 (X:%d,Y:%d,Z:%d): %w", region.X, region.Y, region.Z, err)
			}
		}

		var lenBuf [4]byte
		if _, err := io.ReadFull(offsetReader, lenBuf[:]); err != nil {
			return fmt.Errorf("读取区域数据长度失败 (X:%d,Y:%d,Z:%d): %w", region.X, region.Y, region.Z, err)
		}
		numLongs := int32(binary.BigEndian.Uint32(lenBuf[:]))
		if numLongs < 0 {
			return fmt.Errorf("区域数据长度无效 (X:%d,Y:%d,Z:%d): %d", region.X, region.Y, region.Z, numLongs)
		}

		longs := make([]uint64, int(numLongs))
		for i := 0; i < int(numLongs); i++ {
			var longBuf [8]byte
			if _, err := io.ReadFull(offsetReader, longBuf[:]); err != nil {
				return fmt.Errorf("读取区域 long 数据失败 (X:%d,Y:%d,Z:%d, index:%d): %w", region.X, region.Y, region.Z, i, err)
			}
			longs[i] = binary.BigEndian.Uint64(longBuf[:])
		}

		paletteSize := len(region.Palette)
		if paletteSize == 0 {
			continue
		}
		bitsPerBlock := int(math.Ceil(math.Log2(float64(paletteSize))))
		if bitsPerBlock < 4 {
			bitsPerBlock = 4
		}
		valuesPerLong := 64 / bitsPerBlock
		mask := uint64((1 << bitsPerBlock) - 1)
		expectedEntries := 4096

		for blockIndex := 0; blockIndex < expectedEntries; blockIndex++ {
			longIndex := blockIndex / valuesPerLong
			bitIndex := (blockIndex % valuesPerLong) * bitsPerBlock
			if longIndex >= len(longs) {
				break
			}

			paletteIdx := (longs[longIndex] >> bitIndex) & mask
			if int(paletteIdx) >= paletteSize {
				continue
			}

			blockRuntimeID := region.Palette[int32(paletteIdx)]
			if blockRuntimeID == block.AirRuntimeID {
				continue
			}

			nonAirBlocks++

			localY := blockIndex / 256
			rem := blockIndex % 256
			localZ := rem / 16
			localX := rem % 16

			blockX := int(region.X)*16 + localX
			blockY := int(region.Y)*16 + localY
			blockZ := int(region.Z)*16 + localZ

			if blockX > maxBlockX {
				maxBlockX = blockX
			}
			if blockY > maxBlockY {
				maxBlockY = blockY
			}
			if blockZ > maxBlockZ {
				maxBlockZ = blockZ
			}
		}
	}

	width := 0
	height := 0
	length := 0
	if maxBlockX >= 0 {
		width = maxBlockX + 1
	}
	if maxBlockY >= 0 {
		height = maxBlockY + 1
	}
	if maxBlockZ >= 0 {
		length = maxBlockZ + 1
	}

	a.size.Width = width
	a.size.Height = height
	a.size.Length = length
	a.originalSize.Width = width
	a.originalSize.Height = height
	a.originalSize.Length = length
	a.nonAirBlocks = nonAirBlocks
	a.sizeComputed = true
	a.nonAirCountComputed = true
	return nil
}

func (a *AxiomBP) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(num int),
	progressCallback func(),
) error {
	// 直接使用所有Region, 按DataOffset排序（最快顺序）
	targetRegions := make([]*AxiomRegion, len(a.regions))
	copy(targetRegions, a.regions)
	sort.Slice(targetRegions, func(i, j int) bool {
		return targetRegions[i].DataOffset < targetRegions[j].DataOffset
	})

	// 回调总进度数（每个Region对应1个进度单位）
	if startCallback != nil {
		startCallback(len(targetRegions))
	}

	// 重新打开文件准备读取
	file, err := os.Open(a.file.Name())
	if err != nil {
		return fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	// 定位到区块数据起始位置
	if _, err := file.Seek(a.blockDataStart, io.SeekStart); err != nil {
		return fmt.Errorf("定位到方块数据失败: %w", err)
	}

	// 创建gzip读取器
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("创建 gzip 读取器失败: %w", err)
	}
	defer gzipReader.Close()

	offsetReader := nbt.NewOffsetReader(gzipReader)

	// 遍历排序后的Region
	for _, region := range targetRegions {
		// 定位到当前Region的BlockStates.data位置
		currentOffset := offsetReader.GetOffset()
		if currentOffset < region.DataOffset {
			skipBytes := region.DataOffset - currentOffset
			if _, err := io.CopyN(io.Discard, offsetReader, skipBytes); err != nil {
				return fmt.Errorf("定位到区域数据失败 (X:%d,Y:%d,Z:%d): %w", region.X, region.Y, region.Z, err)
			}
		}

		// 读取LongArray长度
		var lenBuf [4]byte
		if _, err := io.ReadFull(offsetReader, lenBuf[:]); err != nil {
			return fmt.Errorf("读取区域数据长度失败 (X:%d,Y:%d,Z:%d): %w", region.X, region.Y, region.Z, err)
		}
		numLongs := int(int32(binary.BigEndian.Uint32(lenBuf[:])))
		if numLongs < 0 {
			return fmt.Errorf("区域数据长度无效 (X:%d,Y:%d,Z:%d): %d", region.X, region.Y, region.Z, numLongs)
		}

		// 读取LongArray数据
		longs := make([]uint64, numLongs)
		for i := 0; i < numLongs; i++ {
			var longBuf [8]byte
			if _, err := io.ReadFull(offsetReader, longBuf[:]); err != nil {
				return fmt.Errorf("读取区域 long 数据失败 (X:%d,Y:%d,Z:%d, index:%d): %w", region.X, region.Y, region.Z, i, err)
			}
			longs[i] = binary.BigEndian.Uint64(longBuf[:])
		}

		// 解析调色板和位数据
		paletteSize := len(region.Palette)
		if paletteSize == 0 {
			if progressCallback != nil {
				go progressCallback()
			}
			continue
		}

		bitsPerBlock := int(math.Ceil(math.Log2(float64(paletteSize))))
		if bitsPerBlock < 4 {
			bitsPerBlock = 4
		}
		valuesPerLong := 64 / bitsPerBlock
		mask := uint64((1 << bitsPerBlock) - 1)
		expectedEntries := 4096 // 16x16x16子区块固定大小

		// 创建当前Region对应的子区块（16x16x16）
		subChunk := chunk.NewSubChunk(block.AirRuntimeID)

		// 遍历Region内所有方块
		for blockIndex := 0; blockIndex < expectedEntries; blockIndex++ {
			// 计算当前方块在long数组中的位置
			longIndex := blockIndex / valuesPerLong
			bitIndex := (blockIndex % valuesPerLong) * bitsPerBlock
			if longIndex >= len(longs) {
				continue
			}

			// 提取调色板索引
			paletteIdx := (longs[longIndex] >> bitIndex) & mask
			if int32(paletteIdx) >= int32(paletteSize) {
				continue
			}

			// 获取方块运行时ID
			blockRuntimeID, ok := region.Palette[int32(paletteIdx)]
			if !ok || blockRuntimeID == block.AirRuntimeID {
				continue
			}

			// 计算方块在子区块内的局部坐标（0-15）
			localY := byte(blockIndex / 256) // 256 = 16*16
			rem := blockIndex % 256
			localZ := byte(rem / 16)
			localX := byte(rem % 16)

			// 设置方块到子区块
			subChunk.SetBlock(localX, localY, localZ, 0, blockRuntimeID)
		}

		// 计算子区块在世界中的位置（叠加起始偏移）
		subChunkPos := bwo_define.SubChunkPos{
			region.X + startSubChunkPos.X(),
			region.Y + startSubChunkPos.Y(),
			region.Z + startSubChunkPos.Z(),
		}

		// 保存子区块到世界（默认主世界维度）
		if err := bedrockWorld.SaveSubChunk(bwo_define.DimensionIDOverworld, subChunkPos, subChunk); err != nil {
			return fmt.Errorf("保存子区块 %v 失败: %w", subChunkPos, err)
		}

		// 触发进度回调
		if progressCallback != nil {
			go progressCallback()
		}
	}

	return nil
}

func packBlockIndices(indices []int32, paletteSize int) ([]int64, error) {
	if paletteSize <= 0 {
		return nil, fmt.Errorf("调色板为空")
	}
	bitsPerBlock := 4
	if paletteSize > 1 {
		bitsPerBlock = int(math.Ceil(math.Log2(float64(paletteSize))))
		if bitsPerBlock < 4 {
			bitsPerBlock = 4
		}
	}
	valuesPerLong := 64 / bitsPerBlock
	if valuesPerLong <= 0 {
		valuesPerLong = 1
	}
	longCount := int(math.Ceil(float64(len(indices)) / float64(valuesPerLong)))
	if longCount <= 0 {
		longCount = 1
	}
	packed := make([]uint64, longCount)
	mask := uint64((1 << bitsPerBlock) - 1)
	for i, paletteIndex := range indices {
		if paletteIndex < 0 {
			paletteIndex = 0
		}
		longIndex := i / valuesPerLong
		bitIndex := (i % valuesPerLong) * bitsPerBlock
		packed[longIndex] |= (uint64(paletteIndex) & mask) << bitIndex
	}
	result := make([]int64, len(packed))
	for i, v := range packed {
		result[i] = int64(v)
	}
	return result, nil
}

func convertRuntimeIDToAxiomPaletteState(runtimeID uint32) (string, map[string]any) {
	name, properties, found := blocks.RuntimeIDToJavaBlockNameAndState(runtimeID)
	if !found || name == "" {
		baseName, bedrockProps, ok := block.RuntimeIDToState(runtimeID)
		if ok {
			javaName, javaProps, converted := blocks.BedrockBlockNameAndStateToJavaBlock(baseName, bedrockProps)
			if converted {
				name = javaName
				properties = javaProps
			}
		}
	}
	if name == "" {
		name = "minecraft:air"
	}
	if !strings.Contains(name, ":") {
		name = "minecraft:" + name
	}
	if properties == nil {
		properties = map[string]any{}
	}
	return name, properties
}

func buildAxiomMetadata(name, author string, blockCount int32, containsAir bool) ([]byte, error) {
	metaBuffer := bytes.NewBuffer(nil)
	tagWriter := nbt.NewTagWriter(nbt.BigEndian)
	offsetWriter := nbt.NewOffsetWriter(metaBuffer)

	if err := tagWriter.WriteTag(offsetWriter, nbt.TagStruct, ""); err != nil {
		return nil, err
	}

	if err := tagWriter.WriteTag(offsetWriter, nbt.TagFloat32, "ThumbnailYaw"); err != nil {
		return nil, err
	}
	if err := tagWriter.WriteTagFloat32(offsetWriter, 0); err != nil {
		return nil, err
	}

	if err := tagWriter.WriteTag(offsetWriter, nbt.TagByte, "ContainsAir"); err != nil {
		return nil, err
	}
	containsAirByte := byte(0)
	if containsAir {
		containsAirByte = 1
	}
	if err := tagWriter.WriteTagByte(offsetWriter, containsAirByte); err != nil {
		return nil, err
	}

	if err := tagWriter.WriteTag(offsetWriter, nbt.TagInt32, "Version"); err != nil {
		return nil, err
	}
	if err := tagWriter.WriteTagInt32(offsetWriter, 2); err != nil {
		return nil, err
	}

	if err := tagWriter.WriteTag(offsetWriter, nbt.TagByte, "LockedThumbnail"); err != nil {
		return nil, err
	}
	if err := tagWriter.WriteTagByte(offsetWriter, 0); err != nil {
		return nil, err
	}

	if err := tagWriter.WriteTag(offsetWriter, nbt.TagInt32, "BlockCount"); err != nil {
		return nil, err
	}
	if err := tagWriter.WriteTagInt32(offsetWriter, blockCount); err != nil {
		return nil, err
	}

	if err := tagWriter.WriteTag(offsetWriter, nbt.TagString, "Author"); err != nil {
		return nil, err
	}
	if err := tagWriter.WriteTagString(offsetWriter, author); err != nil {
		return nil, err
	}

	if err := tagWriter.WriteTag(offsetWriter, nbt.TagSlice, "Tags"); err != nil {
		return nil, err
	}
	if err := tagWriter.WriteTagType(offsetWriter, nbt.TagString); err != nil {
		return nil, err
	}
	if err := tagWriter.WriteTagInt32(offsetWriter, 0); err != nil {
		return nil, err
	}

	if err := tagWriter.WriteTag(offsetWriter, nbt.TagString, "Name"); err != nil {
		return nil, err
	}
	if err := tagWriter.WriteTagString(offsetWriter, name); err != nil {
		return nil, err
	}

	if err := tagWriter.WriteTag(offsetWriter, nbt.TagFloat32, "ThumbnailPitch"); err != nil {
		return nil, err
	}
	if err := tagWriter.WriteTagFloat32(offsetWriter, 0); err != nil {
		return nil, err
	}

	if err := tagWriter.WriteTag(offsetWriter, nbt.TagEnd, ""); err != nil {
		return nil, err
	}
	return metaBuffer.Bytes(), nil
}

func (a *AxiomBP) Close() error {
	return nil
}
