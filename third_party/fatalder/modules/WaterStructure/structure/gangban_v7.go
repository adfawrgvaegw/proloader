package structure

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type GangBanV7 struct {
	GangBanV6
}

func (g *GangBanV7) ID() uint8 {
	return IDGangBanV7
}

func (g *GangBanV7) Name() string {
	return NameGangBanV7
}

func (g *GangBanV7) FromFile(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	zr, err := zlib.NewReader(file)
	if err != nil {
		return fmt.Errorf("创建 zlib 读取器失败: %w", err)
	}
	defer zr.Close()

	data, err := io.ReadAll(zr)
	if err != nil {
		return fmt.Errorf("读取压缩数据失败: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var entries []any
	if err := decoder.Decode(&entries); err != nil {
		return fmt.Errorf("解析 GangBan V7 的 JSON 失败: %w", err)
	}
	if len(entries) < 1 {
		return ErrInvalidFile
	}

	paletteEntry := entries[len(entries)-1]
	stream := entries[:len(entries)-1]

	paletteAny, ok := paletteEntry.([]any)
	if !ok || len(paletteAny) == 0 {
		return ErrInvalidFile
	}
	palette := make([]string, len(paletteAny))
	for i, raw := range paletteAny {
		name, ok := raw.(string)
		if !ok {
			return fmt.Errorf("调色板条目 %d 不是字符串", i)
		}
		palette[i] = name
	}

	g.file = file
	return g.populateFromComponents(stream, palette)
}
