package special_command

import (
	standard_minecraft "github.com/Yeah114/FunShuttler/core/standard_minecraft"
	standard_packet "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
)

func GetKashShinfuCommand() *game_interface.SpecialCommandDefinition {
	return &game_interface.SpecialCommandDefinition{
		Name:        "kashshinfu",
		Description: "KashShinfu 我爱你。",
		Handler: func(_ []string, standardConn *standard_minecraft.Conn, _ *game_interface.GameInterface) {
			standardConn.WritePacket(&standard_packet.ToastRequest{
				Title:   "§eKashShinfu",
				Message: "§b我爱你。",
			})
		},
	}
}
