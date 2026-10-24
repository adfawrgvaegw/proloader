package main

import (
	"bytes"
	"math/rand"
	"os"

	"github.com/Yeah114/FunShuttler/core/minecraft/nbt"
	"github.com/Yeah114/FunShuttler/mapping"
	"github.com/Yeah114/FunShuttler/utils"
	"github.com/google/uuid"
)

func GenerateRandomSign() {
	var m map[string]any

	itemList := make([]any, 0)
	for index := range 20 {
		m2 := map[string]any{
			"IsWaxed": byte(0),
			"FrontText": map[string]any{
				"IgnoreLighting": byte(0),
				"SignTextColor":  utils.EncodeVarRGBA(0, 0, 0, 255),
				"Text":           "",
			},
			"BackText": map[string]any{
				"IgnoreLighting": byte(0),
				"SignTextColor":  utils.EncodeVarRGBA(0, 0, 0, 255),
				"Text":           "",
			},
		}

		if rand.Intn(2) == 1 {
			m2["IsWaxed"] = byte(1)
		}
		if rand.Intn(2) == 1 {
			if rand.Intn(2) == 1 {
				m2["FrontText"].(map[string]any)["IgnoreLighting"] = byte(1)
			}
			if rand.Intn(2) == 1 {
				color := mapping.DefaultDyeColor[rand.Intn(len(mapping.DefaultDyeColor))]
				m2["FrontText"].(map[string]any)["SignTextColor"] = utils.EncodeVarRGBA(color[0], color[1], color[2], 255)
			}
			if rand.Intn(2) == 1 {
				switch rand.Intn(3) {
				case 0:
					m2["FrontText"].(map[string]any)["Text"] = utils.MakeUUIDSafeString(uuid.New())
				case 1:
					m2["FrontText"].(map[string]any)["Text"] = "爱"
				case 2:
					m2["FrontText"].(map[string]any)["Text"] = "永恒"
				}
			}
		}
		if rand.Intn(2) == 1 {
			if rand.Intn(2) == 1 {
				m2["BackText"].(map[string]any)["IgnoreLighting"] = byte(1)
			}
			if rand.Intn(2) == 1 {
				color := mapping.DefaultDyeColor[rand.Intn(len(mapping.DefaultDyeColor))]
				m2["BackText"].(map[string]any)["SignTextColor"] = utils.EncodeVarRGBA(color[0], color[1], color[2], 255)
			}
			if rand.Intn(2) == 1 {
				switch rand.Intn(3) {
				case 0:
					m2["BackText"].(map[string]any)["Text"] = utils.MakeUUIDSafeString(uuid.New())
				case 1:
					m2["BackText"].(map[string]any)["Text"] = "爱"
				case 2:
					m2["BackText"].(map[string]any)["Text"] = "永恒"
				}
			}
		}

		signItemNames := make([]string, 0)
		for key, value := range mapping.ItemNameToBlockName {
			if mapping.SupportBlocksPool[value] == mapping.SupportNBTBlockTypeSign {
				signItemNames = append(signItemNames, key)
			}
		}

		itemList = append(itemList, map[string]any{
			"Count":       byte(1),
			"Damage":      int16(0),
			"Name":        signItemNames[rand.Intn(len(signItemNames))],
			"Slot":        byte(index),
			"WasPickedUp": byte(0),
			"tag":         m2,
		})
	}

	for index := 20; index <= 26; index++ {
		m2 := map[string]any{
			"IgnoreLighting": byte(0),
			"SignTextColor":  utils.EncodeVarRGBA(0, 0, 0, 255),
			"Text":           "",
		}

		if rand.Intn(2) == 1 {
			m2["IgnoreLighting"] = byte(1)
		}
		if rand.Intn(2) == 1 {
			color := mapping.DefaultDyeColor[rand.Intn(len(mapping.DefaultDyeColor))]
			m2["SignTextColor"] = utils.EncodeVarRGBA(color[0], color[1], color[2], 255)
		}
		if rand.Intn(2) == 1 {
			switch rand.Intn(3) {
			case 0:
				m2["Text"] = utils.MakeUUIDSafeString(uuid.New())
			case 1:
				m2["Text"] = "爱"
			case 2:
				m2["Text"] = "永恒"
			}
		}

		signItemNames := make([]string, 0)
		for key, value := range mapping.ItemNameToBlockName {
			if mapping.SupportBlocksPool[value] == mapping.SupportNBTBlockTypeSign {
				signItemNames = append(signItemNames, key)
			}
		}

		itemList = append(itemList, map[string]any{
			"Count":       byte(1),
			"Damage":      int16(0),
			"Name":        signItemNames[rand.Intn(len(signItemNames))],
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
	os.WriteFile("mcstructure/sign.mcstructure", buf.Bytes(), 0600)
}
