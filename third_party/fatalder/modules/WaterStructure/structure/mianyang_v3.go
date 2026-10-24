package structure

import (
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type MianYangV3 struct {
	MianYangV1
}

func (m *MianYangV3) ID() uint8 {
	return IDMianYangV3
}

func (m *MianYangV3) Name() string {
	return NameMianYangV3
}

func (m *MianYangV3) FromFile(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	zr, err := zlib.NewReader(file)
	if err != nil {
		return fmt.Errorf("创建 zlib 读取器失败: %w", err)
	}
	defer zr.Close()

	decoder := json.NewDecoder(zr)
	decoder.UseNumber()

	var data rawMianYangFile
	if err := decoder.Decode(&data); err != nil {
		return fmt.Errorf("解析 MianYang V3 的 JSON 失败: %w", err)
	}

	m.file = nil
	return m.populateFromData(data)
}
