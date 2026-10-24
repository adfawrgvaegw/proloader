package main

import (
	"bytes"
	"math/rand"
	"os"

	"github.com/Yeah114/FunShuttler/core/minecraft/nbt"
)

func ChangeShulkerFacing() {
	var m map[string]any

	bs, err := os.ReadFile("mcstructure/ori_csf.mcstructure")
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
		v := value.(map[string]any)["block_entity_data"].(map[string]any)
		v["facing"] = byte(rand.Intn(6))
	}

	buf = bytes.NewBuffer(nil)
	nbt.NewEncoderWithEncoding(buf, nbt.LittleEndian).Encode(m)
	os.WriteFile("mcstructure/csf.mcstructure", buf.Bytes(), 0600)
}
