package structure

import (
	"fmt"
	"os"

	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/Yeah114/WaterStructure/define"
)

type SIBI struct {
	BaseReader
}

func (s *SIBI) ID() uint8 {
	return IDSIBI
}

func (s *SIBI) Name() string {
	return NameSIBI
}

func (s *SIBI) GetOffsetPos() define.Offset {
	return define.Offset{}
}

func (s *SIBI) SetOffsetPos(define.Offset) {
}

func (s *SIBI) GetSize() define.Size {
	return define.Size{}
}

func (s *SIBI) GetChunks([]define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	return nil, fmt.Errorf("SIBI 格式解析未实现: 缺少 sibi 规范/样例")
}

func (s *SIBI) GetChunksNBT([]define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	return nil, fmt.Errorf("SIBI 格式解析未实现: 缺少 sibi 规范/样例")
}

func (s *SIBI) FromFile(*os.File) error {
	return fmt.Errorf("SIBI 格式解析未实现: 缺少 sibi 规范/样例")
}

func (s *SIBI) CountNonAirBlocks() (int, error) {
	return 0, fmt.Errorf("SIBI 格式解析未实现: 缺少 sibi 规范/样例")
}

func (s *SIBI) Close() error {
	return nil
}
