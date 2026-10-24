package special_command

import (
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	standard_minecraft "github.com/Yeah114/FunShuttler/core/standard_minecraft"
	standard_protocol "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol"
	standard_packet "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
)

var pixelRequests [][]protocol.PixelRequest
var nowPixelRequestIndex int
var lockMapFlag bool
var isProcessing bool // 新增：标记是否正在处理地图画

func GetBigMapCommand() *game_interface.SpecialCommandDefinition {
	return &game_interface.SpecialCommandDefinition{
		Name:        "bigmap",
		Description: "生成大型地图画",
		Handler: func(commandArgs []string, standardConn *standard_minecraft.Conn, gameInterface *game_interface.GameInterface) {
			if isProcessing {
				standardConn.WritePacket(&standard_packet.ToastRequest{
					Title:   "§eFunShuttler",
					Message: "§c已经有正在进行的制作",
				})
				return
			}
			// 参数校验：至少需要1个参数（图片路径）
			if len(commandArgs) < 1 {
				standardConn.WritePacket(&standard_packet.ToastRequest{
					Title:   "§eFunShuttler",
					Message: "§c参数不足",
				})
				return
			}

			// 解析图片路径
			filePath := commandArgs[0]
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				standardConn.WritePacket(&standard_packet.ToastRequest{
					Title:   "§eFunShuttler",
					Message: "§c图片文件不存在",
				})
				return
			}

			// 解析行列数量（默认1x1）
			mapsPerRow := 1
			mapsPerCol := 1
			if len(commandArgs) >= 2 {
				if row, err := strconv.Atoi(commandArgs[1]); err == nil && row > 0 {
					mapsPerRow = row
				} else {
					standardConn.WritePacket(&standard_packet.ToastRequest{
						Title:   "§eFunShuttler",
						Message: "§c每行数量必须为正整数",
					})
					return
				}
			}
			if len(commandArgs) >= 3 {
				if col, err := strconv.Atoi(commandArgs[2]); err == nil && col > 0 {
					mapsPerCol = col
				} else {
					standardConn.WritePacket(&standard_packet.ToastRequest{
						Title:   "§eFunShuttler",
						Message: "§c每列数量必须为正整数",
					})
					return
				}
			}

			// 解析是否锁定地图（默认true）
			lockMapFlag = true
			if len(commandArgs) >= 4 {
				lockMapFlag = ParseBool(commandArgs[3])
			}

			// 处理图片
			img, err := loadAndResizeImage(filePath, mapsPerRow, mapsPerCol)
			if err != nil {
				standardConn.WritePacket(&standard_packet.ToastRequest{
					Title:   "§eFunShuttler",
					Message: fmt.Sprintf("§c图片处理错误: %v", err),
				})
				return
			}

			mapTotal := mapsPerRow * mapsPerCol
			pixelRequests = make([][]protocol.PixelRequest, 0, mapTotal) // 预分配容量
			isProcessing = true                                          // 标记开始处理
			for mapIndex := 0; mapIndex < mapTotal; mapIndex++ {
				row := mapIndex / mapsPerCol
				col := mapIndex % mapsPerCol

				// 处理像素数据
				pixels, err := processImagePart(img, row, col)
				if err != nil {
					standardConn.WritePacket(&standard_packet.ToastRequest{
						Title:   "§eFunShuttler",
						Message: fmt.Sprintf("§c像素处理错误: %v", err),
					})
					cleanupProcessing() // 清理状态
					return
				}
				pixelRequests = append(pixelRequests, pixels)
			}
			standardConn.WritePacket(&standard_packet.ToastRequest{
				Title:   "§eFunShuttler",
				Message: "§b图片处理完成",
			})
		},
		Init: func(gameInterface *game_interface.GameInterface, standardConn *standard_minecraft.Conn) {
			cleanupProcessing()
			doneMaps := make(map[int64]bool)
			var mu sync.Mutex

			gameInterface.Resources().PacketInterceptor().InterceptPacket(
				[]uint32{standard_packet.IDMobEquipment},
				func(pk *packet.Packet) (allow bool) {
					allow = true
					p, _ := (*pk).(*packet.MobEquipment)

					mu.Lock()
					// 检查是否还有未处理的像素请求
					if !isProcessing || len(pixelRequests) <= nowPixelRequestIndex {
						mu.Unlock()
						return
					}

					// 解析地图UUID
					var mapUUID int64
					mapUUIDVal, ok := p.NewItem.Stack.NBTData["map_uuid"]
					if !ok {
						mu.Unlock()
						return
					}
					mapUUID, ok = mapUUIDVal.(int64)
					if !ok {
						mu.Unlock()
						return
					}

					// 检查地图是否已处理过
					if doneMaps[mapUUID] {
						mu.Unlock()
						return
					}

					// 记录当前索引快照
					currentIndex := nowPixelRequestIndex
					mu.Unlock()

					// 异步处理地图像素
					go func(index int, uuid int64) {
						mu.Lock()
						// 二次检查：确保处理状态有效
						if !isProcessing || index >= len(pixelRequests) || doneMaps[uuid] {
							mu.Unlock()
							return
						}
						// 安全获取像素数据（加锁状态下）
						pixels := pixelRequests[index]
						mu.Unlock()

						// 锁定地图（如果需要）
						if lockMapFlag {
							if err := lockMap(gameInterface, uuid); err != nil {
								standardConn.WritePacket(&standard_packet.ToastRequest{
									Title:   "§eFunShuttler",
									Message: fmt.Sprintf("§c锁定地图失败: %v", err),
								})
								return
							}
						}

						// 发送像素数据
						if err := sendMapPixels(gameInterface, uuid, pixels); err != nil {
							standardConn.WritePacket(&standard_packet.ToastRequest{
								Title:   "§eFunShuttler",
								Message: fmt.Sprintf("§c发送像素失败: %v", err),
							})
							return
						}

						// 更新状态（加锁保护）
						mu.Lock()
						doneMaps[uuid] = true
						nowPixelRequestIndex = index + 1

						// 检查是否全部完成
						if nowPixelRequestIndex >= len(pixelRequests) {
							cleanupProcessing()
							standardConn.WritePacket(&standard_packet.ToastRequest{
								Title:   "§eFunShuttler",
								Message: "§b地图已制作完成",
							})
						}
						mu.Unlock()

						// 发送成功消息
						standardConn.WritePacket(&standard_packet.Text{
							TextType: standard_packet.TextTypeSystem,
							Message:  fmt.Sprintf("成功发送像素到地图 %d", uuid),
						})
					}(currentIndex, mapUUID)

					return
				},
			)
		},
		Overloads: []standard_protocol.CommandOverload{
			{
				Parameters: []standard_protocol.CommandParameter{
					{
						Name: "图片路径",
						Type: standard_protocol.CommandArgTypeFilepath | standard_protocol.CommandArgValid,
					},
					{
						Name:     "每行地图数",
						Type:     standard_protocol.CommandArgTypeInt | standard_protocol.CommandArgValid,
						Optional: true,
					},
					{
						Name:     "每列地图数",
						Type:     standard_protocol.CommandArgTypeInt | standard_protocol.CommandArgValid,
						Optional: true,
					},
					{
						Name:     "是否锁定",
						Type:     standard_protocol.CommandArgTypeWildcardInt | standard_protocol.CommandArgValid | standard_protocol.CommandArgEnum,
						Optional: true,
					},
				},
			},
		},
	}
}

// 新增：清理处理状态的辅助函数
func cleanupProcessing() {
	pixelRequests = make([][]protocol.PixelRequest, 0)
	nowPixelRequestIndex = 0
	lockMapFlag = false
	isProcessing = false
}
