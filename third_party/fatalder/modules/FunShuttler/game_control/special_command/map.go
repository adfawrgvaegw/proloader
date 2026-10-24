package special_command

import (
	"fmt"
	"github.com/disintegration/imaging"
	"image"
	"image/color"
	"os"
	"strconv"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	standard_minecraft "github.com/Yeah114/FunShuttler/core/standard_minecraft"
	standard_protocol "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol"
	standard_packet "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
)

func GetMapCommand() *game_interface.SpecialCommandDefinition {
	return &game_interface.SpecialCommandDefinition{
		Name:        "map",
		Description: "加载地图画于一个地图 UUID 上",
		Handler: func(commandArgs []string, standardConn *standard_minecraft.Conn, gameInterface *game_interface.GameInterface) {
			// 参数校验：至少需要2个参数（地图UUID和图片路径）
			if len(commandArgs) < 2 {
				standardConn.WritePacket(&standard_packet.ToastRequest{
					Title:   "§eFunShuttler",
					Message: "§c参数不足",
				})
				return
			}

			// 解析地图UUID
			mapUUID, err := strconv.ParseInt(commandArgs[0], 10, 64)
			if err != nil {
				standardConn.WritePacket(&standard_packet.ToastRequest{
					Title:   "§eFunShuttler",
					Message: "§c地图UUID必须是整数",
				})
				return
			}

			// 解析图片路径
			filePath := commandArgs[1]
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				standardConn.WritePacket(&standard_packet.ToastRequest{
					Title:   "§eFunShuttler",
					Message: "§c图片文件不存在",
				})
				return
			}

			// 解析是否锁定地图（第三个参数可选，默认true）
			lockMapFlag := true // 默认锁定
			if len(commandArgs) >= 3 {
				// 若提供了第三个参数，则解析
				lockMapFlag = ParseBool(commandArgs[2])
			}

			if mapUUID == 0 {
				holdingMapUUID := gameInterface.Resources().UQHolder().Micro().GetExtendInfo().GetClientHoldingItem().Stack.NBTData["map_uuid"]
				if holdingMapUUIDInt64, ok := holdingMapUUID.(int64); ok {
					mapUUID = holdingMapUUIDInt64
				} else {
					standardConn.WritePacket(&standard_packet.ToastRequest{
						Title:   "§eFunShuttler",
						Message: fmt.Sprintf("§c无法获取手持物品地图UUID"),
					})
					return
				}
			}

			// 处理图片（调整为128×128）
			img, err := loadAndResizeImage(filePath, 1, 1)
			if err != nil {
				standardConn.WritePacket(&standard_packet.ToastRequest{
					Title:   "§eFunShuttler",
					Message: fmt.Sprintf("§c图片处理错误: %v", err),
				})
				return
			}

			// 处理像素数据
			pixels, err := processImagePart(img, 0, 0)
			if err != nil {
				standardConn.WritePacket(&standard_packet.ToastRequest{
					Title:   "§eFunShuttler",
					Message: fmt.Sprintf("§c像素处理错误: %v", err),
				})
				return
			}

			// 根据参数决定是否锁定地图
			if lockMapFlag {
				if err := lockMap(gameInterface, mapUUID); err != nil {
					standardConn.WritePacket(&standard_packet.ToastRequest{
						Title:   "§eFunShuttler",
						Message: fmt.Sprintf("§c锁定地图失败: %v", err),
					})
					return
				}
			}

			// 发送像素数据到地图
			if err := sendMapPixels(gameInterface, mapUUID, pixels); err != nil {
				standardConn.WritePacket(&standard_packet.ToastRequest{
					Title:   "§eFunShuttler",
					Message: fmt.Sprintf("§c发送像素失败: %v", err),
				})
				return
			}

			// 处理完成提示（包含锁定状态）
			lockStatus := "已锁定"
			if !lockMapFlag {
				lockStatus = "未锁定"
			}
			standardConn.WritePacket(&standard_packet.ToastRequest{
				Title:   "§eFunShuttler",
				Message: fmt.Sprintf("§b图片已成功加载到地图（%s）", lockStatus),
			})
		},
		Overloads: []standard_protocol.CommandOverload{
			{
				Parameters: []standard_protocol.CommandParameter{
					{
						Name: "地图 UUID（为0自动获取手持）",
						Type: standard_protocol.CommandArgTypeInt | standard_protocol.CommandArgValid,
					},
					{
						Name: "图片路径",
						Type: standard_protocol.CommandArgTypeFilepath | standard_protocol.CommandArgValid,
					},
					{
						Name:     "是否锁定地图",
						Type:     standard_protocol.CommandArgTypeWildcardInt | standard_protocol.CommandArgValid | standard_protocol.CommandArgEnum,
						Optional: true, // 标记为可选参数
					},
				},
			},
		},
	}
}

// 加载并调整图片
func loadAndResizeImage(imagePath string, mapsPerRow, mapsPerCol int) (image.Image, error) {
	// 打开图片文件
	file, err := os.Open(imagePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// 解码图片
	img, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}

	// 计算目标尺寸
	targetWidth := mapsPerRow * 128
	targetHeight := mapsPerCol * 128

	// 调整图片尺寸
	img = resizeImage(img, targetWidth, targetHeight)
	return img, nil
}

// 调整图像大小为目标尺寸
func resizeImage(img image.Image, targetWidth, targetHeight int) image.Image {
	srcBounds := img.Bounds()
	srcWidth, srcHeight := srcBounds.Dx(), srcBounds.Dy()
	srcAspect := float64(srcWidth) / float64(srcHeight)
	targetAspect := float64(targetWidth) / float64(targetHeight)

	var scaledWidth, scaledHeight int
	var padLeft, padTop int

	if srcAspect > targetAspect {
		scaledWidth = targetWidth
		scaledHeight = int(float64(targetWidth) / srcAspect)
		padTop = (targetHeight - scaledHeight) / 2
	} else {
		scaledHeight = targetHeight
		scaledWidth = int(float64(targetHeight) * srcAspect)
		padLeft = (targetWidth - scaledWidth) / 2
	}

	scaledImg := imaging.Resize(img, scaledWidth, scaledHeight, imaging.Lanczos)
	background := imaging.New(targetWidth, targetHeight, color.RGBA{0, 0, 0, 0})
	return imaging.Paste(background, scaledImg, image.Pt(padLeft, padTop))
}

// 处理128×128图片为地图像素数据
func processImagePart(img image.Image, xOffset, yOffset int) ([]protocol.PixelRequest, error) {
	bounds := img.Bounds()
	clientPixels := make([]protocol.PixelRequest, 0, 128*128)

	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			actualX := (xOffset * 128) + x
			actualY := (yOffset * 128) + y

			if actualX >= bounds.Max.X || actualY >= bounds.Max.Y {
				clientPixels = append(clientPixels, protocol.PixelRequest{
					Colour: color.RGBA{0, 0, 0, 0},
					Index:  uint16(y*128 + x),
				})
				continue
			}

			rgba := color.RGBAModel.Convert(img.At(actualX, actualY)).(color.RGBA)
			clientPixels = append(clientPixels, protocol.PixelRequest{
				Colour: rgba,
				Index:  uint16(y*128 + x),
			})
		}
	}

	return clientPixels, nil
}

// 锁定地图
func lockMap(api *game_interface.GameInterface, mapID int64) error {
	lockPacket := &packet.MapCreateLockedCopy{
		OriginalMapID: mapID,
		NewMapID:      mapID,
	}

	if err := api.Resources().WritePacket(lockPacket); err != nil {
		return err
	}
	api.Commands().AwaitChangesGeneral()
	return nil
}

// 发送像素数据到地图
func sendMapPixels(api *game_interface.GameInterface, mapID int64, pixels []protocol.PixelRequest) error {
	pixelPacket := &packet.MapInfoRequest{
		MapID:        mapID,
		ClientPixels: pixels,
	}

	if err := api.Resources().WritePacket(pixelPacket); err != nil {
		return err
	}
	api.Commands().AwaitChangesGeneral()
	return nil
}
