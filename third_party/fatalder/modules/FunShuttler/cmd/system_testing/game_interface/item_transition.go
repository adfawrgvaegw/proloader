package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/Yeah114/FunShuttler/game_control/game_interface"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/pterm/pterm"
)

func SystemTestingItemTransition() {
	tA := time.Now()

	api.Commands().SendSettingsCommand("clear", true)
	api.Commands().SendSettingsCommand("tp 0 0 0", true)
	api.Commands().AwaitChangesGeneral()
	api.Commands().SendSettingsCommand("give @s apple 192", true) // Slot 0, 1, 2
	api.Commands().AwaitChangesGeneral()
	api.Commands().SendSettingsCommand("give @s deny 192", true) // Slot 3, 4, 5
	api.Commands().AwaitChangesGeneral()
	api.Commands().SendSettingsCommand("give @s allow 192", true) // Slot 6, 7, 8
	api.Commands().AwaitChangesGeneral()

	api.SetBlock().SetBlockAsync([3]int32{0, 1, 0}, "air", "[]")
	api.SetBlock().SetBlock([3]int32{0, 0, 0}, "air", `[]`)
	api.SetBlock().SetBlock([3]int32{0, 0, 0}, "chest", `["minecraft:cardinal_direction"="east"]`)
	success, _ := api.ContainerOpenAndClose().OpenContainer(
		game_interface.UseItemOnBlocks{
			HotbarSlotID: 0,
			BotPos:       mgl32.Vec3{0, 0, 0},
			BlockPos:     [3]int32{0, 0, 0},
			BlockName:    "chest",
			BlockStates: map[string]any{
				`minecraft:cardinal_direction`: "east",
			},
		},
		true,
	)
	if !success {
		panic("SystemTestingItemTransition: Failed on test round 1")
	}

	aCount := 192
	bCount := 192
	cCount := 192

	dst := make([]game_interface.ItemInfoWithSlot, 0)
	for index := range 27 {
		var wantCount int
		itemType := rand.Intn(3)

		switch itemType {
		case 0:
			if aCount == 0 {
				break
			}
			wantCount = rand.Intn(min(65, aCount+1))
			aCount -= wantCount
		case 1:
			if bCount == 0 {
				break
			}
			wantCount = rand.Intn(min(65, bCount+1))
			bCount -= wantCount
		case 2:
			if cCount == 0 {
				break
			}
			wantCount = rand.Intn(min(65, cCount+1))
			cCount -= wantCount
		}

		if wantCount == 0 {
			continue
		}

		dst = append(dst, game_interface.ItemInfoWithSlot{
			Slot: resources_control.SlotID(index),
			ItemInfo: game_interface.ItemInfo{
				Count:    uint8(wantCount),
				ItemType: game_interface.ItemType(itemType),
			},
		})
	}

	src := make([]game_interface.ItemInfoWithSlot, 0)
	for itemType := range 3 {
		for index := range 3 {
			src = append(src, game_interface.ItemInfoWithSlot{
				Slot: resources_control.SlotID(itemType*3 + index),
				ItemInfo: game_interface.ItemInfo{
					Count:    64,
					ItemType: game_interface.ItemType(itemType),
				},
			})
		}
	}
	success, err := api.ItemTransition().TransitionToContainer(src, dst)
	if err != nil {
		panic(fmt.Sprintf("SystemTestingItemTransition: Failed on test round 1 due to %v (stage 1)", err))
	}
	if !success {
		panic("SystemTestingItemTransition: Failed on test round 1")
	}

	err = api.ContainerOpenAndClose().CloseContainer()
	if err != nil {
		panic(fmt.Sprintf("SystemTestingItemTransition: Failed on test round 1 due to %v (stage 2)", err))
	}

	pterm.Success.Printfln("SystemTestingItemTransition: PASS (Time used = %v)", time.Since(tA))
}
