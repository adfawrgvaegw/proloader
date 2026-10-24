package special_command

import (
	"encoding/json"
	"fmt"

	standard_minecraft "github.com/Yeah114/FunShuttler/core/standard_minecraft"
	standard_packet "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
)

func GetGetHoldNBTCommand() *game_interface.SpecialCommandDefinition {
	return &game_interface.SpecialCommandDefinition{
		Name:        "getholdnbt",
		Description: "获取手持物品的 NBT 数据",
		Handler: func(commandArgs []string, standardConn *standard_minecraft.Conn, gameInterface *game_interface.GameInterface) {
			resources := gameInterface.Resources()
			uqholder := resources.UQHolder().Micro()
			item := uqholder.GetExtendInfo().GetClientHoldingItem()
			if item.Stack.Count == 0 {
				standardConn.WritePacket(&standard_packet.ToastRequest{
					Title:   "§eFunShuttler",
					Message: "§c你似乎没有手持物品",
				})
			}
			nbtJsonBytes, _ := json.Marshal(item.Stack.NBTData)
			standardConn.WritePacket(&standard_packet.Text{
				TextType: standard_packet.TextTypeSystem,
				Message: fmt.Sprintf(
					"手持物品 NBT 数据为: %s",
					string(nbtJsonBytes),
				),
			})
		},
	}
}
