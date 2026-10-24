package main

import (
	"bytes"
	"math/rand"
	"os"

	"github.com/Yeah114/FunShuttler/core/minecraft/nbt"
	"github.com/Yeah114/FunShuttler/mapping"
)

func GenerateRandomBanner() {
	var m map[string]any

	itemList := make([]any, 0)
	for index := range 27 {
		base := int32(rand.Intn(16))
		m2 := map[string]any{
			"Base": base,
			"Type": int32(0),
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

		m2["Patterns"] = patterns

		itemList = append(itemList, map[string]any{
			"Count":       byte(1),
			"Damage":      int16(base),
			"Name":        "minecraft:banner",
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
	os.WriteFile("mcstructure/banner.mcstructure", buf.Bytes(), 0600)
}
