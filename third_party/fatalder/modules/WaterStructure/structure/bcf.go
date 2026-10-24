package structure

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/WaterStructure/utils"
)

type BCF struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset
	origin       define.Origin

	subChunkOffsets []int64
	paletteRuntime  map[uint32]uint32

	nonAirBlocks int
}

func (b *BCF) ID() uint8 {
	return IDBCF
}

func (b *BCF) Name() string {
	return NameBCF
}

func (b *BCF) FromFile(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	magic := make([]byte, 3)
	if _, err := io.ReadFull(file, magic); err != nil {
		return fmt.Errorf("读取 BCF 头部失败: %w", err)
	}
	if string(magic) != "BCF" {
		return ErrInvalidFile
	}

	version, err := readU8(file)
	if err != nil {
		return fmt.Errorf("读取 BCF 版本失败: %w", err)
	}
	_ = version

	// width,length,height
	_, err = readU16(file)
	if err != nil {
		return ErrInvalidFile
	}
	_, err = readU16(file)
	if err != nil {
		return ErrInvalidFile
	}
	_, err = readU16(file)
	if err != nil {
		return ErrInvalidFile
	}
	_, err = readU8(file) // subChunkBaseSize
	if err != nil {
		return ErrInvalidFile
	}
	_, err = readU64(file) // subChunkCount
	if err != nil {
		return ErrInvalidFile
	}

	subChunkOffsetsTableOffset, err := readU64(file)
	if err != nil {
		return ErrInvalidFile
	}
	paletteOffset, err := readU64(file)
	if err != nil {
		return ErrInvalidFile
	}
	blockTypeMapOffset, err := readU64(file)
	if err != nil {
		return ErrInvalidFile
	}
	stateNameMapOffset, err := readU64(file)
	if err != nil {
		return ErrInvalidFile
	}
	stateValueMapOffset, err := readU64(file)
	if err != nil {
		return ErrInvalidFile
	}

	stat, err := file.Stat()
	if err != nil {
		return err
	}
	fileSize := stat.Size()

	if subChunkOffsetsTableOffset <= 0 || int64(subChunkOffsetsTableOffset) >= fileSize {
		return ErrInvalidFile
	}

	if _, err := file.Seek(int64(subChunkOffsetsTableOffset), io.SeekStart); err != nil {
		return err
	}
	offsetCount, err := readU64(file)
	if err != nil {
		return err
	}
	if offsetCount > uint64(math.MaxInt32) {
		return ErrInvalidFile
	}
	subChunkOffsets := make([]int64, 0, int(offsetCount))
	for i := uint64(0); i < offsetCount; i++ {
		off, err := readU64(file)
		if err != nil {
			return err
		}
		subChunkOffsets = append(subChunkOffsets, int64(off))
	}

	typeMap, err := readBCFTypeMap(file, int64(blockTypeMapOffset), fileSize)
	if err != nil {
		return err
	}
	stateNameMap, err := readBCFStateNameMap(file, int64(stateNameMapOffset), fileSize)
	if err != nil {
		return err
	}
	stateValueMap, err := readBCFStateValueMap(file, int64(stateValueMapOffset), fileSize)
	if err != nil {
		return err
	}

	paletteRuntime, err := readBCFPaletteRuntime(file, int64(paletteOffset), fileSize, typeMap, stateNameMap, stateValueMap)
	if err != nil {
		return err
	}

	minX, minY, minZ := math.MaxInt, math.MaxInt, math.MaxInt
	maxX, maxY, maxZ := math.MinInt, math.MinInt, math.MinInt

	for _, off := range subChunkOffsets {
		if off <= 0 || off >= fileSize {
			continue
		}
		if _, err := file.Seek(off, io.SeekStart); err != nil {
			return err
		}
		if _, err := readU64(file); err != nil { // subchunkSize
			return err
		}
		ox, err := readI16(file)
		if err != nil {
			return err
		}
		oy, err := readI16(file)
		if err != nil {
			return err
		}
		oz, err := readI16(file)
		if err != nil {
			return err
		}
		regionCount, err := readU32(file)
		if err != nil {
			return err
		}
		for i := uint32(0); i < regionCount; i++ {
			pid, err := readU32(file)
			if err != nil {
				return err
			}
			x1, err := readI16(file)
			if err != nil {
				return err
			}
			y1, err := readI16(file)
			if err != nil {
				return err
			}
			z1, err := readI16(file)
			if err != nil {
				return err
			}
			x2, err := readI16(file)
			if err != nil {
				return err
			}
			y2, err := readI16(file)
			if err != nil {
				return err
			}
			z2, err := readI16(file)
			if err != nil {
				return err
			}

			rt := paletteRuntime[pid]
			if rt == 0 || rt == block.AirRuntimeID {
				continue
			}

			ax1 := int(ox) + min(int(x1), int(x2))
			ay1 := int(oy) + min(int(y1), int(y2))
			az1 := int(oz) + min(int(z1), int(z2))
			ax2 := int(ox) + max(int(x1), int(x2))
			ay2 := int(oy) + max(int(y1), int(y2))
			az2 := int(oz) + max(int(z1), int(z2))

			if ax1 < minX {
				minX = ax1
			}
			if ay1 < minY {
				minY = ay1
			}
			if az1 < minZ {
				minZ = az1
			}
			if ax2 > maxX {
				maxX = ax2
			}
			if ay2 > maxY {
				maxY = ay2
			}
			if az2 > maxZ {
				maxZ = az2
			}
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
	b.subChunkOffsets = subChunkOffsets
	b.paletteRuntime = paletteRuntime
	b.nonAirBlocks = -1

	return nil
}

func (b *BCF) GetOffsetPos() define.Offset {
	return b.offsetPos
}

func (b *BCF) SetOffsetPos(offset define.Offset) {
	b.offsetPos = offset
	b.size.Width = b.originalSize.Width + int(math.Abs(float64(offset.X())))
	b.size.Length = b.originalSize.Length + int(math.Abs(float64(offset.Z())))
	b.size.Height = b.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (b *BCF) GetSize() define.Size {
	return *b.size
}

func (b *BCF) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
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
		return nil, fmt.Errorf("BCF 文件未初始化")
	}

	file, err := os.Open(b.file.Name())
	if err != nil {
		return nil, fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	// skip header (we only need subchunk content)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	header := make([]byte, 4)
	if _, err := io.ReadFull(file, header); err != nil {
		return nil, err
	}
	if string(header[:3]) != "BCF" {
		return nil, ErrInvalidFile
	}

	offsetX := int(b.offsetPos.X())
	offsetY := int(b.offsetPos.Y())
	offsetZ := int(b.offsetPos.Z())
	originX := int(b.origin.X())
	originY := int(b.origin.Y())
	originZ := int(b.origin.Z())

	for _, off := range b.subChunkOffsets {
		if off <= 0 {
			continue
		}
		if _, err := file.Seek(off, io.SeekStart); err != nil {
			return nil, err
		}
		if _, err := readU64(file); err != nil {
			return nil, err
		}
		ox, err := readI16(file)
		if err != nil {
			return nil, err
		}
		oy, err := readI16(file)
		if err != nil {
			return nil, err
		}
		oz, err := readI16(file)
		if err != nil {
			return nil, err
		}
		regionCount, err := readU32(file)
		if err != nil {
			return nil, err
		}
		for i := uint32(0); i < regionCount; i++ {
			pid, err := readU32(file)
			if err != nil {
				return nil, err
			}
			x1, err := readI16(file)
			if err != nil {
				return nil, err
			}
			y1, err := readI16(file)
			if err != nil {
				return nil, err
			}
			z1, err := readI16(file)
			if err != nil {
				return nil, err
			}
			x2, err := readI16(file)
			if err != nil {
				return nil, err
			}
			y2, err := readI16(file)
			if err != nil {
				return nil, err
			}
			z2, err := readI16(file)
			if err != nil {
				return nil, err
			}

			rt := b.paletteRuntime[pid]
			if rt == 0 || rt == block.AirRuntimeID {
				continue
			}

			axMin := int(ox) + min(int(x1), int(x2))
			ayMin := int(oy) + min(int(y1), int(y2))
			azMin := int(oz) + min(int(z1), int(z2))
			axMax := int(ox) + max(int(x1), int(x2))
			ayMax := int(oy) + max(int(y1), int(y2))
			azMax := int(oz) + max(int(z1), int(z2))

			lxMin := axMin - originX + offsetX
			lyMin := ayMin - originY + offsetY
			lzMin := azMin - originZ + offsetZ
			lxMax := axMax - originX + offsetX
			lyMax := ayMax - originY + offsetY
			lzMax := azMax - originZ + offsetZ

			minChunkX := floorDiv(lxMin, 16)
			maxChunkX := floorDiv(lxMax, 16)
			minChunkZ := floorDiv(lzMin, 16)
			maxChunkZ := floorDiv(lzMax, 16)

			for cx := minChunkX; cx <= maxChunkX; cx++ {
				for cz := minChunkZ; cz <= maxChunkZ; cz++ {
					chunkPos := define.ChunkPos{int32(cx), int32(cz)}
					target, ok := chunks[chunkPos]
					if !ok {
						continue
					}
					chunkStartX := cx * 16
					chunkEndX := chunkStartX + 15
					chunkStartZ := cz * 16
					chunkEndZ := chunkStartZ + 15

					xStart := max(lxMin, chunkStartX)
					xEnd := min(lxMax, chunkEndX)
					zStart := max(lzMin, chunkStartZ)
					zEnd := min(lzMax, chunkEndZ)

					for x := xStart; x <= xEnd; x++ {
						for y := lyMin; y <= lyMax; y++ {
							for z := zStart; z <= zEnd; z++ {
								localXInChunk := x - chunkStartX
								localZInChunk := z - chunkStartZ
								target.SetBlock(uint8(localXInChunk), int16(y)-64, uint8(localZInChunk), 0, rt)
							}
						}
					}
				}
			}
		}
	}

	return chunks, nil
}

func (b *BCF) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any, len(posList))
	for _, pos := range posList {
		if _, exists := result[pos]; !exists {
			result[pos] = make(map[define.BlockPos]map[string]any)
		}
	}
	return result, nil
}

func (b *BCF) CountNonAirBlocks() (int, error) {
	if b.nonAirBlocks >= 0 {
		return b.nonAirBlocks, nil
	}
	if b.file == nil {
		return 0, fmt.Errorf("BCF 文件未初始化")
	}
	if len(b.subChunkOffsets) == 0 {
		b.nonAirBlocks = 0
		return 0, nil
	}

	file, err := os.Open(b.file.Name())
	if err != nil {
		return 0, fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return 0, err
	}
	fileSize := stat.Size()

	nonAir := int64(0)
	for _, off := range b.subChunkOffsets {
		if off <= 0 || off >= fileSize {
			continue
		}
		if _, err := file.Seek(off, io.SeekStart); err != nil {
			return 0, err
		}
		if _, err := readU64(file); err != nil {
			return 0, err
		}
		if _, err := readI16(file); err != nil {
			return 0, err
		}
		if _, err := readI16(file); err != nil {
			return 0, err
		}
		if _, err := readI16(file); err != nil {
			return 0, err
		}
		regionCount, err := readU32(file)
		if err != nil {
			return 0, err
		}
		for i := uint32(0); i < regionCount; i++ {
			pid, err := readU32(file)
			if err != nil {
				return 0, err
			}
			x1, err := readI16(file)
			if err != nil {
				return 0, err
			}
			y1, err := readI16(file)
			if err != nil {
				return 0, err
			}
			z1, err := readI16(file)
			if err != nil {
				return 0, err
			}
			x2, err := readI16(file)
			if err != nil {
				return 0, err
			}
			y2, err := readI16(file)
			if err != nil {
				return 0, err
			}
			z2, err := readI16(file)
			if err != nil {
				return 0, err
			}

			rt := b.paletteRuntime[pid]
			if rt == 0 || rt == block.AirRuntimeID {
				continue
			}
			dx := int64(abs(int(x2) - int(x1) + 1))
			dy := int64(abs(int(y2) - int(y1) + 1))
			dz := int64(abs(int(z2) - int(z1) + 1))
			nonAir += dx * dy * dz
		}
	}

	if nonAir > int64(math.MaxInt) {
		b.nonAirBlocks = math.MaxInt
		return math.MaxInt, nil
	}
	b.nonAirBlocks = int(nonAir)
	return b.nonAirBlocks, nil
}

func (b *BCF) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
) error {
	if bedrockWorld == nil {
		return fmt.Errorf("bedrock 世界为 nil")
	}
	if b.file == nil {
		return fmt.Errorf("BCF 文件未初始化")
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

	stat, err := file.Stat()
	if err != nil {
		return err
	}
	fileSize := stat.Size()

	offsetX := int(b.offsetPos.X())
	offsetY := int(b.offsetPos.Y())
	offsetZ := int(b.offsetPos.Z())
	originX := int(b.origin.X())
	originY := int(b.origin.Y())
	originZ := int(b.origin.Z())

	totalItems := len(b.subChunkOffsets)
	if totalItems <= 0 {
		totalItems = 1
	}
	currentItem := 0
	lastReportedProgress := -1

	for _, off := range b.subChunkOffsets {
		if off <= 0 || off >= fileSize {
			currentItem++
			continue
		}
		if _, err := file.Seek(off, io.SeekStart); err != nil {
			return err
		}
		if _, err := readU64(file); err != nil {
			return err
		}
		ox, err := readI16(file)
		if err != nil {
			return err
		}
		oy, err := readI16(file)
		if err != nil {
			return err
		}
		oz, err := readI16(file)
		if err != nil {
			return err
		}
		regionCount, err := readU32(file)
		if err != nil {
			return err
		}
		for i := uint32(0); i < regionCount; i++ {
			pid, err := readU32(file)
			if err != nil {
				return err
			}
			x1, err := readI16(file)
			if err != nil {
				return err
			}
			y1, err := readI16(file)
			if err != nil {
				return err
			}
			z1, err := readI16(file)
			if err != nil {
				return err
			}
			x2, err := readI16(file)
			if err != nil {
				return err
			}
			y2, err := readI16(file)
			if err != nil {
				return err
			}
			z2, err := readI16(file)
			if err != nil {
				return err
			}

			rt := b.paletteRuntime[pid]
			if rt == 0 || rt == block.AirRuntimeID {
				continue
			}

			axMin := int(ox) + min(int(x1), int(x2))
			ayMin := int(oy) + min(int(y1), int(y2))
			azMin := int(oz) + min(int(z1), int(z2))
			axMax := int(ox) + max(int(x1), int(x2))
			ayMax := int(oy) + max(int(y1), int(y2))
			azMax := int(oz) + max(int(z1), int(z2))

			lxMin := axMin - originX + offsetX
			lyMin := ayMin - originY + offsetY
			lzMin := azMin - originZ + offsetZ
			lxMax := axMax - originX + offsetX
			lyMax := ayMax - originY + offsetY
			lzMax := azMax - originZ + offsetZ

			for x := lxMin; x <= lxMax; x++ {
				for y := lyMin; y <= lyMax; y++ {
					for z := lzMin; z <= lzMax; z++ {
						ax := startX + int32(x)
						ay := int16(int(startY) + y)
						az := startZ + int32(z)
						if err := mcworld.SetBlock(ax, ay, az, rt); err != nil {
							return err
						}
					}
				}
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

func (b *BCF) Close() error {
	return nil
}

func readU8(r io.Reader) (uint8, error) {
	var v uint8
	err := binary.Read(r, binary.LittleEndian, &v)
	return v, err
}

func readU16(r io.Reader) (uint16, error) {
	var v uint16
	err := binary.Read(r, binary.LittleEndian, &v)
	return v, err
}

func readU32(r io.Reader) (uint32, error) {
	var v uint32
	err := binary.Read(r, binary.LittleEndian, &v)
	return v, err
}

func readU64(r io.Reader) (uint64, error) {
	var v uint64
	err := binary.Read(r, binary.LittleEndian, &v)
	return v, err
}

func readI16(r io.Reader) (int16, error) {
	var v int16
	err := binary.Read(r, binary.LittleEndian, &v)
	return v, err
}

func readString16(r io.Reader) (string, error) {
	l, err := readU16(r)
	if err != nil {
		return "", err
	}
	if l == 0 {
		return "", nil
	}
	buf := make([]byte, l)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func readBCFTypeMap(file *os.File, offset, fileSize int64) (map[uint16]string, error) {
	if offset <= 0 || offset >= fileSize {
		return map[uint16]string{}, nil
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	count, err := readU32(file)
	if err != nil {
		return nil, err
	}
	out := make(map[uint16]string, int(count))
	for i := uint32(0); i < count; i++ {
		tid, err := readU16(file)
		if err != nil {
			return nil, err
		}
		name, err := readString16(file)
		if err != nil {
			return nil, err
		}
		out[tid] = name
	}
	return out, nil
}

func readBCFStateNameMap(file *os.File, offset, fileSize int64) (map[uint8]string, error) {
	if offset <= 0 || offset >= fileSize {
		return map[uint8]string{}, nil
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	count, err := readU32(file)
	if err != nil {
		return nil, err
	}
	out := make(map[uint8]string, int(count))
	for i := uint32(0); i < count; i++ {
		sid, err := readU8(file)
		if err != nil {
			return nil, err
		}
		name, err := readString16(file)
		if err != nil {
			return nil, err
		}
		out[sid] = name
	}
	return out, nil
}

func readBCFStateValueMap(file *os.File, offset, fileSize int64) (map[uint8]string, error) {
	if offset <= 0 || offset >= fileSize {
		return map[uint8]string{}, nil
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	count, err := readU32(file)
	if err != nil {
		return nil, err
	}
	out := make(map[uint8]string, int(count))
	for i := uint32(0); i < count; i++ {
		vid, err := readU8(file)
		if err != nil {
			return nil, err
		}
		val, err := readString16(file)
		if err != nil {
			return nil, err
		}
		out[vid] = val
	}
	return out, nil
}

func readBCFPaletteRuntime(file *os.File, offset, fileSize int64, typeMap map[uint16]string, stateNameMap map[uint8]string, stateValueMap map[uint8]string) (map[uint32]uint32, error) {
	if offset <= 0 || offset >= fileSize {
		return map[uint32]uint32{}, nil
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	count, err := readU32(file)
	if err != nil {
		return nil, err
	}
	out := make(map[uint32]uint32, int(count))
	for i := uint32(0); i < count; i++ {
		pid, err := readU32(file)
		if err != nil {
			return nil, err
		}
		typeID, err := readU16(file)
		if err != nil {
			return nil, err
		}
		stateCount, err := readU16(file)
		if err != nil {
			return nil, err
		}
		name := typeMap[typeID]
		if name == "" {
			name = "minecraft:air"
		}

		states := make(map[string]any, int(stateCount))
		for j := uint16(0); j < stateCount; j++ {
			sid, err := readU8(file)
			if err != nil {
				return nil, err
			}
			vid, err := readU8(file)
			if err != nil {
				return nil, err
			}
			sname := stateNameMap[sid]
			sval := stateValueMap[vid]
			if sname == "" || sval == "" {
				continue
			}
			parsed, err := parseStateValue(sval)
			if err != nil {
				continue
			}
			states[sname] = parsed
		}

		out[pid] = runtimeIDForBlock(name, states)
	}
	return out, nil
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
