package special_command

import (
	"strconv"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	standard_minecraft "github.com/Yeah114/FunShuttler/core/standard_minecraft"
	standard_protocol "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol"
	standard_packet "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
	"github.com/Yeah114/FunShuttler/translate"
)

// GetFlySpeedCommand 获取设置飞行速度命令定义
func GetFlySpeedCommand() *game_interface.SpecialCommandDefinition {
	return &game_interface.SpecialCommandDefinition{
		Name:        "flyspeed",
		Description: "设置飞行速度",
		Handler: func(commandArgs []string, standardConn *standard_minecraft.Conn, gameInterface *game_interface.GameInterface) {
			uqholder := gameInterface.Resources().UQHolder().Micro()
			player, _ := uqholder.GetPlayersInfo().GetPlayerByName(uqholder.GetBotBasicInfo().GetBotName())
			playerPermissions, _ := player.GetPlayerPermissions()
			commandPermissions, _ := player.GetCommandPermissions()
			values, _ := player.GetValues()
			flySpeed, _ := player.GetFlySpeed()
			walkSpeed, _ := player.GetWalkSpeed()

			// 处理命令参数，设置飞行速度
			if len(commandArgs) == 1 {
				speed, err := strconv.ParseFloat(commandArgs[0], 64)
				if err == nil {
					flySpeed = float32(speed)
					standardConn.WritePacket(&standard_packet.ToastRequest{
						Title:   "§eFunShuttler",
						Message: "§b已设置飞行速度为: " + commandArgs[0],
					})
				} else {
					standardConn.WritePacket(&standard_packet.ToastRequest{
						Title:   "§eFunShuttler",
						Message: "§c参数错误，请输入数字",
					})
					return
				}
			} else {
				standardConn.WritePacket(&standard_packet.ToastRequest{
					Title:   "§eFunShuttler",
					Message: "§c请输入速度值",
				})
				return
			}

			// 构建能力数据并发送更新
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
						Name: "速度值",
						Type: standard_protocol.CommandArgTypeFloat | standard_protocol.CommandArgValid,
					},
				},
			},
		},
	}
}
