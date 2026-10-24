package special_command

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"strings"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	standard_minecraft "github.com/Yeah114/FunShuttler/core/standard_minecraft"
	standard_protocol "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol"
	standard_packet "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
)

var ClientWebSocketCommandUUID, _ = uuid.Parse("96045347-a6a3-4114-94c0-1bc4cc561694")

func GetSendWebSocketCommand() *game_interface.SpecialCommandDefinition {
	return &game_interface.SpecialCommandDefinition{
		Name:        "!",
		Description: "发送WebSocket命令",
		Handler: func(commandArgs []string, _ *standard_minecraft.Conn, game_interface *game_interface.GameInterface) {
			commandLine := commandArgs[0]
			commandLine = strings.TrimPrefix(commandLine, "!")
			commandLine = strings.TrimLeft(commandLine, " ")
			game_interface.Resources().WritePacket(&packet.CommandRequest{
				CommandLine: commandLine,
				CommandOrigin: protocol.CommandOrigin{
					Origin:    protocol.CommandOriginAutomationPlayer,
					UUID:      ClientWebSocketCommandUUID,
					RequestID: "96045347-a6a3-4114-94c0-1bc4cc561694",
				},
				Internal:  false,
				UnLimited: false,
				Version:   39,
			})
		},
		Init: func(gameInterface *game_interface.GameInterface, standardConn *standard_minecraft.Conn) {
			gameInterface.Resources().PacketInterceptor().InterceptPacket(
				[]uint32{standard_packet.IDCommandOutput},
				func(pk *packet.Packet) bool {
					p, _ := (*pk).(*packet.CommandOutput)
					if p.CommandOrigin.Origin == standard_protocol.CommandOriginAutomationPlayer && p.CommandOrigin.UUID == ClientWebSocketCommandUUID {
						websocketCommandOutputJsonBytes, _ := json.Marshal(p)
						p.CommandOrigin.Origin = standard_protocol.CommandOriginPlayer
						standardConn.WritePacket(&standard_packet.Text{
							TextType: standard_packet.TextTypeSystem,
							Message:  fmt.Sprintf("WebSocket命令返回: %s", string(websocketCommandOutputJsonBytes)),
						})
					}
					return true
				},
			)
		},
		Overloads: []standard_protocol.CommandOverload{
			{
				Parameters: []standard_protocol.CommandParameter{
					{
						Name: "WebSocket命令",
						Type: standard_protocol.CommandArgTypeCommand | standard_protocol.CommandArgValid | standard_protocol.CommandArgEnum,
					},
				},
			},
		},
		NoParse: true,
	}
}
