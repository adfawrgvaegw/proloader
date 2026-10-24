package main

import (
	"fmt"
	"time"

	"github.com/Yeah114/FunShuttler/game_control/game_interface"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/pterm/pterm"
)

func SystemTestingContainer() {
	tA := time.Now()

	// OpenInventory
	{
		success, err := api.ContainerOpenAndClose().OpenInventory()
		if err != nil {
			panic(fmt.Sprintf("SystemTestingContainer: `OpenInventory` failed due to %v (stage 1)", err))
		}
		if !success {
			panic("SystemTestingContainer: `OpenInventory` failed on test round 1")
		}

		channel := make(chan struct{})
		timer := time.NewTimer(time.Second)
		defer timer.Stop()

		go func() {
			api.ContainerOpenAndClose().OpenInventory()
			close(channel)
		}()

		select {
		case <-timer.C:
			err = api.ContainerOpenAndClose().CloseContainer()
			if err != nil {
				panic(fmt.Sprintf("SystemTestingContainer: `OpenInventory` failed due to %v (stage 2)", err))
			}

			newTimer := time.NewTimer(time.Second)
			defer newTimer.Stop()

			select {
			case <-newTimer.C:
				panic("SystemTestingContainer: `OpenInventory` failed on test round 3")
			case <-channel:
			}

			err = api.ContainerOpenAndClose().CloseContainer()
			if err != nil {
				panic(fmt.Sprintf("SystemTestingContainer: `OpenInventory` failed due to %v (stage 3)", err))
			}
		case <-channel:
			panic("SystemTestingContainer: `OpenInventory` failed on test round 2")
		}
	}

	// OpenContainer
	{
		api.BotClick().ChangeSelectedHotbarSlot(5)
		api.Commands().SendSettingsCommand("tp 0 0 0", true)
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand(`setblock 0 0 0 air`, true)
		api.Commands().SendSettingsCommand(`setblock 0 1 0 air`, true)
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand(`setblock 0 0 0 chest ["minecraft:cardinal_direction"="east"]`, true)
		api.Commands().AwaitChangesGeneral()

		success, err := api.ContainerOpenAndClose().OpenContainer(
			game_interface.UseItemOnBlocks{
				HotbarSlotID: 8,
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
			panic(fmt.Sprintf("SystemTestingContainer: `OpenContainer` failed due to %v", err))
		}
		if !success {
			panic("SystemTestingContainer: `OpenContainer` failed on test round 1")
		}
	}

	pterm.Success.Printfln("SystemTestingContainer: PASS (Time used = %v)", time.Since(tA))
}
