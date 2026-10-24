package main

import (
	"bytes"
	"math/rand"
	"os"

	"github.com/Yeah114/FunShuttler/core/minecraft/nbt"
	"github.com/Yeah114/FunShuttler/utils"
	"github.com/google/uuid"
)

func GenerateRandomCommandBlock() {
	var m map[string]any

	itemList := make([]any, 0)
	for index := range 27 {
		m2 := map[string]any{
			"Command":            "",
			"CustomName":         "",
			"TickDelay":          int32(0),
			"ExecuteOnFirstTick": byte(0),
			"TrackOutput":        byte(0),
			"conditionalMode":    byte(0),
			"auto":               byte(0),
		}

		if rand.Intn(2) == 1 {
			switch rand.Intn(3) {
			case 0:
				m2["Command"] = utils.MakeUUIDSafeString(uuid.New())
			case 1:
				m2["Command"] = "say 123"
			case 2:
				m2["Command"] = "/YoRHa"
			}
		}
		if rand.Intn(2) == 1 {
			switch rand.Intn(3) {
			case 0:
				m2["CustomName"] = "Liliya233"
			case 1:
				m2["CustomName"] = "Happy2018new"
			case 2:
				m2["CustomName"] = "CMA2401PT"
			}
		}
		if rand.Intn(2) == 1 {
			switch rand.Intn(3) {
			case 0:
				m2["TickDelay"] = int32(2018)
			case 1:
				m2["TickDelay"] = int32(233)
			case 2:
				m2["TickDelay"] = int32(rand.Intn(2402))
			}
		}
		if rand.Intn(2) == 1 {
			m2["ExecuteOnFirstTick"] = byte(1)
		}
		if rand.Intn(2) == 1 {
			m2["TrackOutput"] = byte(1)
		}
		if rand.Intn(2) == 1 {
			m2["conditionalMode"] = byte(1)
		}
		if rand.Intn(2) == 1 {
			m2["auto"] = byte(1)
		}

		commandBlockName := "minecraft:command_block"
		switch rand.Intn(3) {
		case 1:
			commandBlockName = "minecraft:chain_command_block"
		case 2:
			commandBlockName = "minecraft:repeating_command_block"
		}

		itemList = append(itemList, map[string]any{
			"Count":       byte(1),
			"Damage":      int16(0),
			"Name":        commandBlockName,
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
	os.WriteFile("mcstructure/command_block.mcstructure", buf.Bytes(), 0600)
}
