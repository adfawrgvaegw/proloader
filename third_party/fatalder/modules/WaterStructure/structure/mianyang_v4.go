package structure

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/TriM-Organization/bedrock-world-operator/block"

	"github.com/Yeah114/WaterStructure/define"
)

// MianYangV4 reads the BuildingX format produced by 建筑助手脚本 (see 建筑助手-测试-buffer.js).
// The format is a gzip-compressed binary blob with a fixed header, namespace table,
// and a delta-encoded stream of blocks with optional SNBT payloads.
type MianYangV4 struct {
	MianYangV1
}

func (m *MianYangV4) ID() uint8 {
	return IDMianYangV4
}

func (m *MianYangV4) Name() string {
	return NameMianYangV4
}

func (m *MianYangV4) FromFile(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("创建 gzip 读取器失败: %w", err)
	}
	defer gz.Close()

	br := bufio.NewReader(gz)

	if _, err := readInt32LE(br); err != nil {
		return fmt.Errorf("读取 minX 失败: %w", err)
	}
	if _, err := readInt32LE(br); err != nil {
		return fmt.Errorf("读取 minY 失败: %w", err)
	}
	if _, err := readInt32LE(br); err != nil {
		return fmt.Errorf("读取 minZ 失败: %w", err)
	}
	width, err := readPositiveInt(br, "width")
	if err != nil {
		return err
	}
	height, err := readPositiveInt(br, "height")
	if err != nil {
		return err
	}
	length, err := readPositiveInt(br, "length")
	if err != nil {
		return err
	}
	namespaceCount, err := readPositiveInt(br, "namespace count")
	if err != nil {
		return err
	}
	blockCountRaw, err := readInt32LE(br)
	if err != nil {
		return fmt.Errorf("读取方块总数失败: %w", err)
	}
	if blockCountRaw <= 0 {
		return ErrInvalidFile
	}
	blockCount := int(blockCountRaw)

	namespaces := make([]string, namespaceCount)
	for i := range namespaces {
		length, err := readUint16LE(br)
		if err != nil {
			return fmt.Errorf("读取命名空间 %d 长度失败: %w", i, err)
		}
		if length == 0 {
			return fmt.Errorf("命名空间 %d 长度为 0", i)
		}
		buf := make([]byte, length)
		if _, err := io.ReadFull(br, buf); err != nil {
			return fmt.Errorf("读取命名空间 %d 数据失败: %w", i, err)
		}
		namespaces[i] = string(buf)
	}

	m.namespaces = namespaces
	m.paletteCache = make(map[paletteCacheKey]uint32, len(namespaces)*2)
	m.offsetPos = define.Offset{}
	m.file = nil

	type rawBlock struct {
		x, y, z int
		id      uint8
		aux     uint8
		nbt     map[string]any
	}

	rawBlocks := make([]rawBlock, 0, blockCount)

	curX, curY, curZ := 0, 0, 0

	for i := 0; i < blockCount; i++ {
		dx, err := readInt16LE(br)
		if err != nil {
			return fmt.Errorf("读取方块 %d dx 失败: %w", i, err)
		}
		dy, err := readInt16LE(br)
		if err != nil {
			return fmt.Errorf("读取方块 %d dy 失败: %w", i, err)
		}
		dz, err := readInt16LE(br)
		if err != nil {
			return fmt.Errorf("读取方块 %d dz 失败: %w", i, err)
		}
		curX += int(dx)
		curY += int(dy)
		curZ += int(dz)

		localX := mod(curX, width)
		localY := mod(curY, height)
		localZ := mod(curZ, length)

		nsIdx, err := br.ReadByte()
		if err != nil {
			return fmt.Errorf("读取方块 %d 命名空间索引失败: %w", i, err)
		}
		aux, err := br.ReadByte()
		if err != nil {
			return fmt.Errorf("读取方块 %d 辅助值失败: %w", i, err)
		}
		if int(nsIdx) >= len(m.namespaces) {
			return fmt.Errorf("方块 %d 的命名空间索引 %d 越界", i, nsIdx)
		}

		nbtLen, err := readInt32LE(br)
		if err != nil {
			return fmt.Errorf("读取方块 %d NBT 长度失败: %w", i, err)
		}
		var blockNBT map[string]any
		if nbtLen < 0 {
			return fmt.Errorf("方块 %d 的 NBT 长度为负数", i)
		}
		if nbtLen > 0 {
			buf := make([]byte, nbtLen)
			if _, err := io.ReadFull(br, buf); err != nil {
				return fmt.Errorf("读取方块 %d NBT 数据失败: %w", i, err)
			}
			blockNBT, err = parseMianYangNBT(string(buf))
			if err != nil {
				return fmt.Errorf("解析方块 %d 的 NBT 失败: %w", i, err)
			}
		}

		entLen, err := readInt32LE(br)
		if err != nil {
			return fmt.Errorf("读取方块 %d 实体长度失败: %w", i, err)
		}
		if entLen < 0 {
			return fmt.Errorf("方块 %d 的实体长度为负数", i)
		}
		if entLen > 0 {
			if _, err := io.CopyN(io.Discard, br, int64(entLen)); err != nil {
				return fmt.Errorf("跳过方块 %d 的实体数据失败: %w", i, err)
			}
		}

		rawBlocks = append(rawBlocks, rawBlock{
			x:   localX,
			y:   localY,
			z:   localZ,
			id:  nsIdx,
			aux: aux,
			nbt: blockNBT,
		})
	}

	if len(rawBlocks) == 0 {
		return ErrInvalidFile
	}

	m.originalSize = &define.Size{Width: width, Height: height, Length: length}
	m.size = &define.Size{Width: width, Height: height, Length: length}
	m.blocks = make([]mianyangBlock, 0, len(rawBlocks))
	m.nonAirBlocks = 0

	for _, b := range rawBlocks {
		runtimeID := m.runtimeIDFor(int(b.id), int(b.aux))

		m.blocks = append(m.blocks, mianyangBlock{
			LocalX:    b.x,
			LocalY:    b.y,
			LocalZ:    b.z,
			RuntimeID: runtimeID,
			NBT:       b.nbt,
		})

		if runtimeID != block.AirRuntimeID {
			m.nonAirBlocks++
		}
	}

	return nil
}

func readPositiveInt(r io.Reader, field string) (int, error) {
	value, err := readInt32LE(r)
	if err != nil {
		return 0, fmt.Errorf("读取 %s 失败: %w", field, err)
	}
	if value <= 0 {
		return 0, ErrInvalidFile
	}
	return int(value), nil
}

func readInt16LE(r io.Reader) (int16, error) {
	var buf [2]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return int16(binary.LittleEndian.Uint16(buf[:])), nil
}

func readUint16LE(r io.Reader) (uint16, error) {
	var buf [2]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(buf[:]), nil
}

func readInt32LE(r io.Reader) (int32, error) {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return int32(binary.LittleEndian.Uint32(buf[:])), nil
}
