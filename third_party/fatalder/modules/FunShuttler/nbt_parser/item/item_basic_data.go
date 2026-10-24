package nbt_parser_item

import (
	"fmt"
	"strings"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"

	"github.com/df-mc/worldupgrader/itemupgrader"
	"github.com/mitchellh/mapstructure"
)

// ItemBasicData 是物品的基本数据
type ItemBasicData struct {
	Name     string `mapstructure:"Name"`   // 物品名称
	Count    uint8  `mapstructure:"Count"`  // 物品数量
	Metadata int16  `mapstructure:"Damage"` // 物品元数据
}

func ParseItemBasicData(nbtMap map[string]any) (result ItemBasicData, err error) {
	err = mapstructure.Decode(&nbtMap, &result)
	if err != nil {
		return result, fmt.Errorf("ParseItemBasicData: %v", err)
	}

	newItem := itemupgrader.Upgrade(itemupgrader.ItemMeta{
		Name: result.Name,
		Meta: result.Metadata,
	})
	result.Name = newItem.Name
	result.Metadata = newItem.Meta

	switch result.Name {
	case "minecraft:npc_spawn_egg":
		result.Name = "minecraft:spawn_egg"
		result.Metadata = 51
	case "minecraft:agent_spawn_egg":
		result.Name = "minecraft:spawn_egg"
		result.Metadata = 56
	}

	tag, ok := nbtMap["tag"].(map[string]any)
	if ok {
		damage, ok := tag["Damage"].(int32)
		if ok {
			result.Metadata = int16(damage)
		}
	}

	return result, nil
}

func ParseItemBasicDataNetwork(item protocol.ItemStack, itemName string) (result ItemBasicData, err error) {
	result.Name = strings.ToLower(itemName)
	if !strings.HasPrefix(result.Name, "minecraft:") {
		result.Name = "minecraft:" + result.Name
	}
	switch result.Name {
	case "minecraft:npc_spawn_egg":
		result.Name = "minecraft:spawn_egg"
		result.Metadata = 51
	case "minecraft:agent_spawn_egg":
		result.Name = "minecraft:spawn_egg"
		result.Metadata = 56
	}

	result.Count = uint8(item.Count)
	result.Metadata = int16(item.MetadataValue)

	if item.NBTData != nil {
		damage, ok := item.NBTData["Damage"].(int32)
		if ok {
			result.Metadata = int16(damage)
		}
	}

	return
}
