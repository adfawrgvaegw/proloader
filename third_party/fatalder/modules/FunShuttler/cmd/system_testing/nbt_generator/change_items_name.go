package main

import (
	"bytes"
	"math/rand"
	"os"

	"github.com/Yeah114/FunShuttler/core/minecraft/nbt"
)

func ChangeItemsName() {
	var m map[string]any

	bs, err := os.ReadFile("mcstructure/ori_cin.mcstructure")
	if err != nil {
		panic(err)
	}

	buf := bytes.NewBuffer(bs)
	err = nbt.NewDecoderWithEncoding(buf, nbt.LittleEndian).Decode(&m)
	if err != nil {
		panic(err)
	}

	m1 := m["structure"].(map[string]any)["palette"].(map[string]any)["default"].(map[string]any)["block_position_data"].(map[string]any)

	for _, value := range m1 {
		m2 := value.(map[string]any)["block_entity_data"].(map[string]any)
		m3, _ := m2["Items"].([]any)
		for _, val := range m3 {
			var itemName string
			v := val.(map[string]any)

			switch rand.Intn(4) {
			case 0:
				itemName = "Happy2018new"
			case 1:
				itemName = "Liliya233"
			case 2:
				itemName = "CMA2401PT"
			case 3:
				itemName = ""
			}

			if len(itemName) == 0 {
				continue
			}

			tag, ok := v["tag"].(map[string]any)
			if !ok {
				tag = make(map[string]any)
				v["tag"] = tag
			}

			display, ok := tag["display"].(map[string]any)
			if !ok {
				tag["display"] = map[string]any{"Name": itemName}
			} else {
				display["Name"] = itemName
			}
		}
	}

	buf = bytes.NewBuffer(nil)
	nbt.NewEncoderWithEncoding(buf, nbt.LittleEndian).Encode(m)
	os.WriteFile("mcstructure/cin.mcstructure", buf.Bytes(), 0600)
}
