package main

import (
	"bytes"
	"math/rand"
	"os"

	"github.com/Yeah114/FunShuttler/core/minecraft/nbt"
	"github.com/Yeah114/FunShuttler/mapping"
)

func GenerateRandomShield() {
	var m map[string]any

	itemList := make([]any, 0)
	for index := range 27 {
		m2 := map[string]any{
			"Damage": int32(rand.Intn(233)),
		}

		enchs := make([]any, 0)
		if rand.Intn(2) == 1 {
			enchs = append(enchs, map[string]any{
				"id":  int16(17),
				"lvl": int16(rand.Intn(3) + 1),
			})
		}
		if rand.Intn(2) == 1 {
			enchs = append(enchs, map[string]any{
				"id":  int16(26),
				"lvl": int16(1),
			})
		}
		if len(enchs) > 0 {
			m2["ench"] = enchs
		}

		if rand.Intn(101) > 80 {
			itemList = append(itemList, map[string]any{
				"Count":       byte(1),
				"Damage":      int16(0),
				"Name":        "minecraft:shield",
				"Slot":        byte(index),
				"WasPickedUp": byte(0),
				"tag":         m2,
			})
			continue
		}

		if rand.Intn(101) > 70 {
			m2["base"] = int32(15)
			m2["Patterns"] = []any{
				map[string]any{
					"Pattern": "ill",
					"Color":   int32(15),
				},
			}
			itemList = append(itemList, map[string]any{
				"Count":       byte(1),
				"Damage":      int16(0),
				"Name":        "minecraft:shield",
				"Slot":        byte(index),
				"WasPickedUp": byte(0),
				"tag":         m2,
			})
			continue
		}

		patterns := make([]any, 0)
		dyeCount := rand.Intn(7)

		allPatterns := make([]string, 0)
		for pattern := range mapping.BannerPatternToItemName {
			allPatterns = append(allPatterns, pattern)
		}

		for range dyeCount {
			patterns = append(patterns, map[string]any{
				"Color":   int32(rand.Intn(16)),
				"Pattern": allPatterns[rand.Intn(len(allPatterns))],
			})
		}

		m2["base"] = int32(rand.Intn(16))
		m2["Patterns"] = patterns

		itemList = append(itemList, map[string]any{
			"Count":       byte(1),
			"Damage":      int16(0),
			"Name":        "minecraft:shield",
			"Slot":        byte(index),
			"WasPickedUp": byte(0),
			"tag":         m2,
		})
	}

	bs, err := os.ReadFile("mcstructure/ori.mcstructure")
	if err != nil {
		panic(err)
	}

	buf := bytes.NewBuffer(bs)
	err = nbt.NewDecoderWithEncoding(buf, nbt.LittleEndian).Decode(&m)
	if err != nil {
		panic(err)
	}

	m1 := m["structure"].(map[string]any)["palette"].(map[string]any)["default"].(map[string]any)
	m1 = m1["block_position_data"].(map[string]any)["0"].(map[string]any)["block_entity_data"].(map[string]any)
	m1["Items"] = itemList

	buf = bytes.NewBuffer(nil)
	nbt.NewEncoderWithEncoding(buf, nbt.LittleEndian).Encode(m)
	os.WriteFile("mcstructure/shield.mcstructure", buf.Bytes(), 0600)
}
