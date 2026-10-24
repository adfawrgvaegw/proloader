package structure

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/world"

	"github.com/Yeah114/WaterStructure/define"
)

var ErrInvalidRootTagType = errors.New("根标签类型无效")
var ErrInvalidRootTagName = errors.New("根标签名称无效")
var ErrInvalidFile = errors.New("文件无效")
var ErrInvalidVarint = errors.New("在给定偏移处，字节数组不包含有效的 varint")

type Structure interface {
	ID() uint8
	Name() string
	GetOffsetPos() define.Offset
	SetOffsetPos(define.Offset)
	GetSize() define.Size
	GetChunks([]define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error)
	GetChunksNBT([]define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error)
	FromFile(reader *os.File) error
	FromMCWorld(
		world *world.BedrockWorld,
		target *os.File,
		point1BlockPos define.BlockPos,
		point2BlockPos define.BlockPos,
		startCallback func(int),
		progressCallback func(),
	) error
	CountNonAirBlocks() (int, error)
	ToMCWorld(
		bedrockWorld *world.BedrockWorld,
		startSubChunkPos define.SubChunkPos,
		startCallback func(int),
		progressCallback func(),
	) error
	Close() error
}

func StructureFromFile(file *os.File) (Structure, error) {
	ext := strings.ToLower(filepath.Ext(file.Name()))
	var err error
	switch ext {
	case ".mcworld", ".zip":
		file.Seek(0, io.SeekStart)
		rMCWorld := &MCWorld{}
		if rMCWorld.FromFile(file) == nil {
			return rMCWorld, nil
		}
	case ".kbdx":
		file.Seek(0, io.SeekStart)
		rKBDX := &KBDX{}
		if rKBDX.FromFile(file) == nil {
			return rKBDX, nil
		}
	case ".tibi":
		file.Seek(0, io.SeekStart)
		rTIBI := &TIBI{}
		if rTIBI.FromFile(file) == nil {
			return rTIBI, nil
		}
	}

	header := make([]byte, 2)
	n, err := file.Read(header)
	if err != nil {
		return nil, err
	}
	header = header[:n]
	if len(header) < 2 {
		return nil, ErrInvalidFile
	}

	if header[0] == 0x1f && header[1] == 0x8b {
		file.Seek(0, io.SeekStart)
		rLitematic := &Litematic{}
		if rLitematic.FromFile(file) == nil {
			return rLitematic, nil
		}

		file.Seek(0, io.SeekStart)
		rSchemV1 := &SchemV1{}
		if rSchemV1.FromFile(file) == nil {
			return rSchemV1, nil
		}

		file.Seek(0, io.SeekStart)
		rSchemV2 := &SchemV2{}
		if rSchemV2.FromFile(file) == nil {
			return rSchemV2, nil
		}

		file.Seek(0, io.SeekStart)
		rSchematic := &Schematic{}
		if rSchematic.FromFile(file) == nil {
			return rSchematic, nil
		}

		file.Seek(0, io.SeekStart)
		rMianYangV4 := &MianYangV4{}
		if rMianYangV4.FromFile(file) == nil {
			return rMianYangV4, nil
		}
	} else if header[0] == 0x0a && header[1] == 0xe5 {
		file.Seek(0, io.SeekStart)
		rAxiomBP := &AxiomBP{}
		if rAxiomBP.FromFile(file) == nil {
			return rAxiomBP, nil
		}
	} else if header[0] == 0x0a {
		file.Seek(0, io.SeekStart)
		rMCStructure := &MCStructure{}
		if rMCStructure.FromFile(file) == nil {
			return rMCStructure, nil
		}
	} else if string(header) == "BC" {
		file.Seek(0, io.SeekStart)
		rBCF := &BCF{}
		if rBCF.FromFile(file) == nil {
			return rBCF, nil
		}
	} else if string(header) == "PK" {
		file.Seek(0, io.SeekStart)
		rMCWorld := &MCWorld{}
		if rMCWorld.FromFile(file) == nil {
			return rMCWorld, nil
		}
	} else if string(header) == "BD" {
		file.Seek(0, io.SeekStart)
		rBDX := &BDX{}
		if rBDX.FromFile(file) == nil {
			return rBDX, nil
		}
	} else if string(header) == "IB" {
		file.Seek(0, io.SeekStart)
		rIBImport := &IBImport{}
		if rIBImport.FromFile(file) == nil {
			return rIBImport, nil
		}
	} else if string(header) == "co" {
		file.Seek(0, io.SeekStart)
		rConstruction := &Construction{}
		if rConstruction.FromFile(file) == nil {
			return rConstruction, nil
		}
	} else if string(header) == "H4" {
		file.Seek(0, io.SeekStart)
		rSIBI := &SIBI{}
		if rSIBI.FromFile(file) == nil {
			return rSIBI, nil
		}
	} else if string(header) == "se" || string(header) == "fi" || string(header) == "ti" {
		file.Seek(0, io.SeekStart)
		rMCFunction := &MCFunction{}
		if rMCFunction.FromFile(file) == nil {
			return rMCFunction, nil
		}
	} else if string(header[0]) == "{" {
		file.Seek(0, io.SeekStart)
		rFuHongV4 := &FuHongV4{}
		if rFuHongV4.FromFile(file) == nil {
			return rFuHongV4, nil
		}
		file.Seek(0, io.SeekStart)
		rFuHongV3 := &FuHongV3{}
		if rFuHongV3.FromFile(file) == nil {
			return rFuHongV3, nil
		}
		file.Seek(0, io.SeekStart)
		rFuHongV2 := &FuHongV2{}
		if rFuHongV2.FromFile(file) == nil {
			return rFuHongV2, nil
		}
		file.Seek(0, io.SeekStart)
		rTimeBuilderV1 := &TimeBuilderV1{}
		if rTimeBuilderV1.FromFile(file) == nil {
			return rTimeBuilderV1, nil
		}
		file.Seek(0, io.SeekStart)
		rMianYangV1 := &MianYangV1{}
		if rMianYangV1.FromFile(file) == nil {
			return rMianYangV1, nil
		}
		file.Seek(0, io.SeekStart)
		rQingXuV1 := &QingXuV1{}
		if rQingXuV1.FromFile(file) == nil {
			return rQingXuV1, nil
		}
		file.Seek(0, io.SeekStart)
		rCov := &CovStructure{}
		if rCov.FromFile(file) == nil {
			return rCov, nil
		}
	} else if string(header) == "[[" {
		file.Seek(0, io.SeekStart)
		rGangBanV6 := &GangBanV6{}
		if rGangBanV6.FromFile(file) == nil {
			return rGangBanV6, nil
		}
	} else if string(header) == "[{" {
		file.Seek(0, io.SeekStart)
		rRunAway := &RunAway{}
		if rRunAway.FromFile(file) == nil {
			return rRunAway, nil
		}
	} else if string(header[0]) == "[" {
		file.Seek(0, io.SeekStart)
		rFuHongV1 := &FuHongV1{}
		if rFuHongV1.FromFile(file) == nil {
			return rFuHongV1, nil
		}
		file.Seek(0, io.SeekStart)
		rGangBanV5 := &GangBanV5{}
		if rGangBanV5.FromFile(file) == nil {
			return rGangBanV5, nil
		}
		file.Seek(0, io.SeekStart)
		rGangBanV4 := &GangBanV4{}
		if rGangBanV4.FromFile(file) == nil {
			return rGangBanV4, nil
		}
		file.Seek(0, io.SeekStart)
		rGangBanV3 := &GangBanV3{}
		if rGangBanV3.FromFile(file) == nil {
			return rGangBanV3, nil
		}
		file.Seek(0, io.SeekStart)
		rGangBanV1 := &GangBanV1{}
		if rGangBanV1.FromFile(file) == nil {
			return rGangBanV1, nil
		}
		file.Seek(0, io.SeekStart)
		rRunAway := &RunAway{}
		if rRunAway.FromFile(file) == nil {
			return rRunAway, nil
		}
	} else if header[0] == 0x78 {
		file.Seek(0, io.SeekStart)
		rFuHongV5 := &FuHongV5{}
		if rFuHongV5.FromFile(file) == nil {
			return rFuHongV5, nil
		}
		file.Seek(0, io.SeekStart)
		rMianYangV3 := &MianYangV3{}
		if rMianYangV3.FromFile(file) == nil {
			return rMianYangV3, nil
		}
		file.Seek(0, io.SeekStart)
		rGangBanV7 := &GangBanV7{}
		if rGangBanV7.FromFile(file) == nil {
			return rGangBanV7, nil
		}
	} else if header[0] == 0x91 {
		file.Seek(0, io.SeekStart)
		rBDS := &BDS{}
		if rBDS.FromFile(file) == nil {
			return rBDS, nil
		}
		file.Seek(0, io.SeekStart)
		rNP := &NexusNP{}
		if rNP.FromFile(file) == nil {
			return rNP, nil
		}
	} else if header[0] == 0x28 && header[1] == 0x15 {
		file.Seek(0, io.SeekStart)
		rMianYangV3 := &MianYangV3{}
		if rMianYangV3.FromFile(file) == nil {
			return rMianYangV3, nil
		}
	}
	file.Seek(0, io.SeekStart)
	rKBDX := &KBDX{}
	if rKBDX.FromFile(file) == nil {
		return rKBDX, nil
	}
	return nil, ErrInvalidFile
}
