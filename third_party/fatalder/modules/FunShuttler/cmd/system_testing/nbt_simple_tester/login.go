package main

import (
	"fmt"
	"time"

	"github.com/Yeah114/FunShuttler/client"
	"github.com/Yeah114/FunShuttler/core/bunker/auth"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_console"

	"github.com/pterm/pterm"
)

func SystemTestingLogin() {
	var err error
	tA := time.Now()

	cfg := client.Config{
		AuthServerAddress:    "http://127.0.0.1:8080",
		AuthServerToken:      "",
		RentalServerCode:     "48285363",
		RentalServerPasscode: "",
	}

	authClient, err := auth.CreateClient(&auth.ClientOptions{AuthServer: cfg.AuthServerAddress})
	if err != nil {
		panic(err)
	}
	cfg.AccessWrapper = auth.NewAccessWrapper(
		authClient,
		cfg.RentalServerCode,
		cfg.RentalServerPasscode,
		cfg.AuthServerToken,
		"", "",
	)

	c, err = client.LoginRentalServer(cfg)
	if err != nil {
		panic(err)
	}

	resources = resources_control.NewResourcesControl(c)
	api = game_interface.NewGameInterface(resources)

	console, err = nbt_console.NewConsole(api, 0, [3]int32{23, 12, -21})
	if err != nil {
		panic(fmt.Sprintf("SystemTestingSetblock: Failed on init new console, and the err is %v", err))
	}

	pterm.Success.Printfln("SystemTestingLogin: PASS (Time used = %v)", time.Since(tA))
}
