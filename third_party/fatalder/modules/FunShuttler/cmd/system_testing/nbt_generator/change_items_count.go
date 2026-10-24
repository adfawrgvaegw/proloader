package main

import (
	"bytes"
	"math/rand"
	"os"
	"strings"

	"github.com/Yeah114/FunShuttler/core/minecraft/nbt"
)

func ChangeItemsCount() {
	var m map[string]any

	bs, err := os.ReadFile("mcstructure/ori_cic.mcstructure")
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
			var itemCount byte

			v := val.(map[string]any)
			itemName := v["Name"].(string)

			if strings.Contains(itemName, "sign") || strings.Contains(itemName, "banner") {
				itemCount = byte(rand.Intn(16) + 1)
			} else if strings.Contains(itemName, "shield") {
				itemCount = 1
			} else {
				itemCount = byte(rand.Intn(64) + 1)
			}

			tag, ok := v["tag"].(map[string]any)
			if ok {
				ench, _ := tag["ench"].([]any)
				if len(ench) > 0 {
					itemCount = 1
				}
			}

			v["Count"] = itemCount
		}
	}

	buf = bytes.NewBuffer(nil)
	nbt.NewEncoderWithEncoding(buf, nbt.LittleEndian).Encode(m)
	os.WriteFile("mcstructure/cic.mcstructure", buf.Bytes(), 0600)
}
