package main

import (
	"time"

	"github.com/pterm/pterm"
)

func SystemTestingSetblock() {
	tA := time.Now()

	// SetBlock & Test round 1
	{
		api.Commands().SendSettingsCommand("tp 64 89 64", true)
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand("setblock 64 89 64 air", true)
		api.Commands().AwaitChangesGeneral()

		api.SetBlock().SetBlock([3]int32{64, 89, 64}, "diamond_ore", "[]")
		resp, err := api.Commands().SendWSCommandWithResp(
			`execute as @s at @s positioned 64 ~ ~ positioned ~ 89 ~ positioned ~ ~ 64 run testforblock ~ ~ ~ diamond_ore`,
		)
		if err != nil || resp.SuccessCount == 0 {
			panic("SystemTestingSetblock: Test round 1 failed")
		}
	}

	// SetBlock & Test round 2
	{
		api.Commands().SendSettingsCommand("tp 64 88 64", true)
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand("setblock 64 88 64 air", true)
		api.Commands().AwaitChangesGeneral()

		api.SetBlock().SetBlock([3]int32{64, 88, 64}, "diamond_ore", "[]")
		resp, err := api.Commands().SendWSCommandWithResp(
			`testforblock 64 88 64 diamond_ore`,
		)
		if err != nil || resp.SuccessCount == 0 {
			panic("SystemTestingSetblock: Test round 2 failed")
		}
	}

	// SetBlockAsync (Test round 3)
	{
		api.Commands().SendSettingsCommand("tp 64 89 64", true)
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand("setblock 64 89 64 air", true)
		api.Commands().AwaitChangesGeneral()

		api.SetBlock().SetBlockAsync([3]int32{64, 89, 64}, "diamond_ore", "[]")
		api.Commands().AwaitChangesGeneral()
		resp, err := api.Commands().SendWSCommandWithResp(
			`execute as @s at @s positioned 64 ~ ~ positioned ~ 89 ~ positioned ~ ~ 64 run testforblock ~ ~ ~ diamond_ore`,
		)
		if err != nil || resp.SuccessCount == 0 {
			panic("SystemTestingSetblock: Test round 3 failed")
		}
	}

	pterm.Success.Printfln("SystemTestingSetblock: PASS (Time used = %v)", time.Since(tA))
}
