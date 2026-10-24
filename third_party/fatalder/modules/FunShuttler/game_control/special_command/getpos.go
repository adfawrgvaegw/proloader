package special_command

import (
	"fmt"
	"sync"

	"github.com/google/uuid"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	standard_minecraft "github.com/Yeah114/FunShuttler/core/standard_minecraft"
	standard_protocol "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol"
	standard_packet "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
)

func GetGetPosCommand() *game_interface.SpecialCommandDefinition {
	var (
		commandCallbacks = make(map[uuid.UUID]func(p *packet.CommandOutput))
		mutex            sync.Mutex
	)

	return &game_interface.SpecialCommandDefinition{
		Name:        "getpos",
		Description: "获取一个玩家的坐标",
		Handler: func(commandArgs []string, standardConn *standard_minecraft.Conn, gameInterface *game_interface.GameInterface) {
			if len(commandArgs) < 1 {
				standardConn.WritePacket(&standard_packet.ToastRequest{
					Title:   "§eFunShuttler",
					Message: "§c请输入目标玩家",
				})
				return
			}
			targetPlayerName := commandArgs[0]
			commandUUID := uuid.New()
			commandChan := make(chan *packet.CommandOutput, 1)

			// 注册回调，注意并发安全
			mutex.Lock()
			commandCallbacks[commandUUID] = func(p *packet.CommandOutput) {
				commandChan <- p
			}
			mutex.Unlock()

			defer func() {
				mutex.Lock()
				delete(commandCallbacks, commandUUID)
				mutex.Unlock()
			}()

			// 初始扩大范围查找玩家是否在范围内(XZ平面)
			r := 1
			for {
				commandLine := fmt.Sprintf(`/codebuilder_actorinfo inventory @a[name="%s",x=%d,z=%d,dx=%d,dz=%d]`, targetPlayerName, -r, -r, r*2, r*2)
				gameInterface.Resources().WritePacket(&packet.CommandRequest{
					CommandLine: commandLine,
					CommandOrigin: protocol.CommandOrigin{
						Origin:    protocol.CommandOriginAutomationPlayer,
						UUID:      commandUUID,
						RequestID: "96045347-a6a3-4114-94c0-1bc4cc561694",
					},
					Version: 39,
				})

				output := <-commandChan
				if output.SuccessCount == 1 {
					break
				}
				r *= 2
			}

			// 二分法缩小XZ范围，找到最小有效r
			low := r / 2
			high := r
			for low <= high {
				mid := (low + high) / 2
				commandLine := fmt.Sprintf(`/codebuilder_actorinfo inventory @a[name="%s",x=%d,z=%d,dx=%d,dz=%d]`, targetPlayerName, -mid, -mid, mid*2, mid*2)
				gameInterface.Resources().WritePacket(&packet.CommandRequest{
					CommandLine: commandLine,
					CommandOrigin: protocol.CommandOrigin{
						Origin:    protocol.CommandOriginAutomationPlayer,
						UUID:      commandUUID,
						RequestID: "96045347-a6a3-4114-94c0-1bc4cc561694",
					},
					Version: 39,
				})
				output := <-commandChan
				if output.SuccessCount == 1 {
					r = mid
					high = mid - 1
				} else {
					low = mid + 1
				}
			}

			// 判断X和Z轴坐标正负方向
			var xPositive, zPositive bool

			// 判断Z方向
			commandLine := fmt.Sprintf(`/codebuilder_actorinfo inventory @a[name="%s",x=%d,z=%d,dx=%d,dz=%d]`, targetPlayerName, -r, 0, r*2, r)
			gameInterface.Resources().WritePacket(&packet.CommandRequest{
				CommandLine: commandLine,
				CommandOrigin: protocol.CommandOrigin{
					Origin:    protocol.CommandOriginAutomationPlayer,
					UUID:      commandUUID,
					RequestID: "96045347-a6a3-4114-94c0-1bc4cc561694",
				},
				Version: 39,
			})
			output := <-commandChan
			if output.SuccessCount == 1 {
				zPositive = true

				commandLine = fmt.Sprintf(`/codebuilder_actorinfo inventory @a[name="%s",x=%d,z=%d,dx=%d,dz=%d]`, targetPlayerName, 0, 0, r, r)
				gameInterface.Resources().WritePacket(&packet.CommandRequest{
					CommandLine: commandLine,
					CommandOrigin: protocol.CommandOrigin{
						Origin:    protocol.CommandOriginAutomationPlayer,
						UUID:      commandUUID,
						RequestID: "96045347-a6a3-4114-94c0-1bc4cc561694",
					},
					Version: 39,
				})
				output2 := <-commandChan
				if output2.SuccessCount == 1 {
					xPositive = true
				}
			} else {
				commandLine = fmt.Sprintf(`/codebuilder_actorinfo inventory @a[name="%s",x=%d,z=%d,dx=%d,dz=%d]`, targetPlayerName, -r, -r, r, r)
				gameInterface.Resources().WritePacket(&packet.CommandRequest{
					CommandLine: commandLine,
					CommandOrigin: protocol.CommandOrigin{
						Origin:    protocol.CommandOriginAutomationPlayer,
						UUID:      commandUUID,
						RequestID: "96045347-a6a3-4114-94c0-1bc4cc561694",
					},
					Version: 39,
				})
				output3 := <-commandChan
				if output3.SuccessCount != 1 {
					xPositive = true
				}
			}

			var x, z int
			xS := r
			if !xPositive {
				xS = -xS
			}

			// 判断X或Z坐标具体值
			commandLine = fmt.Sprintf(`/codebuilder_actorinfo inventory @a[name="%s",x=%d,z=%d,dx=%d,dz=%d]`, targetPlayerName, xS, -r, 0, r*2)
			gameInterface.Resources().WritePacket(&packet.CommandRequest{
				CommandLine: commandLine,
				CommandOrigin: protocol.CommandOrigin{
					Origin:    protocol.CommandOriginAutomationPlayer,
					UUID:      commandUUID,
					RequestID: "96045347-a6a3-4114-94c0-1bc4cc561694",
				},
				Version: 39,
			})
			output = <-commandChan
			if output.SuccessCount == 1 {
				x = r
				if !xPositive {
					x = -x
				}
			} else {
				z = r
				if !zPositive {
					z = -z
				}
			}

			// 进一步精确X或Z坐标，使用二分法
			var sign string
			if z == 0 {
				sign = "z"
				low, high = 0, r
				if !zPositive {
					low, high = -r, 0
				}
			} else {
				sign = "x"
				low, high = 0, r
				if !xPositive {
					low, high = -r, 0
				}
			}

			p := 0
			for low <= high {
				mid := (low + high) / 2

				commandLine := fmt.Sprintf(
					`/codebuilder_actorinfo inventory @a[name="%s",%s=%d,d%s=%d]`,
					targetPlayerName,
					sign,
					low,
					sign,
					mid-low,
				)

				gameInterface.Resources().WritePacket(&packet.CommandRequest{
					CommandLine: commandLine,
					CommandOrigin: protocol.CommandOrigin{
						Origin:    protocol.CommandOriginAutomationPlayer,
						UUID:      commandUUID,
						RequestID: "96045347-a6a3-4114-94c0-1bc4cc561694",
					},
					Version: 39,
				})
				output := <-commandChan
				if output.SuccessCount == 1 {
					p = mid
					high = mid - 1
				} else {
					low = mid + 1
				}
			}

			if sign == "x" {
				x = p
			} else {
				z = p
			}

			// 新增Y轴探测逻辑
			var y int
			// 1. 初始扩大Y轴范围查找
			yR := 1
			for {
				commandLine := fmt.Sprintf(`/codebuilder_actorinfo inventory @a[name="%s",y=%d,dy=%d]`, targetPlayerName, -yR, yR*2)
				gameInterface.Resources().WritePacket(&packet.CommandRequest{
					CommandLine: commandLine,
					CommandOrigin: protocol.CommandOrigin{
						Origin:    protocol.CommandOriginAutomationPlayer,
						UUID:      commandUUID,
						RequestID: "96045347-a6a3-4114-94c0-1bc4cc561694",
					},
					Version: 39,
				})
				output := <-commandChan
				if output.SuccessCount == 1 {
					break
				}
				yR *= 2
			}

			// 2. 二分法缩小Y轴范围
			yLow, yHigh := -yR, yR
			for yLow <= yHigh {
				yMid := (yLow + yHigh) / 2
				commandLine := fmt.Sprintf(`/codebuilder_actorinfo inventory @a[name="%s",x=%d,z=%d,y=%d,dy=%d]`, targetPlayerName, x, z, yLow, yMid-yLow)
				gameInterface.Resources().WritePacket(&packet.CommandRequest{
					CommandLine: commandLine,
					CommandOrigin: protocol.CommandOrigin{
						Origin:    protocol.CommandOriginAutomationPlayer,
						UUID:      commandUUID,
						RequestID: "96045347-a6a3-4114-94c0-1bc4cc561694",
					},
					Version: 39,
				})
				output := <-commandChan
				if output.SuccessCount == 1 {
					y = yMid
					yHigh = yMid - 1
				} else {
					yLow = yMid + 1
				}
			}

			standardConn.WritePacket(&standard_packet.Text{
				TextType: standard_packet.TextTypeSystem,
				Message:  fmt.Sprintf("玩家 %s 的坐标约为：x=%d, y=%d, z=%d", targetPlayerName, x, y, z),
			})
		},
		Init: func(gameInterface *game_interface.GameInterface, standardConn *standard_minecraft.Conn) {
			gameInterface.Resources().PacketInterceptor().InterceptPacket(
				[]uint32{standard_packet.IDCommandOutput},
				func(pk *packet.Packet) (allow bool) {
					allow = true
					p, _ := (*pk).(*packet.CommandOutput)
					mutex.Lock()
					cb := commandCallbacks[p.CommandOrigin.UUID]
					mutex.Unlock()
					if cb != nil {
						go cb(p)
						return false
					}
					return
				},
			)
		},
		Overloads: []standard_protocol.CommandOverload{
			{
				Parameters: []standard_protocol.CommandParameter{
					{
						Name: "目标玩家",
						Type: standard_protocol.CommandArgTypeTarget | standard_protocol.CommandArgValid,
					},
				},
			},
		},
	}
}
