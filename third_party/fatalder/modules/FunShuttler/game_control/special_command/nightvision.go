package special_command

import (
	standard_minecraft "github.com/Yeah114/FunShuttler/core/standard_minecraft"
	standard_packet "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
)

func GetNightVisionCommand() *game_interface.SpecialCommandDefinition {
	return &game_interface.SpecialCommandDefinition{
		Name:        "nightvision",
		Description: "开启夜视",
		Handler: func(_ []string, standardConn *standard_minecraft.Conn, gameInterface *game_interface.GameInterface) {
			uqholder := gameInterface.Resources().UQHolder().Micro()
			standardConn.WritePacket(&standard_packet.ToastRequest{
				Title:   "§eFunShuttler",
				Message: "§b已开启夜视",
			})
			standardConn.WritePacket(&standard_packet.MobEffect{
				EntityRuntimeID: uqholder.GetBotBasicInfo().GetBotRuntimeID(),
				Operation:       standard_packet.MobEffectAdd,
				EffectType:      standard_packet.EffectNightVision,
				Amplifier:       2147483647,
				Particles:       false,
				Duration:        2147483647,
				Tick:            0,
			})
		},
	}
}
