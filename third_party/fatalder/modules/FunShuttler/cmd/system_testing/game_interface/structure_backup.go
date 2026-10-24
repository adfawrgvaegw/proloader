package main

import (
	"time"

	"github.com/pterm/pterm"
)

func SystemTestingStructrueBackup() {
	tA := time.Now()

	api.Commands().SendSettingsCommand("tp 64 89 64", true)
	api.Commands().AwaitChangesGeneral()

	uniqueID, err := api.StructureBackup().BackupStructure([3]int32{64, 89, 64})
	if err != nil {
		panic("SystemTestingQuerytarget: Failed on stage 1")
	}

	err = api.StructureBackup().RevertStructure(uniqueID, [3]int32{64, 89, 64})
	if err != nil {
		panic("SystemTestingQuerytarget: Failed on stage 2")
	}

	err = api.StructureBackup().DeleteStructure(uniqueID)
	if err != nil {
		panic("SystemTestingQuerytarget: Failed on stage 3")
	}

	api.Commands().AwaitChangesGeneral()

	err = api.StructureBackup().RevertStructure(uniqueID, [3]int32{64, 89, 64})
	if err != nil {
		panic("SystemTestingQuerytarget: Failed on stage 4")
	}

	err = api.StructureBackup().RevertStructure(uniqueID, [3]int32{64, 88, 64})
	if err == nil {
		panic("SystemTestingQuerytarget: Failed on stage 5")
	}

	pterm.Success.Printfln("SystemTestingStructrueBackup: PASS (Time used = %v)", time.Since(tA))
}
