package structure

import (
	"fmt"

	"github.com/TriM-Organization/merry-memory/command"
	"github.com/TriM-Organization/merry-memory/protocol/encoding"
)

func floorDiv(value, divisor int) int {
	if divisor == 0 {
		return 0
	}
	result := value / divisor
	if value < 0 && value%divisor != 0 {
		result--
	}
	return result
}

type Number interface {
	int | int8 | int16 | int32 | int64 |
		uint | uint8 | uint16 | uint32 | uint64 | uintptr
}

// mod 模拟Python风格取余（结果与除数同号）, 支持所有数字类型
func mod[T Number](x, y T) T {
	if y == 0 {
		panic("除数为零")
	}
	rem := x % y // 基础取余（整数）或取模（浮点数）
	// 调整符号: 当除数与余数符号不同时, 加上除数
	if (y > 0 && rem < 0) || (y < 0 && rem > 0) {
		rem += y
	}
	return rem
}

func WriteBDXCommand(cmd command.Command, writer *encoding.Writer) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("写入 BDX 指令失败: %v", r)
		}
	}()
	cmdID := uint8(cmd.ID())
	writer.Uint8(&cmdID)
	cmd.Marshal(writer)
	return nil
}

func ReadBDXCommand(reader *encoding.Reader) (cmd command.Command, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("读取 BDX 指令失败: %v", r)
		}
	}()

	var commandID uint8
	reader.Uint8(&commandID)

	commandFunc, ok := command.BDumpCommandPool[uint16(commandID)]
	if !ok {
		return nil, fmt.Errorf("读取 BDX 指令: 不支持的命令 ID: %d", commandID)
	}

	cmd = commandFunc()
	cmd.Marshal(reader)

	return cmd, nil
}
