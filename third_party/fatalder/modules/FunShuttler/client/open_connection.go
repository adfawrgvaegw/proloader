package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/Happy2018new/nemc-tan-lobby-solver/protocol/service"
	bunkerauth "github.com/Yeah114/FunShuttler/core/bunker/auth"
	"github.com/Yeah114/FunShuttler/core/minecraft"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/core/py_rpc"
	cts "github.com/Yeah114/FunShuttler/core/py_rpc/mod_event/client_to_server"
	cts_mc "github.com/Yeah114/FunShuttler/core/py_rpc/mod_event/client_to_server/minecraft"
	cts_mc_p "github.com/Yeah114/FunShuttler/core/py_rpc/mod_event/client_to_server/minecraft/preset"
	cts_mc_v "github.com/Yeah114/FunShuttler/core/py_rpc/mod_event/client_to_server/minecraft/vip_event_system"
	mei "github.com/Yeah114/FunShuttler/core/py_rpc/mod_event/interface"
	"github.com/Yeah114/FunShuttler/i18n"
	"github.com/Yeah114/FunShuttler/utils"
)

// openConnection 通过 authenticator 连接到租赁服，
// 并初始化 Minecraft 连接
func openConnection(
	ctx context.Context,
	authenticator AccessWrapper,
	isTanLobby bool,
) (conn *minecraft.Conn, err error) {
	// prepare
	var dialer minecraft.Dialer
	var authResponse bunkerauth.AuthResponse

	// create connection
	dialer = minecraft.Dialer{
		Authenticator: authenticator,
	}
	if isTanLobby {
		roomCode := authenticator.RentalServerCode()
		roomID := strings.TrimPrefix(roomCode, "TanLobby:")
		netConn, tanLobbyLoginResp, err := service.Dial(
			roomID,
			authenticator.RentalServerPasscode(),
			authenticator.TanLobbyAuthenticator(),
		)
		if err != nil {
			return nil, err
		}
		conn, err = dialer.TanLobbyDialContext(ctx, netConn, tanLobbyLoginResp)
		if err != nil {
			return nil, err
		}
		authResponse = bunkerauth.AuthResponse{
			BotComponent: tanLobbyLoginResp.BotComponent,
		}
	} else {
		conn, authResponse, err = dialer.DialContext(ctx, "raknet")
	}
	if err != nil {
		return nil, err
	}

	// get constant and send pre-login packet
	runtimeid := fmt.Sprintf("%d", conn.GameData().EntityUniqueID)
	utils.Log.Info(i18n.T(i18n.Frame_LoginSequenceCompleted))
	conn.WritePacket(&packet.ClientCacheStatus{
		Enabled: false,
	})

	// send netease related packet
	utils.Log.Info(i18n.T(i18n.Frame_SendingAdditionalInfo))
	conn.WritePacket(&packet.NeteaseJson{
		Data: []byte(
			fmt.Sprintf(
				`{"eventName":"LOGIN_UID","resid":"","uid":"%d"}`,
				conn.IdentityData().Uid,
			),
		),
	})

	// DomainGame need
	conn.WritePacket(&packet.PyRpc{
		Value:         []any{
			"e",
			[]any{},
			nil,
		},
		OperationType: packet.PyRpcOperationTypeSend,
	})

	{
		modUUIDs := make([]any, 0)
		botComponent := make(map[string]int64, 0)
		for modUUID, outfitType := range authResponse.BotComponent {
			modUUIDs = append(modUUIDs, modUUID)
			if outfitType != nil {
				botComponent[modUUID] = int64(*outfitType)
			}
		}
		conn.WritePacket(&packet.PyRpc{
			Value: py_rpc.Marshal(&py_rpc.SyncUsingMod{
				modUUIDs,
				conn.ClientData().SkinID,
				"",
				true,
				botComponent,
			}),
			OperationType: packet.PyRpcOperationTypeSend,
		})
	}

	conn.WritePacket(&packet.PyRpc{
		Value:         py_rpc.Marshal(&py_rpc.SyncVipSkinUUID{nil}),
		OperationType: packet.PyRpcOperationTypeSend,
	})
	conn.WritePacket(&packet.PyRpc{
		Value:         py_rpc.Marshal(&py_rpc.ClientLoadAddonsFinishedFromGac{}),
		OperationType: packet.PyRpcOperationTypeSend,
	})

	{
		event := cts_mc_p.GetLoadedInstances{PlayerRuntimeID: runtimeid}
		module := cts_mc.Preset{Module: &mei.DefaultModule{Event: &event}}
		park := cts.Minecraft{Default: mei.Default{Module: &module}}
		conn.WritePacket(&packet.PyRpc{
			Value: py_rpc.Marshal(&py_rpc.ModEvent{
				Package: &park,
				Type:    py_rpc.ModEventClientToServer,
			}),
			OperationType: packet.PyRpcOperationTypeSend,
		})
	}

	conn.WritePacket(&packet.PyRpc{
		Value:         py_rpc.Marshal(&py_rpc.ArenaGamePlayerFinishLoad{}),
		OperationType: packet.PyRpcOperationTypeSend,
	})

	{
		event := cts_mc_v.PlayerUiInit{RuntimeID: runtimeid}
		module := cts_mc.VIPEventSystem{Module: &mei.DefaultModule{Event: &event}}
		park := cts.Minecraft{Default: mei.Default{Module: &module}}
		conn.WritePacket(&packet.PyRpc{
			Value: py_rpc.Marshal(&py_rpc.ModEvent{
				Package: &park,
				Type:    py_rpc.ModEventClientToServer,
			}),
			OperationType: packet.PyRpcOperationTypeSend,
		})
	}

	// return
	utils.Log.Info(i18n.T(i18n.Frame_PackingKeyData))
	return
}
