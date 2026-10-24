package main

import (
	"fmt"
	"github.com/Yeah114/Fatalder/utils/command_upgrader"
)

func main() {
	command := "/execute @e[type=zombie] ~ ~ ~ detect ~ ~-1 ~ minecraft:sand 2 summon lightning_bolt"
	command, _, err := command_upgrader.UpgradeExecuteCommand(command)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println(command)
}