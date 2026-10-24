// Isolated palette bridge using the same pinned mapper as Fatalder/WaterStructure.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Yeah114/blocks"
)

type mapped struct {
	Name   string         `json:"name"`
	States map[string]any `json:"states"`
	Found  bool           `json:"found"`
	Score  any            `json:"score"`
}

// Java 1.21.9 renamed the original iron chain to iron_chain. The pinned
// pre-rename converter calls that same block chain; retain every property.
func legacyJavaName(source string) string {
	for _, prefix := range []string{"minecraft:", ""} {
		name := prefix + "iron_chain"
		if source == name || strings.HasPrefix(source, name+"[") {
			return prefix + "chain" + strings.TrimPrefix(source, name)
		}
	}
	return source
}

func main() {
	var input json.RawMessage
	if json.NewDecoder(os.Stdin).Decode(&input) != nil {
		os.Exit(2)
	}
	if len(input) > 0 && input[0] == '{' {
		var request struct {
			Schematic [][2]int `json:"schematic"`
		}
		if json.Unmarshal(input, &request) != nil {
			os.Exit(2)
		}
		result := make(map[string]mapped, len(request.Schematic))
		for _, pair := range request.Schematic {
			id, data := pair[0], pair[1]
			key := fmt.Sprintf("%d:%d", id, data)
			if id < 0 || id > 255 || data < 0 || data > 15 {
				result[key] = mapped{Found: false}
				continue
			}
			rid := blocks.SchematicToRuntimeID(uint8(id), uint8(data))
			name, states, found := blocks.RuntimeIDToState(rid)
			for k, value := range states {
				switch v := value.(type) {
				case uint8:
					states[k] = v != 0
				case int8:
					states[k] = v != 0
				}
			}
			if !strings.Contains(name, ":") {
				name = "minecraft:" + name
			}
			result[key] = mapped{name, states, found && (id == 0 || name != "minecraft:air"), nil}
		}
		if json.NewEncoder(os.Stdout).Encode(result) != nil {
			os.Exit(2)
		}
		return
	}
	var palette []string
	if json.Unmarshal(input, &palette) != nil {
		os.Exit(2)
	}
	result := make(map[string]mapped, len(palette))
	for _, source := range palette {
		name, props := blocks.ConvertStringToBlockNameAndPropsForSearch(legacyJavaName(source))
		rid, score, ok := blocks.DefaultAnyToNemcConvertor.TryBestSearchByState(name, props)
		mappedName, states, found := blocks.RuntimeIDToState(rid)
		// NBT byte properties are Bedrock boolean states; JSON would erase the type.
		for k, value := range states {
			switch v := value.(type) {
			case uint8:
				states[k] = v != 0
			case int8:
				states[k] = v != 0
			}
		}
		if !strings.Contains(mappedName, ":") {
			mappedName = "minecraft:" + mappedName
		}
		result[source] = mapped{mappedName, states, ok && found, score}
	}
	if json.NewEncoder(os.Stdout).Encode(result) != nil {
		os.Exit(2)
	}
}
