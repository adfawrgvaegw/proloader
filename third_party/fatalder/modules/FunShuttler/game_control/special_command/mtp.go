package special_command

import (
	"fmt"

	standard_minecraft "github.com/Yeah114/FunShuttler/core/standard_minecraft"
	standard_protocol "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol"
	standard_packet "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
)

func GetMTPCommand() *game_interface.SpecialCommandDefinition {
	return &game_interface.SpecialCommandDefinition{
		Name:        "mtp",
		Description: "尝试传送到某地",
		Handler: func(commandArgs []string, standardConn *standard_minecraft.Conn, gameInterface *game_interface.GameInterface) {
			uqholder := gameInterface.Resources().UQHolder().Micro()
			position, _ := uqholder.GetExtendInfo().GetBotPosition()
			position, _ = ParseCoordinates(position, commandArgs)
			standardConn.WritePacket(&standard_packet.ToastRequest{
				Title:   "§eFunShuttler",
				Message: fmt.Sprintf("§b已尝试传送到 %v", position),
			})
			// 发送完 MovePlayer 之后无需同步给 UQHolder，因为后续客户端会发送 PlayerAuthInput
			standardConn.WritePacket(&standard_packet.MovePlayer{
				EntityRuntimeID: uqholder.GetBotBasicInfo().GetBotRuntimeID(),
				Position:        position,
			})
		},
		Overloads: []standard_protocol.CommandOverload{
			{
				Parameters: []standard_protocol.CommandParameter{
					{
						Name: "坐标",
						Type: standard_protocol.CommandArgTypeBlockPosition | standard_protocol.CommandArgValid,
					},
				},
			},
		},
	}
}
