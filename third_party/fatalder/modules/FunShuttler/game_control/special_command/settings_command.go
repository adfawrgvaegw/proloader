package special_command

import (
	"strings"

	standard_minecraft "github.com/Yeah114/FunShuttler/core/standard_minecraft"
	standard_protocol "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
)

func GetSendSettingsCommand() *game_interface.SpecialCommandDefinition {
	return &game_interface.SpecialCommandDefinition{
		Name:        "*",
		Description: "发送Settings命令",
		Handler: func(commandArgs []string, _ *standard_minecraft.Conn, game_interface *game_interface.GameInterface) {
			commandLine := commandArgs[0]
			commandLine = strings.TrimPrefix(commandLine, "*")
			commandLine = strings.TrimLeft(commandLine, " ")
			game_interface.Commands().SendSettingsCommand(commandLine, false)
		},
		Overloads: []standard_protocol.CommandOverload{
			{
				Parameters: []standard_protocol.CommandParameter{
					{
						Name: "Settings命令",
						Type: standard_protocol.CommandArgTypeCommand | standard_protocol.CommandArgValid | standard_protocol.CommandArgEnum,
					},
				},
			},
		},
		NoParse: true,
	}
}
