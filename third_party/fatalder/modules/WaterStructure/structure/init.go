package structure

import (
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/Yeah114/blocks"
)

//go:embed bdx_runtimeIds_117.json
var bdxRuntimeBlockPool117Byte []byte

var UnknownBlockRuntimeID, _ = block.StateToRuntimeID("minecraft:unknown", nil)
var BDXRuntimeBlockPools = make(map[uint8][]uint32)
var SchematicBlockMapping = [256][256]uint32{}
var JavaDataVersion int32 = 4556
var MCWorldOverworldRange = define.Dimension(define.DimensionIDOverworld).Range()

func chunkLocalYFromWorld(y int) int32 {
	return int32(y) + int32(MCWorldOverworldRange.Min())
}

func init() {
	bdxRuntimeBlockPool117 := make([][2]any, 0)
	err := json.Unmarshal(bdxRuntimeBlockPool117Byte, &bdxRuntimeBlockPool117)
	if err != nil {
		panic(err)
	}
	BDXRuntimeBlockPools[117] = make([]uint32, 0)
	for _, v := range bdxRuntimeBlockPool117 {
		runtimeID, found := blocks.LegacyBlockToRuntimeID(v[0].(string), uint16(v[1].(float64)))
		if !found {
			BDXRuntimeBlockPools[117] = append(BDXRuntimeBlockPools[117], UnknownBlockRuntimeID)
			continue
		}
		name, properties, found := blocks.RuntimeIDToState(runtimeID)
		if !found {
			BDXRuntimeBlockPools[117] = append(BDXRuntimeBlockPools[117], UnknownBlockRuntimeID)
			continue
		}
		blockRuntimeID, found := block.StateToRuntimeID(name, properties)
		if !found {
			BDXRuntimeBlockPools[117] = append(BDXRuntimeBlockPools[117], UnknownBlockRuntimeID)
			continue
		}
		BDXRuntimeBlockPools[117] = append(BDXRuntimeBlockPools[117], blockRuntimeID)
	}

	schematicMapping := blocks.GetSchematicMapping()
	for blockID := 0; blockID < len(schematicMapping); blockID++ {
		for dataValue := 0; dataValue < len(schematicMapping[blockID]); dataValue++ {
			runtimeID := schematicMapping[blockID][dataValue]
			mappedRuntimeID := UnknownBlockRuntimeID
			if baseName, properties, found := blocks.RuntimeIDToState(runtimeID); found && baseName != "" {
				name := baseName
				if !strings.Contains(name, ":") {
					name = "minecraft:" + name
				}
				if bedrockRuntimeID, ok := block.StateToRuntimeID(name, properties); ok {
					mappedRuntimeID = bedrockRuntimeID
				}
			}
			SchematicBlockMapping[blockID][dataValue] = mappedRuntimeID
		}
	}
}
