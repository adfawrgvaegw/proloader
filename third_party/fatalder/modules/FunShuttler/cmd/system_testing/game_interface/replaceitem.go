package main

import (
	"fmt"
	"slices"
	"time"

	"github.com/Yeah114/FunShuttler/game_control/game_interface"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/pterm/pterm"
)

func SystemTestingReplaceitem() {
	tA := time.Now()

	// Test round 1
	{
		api.Commands().SendSettingsCommand("tp 0 0 0", true)
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand("setblock 0 0 0 air", true)
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand("setblock 0 1 0 air", true)
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand(`setblock 0 0 0 chest ["minecraft:cardinal_direction"="east"]`, true)
		api.Commands().AwaitChangesGeneral()

		api.Replaceitem().ReplaceitemInContainerAsync(
			[3]int32{0, 0, 0},
			game_interface.ReplaceitemInfo{
				Name:     "apple",
				Count:    10,
				MetaData: 0,
				Slot:     25,
			},
			`{"can_place_on":{"blocks":["glass"]}}`,
		)
		api.Commands().AwaitChangesGeneral()

		success, err := api.ContainerOpenAndClose().OpenContainer(
			game_interface.UseItemOnBlocks{
				HotbarSlotID: 0,
				BotPos:       mgl32.Vec3{0, 0, 0},
				BlockPos:     [3]int32{0, 0, 0},
				BlockName:    "chest",
				BlockStates: map[string]any{
					"minecraft:cardinal_direction": "east",
				},
			},
			true,
		)
		if err != nil {
			panic(fmt.Sprintf("SystemTestingReplaceitem: Test round 1 failed due to %v (stage 1)", err))
		}
		if !success {
			panic("SystemTestingReplaceitem: Test round 1 failed")
		}

		containerData, _, existed := api.Resources().Container().ContainerData()
		if !existed {
			panic("SystemTestingReplaceitem: Test round 1 failed")
		}

		item, inventoryExisted := api.Resources().Inventories().GetItemStack(
			resources_control.WindowID(containerData.WindowID),
			25,
		)
		if !inventoryExisted {
			panic("SystemTestingReplaceitem: Test round 1 failed")
		}

		if item.Stack.Count != 10 {
			panic("SystemTestingReplaceitem: Test round 1 failed")
		}
		if !slices.Equal(item.Stack.CanBePlacedOn, []string{"minecraft:glass"}) {
			panic("SystemTestingReplaceitem: Test round 1 failed")
		}

		err = api.ContainerOpenAndClose().CloseContainer()
		if err != nil {
			panic(fmt.Sprintf("SystemTestingReplaceitem: Test round 1 failed due to %v (stage 2)", err))
		}
	}

	// Test round 2
	{
		api.Commands().SendSettingsCommand("clear", true)
		api.Commands().AwaitChangesGeneral()

		api.Replaceitem().ReplaceitemInInventory(
			"@s", game_interface.ReplacePathHotbarOnly,
			game_interface.ReplaceitemInfo{
				Name:     "apple",
				Count:    8,
				MetaData: 0,
				Slot:     4,
			},
			`{"can_place_on":{"blocks":["grass"]}}`,
			true,
		)

		item, inventoryExisted := api.Resources().Inventories().GetItemStack(0, 4)
		if !inventoryExisted {
			panic("SystemTestingReplaceitem: Test round 2 failed")
		}

		if item.Stack.Count != 8 {
			panic("SystemTestingReplaceitem: Test round 2 failed")
		}
		if !slices.Equal(item.Stack.CanBePlacedOn, []string{"minecraft:grass_block"}) {
			panic("SystemTestingReplaceitem: Test round 2 failed")
		}
	}

	// Test round 3
	{
		api.Commands().SendSettingsCommand("clear", true)
		api.Commands().AwaitChangesGeneral()

		api.Replaceitem().ReplaceitemInInventory(
			"@s", game_interface.ReplacePathHotbarOnly,
			game_interface.ReplaceitemInfo{
				Name:     "apple",
				Count:    64,
				MetaData: 89,
				Slot:     6,
			},
			`{"can_place_on":{"blocks":["bed"]}}`,
			true,
		)

		item, inventoryExisted := api.Resources().Inventories().GetItemStack(0, 6)
		if !inventoryExisted {
			panic("SystemTestingReplaceitem: Test round 3 failed")
		}

		if item.Stack.Count != 64 {
			panic("SystemTestingReplaceitem: Test round 3 failed")
		}
		if !slices.Equal(item.Stack.CanBePlacedOn, []string{"minecraft:bed"}) {
			panic("SystemTestingReplaceitem: Test round 3 failed")
		}
	}

	pterm.Success.Printfln("SystemTestingReplaceitem: PASS (Time used = %v)", time.Since(tA))
}
