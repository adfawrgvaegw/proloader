package client

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/i18n"
	"github.com/Yeah114/FunShuttler/utils"
)

// LoginRentalServer ..
func LoginRentalServer(cfg Config) (client *Client, err error) {
	if cfg.AccessWrapper == nil {
		return nil, fmt.Errorf("LoginRentalServer: access wrapper is required")
	}

	client, err = CreateClient(cfg, cfg.AccessWrapper)
	if err != nil {
		return nil, fmt.Errorf("LoginRentalServer: %v", err)
	}

	return
}

// CreateClient
func CreateClient(cfg Config, authenticator AccessWrapper) (client *Client, err error) {
	isNetworkGame := strings.HasPrefix(authenticator.RentalServerCode(), "NetworkGame:")
	isTanLobby := strings.HasPrefix(authenticator.RentalServerCode(), "TanLobby:")

	utils.Log.Info(i18n.T(i18n.Frame_CreateNewAccessWrapper))
	authClient := authenticator.AuthClient()
	if authClient == nil {
		return nil, fmt.Errorf("CreateClient: nil auth client in access wrapper")
	}
	ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*120)
	defer cancelFunc()

	utils.Log.Info(i18n.T(i18n.Frame_InitializingConnection))
	conn, err := openConnection(ctx, authenticator, isTanLobby)
	if err != nil {
		utils.ShowError(i18n.T(i18n.Frame_Error_InitializingConnection), err.Error())
		return nil, fmt.Errorf("CreateClient: %v", err)
	}
	utils.Log.Info(i18n.T(i18n.Frame_LoginDone))

	client = &Client{connection: conn, authClient: authClient, authenticator: authenticator, CacheAllPacket: cfg.CacheAllPacket}
	utils.Log.Info(i18n.T(i18n.Frame_MCPCheckChallenge))
	challengeSolver := NewChallengeSolver(client)
	if !isTanLobby && !isNetworkGame {
		err = challengeSolver.CopeChallenge()
		if err != nil {
			return nil, fmt.Errorf("CreateClient: %v", err)
		}
	} else {
		cachePkt := make(chan packet.Packet, 32767)
		/*
			commandOutput := make(chan packet.CommandOutput, 1)
			waitError := make(chan error, 1)
			mcpError := make(chan error, 1)

			go func() {
				for {
					var pk packet.Packet
					pk, err = client.Conn().ReadPacket()
					if err != nil {
						waitError <- err
						close(waitError)
						fmt.Println("WaitError")
						return
					}
					cachePkt <- pk
					if pk.ID() == packet.IDCommandOutput {
						commandOutput <- *(pk.(*packet.CommandOutput))
						pkJson, _ := json.Marshal(pk)
						fmt.Println(string(pkJson))
						close(cachePkt)
						fmt.Println("CommandOutput")
						return
					}
				}
			}()

			// 并发执行waitMCPCheckChallengesDown
			go func() {
				err := challengeSolver.waitMCPCheckChallengesDown(commandOutput)
				if err != nil {
					mcpError <- err
					fmt.Println("MCPError")
				}
				fmt.Println("MCPError close")
				close(mcpError)
			}()

			// 同时等待MCP检查和goroutine错误
			select {
			case goroutineErr := <-waitError:
				if goroutineErr != nil {
					close(commandOutput)
					return nil, fmt.Errorf("CreateClient: goroutine error: %v", goroutineErr)
				}
			case mcpErr := <-mcpError:
				if mcpErr != nil {
					return nil, fmt.Errorf("CreateClient: MCP error: %v", mcpErr)
				}
			}
		*/

		close(cachePkt)
		client.cachedPacket = cachePkt
		//time.Sleep(time.Second * 5)
	}
	utils.Log.Info(i18n.T(i18n.Frame_MCPCheckChallengeDone))

	return
}
