package special_command

import (
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	standard_minecraft "github.com/Yeah114/FunShuttler/core/standard_minecraft"
	standard_protocol "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol"
	standard_packet "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
	"github.com/Yeah114/FunShuttler/translate"
)

// GetFlyCommand 获取飞行命令定义
func GetFlyCommand() *game_interface.SpecialCommandDefinition {
	return &game_interface.SpecialCommandDefinition{
		Name:        "fly",
		Description: "开启飞行",
		Handler: func(commandArgs []string, standardConn *standard_minecraft.Conn, gameInterface *game_interface.GameInterface) {
			uqholder := gameInterface.Resources().UQHolder().Micro()
			player, _ := uqholder.GetPlayersInfo().GetPlayerByName(uqholder.GetBotBasicInfo().GetBotName())
			playerPermissions, _ := player.GetPlayerPermissions()
			commandPermissions, _ := player.GetCommandPermissions()
			values, _ := player.GetValues()
			flySpeed, _ := player.GetFlySpeed()
			walkSpeed, _ := player.GetWalkSpeed()

			if len(commandArgs) == 1 && !ParseBool(commandArgs[0]) {
				values &^= standard_protocol.AbilityFlying | standard_protocol.AbilityMayFly
				standardConn.WritePacket(&standard_packet.ToastRequest{
					Title:   "§eFunShuttler",
					Message: "§b已关闭飞行",
				})
			} else {
				values = values | standard_protocol.AbilityFlying | standard_protocol.AbilityMayFly
				standardConn.WritePacket(&standard_packet.ToastRequest{
					Title:   "§eFunShuttler",
					Message: "§b已开启飞行",
				})
			}
			standardAbilityData := standard_protocol.AbilityData{
				EntityUniqueID:     uqholder.GetBotBasicInfo().GetBotUniqueID(),
				PlayerPermissions:  playerPermissions,
				CommandPermissions: commandPermissions,
				Layers: []standard_protocol.AbilityLayer{
					{
						Type:      standard_protocol.AbilityLayerTypeBase,
						Abilities: (1 << 19) - 1,
						Values:    values,
						FlySpeed:  flySpeed,
						WalkSpeed: walkSpeed,
					},
				},
			}
			abilityData := protocol.AbilityData{}
			translate.ConvertStruct(&standardAbilityData, &abilityData)
			player.UpdateAbility(abilityData)
			standardConn.WritePacket(&standard_packet.UpdateAbilities{
				AbilityData: standardAbilityData,
			})
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
