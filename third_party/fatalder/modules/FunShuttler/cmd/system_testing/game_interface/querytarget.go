package main

import (
	"time"

	"github.com/pterm/pterm"
)

func SystemTestingQuerytarget() {
	tA := time.Now()

	_, err := api.Querytarget().DoQuerytarget("@a")
	if err != nil {
		panic("SystemTestingQuerytarget: Failed")
	}

	pterm.Success.Printfln("SystemTestingQuerytarget: PASS (Time used = %v)", time.Since(tA))
}
