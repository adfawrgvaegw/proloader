package main

import (
	"bytes"
	"math/rand"
	"os"

	"github.com/Yeah114/FunShuttler/core/minecraft/nbt"
)

func ChangeContainerName() {
	var m map[string]any

	bs, err := os.ReadFile("mcstructure/ori_ccn.mcstructure")
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
		var containerName string

		switch rand.Intn(3) {
		case 0:
			containerName = "Happy2018new"
		case 1:
			containerName = "Liliya233"
		case 2:
			containerName = "CMA2401PT"
		}

		value.(map[string]any)["block_entity_data"].(map[string]any)["CustomName"] = containerName
	}

	buf = bytes.NewBuffer(nil)
	nbt.NewEncoderWithEncoding(buf, nbt.LittleEndian).Encode(m)
	os.WriteFile("mcstructure/ccn.mcstructure", buf.Bytes(), 0600)
}
