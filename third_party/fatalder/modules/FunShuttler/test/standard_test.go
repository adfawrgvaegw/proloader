package test_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Yeah114/FunShuttler/client"
	"github.com/Yeah114/FunShuttler/core/bunker/auth"
	"github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
)

func TestStandard(t *testing.T) {
	cfg := client.Config{
		//RentalServerCode: "48285363",
		//RentalServerCode: "4682410281554073467",
		//RentalServerCode: "TanLobby:242699",
		RentalServerCode:     "NetworkGame:4619956744566574402",
		RentalServerPasscode: "",
		AuthServerAddress:    "http://127.0.0.1:8080",
		AuthServerToken:      "",
	}

	serverCodeForWrapper := cfg.RentalServerCode
	if strings.HasPrefix(serverCodeForWrapper, "TanLobby:") {
		serverCodeForWrapper = strings.TrimPrefix(serverCodeForWrapper, "TanLobby:")
	}
	authClient, err := auth.CreateClient(&auth.ClientOptions{AuthServer: cfg.AuthServerAddress})
	if err != nil {
		t.Fatalf("create auth client: %v", err)
	}
	cfg.AccessWrapper = auth.NewAccessWrapper(
		authClient,
		serverCodeForWrapper,
		cfg.RentalServerPasscode,
		cfg.AuthServerToken,
		"", "",
	)

	c, err := client.LoginRentalServer(cfg)
	if err != nil {
		fmt.Println(err)
		return
	}
	resourcesControl := resources_control.NewResourcesControl(c)
	gameInterface := game_interface.NewGameInterface(resourcesControl)

	for {
		pk := <-c.CachedPacket()
		if pk == nil {
			break
		}
		_ = resourcesControl.HandlePacket(&pk)
	}

	go func() {
		for {
			pk, err := c.Conn().ReadPacket()
			if err != nil {
				panic(err)
			}
			if pk.ID() == packet.IDCommandOutput {
				pkJson, _ := json.Marshal(pk)
				fmt.Println(string(pkJson))
			}
			_ = resourcesControl.HandlePacket(&pk)
		}
	}()

	resp, err := gameInterface.Commands().SendWSCommandWithResp("list")
	if err != nil {
		panic(err)
	}
	fmt.Println(resp)
}
