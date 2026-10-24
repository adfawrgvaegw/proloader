package special_command

import (
	"github.com/go-gl/mathgl/mgl32"
	"math"
	"strconv"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	standard_minecraft "github.com/Yeah114/FunShuttler/core/standard_minecraft"
	standard_protocol "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol"
	standard_packet "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
	"github.com/Yeah114/FunShuttler/uqholder"
	"github.com/Yeah114/FunShuttler/uqholder/defines"
)

var killAuraRange = int64(0)

func GetKillAuraCommand() *game_interface.SpecialCommandDefinition {
	return &game_interface.SpecialCommandDefinition{
		Name:        "killaura",
		Description: "自动攻击范围内最近的玩家",
		Handler: func(commandArgs []string, standardConn *standard_minecraft.Conn, gameInterface *game_interface.GameInterface) {
			if len(commandArgs) == 1 {
				Range, err := strconv.ParseInt(commandArgs[0], 10, 64)
				if err == nil {
					killAuraRange = Range
					standardConn.WritePacket(&standard_packet.ToastRequest{
						Title:   "§eFunShuttler",
						Message: "§b已设置攻击范围为: " + commandArgs[0],
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
					Message: "§c请输入范围",
				})
				return
			}
		},
		Init: func(gameInterface *game_interface.GameInterface, standardConn *standard_minecraft.Conn) {
			killAuraRange = 0
			uqholder := gameInterface.Resources().UQHolder().Micro()
			gameInterface.Resources().PacketInterceptor().InterceptPacket(
				[]uint32{standard_packet.IDPlayerAction, standard_packet.IDInventoryTransaction},
				func(pk *packet.Packet) (allow bool) {
					allow = true
					// 当范围为0时不执行攻击
					if killAuraRange <= 0 {
						return
					}
					targetPlayer, found := FindNearestPlayerInRange(uqholder, killAuraRange)
					if !found {
						return
					}
					hotBarSlot := uqholder.GetExtendInfo().GetClientHotBarSlot()
					heldItem := uqholder.GetExtendInfo().GetClientHoldingItem()
					targetEntityRuntimeID, _ := targetPlayer.GetEntityRuntimeID()
					position, _ := targetPlayer.GetPosition()
					position[1] += 1
					selfPosition, _ := uqholder.GetExtendInfo().GetBotPosition()

					gameInterface.BotAction().MoveToPosition(position, mgl32.Vec3{})
					gameInterface.Resources().WritePacket(&packet.InventoryTransaction{
						TransactionData: &protocol.UseItemOnEntityTransactionData{
							TargetEntityRuntimeID: targetEntityRuntimeID,
							ActionType:            protocol.UseItemOnEntityActionAttack,
							HotBarSlot:            int32(hotBarSlot),
							HeldItem:              heldItem,
							Position:              position,
						},
					})
					gameInterface.BotAction().MoveToPosition(selfPosition, mgl32.Vec3{})
					return
				},
			)
		},
		Overloads: []standard_protocol.CommandOverload{
			{
				Parameters: []standard_protocol.CommandParameter{
					{
						Name: "范围",
						Type: standard_protocol.CommandArgTypeInt | standard_protocol.CommandArgValid,
					},
				},
			},
		},
	}
}

// 寻找范围内最近的玩家（排除自己）
func FindNearestPlayerInRange(uqholder *uqholder.MicroUQHolder, r int64) (nearest defines.PlayerUQReader, found bool) {
	selfName := uqholder.GetBotBasicInfo().GetBotName()
	selfPos, _ := uqholder.GetExtendInfo().GetBotPosition()
	allPlayers := uqholder.GetPlayersInfo().GetAllOnlinePlayers()
	var minDistance float64 = -1

	for _, player := range allPlayers {
		// 排除自身
		playerName, _ := player.GetUsername()
		if playerName == selfName {
			continue
		}

		// 获取玩家位置
		playerPos, hasPos := player.GetPosition()
		if !hasPos {
			continue
		}

		// 计算三维欧氏距离
		dx := playerPos[0] - selfPos[0]
		dy := playerPos[1] - selfPos[1]
		dz := playerPos[2] - selfPos[2]
		distance := math.Sqrt(float64(dx*dx + dy*dy + dz*dz))

		// 检查是否在设置的范围内
		if float64(r) > 0 && distance > float64(r) {
			continue // 超出范围则跳过
		}

		// 更新最近玩家（首次找到或距离更近）
		if !found || distance < minDistance {
			minDistance = distance
			nearest = player
			found = true
		}
	}

	return nearest, found
}
