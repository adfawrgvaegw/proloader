package special_command

import (
	"github.com/go-gl/mathgl/mgl32"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	standard_minecraft "github.com/Yeah114/FunShuttler/core/standard_minecraft"
	standard_protocol "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol"
	standard_packet "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
)

func GetSouloutCommand() *game_interface.SpecialCommandDefinition {
	var souloutEnable bool
	var lastPosition mgl32.Vec3
	return &game_interface.SpecialCommandDefinition{
		Name:        "soulout",
		Description: "开启灵魂出窍",
		Handler: func(commandArgs []string, standardConn *standard_minecraft.Conn, gameInterface *game_interface.GameInterface) {
			uqholder := gameInterface.Resources().UQHolder().Micro()
			if len(commandArgs) == 1 && !ParseBool(commandArgs[0]) {
				souloutEnable = false
				standardConn.WritePacket(&standard_packet.ToastRequest{
					Title:   "§eFunShuttler",
					Message: "§b已关闭灵魂出窍",
				})
			} else {
				if !souloutEnable {
					souloutEnable = true
					lastPosition, _ = uqholder.GetExtendInfo().GetBotPosition()
				}
				standardConn.WritePacket(&standard_packet.ToastRequest{
					Title:   "§eFunShuttler",
					Message: "§b已开启灵魂出窍",
				})
			}
			if souloutEnable {
				standardConn.WritePacket(&standard_packet.SetPlayerGameType{
					GameType: standard_packet.GameTypeSpectator,
				})
			} else {
				gameType, _ := uqholder.GetExtendInfo().GetGameMode()
				standardConn.WritePacket(&standard_packet.MovePlayer{
					EntityRuntimeID: uqholder.GetBotBasicInfo().GetBotRuntimeID(),
					Position:        lastPosition,
				})
				standardConn.WritePacket(&standard_packet.SetPlayerGameType{
					GameType: gameType,
				})
			}
		},
		Init: func(gameInterface *game_interface.GameInterface, standardConn *standard_minecraft.Conn) {
			gameInterface.Resources().PacketInterceptor().InterceptPacket(
				[]uint32{standard_packet.IDPlayerAuthInput, standard_packet.IDSetPlayerGameType},
				func(pk *packet.Packet) (allow bool) {
					allow = true
					if souloutEnable {
						return !allow
					}
					return
				},
			)
		},
		Overloads: []standard_protocol.CommandOverload{
			{
				Parameters: []standard_protocol.CommandParameter{
					{
						Name: "是否开启",
						Type: standard_protocol.CommandArgTypeWildcardInt | standard_protocol.CommandArgValid | standard_protocol.CommandArgEnum,
					},
				},
			},
		},
	}
}
