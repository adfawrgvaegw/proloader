package structure

import (
	"strings"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/Yeah114/blocks"
)

func legacyBlockToBedrockRuntimeID(name string, data uint16) uint32 {
	name = strings.TrimSpace(name)
	if name == "" {
		return UnknownBlockRuntimeID
	}
	if !strings.Contains(name, ":") {
		name = "minecraft:" + name
	}

	runtimeID, found := blocks.LegacyBlockToRuntimeID(name, data)
	if !found {
		return UnknownBlockRuntimeID
	}

	baseName, properties, found := blocks.RuntimeIDToState(runtimeID)
	if !found {
		return UnknownBlockRuntimeID
	}

	bedrockName := baseName
	if !strings.Contains(bedrockName, ":") {
		bedrockName = "minecraft:" + bedrockName
	}
	bedrockRuntimeID, found := block.StateToRuntimeID(bedrockName, properties)
	if !found {
		return UnknownBlockRuntimeID
	}
	return bedrockRuntimeID
}
