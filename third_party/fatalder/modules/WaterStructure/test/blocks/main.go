package main

import (
	"fmt"

	"github.com/Yeah114/blocks"
)

func main() {
	runtimeID, _ := blocks.BlockStrToRuntimeID(`minecraft:mangrove_carpet`)
	fmt.Println(runtimeID)
	//runtimeID, _ := blocks.SchemBlockStrToRuntimeID("minecraft:grass_block[snowy=false]")
	//fmt.Println(runtimeID)
	blockName, _ := blocks.RuntimeIDToBlockNameWithStateStr(runtimeID)
	fmt.Println(blockName)
}