package structure

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"compress/flate"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/WaterStructure/utils"
)

type TIBI struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset
	origin       define.Origin

	commandCount int
	nonAirBlocks int
}

func (t *TIBI) ID() uint8 {
	return IDTIBI
}

func (t *TIBI) Name() string {
	return NameTIBI
}

func (t *TIBI) FromFile(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	comp, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("读取 TIBI 失败: %w", err)
	}
	if len(comp) == 0 {
		return ErrInvalidFile
	}
	decoded, err := decodeTIBI(comp)
	if err != nil {
		return err
	}

	minX, minY, minZ := math.MaxInt, math.MaxInt, math.MaxInt
	maxX, maxY, maxZ := math.MinInt, math.MinInt, math.MinInt

	for _, cmd := range decoded.commands {
		switch cmd.qtype {
		case 0: // setblock
			x, y, z := cmd.x, cmd.y, cmd.z
			minX = min(minX, x)
			minY = min(minY, y)
			minZ = min(minZ, z)
			maxX = max(maxX, x)
			maxY = max(maxY, y)
			maxZ = max(maxZ, z)
		case 1: // fill
			x1, y1, z1 := cmd.x, cmd.y, cmd.z
			x2, y2, z2 := cmd.dx, cmd.dy, cmd.dz
			minX = min(minX, min(x1, x2))
			minY = min(minY, min(y1, y2))
			minZ = min(minZ, min(z1, z2))
			maxX = max(maxX, max(x1, x2))
			maxY = max(maxY, max(y1, y2))
			maxZ = max(maxZ, max(z1, z2))
		}
	}

	if minX == math.MaxInt || minY == math.MaxInt || minZ == math.MaxInt {
		return ErrInvalidFile
	}

	width := maxX - minX + 1
	height := maxY - minY + 1
	length := maxZ - minZ + 1

	t.file = file
	t.offsetPos = define.Offset{}
	t.origin = define.Origin{int32(minX), int32(minY), int32(minZ)}
	t.size = &define.Size{Width: width, Height: height, Length: length}
	t.originalSize = &define.Size{Width: width, Height: height, Length: length}
	t.commandCount = len(decoded.commands)
	t.nonAirBlocks = -1

	return nil
}

func (t *TIBI) GetOffsetPos() define.Offset {
	return t.offsetPos
}

func (t *TIBI) SetOffsetPos(offset define.Offset) {
	t.offsetPos = offset
	t.size.Width = t.originalSize.Width + int(math.Abs(float64(offset.X())))
	t.size.Length = t.originalSize.Length + int(math.Abs(float64(offset.Z())))
	t.size.Height = t.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (t *TIBI) GetSize() define.Size {
	return *t.size
}

func (t *TIBI) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	chunks := make(map[define.ChunkPos]*chunk.Chunk, len(posList))
	for _, pos := range posList {
		if _, exists := chunks[pos]; !exists {
			chunks[pos] = chunk.NewChunk(block.AirRuntimeID, MCWorldOverworldRange)
		}
	}
	if len(chunks) == 0 {
		return chunks, nil
	}
	if t.file == nil {
		return nil, fmt.Errorf("TIBI 文件未初始化")
	}

	file, err := os.Open(t.file.Name())
	if err != nil {
		return nil, fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	comp, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("读取 TIBI 失败: %w", err)
	}
	decoded, err := decodeTIBI(comp)
	if err != nil {
		return nil, err
	}

	offsetX := int(t.offsetPos.X())
	offsetY := int(t.offsetPos.Y())
	offsetZ := int(t.offsetPos.Z())
	originX := int(t.origin.X())
	originY := int(t.origin.Y())
	originZ := int(t.origin.Z())

	for _, cmd := range decoded.commands {
		blockStr := decoded.blocks[cmd.blockIndex]
		prorStr := decoded.prors[cmd.prorIndex]
		runtimeID, ok := tibiRuntimeID(blockStr, prorStr)
		if !ok || runtimeID == block.AirRuntimeID {
			continue
		}

		switch cmd.qtype {
		case 0:
			lx := cmd.x - originX + offsetX
			ly := cmd.y - originY + offsetY
			lz := cmd.z - originZ + offsetZ
			setChunkBlockIfRequested(chunks, lx, ly, lz, runtimeID)
		case 1:
			x1 := cmd.x - originX + offsetX
			y1 := cmd.y - originY + offsetY
			z1 := cmd.z - originZ + offsetZ
			x2 := cmd.dx - originX + offsetX
			y2 := cmd.dy - originY + offsetY
			z2 := cmd.dz - originZ + offsetZ
			xMin, xMax := min(x1, x2), max(x1, x2)
			yMin, yMax := min(y1, y2), max(y1, y2)
			zMin, zMax := min(z1, z2), max(z1, z2)
			fillChunksIfRequested(chunks, xMin, yMin, zMin, xMax, yMax, zMax, runtimeID)
		}
	}

	return chunks, nil
}

func setChunkBlockIfRequested(chunks map[define.ChunkPos]*chunk.Chunk, x, y, z int, runtimeID uint32) {
	chunkX := floorDiv(x, 16)
	chunkZ := floorDiv(z, 16)
	chunkPos := define.ChunkPos{int32(chunkX), int32(chunkZ)}
	target, ok := chunks[chunkPos]
	if !ok {
		return
	}
	localX := x - chunkX*16
	localZ := z - chunkZ*16
	target.SetBlock(uint8(localX), int16(y)-64, uint8(localZ), 0, runtimeID)
}

func fillChunksIfRequested(chunks map[define.ChunkPos]*chunk.Chunk, xMin, yMin, zMin, xMax, yMax, zMax int, runtimeID uint32) {
	minChunkX := floorDiv(xMin, 16)
	maxChunkX := floorDiv(xMax, 16)
	minChunkZ := floorDiv(zMin, 16)
	maxChunkZ := floorDiv(zMax, 16)

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

			xStart := max(xMin, chunkStartX)
			xEnd := min(xMax, chunkEndX)
			zStart := max(zMin, chunkStartZ)
			zEnd := min(zMax, chunkEndZ)

			for x := xStart; x <= xEnd; x++ {
				for y := yMin; y <= yMax; y++ {
					for z := zStart; z <= zEnd; z++ {
						localXInChunk := x - chunkStartX
						localZInChunk := z - chunkStartZ
						target.SetBlock(uint8(localXInChunk), int16(y)-64, uint8(localZInChunk), 0, runtimeID)
					}
				}
			}
		}
	}
}

func (t *TIBI) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any, len(posList))
	for _, pos := range posList {
		if _, exists := result[pos]; !exists {
			result[pos] = make(map[define.BlockPos]map[string]any)
		}
	}
	return result, nil
}

func (t *TIBI) CountNonAirBlocks() (int, error) {
	if t.nonAirBlocks >= 0 {
		return t.nonAirBlocks, nil
	}
	if t.file == nil {
		return 0, fmt.Errorf("TIBI 文件未初始化")
	}

	file, err := os.Open(t.file.Name())
	if err != nil {
		return 0, fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	comp, err := io.ReadAll(file)
	if err != nil {
		return 0, fmt.Errorf("读取 TIBI 失败: %w", err)
	}
	decoded, err := decodeTIBI(comp)
	if err != nil {
		return 0, err
	}

	total := int64(0)
	for _, cmd := range decoded.commands {
		blockStr := decoded.blocks[cmd.blockIndex]
		prorStr := decoded.prors[cmd.prorIndex]
		runtimeID, ok := tibiRuntimeID(blockStr, prorStr)
		if !ok || runtimeID == block.AirRuntimeID {
			continue
		}
		switch cmd.qtype {
		case 0:
			total++
		case 1:
			dx := int64(absInt(cmd.dx - cmd.x + 1))
			dy := int64(absInt(cmd.dy - cmd.y + 1))
			dz := int64(absInt(cmd.dz - cmd.z + 1))
			total += dx * dy * dz
		}
	}
	if total > int64(math.MaxInt) {
		t.nonAirBlocks = math.MaxInt
		return math.MaxInt, nil
	}
	t.nonAirBlocks = int(total)
	return t.nonAirBlocks, nil
}

func (t *TIBI) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
) error {
	if bedrockWorld == nil {
		return fmt.Errorf("bedrock 世界为 nil")
	}
	if t.file == nil {
		return fmt.Errorf("TIBI 文件未初始化")
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

	file, err := os.Open(t.file.Name())
	if err != nil {
		return fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	comp, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("读取 TIBI 失败: %w", err)
	}
	decoded, err := decodeTIBI(comp)
	if err != nil {
		return err
	}

	offsetX := int(t.offsetPos.X())
	offsetY := int(t.offsetPos.Y())
	offsetZ := int(t.offsetPos.Z())
	originX := int(t.origin.X())
	originY := int(t.origin.Y())
	originZ := int(t.origin.Z())

	totalItems := len(decoded.commands)
	if totalItems <= 0 {
		totalItems = 1
	}
	currentItem := 0
	lastReportedProgress := -1

	for _, cmd := range decoded.commands {
		blockStr := decoded.blocks[cmd.blockIndex]
		prorStr := decoded.prors[cmd.prorIndex]
		runtimeID, ok := tibiRuntimeID(blockStr, prorStr)
		if ok && runtimeID != block.AirRuntimeID {
			switch cmd.qtype {
			case 0:
				x := cmd.x - originX + offsetX
				y := cmd.y - originY + offsetY
				z := cmd.z - originZ + offsetZ
				ax := startX + int32(x)
				ay := int16(int(startY) + y)
				az := startZ + int32(z)
				if err := mcworld.SetBlock(ax, ay, az, runtimeID); err != nil {
					return err
				}
			case 1:
				x1 := cmd.x - originX + offsetX
				y1 := cmd.y - originY + offsetY
				z1 := cmd.z - originZ + offsetZ
				x2 := cmd.dx - originX + offsetX
				y2 := cmd.dy - originY + offsetY
				z2 := cmd.dz - originZ + offsetZ
				xMin, xMax := min(x1, x2), max(x1, x2)
				yMin, yMax := min(y1, y2), max(y1, y2)
				zMin, zMax := min(z1, z2), max(z1, z2)

				for x := xMin; x <= xMax; x++ {
					for y := yMin; y <= yMax; y++ {
						for z := zMin; z <= zMax; z++ {
							ax := startX + int32(x)
							ay := int16(int(startY) + y)
							az := startZ + int32(z)
							if err := mcworld.SetBlock(ax, ay, az, runtimeID); err != nil {
								return err
							}
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

func (t *TIBI) Close() error {
	return nil
}

type tibiDecoded struct {
	blocks   []string
	prors    []string
	commands []tibiCommand
}

type tibiCommand struct {
	qtype      int
	blockIndex int
	x, y, z    int
	dx, dy, dz int
	prorIndex  int
}

func decodeTIBI(comp []byte) (*tibiDecoded, error) {
	decompressed, err := inflateRaw(comp)
	if err != nil {
		return nil, fmt.Errorf("TIBI 解压失败: %w", err)
	}
	if len(decompressed) < 15 {
		return nil, ErrInvalidFile
	}

	buf := make([]byte, len(decompressed))
	copy(buf, decompressed)

	header15 := buf[:15]
	payloadSize := len(buf) - 15
	key := tibiMD5Key(header15, payloadSize)
	xorInPlace(buf, 15, key)

	payload := buf[15:]
	decoded, err := parseTIBIPayload(payload)
	if err != nil {
		return nil, err
	}
	return decoded, nil
}

func inflateRaw(data []byte) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(data))
	defer r.Close()
	return io.ReadAll(r)
}

func tibiMD5Key(header15 []byte, payloadSize int) []byte {
	suffix := []byte("TIBI_2025/5/19-Start" + strconv.Itoa(payloadSize))
	h := md5.Sum(append(append([]byte{}, header15...), suffix...))
	dst := make([]byte, hex.EncodedLen(len(h)))
	hex.Encode(dst, h[:])
	key, _ := hex.DecodeString(string(dst))
	return key
}

func xorInPlace(buf []byte, start int, key []byte) {
	if len(key) == 0 {
		return
	}
	ki := 0
	for i := start; i < len(buf); i++ {
		buf[i] ^= key[ki%len(key)]
		ki++
	}
}

func parseTIBIPayload(payload []byte) (*tibiDecoded, error) {
	off := 0
	readStr := func() (string, error) {
		l, n, err := readVarint(payload, off)
		if err != nil {
			return "", err
		}
		off = n
		if off+int(l) > len(payload) {
			return "", ErrInvalidFile
		}
		s := string(payload[off : off+int(l)])
		off += int(l)
		return s, nil
	}

	blockCountU, n, err := readVarint(payload, off)
	if err != nil {
		return nil, err
	}
	off = n
	if blockCountU > uint64(math.MaxInt32) {
		return nil, ErrInvalidFile
	}
	blocks := make([]string, 0, int(blockCountU))
	for i := 0; i < int(blockCountU); i++ {
		_, n, err := readVarint(payload, off) // line placeholder
		if err != nil {
			return nil, err
		}
		off = n
		s, err := readStr()
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, s)
	}

	prorCountU, n, err := readVarint(payload, off)
	if err != nil {
		return nil, err
	}
	off = n
	if prorCountU > uint64(math.MaxInt32) {
		return nil, ErrInvalidFile
	}
	prors := make([]string, 0, int(prorCountU))
	for i := 0; i < int(prorCountU); i++ {
		_, n, err := readVarint(payload, off) // line placeholder
		if err != nil {
			return nil, err
		}
		off = n
		s, err := readStr()
		if err != nil {
			return nil, err
		}
		prors = append(prors, s)
	}

	cmdCountU, n, err := readVarint(payload, off)
	if err != nil {
		return nil, err
	}
	off = n
	if cmdCountU > uint64(math.MaxInt32) {
		return nil, ErrInvalidFile
	}

	commands := make([]tibiCommand, 0, int(cmdCountU))
	for i := 0; i < int(cmdCountU); i++ {
		qtypeU, n, err := readVarint(payload, off)
		if err != nil {
			return nil, err
		}
		off = n
		blockIndexU, n, err := readVarint(payload, off)
		if err != nil {
			return nil, err
		}
		off = n
		xU, n, err := readVarint(payload, off)
		if err != nil {
			return nil, err
		}
		off = n
		yU, n, err := readVarint(payload, off)
		if err != nil {
			return nil, err
		}
		off = n
		zU, n, err := readVarint(payload, off)
		if err != nil {
			return nil, err
		}
		off = n

		cmd := tibiCommand{
			qtype:      int(qtypeU),
			blockIndex: int(blockIndexU),
			x:          int(xU),
			y:          int(yU),
			z:          int(zU),
			dx:         int(xU),
			dy:         int(yU),
			dz:         int(zU),
			prorIndex:  0,
		}

		if cmd.qtype == 1 {
			dxU, n, err := readVarint(payload, off)
			if err != nil {
				return nil, err
			}
			off = n
			dyU, n, err := readVarint(payload, off)
			if err != nil {
				return nil, err
			}
			off = n
			dzU, n, err := readVarint(payload, off)
			if err != nil {
				return nil, err
			}
			off = n
			prorU, n, err := readVarint(payload, off)
			if err != nil {
				return nil, err
			}
			off = n
			cmd.dx = int(dxU)
			cmd.dy = int(dyU)
			cmd.dz = int(dzU)
			cmd.prorIndex = int(prorU)
		} else {
			prorU, n, err := readVarint(payload, off)
			if err != nil {
				return nil, err
			}
			off = n
			cmd.prorIndex = int(prorU)
		}

		if cmd.blockIndex < 0 || cmd.blockIndex >= len(blocks) {
			return nil, ErrInvalidFile
		}
		if cmd.prorIndex < 0 || cmd.prorIndex >= len(prors) {
			return nil, ErrInvalidFile
		}
		commands = append(commands, cmd)
	}

	return &tibiDecoded{blocks: blocks, prors: prors, commands: commands}, nil
}

func readVarint(buf []byte, offset int) (uint64, int, error) {
	var result uint64
	var shift uint
	pos := offset
	for {
		if pos >= len(buf) {
			return 0, pos, ErrInvalidVarint
		}
		b := buf[pos]
		pos++
		result |= uint64(b&0x7f) << shift
		if (b & 0x80) == 0 {
			break
		}
		shift += 7
		if shift > 63 {
			return 0, pos, ErrInvalidVarint
		}
	}
	return result, pos, nil
}

func tibiRuntimeID(blockStr, prorStr string) (uint32, bool) {
	blockStr = strings.TrimSpace(blockStr)
	if blockStr == "" {
		return 0, false
	}

	name := blockStr
	statePart := ""
	if idx := strings.Index(blockStr, "["); idx != -1 {
		if end := strings.LastIndex(blockStr, "]"); end > idx {
			name = strings.TrimSpace(blockStr[:idx])
			statePart = blockStr[idx : end+1]
		}
	}

	var legacyData *int
	if fields := strings.Fields(prorStr); len(fields) > 0 {
		if i, err := strconv.Atoi(fields[0]); err == nil {
			legacyData = &i
		}
	}

	if legacyData != nil {
		return legacyBlockToBedrockRuntimeID(name, uint16(*legacyData)), true
	}
	if statePart != "" {
		if states, err := parseMCFunctionStates(statePart); err == nil {
			return runtimeIDForBlock(name, states), true
		}
	}
	return runtimeIDForBlock(name, nil), true
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
