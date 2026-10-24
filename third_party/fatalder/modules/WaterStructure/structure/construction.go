package structure

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
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
)

var constructionMagic = []byte("constrct")

// 仅保留解析用结构体，不缓存实例
type constructionSection struct {
	startX int
	startY int
	startZ int
	shapeX int
	shapeY int
	shapeZ int
	blocks []int32
}

type constructionBlockEntity struct {
	x    int
	y    int
	z    int
	data map[string]any
}

type constructionIndexEntry struct {
	startX   int
	startY   int
	startZ   int
	shapeX   int
	shapeY   int
	shapeZ   int
	position int64
	length   int64
}

type Construction struct {
	BaseReader
	file           *os.File
	size           *define.Size
	originalSize   *define.Size
	offsetPos      define.Offset
	selectionMin   define.BlockPos
	formatVersion  uint8
	sectionVersion uint8
	palette        map[int32]uint32
	sectionsIndex  []constructionIndexEntry // 仅存区段索引，不缓存全量数据
}

func (c *Construction) ID() uint8 {
	return IDConstruction
}

func (c *Construction) Name() string {
	return NameConstruction
}

func (c *Construction) FromFile(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	magic := make([]byte, len(constructionMagic))
	if _, err := io.ReadFull(file, magic); err != nil {
		return fmt.Errorf("读取 construction 魔数失败: %w", err)
	}
	if !bytes.Equal(magic, constructionMagic) {
		return ErrInvalidFile
	}

	versionByte := make([]byte, 1)
	if _, err := io.ReadFull(file, versionByte); err != nil {
		return fmt.Errorf("读取 construction 版本失败: %w", err)
	}
	if versionByte[0] != 0 {
		return fmt.Errorf("不支持的 construction 格式版本: %d", versionByte[0])
	}
	c.formatVersion = versionByte[0]

	if _, err := file.Seek(int64(-len(constructionMagic)), io.SeekEnd); err != nil {
		return fmt.Errorf("定位尾部魔数失败: %w", err)
	}
	tail := make([]byte, len(constructionMagic))
	if _, err := io.ReadFull(file, tail); err != nil {
		return fmt.Errorf("读取尾部魔数失败: %w", err)
	}
	if !bytes.Equal(tail, constructionMagic) {
		return ErrInvalidFile
	}

	metadataEnd, err := file.Seek(int64(-len(constructionMagic))-4, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("定位元数据指针失败: %w", err)
	}
	pointerBuf := make([]byte, 4)
	if _, err := io.ReadFull(file, pointerBuf); err != nil {
		return fmt.Errorf("读取元数据指针失败: %w", err)
	}
	metadataStart := int64(binary.BigEndian.Uint32(pointerBuf))
	if metadataStart < 0 || metadataStart > metadataEnd {
		return ErrInvalidFile
	}
	metadataLen := metadataEnd - metadataStart
	if metadataLen <= 0 {
		return ErrInvalidFile
	}
	if _, err := file.Seek(metadataStart, io.SeekStart); err != nil {
		return fmt.Errorf("定位元数据失败: %w", err)
	}
	metadataCompressed := make([]byte, metadataLen)
	if _, err := io.ReadFull(file, metadataCompressed); err != nil {
		return fmt.Errorf("读取元数据失败: %w", err)
	}
	metadataBytes, err := maybeDecompress(metadataCompressed)
	if err != nil {
		return fmt.Errorf("解压元数据失败: %w", err)
	}

	// 仅加载区段索引，不加载全量区段数据
	entries, err := c.loadMetadata(metadataBytes)
	if err != nil {
		return err
	}
	c.sectionsIndex = entries
	c.file = file
	return nil
}

func (c *Construction) loadMetadata(data []byte) ([]constructionIndexEntry, error) {
	offsetReader := nbt.NewOffsetReader(bytes.NewReader(data))
	tagReader := nbt.NewTagReader(nbt.BigEndian)

	rootType, _, err := tagReader.ReadTag(offsetReader)
	if err != nil {
		return nil, fmt.Errorf("读取元数据根标签失败: %w", err)
	}
	if rootType != nbt.TagStruct {
		return nil, ErrInvalidFile
	}

	var (
		selectionBoxes []int32
		paletteList    []any
		sectionIndex   []byte
		sectionVersion byte
	)

	for {
		tagType, tagName, err := tagReader.ReadTag(offsetReader)
		if err != nil {
			return nil, fmt.Errorf("读取元数据标签失败: %w", err)
		}
		if tagType == nbt.TagEnd {
			break
		}

		switch tagName {
		case "selection_boxes":
			if tagType != nbt.TagInt32Array {
				return nil, fmt.Errorf("selection_boxes 必须为 TAG_IntArray, 实际为 %s", tagType)
			}
			selectionBoxes, err = tagReader.ReadTagInt32Array(offsetReader)
			if err != nil {
				return nil, fmt.Errorf("读取 selection_boxes 失败: %w", err)
			}

		case "block_palette":
			if tagType != nbt.TagSlice {
				return nil, fmt.Errorf("block_palette 必须为 TAG_List, 实际为 %s", tagType)
			}
			paletteList, err = tagReader.ReadTagList(offsetReader)
			if err != nil {
				return nil, fmt.Errorf("读取 block_palette 失败: %w", err)
			}

		case "section_index_table":
			if tagType != nbt.TagByteArray {
				return nil, fmt.Errorf("section_index_table 必须为 TAG_ByteArray, 实际为 %s", tagType)
			}
			sectionIndex, err = tagReader.ReadTagByteArray(offsetReader)
			if err != nil {
				return nil, fmt.Errorf("读取 section_index_table 失败: %w", err)
			}

		case "section_version":
			if tagType != nbt.TagByte {
				return nil, fmt.Errorf("section_version 必须为 TAG_Byte, 实际为 %s", tagType)
			}
			sectionVersion, err = tagReader.ReadTagByte(offsetReader)
			if err != nil {
				return nil, fmt.Errorf("读取 section_version 失败: %w", err)
			}

		default:
			if err := tagReader.SkipTagValue(offsetReader, tagType); err != nil {
				return nil, fmt.Errorf("跳过元数据标签 %s 失败: %w", tagName, err)
			}
		}
	}

	if len(sectionIndex) == 0 {
		return nil, ErrInvalidFile
	}
	if len(paletteList) == 0 {
		return nil, ErrInvalidFile
	}

	entries, err := parseConstructionIndex(sectionIndex)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, ErrInvalidFile
	}

	minX, minY, minZ := math.MaxInt32, math.MaxInt32, math.MaxInt32
	maxX, maxY, maxZ := math.MinInt32, math.MinInt32, math.MinInt32

	if len(selectionBoxes) >= 6 {
		for i := 0; i+5 < len(selectionBoxes); i += 6 {
			minX = min(minX, int(selectionBoxes[i+0]))
			minY = min(minY, int(selectionBoxes[i+1]))
			minZ = min(minZ, int(selectionBoxes[i+2]))
			maxX = max(maxX, int(selectionBoxes[i+3]))
			maxY = max(maxY, int(selectionBoxes[i+4]))
			maxZ = max(maxZ, int(selectionBoxes[i+5]))
		}
	}

	if maxX <= minX || maxY <= minY || maxZ <= minZ {
		minX, minY, minZ = math.MaxInt32, math.MaxInt32, math.MaxInt32
		maxX, maxY, maxZ = math.MinInt32, math.MinInt32, math.MinInt32
		for _, entry := range entries {
			if entry.shapeX <= 0 || entry.shapeY <= 0 || entry.shapeZ <= 0 {
				continue
			}
			minX = min(minX, entry.startX)
			minY = min(minY, entry.startY)
			minZ = min(minZ, entry.startZ)
			maxX = max(maxX, entry.startX+entry.shapeX)
			maxY = max(maxY, entry.startY+entry.shapeY)
			maxZ = max(maxZ, entry.startZ+entry.shapeZ)
		}
	}

	if maxX <= minX || maxY <= minY || maxZ <= minZ {
		return nil, ErrInvalidFile
	}

	c.selectionMin = define.BlockPos{int32(minX), int32(minY), int32(minZ)}
	c.originalSize = &define.Size{Width: maxX - minX, Height: maxY - minY, Length: maxZ - minZ}
	c.size = &define.Size{Width: maxX - minX, Height: maxY - minY, Length: maxZ - minZ}
	c.offsetPos = define.Offset{}
	c.sectionVersion = sectionVersion

	c.palette = make(map[int32]uint32, len(paletteList))
	for i, raw := range paletteList {
		entryMap, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("调色板条目 %d 类型为 %T（不支持）", i, raw)
		}
		namespace, ok := entryMap["namespace"].(string)
		if !ok {
			return nil, fmt.Errorf("调色板条目 %d 缺少命名空间", i)
		}
		blockName, ok := entryMap["blockname"].(string)
		if !ok {
			return nil, fmt.Errorf("调色板条目 %d 缺少方块名", i)
		}
		propsRaw, ok := entryMap["properties"].(map[string]any)
		if !ok {
			propsRaw = map[string]any{}
		}
		normalizedProps := normalizeNBTMap(propsRaw)
		delete(normalizedProps, "__version__")
		runtimeID, found := block.StateToRuntimeID(fmt.Sprintf("%s:%s", namespace, blockName), normalizedProps)
		if !found {
			runtimeID = UnknownBlockRuntimeID
		}
		c.palette[int32(i)] = runtimeID
	}

	return entries, nil
}

func parseConstructionIndex(data []byte) ([]constructionIndexEntry, error) {
	const entrySize = 23
	if len(data)%entrySize != 0 {
		return nil, fmt.Errorf("区段索引长度无效: %d", len(data))
	}
	count := len(data) / entrySize
	entries := make([]constructionIndexEntry, 0, count)
	for i := 0; i < len(data); i += entrySize {
		sx := int(int32(binary.LittleEndian.Uint32(data[i+0 : i+4])))
		sy := int(int32(binary.LittleEndian.Uint32(data[i+4 : i+8])))
		sz := int(int32(binary.LittleEndian.Uint32(data[i+8 : i+12])))
		shapex := int(uint8(data[i+12]))
		shapey := int(uint8(data[i+13]))
		shapez := int(uint8(data[i+14]))
		position := int64(int32(binary.LittleEndian.Uint32(data[i+15 : i+19])))
		length := int64(int32(binary.LittleEndian.Uint32(data[i+19 : i+23])))
		entries = append(entries, constructionIndexEntry{
			startX:   sx,
			startY:   sy,
			startZ:   sz,
			shapeX:   shapex,
			shapeY:   shapey,
			shapeZ:   shapez,
			position: position,
			length:   length,
		})
	}
	return entries, nil
}

func readSectionData(file *os.File, offset int64, length int64) ([]byte, error) {
	if offset < 0 || length <= 0 {
		return nil, ErrInvalidFile
	}
	buf := make([]byte, length)
	if _, err := file.ReadAt(buf, offset); err != nil {
		return nil, err
	}
	return buf, nil
}

func parseConstructionSection(data []byte) (constructionSection, []constructionBlockEntity, error) {
	section := constructionSection{}
	blockEntities := make([]constructionBlockEntity, 0)

	offsetReader := nbt.NewOffsetReader(bytes.NewReader(data))
	tagReader := nbt.NewTagReader(nbt.BigEndian)

	rootType, _, err := tagReader.ReadTag(offsetReader)
	if err != nil {
		return section, nil, fmt.Errorf("读取区段根标签失败: %w", err)
	}
	if rootType != nbt.TagStruct {
		return section, nil, ErrInvalidFile
	}

	var (
		blocksArrayType int8 = -1
		rawBlocks       any
		blockCount      int
	)

	for {
		tagType, tagName, err := tagReader.ReadTag(offsetReader)
		if err != nil {
			return section, nil, fmt.Errorf("读取区段标签失败: %w", err)
		}
		if tagType == nbt.TagEnd {
			break
		}

		switch tagName {
		case "blocks_array_type":
			if tagType != nbt.TagByte {
				return section, nil, fmt.Errorf("blocks_array_type 必须为 TAG_Byte, 实际为 %s", tagType)
			}
			val, err := tagReader.ReadTagByte(offsetReader)
			if err != nil {
				return section, nil, fmt.Errorf("读取 blocks_array_type 失败: %w", err)
			}
			blocksArrayType = int8(val)

		case "blocks":
			rawBlocks, err = tagReader.ReadTagValue(offsetReader, tagType)
			if err != nil {
				return section, nil, fmt.Errorf("读取 blocks 失败: %w", err)
			}

		case "block_entities":
			if tagType != nbt.TagSlice {
				return section, nil, fmt.Errorf("block_entities 必须为 TAG_List, 实际为 %s", tagType)
			}
			list, err := tagReader.ReadTagList(offsetReader)
			if err != nil {
				return section, nil, fmt.Errorf("读取 block_entities 失败: %w", err)
			}
			for _, raw := range list {
				entityMap, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				x, errX := toInt(entityMap["x"])
				y, errY := toInt(entityMap["y"])
				z, errZ := toInt(entityMap["z"])
				if errX != nil || errY != nil || errZ != nil {
					continue
				}
				nbtRaw, ok := entityMap["nbt"].(map[string]any)
				if !ok {
					nbtRaw = map[string]any{}
				}
				normalized := normalizeNBTMap(nbtRaw)
				namespace, _ := entityMap["namespace"].(string)
				baseName, _ := entityMap["base_name"].(string)
				if _, exists := normalized["id"]; !exists {
					if baseName != "" {
						if namespace != "" {
							normalized["id"] = namespace + ":" + baseName
						} else {
							normalized["id"] = baseName
						}
					}
				}
				blockEntities = append(blockEntities, constructionBlockEntity{
					x:    x,
					y:    y,
					z:    z,
					data: normalized,
				})
			}

		case "shape":
			if tagType != nbt.TagSlice {
				if err := tagReader.SkipTagValue(offsetReader, tagType); err != nil {
					return section, nil, fmt.Errorf("跳过 shape 标签失败: %w", err)
				}
				continue
			}
			shapeList, err := tagReader.ReadTagList(offsetReader)
			if err != nil {
				return section, nil, fmt.Errorf("读取 shape 失败: %w", err)
			}
			if len(shapeList) == 3 {
				section.shapeX, _ = toInt(shapeList[0])
				section.shapeY, _ = toInt(shapeList[1])
				section.shapeZ, _ = toInt(shapeList[2])
			}

		case "size":
			if tagType != nbt.TagSlice {
				if err := tagReader.SkipTagValue(offsetReader, tagType); err != nil {
					return section, nil, fmt.Errorf("跳过 size 标签失败: %w", err)
				}
				continue
			}
			sizeList, err := tagReader.ReadTagList(offsetReader)
			if err != nil {
				return section, nil, fmt.Errorf("读取 size 失败: %w", err)
			}
			if len(sizeList) == 3 && (section.shapeX == 0 || section.shapeY == 0 || section.shapeZ == 0) {
				section.shapeX, _ = toInt(sizeList[0])
				section.shapeY, _ = toInt(sizeList[1])
				section.shapeZ, _ = toInt(sizeList[2])
			}

		default:
			if err := tagReader.SkipTagValue(offsetReader, tagType); err != nil {
				return section, nil, fmt.Errorf("跳过区段标签 %s 失败: %w", tagName, err)
			}
		}
	}

	if blocksArrayType == -1 || rawBlocks == nil {
		return section, blockEntities, nil
	}

	converted, total, err := convertBlockArray(blocksArrayType, rawBlocks)
	if err != nil {
		return section, nil, err
	}
	blockCount = total
	section.blocks = converted

	if section.shapeX == 0 || section.shapeY == 0 || section.shapeZ == 0 {
		section.shapeX = 0
		section.shapeY = 0
		section.shapeZ = 0
	}

	if section.shapeX > 0 && section.shapeY > 0 && section.shapeZ > 0 {
		expected := section.shapeX * section.shapeY * section.shapeZ
		if blockCount != expected {
			return section, nil, fmt.Errorf("区段内方块数量不匹配: 实际 %d, 期望 %d", blockCount, expected)
		}
	}

	return section, blockEntities, nil
}

func convertBlockArray(arrayType int8, raw any) ([]int32, int, error) {
	switch arrayType {
	case 7:
		data, ok := raw.([]byte)
		if !ok {
			return nil, 0, fmt.Errorf("期望类型为 []byte（blocks_array_type 7）, 实际为 %T", raw)
		}
		result := make([]int32, len(data))
		for i, v := range data {
			result[i] = int32(v)
		}
		return result, len(result), nil
	case 11:
		data, ok := raw.([]int32)
		if !ok {
			return nil, 0, fmt.Errorf("期望类型为 []int32（blocks_array_type 11）, 实际为 %T", raw)
		}
		result := make([]int32, len(data))
		copy(result, data)
		return result, len(result), nil
	case 12:
		data, ok := raw.([]int64)
		if !ok {
			return nil, 0, fmt.Errorf("期望类型为 []int64（blocks_array_type 12）, 实际为 %T", raw)
		}
		result := make([]int32, len(data))
		for i, v := range data {
			result[i] = int32(v)
		}
		return result, len(result), nil
	default:
		return nil, 0, fmt.Errorf("不支持的 blocks_array_type: %d", arrayType)
	}
}

func maybeDecompress(data []byte) ([]byte, error) {
	if len(data) >= 2 {
		if data[0] == 0x1f && data[1] == 0x8b {
			reader, err := gzip.NewReader(bytes.NewReader(data))
			if err != nil {
				return nil, err
			}
			defer reader.Close()
			return io.ReadAll(reader)
		}
		if data[0] == 0x78 {
			reader, err := zlib.NewReader(bytes.NewReader(data))
			if err == nil {
				defer reader.Close()
				return io.ReadAll(reader)
			}
		}
	}
	return data, nil
}

func normalizeNBTMap(m map[string]any) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = normalizeNBTValue(v)
	}
	return result
}

func normalizeNBTValue(v any) any {
	switch val := v.(type) {
	case int8:
		return int32(val)
	case int16:
		return int32(val)
	case int32:
		return val
	case int64:
		return int32(val)
	case uint8:
		return int32(val)
	case uint16:
		return int32(val)
	case uint32:
		return int32(val)
	case uint64:
		return int32(val)
	case float32:
		return float64(val)
	case float64:
		return val
	case bool:
		return val
	case string:
		return val
	case []byte:
		dup := make([]byte, len(val))
		copy(dup, val)
		return dup
	case []int32:
		dup := make([]int32, len(val))
		copy(dup, val)
		return dup
	case []int64:
		dup := make([]int64, len(val))
		copy(dup, val)
		return dup
	case []any:
		dup := make([]any, len(val))
		for i, item := range val {
			dup[i] = normalizeNBTValue(item)
		}
		return dup
	case map[string]any:
		return normalizeNBTMap(val)
	default:
		return val
	}
}

func (c *Construction) runtimeIDFor(index int32) uint32 {
	if runtimeID, ok := c.palette[index]; ok {
		return runtimeID
	}
	return UnknownBlockRuntimeID
}

func (c *Construction) GetOffsetPos() define.Offset {
	return c.offsetPos
}

func (c *Construction) SetOffsetPos(offset define.Offset) {
	c.offsetPos = offset
	c.size.Width = c.originalSize.Width + int(math.Abs(float64(offset.X())))
	c.size.Length = c.originalSize.Length + int(math.Abs(float64(offset.Z())))
	c.size.Height = c.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (c *Construction) GetSize() define.Size {
	return *c.size
}

// 流式读取区段，不缓存全量数据
func (c *Construction) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	result := make(map[define.ChunkPos]*chunk.Chunk, len(posList))
	height := c.size.Height
	if height <= 0 {
		height = 1
	}
	for _, pos := range posList {
		if _, exists := result[pos]; !exists {
			result[pos] = chunk.NewChunk(block.AirRuntimeID, MCWorldOverworldRange)
		}
	}
	if len(result) == 0 {
		return result, nil
	}

	required := make(map[[2]int32]struct{}, len(result))
	for pos := range result {
		required[[2]int32{pos.X(), pos.Z()}] = struct{}{}
	}

	offsetX := int(c.offsetPos.X())
	offsetY := int(c.offsetPos.Y())
	offsetZ := int(c.offsetPos.Z())
	minX := int(c.selectionMin.X())
	minY := int(c.selectionMin.Y())
	minZ := int(c.selectionMin.Z())

	// 遍历区段索引，按需读取单个区段
	for _, entry := range c.sectionsIndex {
		if entry.length <= 0 || entry.shapeX <= 0 || entry.shapeY <= 0 || entry.shapeZ <= 0 {
			continue
		}
		// 读取当前区段数据（用完即释放）
		sectionData, err := readSectionData(c.file, entry.position, entry.length)
		if err != nil {
			return nil, fmt.Errorf("读取区段失败: %w", err)
		}
		sectionBytes, err := maybeDecompress(sectionData)
		if err != nil {
			return nil, fmt.Errorf("解压区段失败: %w", err)
		}
		section, _, err := parseConstructionSection(sectionBytes)
		if err != nil || len(section.blocks) == 0 {
			continue
		}

		// 计算区段相对坐标
		section.startX = entry.startX - minX
		section.startY = entry.startY - minY
		section.startZ = entry.startZ - minZ
		section.shapeX = entry.shapeX
		section.shapeY = entry.shapeY
		section.shapeZ = entry.shapeZ

		// 处理方块数据
		for x := 0; x < section.shapeX; x++ {
			for y := 0; y < section.shapeY; y++ {
				for z := 0; z < section.shapeZ; z++ {
					idx := (x*section.shapeY+y)*section.shapeZ + z
					paletteIndex := section.blocks[idx]
					runtimeID := c.runtimeIDFor(paletteIndex)
					if runtimeID == block.AirRuntimeID {
						continue
					}

					worldX := section.startX + x + offsetX
					worldY := section.startY + y + offsetY
					worldZ := section.startZ + z + offsetZ

					chunkX := floorDiv(worldX, 16)
					chunkZ := floorDiv(worldZ, 16)

					if _, ok := required[[2]int32{int32(chunkX), int32(chunkZ)}]; !ok {
						continue
					}

					chunkPos := define.ChunkPos{int32(chunkX), int32(chunkZ)}
					cChunk, ok := result[chunkPos]
					if !ok {
						continue
					}

					localX := worldX - chunkX*16
					localZ := worldZ - chunkZ*16
					cChunk.SetBlock(uint8(localX), int16(worldY)-64, uint8(localZ), 0, runtimeID)
				}
			}
		}
	}

	return result, nil
}

// 流式读取实体数据，不缓存全量
func (c *Construction) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any, len(posList))
	required := make(map[[2]int32]struct{}, len(posList))
	for _, pos := range posList {
		if _, exists := result[pos]; !exists {
			result[pos] = make(map[define.BlockPos]map[string]any)
		}
		required[[2]int32{pos.X(), pos.Z()}] = struct{}{}
	}

	offsetX := int(c.offsetPos.X())
	offsetY := int(c.offsetPos.Y())
	offsetZ := int(c.offsetPos.Z())
	minX := int(c.selectionMin.X())
	minY := int(c.selectionMin.Y())
	minZ := int(c.selectionMin.Z())

	// 遍历区段索引，按需读取实体
	for _, entry := range c.sectionsIndex {
		if entry.length <= 0 {
			continue
		}
		sectionData, err := readSectionData(c.file, entry.position, entry.length)
		if err != nil {
			return nil, fmt.Errorf("读取区段实体失败: %w", err)
		}
		sectionBytes, err := maybeDecompress(sectionData)
		if err != nil {
			return nil, fmt.Errorf("解压区段实体失败: %w", err)
		}
		_, blockEntities, err := parseConstructionSection(sectionBytes)
		if err != nil || len(blockEntities) == 0 {
			continue
		}

		// 处理实体坐标转换
		for _, be := range blockEntities {
			// 转换为相对原始尺寸的坐标
			be.x -= minX
			be.y -= minY
			be.z -= minZ

			worldX := be.x + offsetX
			worldY := be.y + offsetY
			worldZ := be.z + offsetZ

			chunkX := floorDiv(worldX, 16)
			chunkZ := floorDiv(worldZ, 16)
			if _, ok := required[[2]int32{int32(chunkX), int32(chunkZ)}]; !ok {
				continue
			}

			chunkPos := define.ChunkPos{int32(chunkX), int32(chunkZ)}
			chunkNBT, ok := result[chunkPos]
			if !ok {
				continue
			}

			localX := worldX - chunkX*16
			localZ := worldZ - chunkZ*16
			blockPos := define.BlockPos{int32(localX), chunkLocalYFromWorld(worldY), int32(localZ)}
			chunkNBT[blockPos] = be.data
		}
	}

	return result, nil
}

// 流式统计非空气块，不缓存全量数据
func (c *Construction) CountNonAirBlocks() (int, error) {
	nonAirBlocks := 0

	for _, entry := range c.sectionsIndex {
		if entry.length <= 0 || entry.shapeX <= 0 || entry.shapeY <= 0 || entry.shapeZ <= 0 {
			continue
		}
		sectionData, err := readSectionData(c.file, entry.position, entry.length)
		if err != nil {
			return nonAirBlocks, fmt.Errorf("读取区段统计失败: %w", err)
		}
		sectionBytes, err := maybeDecompress(sectionData)
		if err != nil {
			return nonAirBlocks, fmt.Errorf("解压区段统计失败: %w", err)
		}
		section, _, err := parseConstructionSection(sectionBytes)
		if err != nil || len(section.blocks) == 0 {
			continue
		}

		// 统计当前区段非空气块
		for _, idx := range section.blocks {
			if runtimeID := c.runtimeIDFor(idx); runtimeID != block.AirRuntimeID {
				nonAirBlocks++
			}
		}
	}

	return nonAirBlocks, nil
}

// 实现ToMCWorld，流式写入世界，不缓存全量数据
func (c *Construction) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(num int),
	progressCallback func(),
) error {
	width := c.originalSize.Width
	length := c.originalSize.Length
	height := c.originalSize.Height
	totalVolume := width * length * height

	if totalVolume == 0 {
		if startCallback != nil {
			startCallback(0)
		}
		return nil
	}

	// 计算区块数量，初始化回调
	chunkXNum := (width + 15) / 16
	chunkZNum := (length + 15) / 16
	subChunkYNum := (height + 15) / 16
	layerSubChunkNum := chunkZNum * subChunkYNum
	if startCallback != nil {
		startCallback(chunkXNum)
	}

	minX := int(c.selectionMin.X())
	minY := int(c.selectionMin.Y())
	minZ := int(c.selectionMin.Z())

	// 按X轴区块分批处理，降低内存占用
	for chunkX := 0; chunkX < chunkXNum; chunkX++ {
		subChunks := make([]*chunk.SubChunk, layerSubChunkNum)
		currentSubChunkWidth := min(16, width-chunkX*16)
		startX := chunkX * 16

		// 遍历所有区段，筛选当前X区块范围内的区段
		for _, entry := range c.sectionsIndex {
			if entry.length <= 0 || entry.shapeX <= 0 || entry.shapeY <= 0 || entry.shapeZ <= 0 {
				continue
			}
			// 转换为相对原始尺寸的区段坐标
			secRelX := entry.startX - minX
			secRelY := entry.startY - minY
			secRelZ := entry.startZ - minZ

			// 筛选当前X区块覆盖的区段
			if secRelX+entry.shapeX <= startX || secRelX >= startX+currentSubChunkWidth {
				continue
			}

			// 读取区段数据
			sectionData, err := readSectionData(c.file, entry.position, entry.length)
			if err != nil {
				return fmt.Errorf("读取区段写入失败: %w", err)
			}
			sectionBytes, err := maybeDecompress(sectionData)
			if err != nil {
				return fmt.Errorf("解压区段写入失败: %w", err)
			}
			section, _, err := parseConstructionSection(sectionBytes)
			if err != nil || len(section.blocks) == 0 {
				continue
			}

			// 遍历区段内方块，写入对应子区块
			for x := 0; x < entry.shapeX; x++ {
				localX := secRelX + x - startX
				if localX < 0 || localX >= currentSubChunkWidth {
					continue
				}
				for y := 0; y < entry.shapeY; y++ {
					absY := secRelY + y
					subChunkY := absY / 16
					localY := byte(absY % 16)
					for z := 0; z < entry.shapeZ; z++ {
						absZ := secRelZ + z
						chunkZ := absZ / 16
						localZ := byte(absZ % 16)

						// 计算方块索引，获取runtimeID
						idx := (x*entry.shapeY+y)*entry.shapeZ + z
						if idx >= len(section.blocks) {
							continue
						}
						paletteIndex := section.blocks[idx]
						runtimeID := c.runtimeIDFor(paletteIndex)
						if runtimeID == block.AirRuntimeID {
							continue
						}

						// 初始化子区块并写入方块
						subChunkIndex := subChunkY*chunkZNum + chunkZ
						if subChunks[subChunkIndex] == nil {
							subChunks[subChunkIndex] = chunk.NewSubChunk(block.AirRuntimeID)
						}
						subChunks[subChunkIndex].SetBlock(byte(localX), localY, localZ, 0, runtimeID)
					}
				}
			}
		}

		// 保存当前X区块的所有子区块
		for index, subChunk := range subChunks {
			if subChunk == nil {
				continue
			}
			chunkZ := index % chunkZNum
			subChunkY := index / chunkZNum
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

	// 处理实体NBT写入
	for chunkX := 0; chunkX < chunkXNum; chunkX++ {
		posList := make([]define.ChunkPos, 0, chunkZNum)
		for chunkZ := 0; chunkZ < chunkZNum; chunkZ++ {
			posList = append(posList, define.ChunkPos{int32(chunkX), int32(chunkZ)})
		}
		chunksNBT, err := c.GetChunksNBT(posList)
		if err != nil {
			return fmt.Errorf("获取区块 NBT 失败: %w", err)
		}
		for cpos, blockMap := range chunksNBT {
			bwoPos := bwo_define.ChunkPos{cpos.X(), cpos.Z()}
			list := make([]map[string]any, 0, len(blockMap))
			for bpos, n := range blockMap {
				if n == nil {
					continue
				}
				m := make(map[string]any, len(n)+3)
				for k, v := range n {
					m[k] = v
				}
				// 计算绝对坐标
				absX := int32(bwoPos.X()*16) + bpos.X() + startSubChunkPos.X()*16
				absY := bpos.Y() + startSubChunkPos.Y()*16
				absZ := int32(bwoPos.Z()*16) + bpos.Z() + startSubChunkPos.Z()*16
				m["x"] = absX
				m["y"] = absY
				m["z"] = absZ
				list = append(list, m)
			}
			if len(list) > 0 {
				err := bedrockWorld.SaveNBT(
					bwo_define.DimensionIDOverworld,
					bwo_define.ChunkPos{
						cpos.X() + startSubChunkPos.X(),
						cpos.Z() + startSubChunkPos.Z(),
					},
					list,
				)
				if err != nil {
					return fmt.Errorf("保存区块 NBT 失败: %w", err)
				}
			}
		}
	}

	return nil
}

func (c *Construction) Close() error {
	return nil
}
