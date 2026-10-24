package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/Yeah114/FunShuttler/game_control/game_interface"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/pterm/pterm"
)

func SystemTestingItemCopy() {
	tA := time.Now()

	// Test round 1
	{
		success, err := api.ContainerOpenAndClose().OpenInventory()
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 1 failed due to %v (stage 1)", err))
		}
		if !success {
			panic("SystemTestingItemCopy: Failed on test round 1")
		}

		api.Commands().SendSettingsCommand("clear", true)
		api.Commands().SendSettingsCommand("gamemode 1", true)
		api.Commands().SendSettingsCommand("tp 0 0 0", true)
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand("setblock 0 0 0 air", true)
		success, _, _, _ = api.ItemStackOperation().OpenTransaction().
			GetCreativeItemToInventory(1570, 6, 1).
			Commit()
		if !success {
			panic("SystemTestingItemCopy: Failed on test round 1")
		}

		err = api.ContainerOpenAndClose().CloseContainer()
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 1 failed due to %v (stage 2)", err))
		}

		api.BotClick().ChangeSelectedHotbarSlot(5)
		err = api.SetBlock().SetBlock([3]int32{0, 0, 0}, "barrel", `["facing_direction"=1,"open_bit"=false]`)
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 1 failed due to %v (stage 3)", err))
		}

		targetItems := make([]*game_interface.ItemInfo, 27)
		for index := range targetItems {
			targetItems[index] = &game_interface.ItemInfo{Count: 16, ItemType: 0}
		}

		err = api.ItemCopy().CopyItem(
			game_interface.UseItemOnBlocks{
				HotbarSlotID: 5,
				BotPos:       mgl32.Vec3{0, 0, 0},
				BlockPos:     [3]int32{0, 0, 0},
				BlockName:    "barrel",
				BlockStates: map[string]any{
					"facing_direction": int32(1),
					"open_bit":         byte(0),
				},
			},
			[]game_interface.ItemInfoWithSlot{
				{
					Slot:     6,
					ItemInfo: game_interface.ItemInfo{Count: 1, ItemType: 0},
				},
			},
			targetItems,
		)
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 1 failed due to %v (stage 4)", err))
		}
	}

	// Test round 2
	{
		success, err := api.ContainerOpenAndClose().OpenInventory()
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 2 failed due to %v (stage 1)", err))
		}
		if !success {
			panic("SystemTestingItemCopy: Failed on test round 2")
		}

		api.Commands().SendSettingsCommand("clear", true)
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand("setblock 0 0 0 air", true)
		success, _, _, _ = api.ItemStackOperation().OpenTransaction().
			GetCreativeItemToInventory(1570, 6, 1).
			Commit()
		if !success {
			panic("SystemTestingItemCopy: Failed on test round 2")
		}

		err = api.ContainerOpenAndClose().CloseContainer()
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 2 failed due to %v (stage 2)", err))
		}

		api.BotClick().ChangeSelectedHotbarSlot(8)
		err = api.SetBlock().SetBlock([3]int32{0, 0, 0}, "barrel", `["facing_direction"=1,"open_bit"=false]`)
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 2 failed due to %v (stage 3)", err))
		}

		targetItems := make([]*game_interface.ItemInfo, 27)
		for index := range targetItems {
			targetItems[index] = &game_interface.ItemInfo{Count: 1, ItemType: 0}
		}

		err = api.ItemCopy().CopyItem(
			game_interface.UseItemOnBlocks{
				HotbarSlotID: 8,
				BotPos:       mgl32.Vec3{0, 0, 0},
				BlockPos:     [3]int32{0, 0, 0},
				BlockName:    "barrel",
				BlockStates: map[string]any{
					"facing_direction": int32(1),
					"open_bit":         byte(0),
				},
			},
			[]game_interface.ItemInfoWithSlot{
				{
					Slot:     6,
					ItemInfo: game_interface.ItemInfo{Count: 1, ItemType: 0},
				},
			},
			targetItems,
		)
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 2 failed due to %v (stage 4)", err))
		}
	}

	// Test round 3 (Random)
	{
		success, err := api.ContainerOpenAndClose().OpenInventory()
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 3 failed due to %v (stage 1)", err))
		}
		if !success {
			panic("SystemTestingItemCopy: Failed on test round 3")
		}

		api.Commands().SendSettingsCommand("clear", true)
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand("setblock 0 0 0 air", true)
		success, _, _, _ = api.ItemStackOperation().OpenTransaction().
			GetCreativeItemToInventory(1570, 6, 1).
			Commit()
		if !success {
			panic("SystemTestingItemCopy: Failed on test round 3")
		}

		err = api.ContainerOpenAndClose().CloseContainer()
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 3 failed due to %v (stage 2)", err))
		}

		api.BotClick().ChangeSelectedHotbarSlot(8)
		err = api.SetBlock().SetBlock([3]int32{0, 0, 0}, "barrel", `["facing_direction"=1,"open_bit"=false]`)
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 3 failed due to %v (stage 3)", err))
		}

		targetItems := make([]*game_interface.ItemInfo, 27)
		for index := range targetItems {
			randomCount := rand.Intn(17)
			if randomCount == 0 {
				continue
			}
			targetItems[index] = &game_interface.ItemInfo{Count: uint8(randomCount), ItemType: 0}
		}

		err = api.ItemCopy().CopyItem(
			game_interface.UseItemOnBlocks{
				HotbarSlotID: 8,
				BotPos:       mgl32.Vec3{0, 0, 0},
				BlockPos:     [3]int32{0, 0, 0},
				BlockName:    "barrel",
				BlockStates: map[string]any{
					"facing_direction": int32(1),
					"open_bit":         byte(0),
				},
			},
			[]game_interface.ItemInfoWithSlot{
				{
					Slot:     6,
					ItemInfo: game_interface.ItemInfo{Count: 1, ItemType: 0},
				},
			},
			targetItems,
		)
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 3 failed due to %v (stage 4)", err))
		}
	}

	// Test round 4 (Multiple items & Random)
	{
		success, err := api.ContainerOpenAndClose().OpenInventory()
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 4 failed due to %v (stage 1)", err))
		}
		if !success {
			panic("SystemTestingItemCopy: Failed on test round 4")
		}

		api.Commands().SendSettingsCommand("clear", true)
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand("setblock 0 0 0 air", true)
		success, _, _, _ = api.ItemStackOperation().OpenTransaction().
			GetCreativeItemToInventory(1570, 6, 1). // Slot 6
			Commit()
		if !success {
			panic("SystemTestingItemCopy: Failed on test round 4")
		}

		err = api.ContainerOpenAndClose().CloseContainer()
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 4 failed due to %v (stage 2)", err))
		}

		api.Commands().SendSettingsCommand("give @s enchanted_book", true) // Slot 0
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand("give @s allow", true) // Slot 1
		api.Commands().AwaitChangesGeneral()

		api.BotClick().ChangeSelectedHotbarSlot(3)
		err = api.SetBlock().SetBlock([3]int32{0, 0, 0}, "chest", `["minecraft:cardinal_direction"="east"]`)
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 4 failed due to %v (stage 3)", err))
		}

		targetItems := make([]*game_interface.ItemInfo, 27)
		for index := range targetItems {
			var randomCount int

			itemType := rand.Intn(3)
			switch itemType {
			case 0:
				randomCount = rand.Intn(17)
			case 1:
				randomCount = rand.Intn(65)
			case 2:
				randomCount = rand.Intn(2)
			}

			if randomCount == 0 {
				continue
			}
			targetItems[index] = &game_interface.ItemInfo{Count: uint8(randomCount), ItemType: game_interface.ItemType(itemType)}
		}

		err = api.ItemCopy().CopyItem(
			game_interface.UseItemOnBlocks{
				HotbarSlotID: 3,
				BotPos:       mgl32.Vec3{0, 0, 0},
				BlockPos:     [3]int32{0, 0, 0},
				BlockName:    "chest",
				BlockStates: map[string]any{
					"minecraft:cardinal_direction": "east",
				},
			},
			[]game_interface.ItemInfoWithSlot{
				{
					Slot:     6,
					ItemInfo: game_interface.ItemInfo{Count: 1, ItemType: 0},
				},
				{
					Slot:     1,
					ItemInfo: game_interface.ItemInfo{Count: 1, ItemType: 1},
				},
				{
					Slot:     0,
					ItemInfo: game_interface.ItemInfo{Count: 1, ItemType: 2},
				},
			},
			targetItems,
		)
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 4 failed due to %v (stage 4)", err))
		}
	}

	// Test round 5
	{
		success, err := api.ContainerOpenAndClose().OpenInventory()
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 5 failed due to %v (stage 1)", err))
		}
		if !success {
			panic("SystemTestingItemCopy: Failed on test round 5")
		}

		api.Commands().SendSettingsCommand("clear", true)
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand("setblock 0 0 0 air", true)
		success, _, _, _ = api.ItemStackOperation().OpenTransaction().
			GetCreativeItemToInventory(1570, 6, 16).
			Commit()
		if !success {
			panic("SystemTestingItemCopy: Failed on test round 5")
		}

		err = api.ContainerOpenAndClose().CloseContainer()
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 5 failed due to %v (stage 2)", err))
		}

		api.BotClick().ChangeSelectedHotbarSlot(8)
		err = api.SetBlock().SetBlock([3]int32{0, 0, 0}, "chest", `["minecraft:cardinal_direction"="east"]`)
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 4 failed due to %v (stage 3)", err))
		}

		targetItems := make([]*game_interface.ItemInfo, 27)
		for index := range targetItems {
			targetItems[index] = &game_interface.ItemInfo{Count: 3, ItemType: 7}
		}

		err = api.ItemCopy().CopyItem(
			game_interface.UseItemOnBlocks{
				HotbarSlotID: 8,
				BotPos:       mgl32.Vec3{0, 0, 0},
				BlockPos:     [3]int32{0, 0, 0},
				BlockName:    "chest",
				BlockStates: map[string]any{
					"minecraft:cardinal_direction": "east",
				},
			},
			[]game_interface.ItemInfoWithSlot{
				{
					Slot:     6,
					ItemInfo: game_interface.ItemInfo{Count: 16, ItemType: 7},
				},
			},
			targetItems,
		)
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 5 failed due to %v (stage 4)", err))
		}
	}

	// Test round 6
	{
		success, err := api.ContainerOpenAndClose().OpenInventory()
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 6 failed due to %v (stage 1)", err))
		}
		if !success {
			panic("SystemTestingItemCopy: Failed on test round 6")
		}

		api.Commands().SendSettingsCommand("clear", true)
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand("setblock 0 0 0 air", true)
		success, _, _, _ = api.ItemStackOperation().OpenTransaction().
			GetCreativeItemToInventory(1570, 6, 16).
			Commit()
		if !success {
			panic("SystemTestingItemCopy: Failed on test round 6")
		}

		err = api.ContainerOpenAndClose().CloseContainer()
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 6 failed due to %v (stage 2)", err))
		}

		api.BotClick().ChangeSelectedHotbarSlot(8)
		err = api.SetBlock().SetBlock([3]int32{0, 0, 0}, "chest", `["minecraft:cardinal_direction"="east"]`)
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 6 failed due to %v (stage 3)", err))
		}

		targetItems := make([]*game_interface.ItemInfo, 4)
		for index := range targetItems {
			targetItems[index] = &game_interface.ItemInfo{Count: 3, ItemType: 7}
		}

		err = api.ItemCopy().CopyItem(
			game_interface.UseItemOnBlocks{
				HotbarSlotID: 8,
				BotPos:       mgl32.Vec3{0, 0, 0},
				BlockPos:     [3]int32{0, 0, 0},
				BlockName:    "chest",
				BlockStates: map[string]any{
					"minecraft:cardinal_direction": "east",
				},
			},
			[]game_interface.ItemInfoWithSlot{
				{
					Slot:     6,
					ItemInfo: game_interface.ItemInfo{Count: 16, ItemType: 7},
				},
			},
			targetItems,
		)
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 6 failed due to %v (stage 4)", err))
		}
	}

	// Test round 7
	{

		api.Commands().SendSettingsCommand("clear", true)
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand("setblock 0 0 0 air", true)
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand("give @s deny 1", true)
		api.Commands().AwaitChangesGeneral()

		api.BotClick().ChangeSelectedHotbarSlot(4)
		err := api.SetBlock().SetBlock([3]int32{0, 0, 0}, "chest", `["minecraft:cardinal_direction"="east"]`)
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 7 failed due to %v (stage 1)", err))
		}

		targetItems := make([]*game_interface.ItemInfo, 27)
		for index := range targetItems {
			targetItems[index] = &game_interface.ItemInfo{Count: 64, ItemType: 240}
		}

		err = api.ItemCopy().CopyItem(
			game_interface.UseItemOnBlocks{
				HotbarSlotID: 4,
				BotPos:       mgl32.Vec3{0, 0, 0},
				BlockPos:     [3]int32{0, 0, 0},
				BlockName:    "chest",
				BlockStates: map[string]any{
					"minecraft:cardinal_direction": "east",
				},
			},
			[]game_interface.ItemInfoWithSlot{
				{
					Slot:     0,
					ItemInfo: game_interface.ItemInfo{Count: 1, ItemType: 240},
				},
			},
			targetItems,
		)
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 7 failed due to %v (stage 2)", err))
		}
	}

	// Test round 8 (Multiple items also base items & Random)
	{
		success, err := api.ContainerOpenAndClose().OpenInventory()
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 8 failed due to %v (stage 1)", err))
		}
		if !success {
			panic("SystemTestingItemCopy: Failed on test round 8")
		}

		api.Commands().SendSettingsCommand("clear", true)
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand("setblock 0 0 0 air", true)
		success, _, _, _ = api.ItemStackOperation().OpenTransaction().
			GetCreativeItemToInventory(1570, 6, 9). // Slot 6
			Commit()
		if !success {
			panic("SystemTestingItemCopy: Failed on test round 8")
		}

		err = api.ContainerOpenAndClose().CloseContainer()
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 8 failed due to %v (stage 2)", err))
		}

		api.Commands().SendSettingsCommand("give @s enchanted_book", true) // Slot 0
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand("give @s allow 25", true) // Slot 1
		api.Commands().AwaitChangesGeneral()

		api.BotClick().ChangeSelectedHotbarSlot(3)
		err = api.SetBlock().SetBlock([3]int32{0, 0, 0}, "chest", `["minecraft:cardinal_direction"="east"]`)
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 8 failed due to %v (stage 3)", err))
		}

		targetItems := make([]*game_interface.ItemInfo, 27)
		for index := range targetItems {
			var randomCount int

			itemType := rand.Intn(3)
			switch itemType {
			case 0:
				randomCount = rand.Intn(17)
			case 1:
				randomCount = rand.Intn(65)
			case 2:
				randomCount = rand.Intn(2)
			}

			if randomCount == 0 {
				continue
			}
			targetItems[index] = &game_interface.ItemInfo{Count: uint8(randomCount), ItemType: game_interface.ItemType(itemType)}
		}

		err = api.ItemCopy().CopyItem(
			game_interface.UseItemOnBlocks{
				HotbarSlotID: 3,
				BotPos:       mgl32.Vec3{0, 0, 0},
				BlockPos:     [3]int32{0, 0, 0},
				BlockName:    "chest",
				BlockStates: map[string]any{
					"minecraft:cardinal_direction": "east",
				},
			},
			[]game_interface.ItemInfoWithSlot{
				{
					Slot:     6,
					ItemInfo: game_interface.ItemInfo{Count: 9, ItemType: 0},
				},
				{
					Slot:     1,
					ItemInfo: game_interface.ItemInfo{Count: 25, ItemType: 1},
				},
				{
					Slot:     0,
					ItemInfo: game_interface.ItemInfo{Count: 1, ItemType: 2},
				},
			},
			targetItems,
		)
		if err != nil {
			panic(fmt.Sprintf("SystemTestingItemCopy: Test round 8 failed due to %v (stage 4)", err))
		}
	}

	pterm.Success.Printfln("SystemTestingItemCopy: PASS (Time used = %v)", time.Since(tA))
}
