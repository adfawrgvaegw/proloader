package special_command

import (
	"encoding/json"
	"fmt"
	"github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/go-gl/mathgl/mgl32"
	"math"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	standard_minecraft "github.com/Yeah114/FunShuttler/core/standard_minecraft"
	standard_protocol "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol"
	standard_packet "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
)

// GetGetNBTCommand 获取NBT数据命令定义
func GetGetNBTCommand() *game_interface.SpecialCommandDefinition {
	return &game_interface.SpecialCommandDefinition{
		Name:        "getnbt",
		Description: "获取某地的 NBT 数据",
		Handler: func(commandArgs []string, standardConn *standard_minecraft.Conn, gameInterface *game_interface.GameInterface) {
			uqholder := gameInterface.Resources().UQHolder().Micro()
			position, _ := uqholder.GetExtendInfo().GetBotPosition()
			dim, _ := uqholder.GetExtendInfo().GetBotDimension()
			dimension := define.Dimension(dim)
			position = mgl32.Vec3{
				float32(math.Floor(float64(position[0]))),
				float32(math.Floor(float64(position[1]))) - 1,
				float32(math.Floor(float64(position[2]))),
			}
			position, _ = ParseCoordinates(position, commandArgs)
			pos := protocol.BlockPos{
				int32(position[0]),
				int32(position[1]),
				int32(position[2]),
			}

			// 调用新函数获取NBT数据
			nbt, err := GetNBT(pos, dimension, gameInterface)
			if err != nil {
				standardConn.WritePacket(&standard_packet.ToastRequest{
					Title:   "发生错误",
					Message: fmt.Sprintf("§c%v", err),
				})
				return
			}

			if nbt != nil {
				nbtJsonBytes, _ := json.Marshal(nbt)
				standardConn.WritePacket(&standard_packet.Text{
					TextType: standard_packet.TextTypeSystem,
					Message: fmt.Sprintf(
						"坐标 %d, %d, %d 处的方块 NBT 数据为: %s",
						pos.X(),
						pos.Y(),
						pos.Z(),
						string(nbtJsonBytes),
					),
				})
			} else {
				standardConn.WritePacket(&standard_packet.Text{
					TextType: standard_packet.TextTypeSystem,
					Message:  fmt.Sprintf("坐标 %d, %d, %d 处的方块没有 NBT 数据", pos.X(), pos.Y(), pos.Z()),
				})
			}
		},
		Overloads: []standard_protocol.CommandOverload{
			{
				Parameters: []standard_protocol.CommandParameter{
					{
						Name: "坐标",
						Type: standard_protocol.CommandArgTypeBlockPosition | standard_protocol.CommandArgValid,
					},
				},
			},
		},
	}
}

// GetNBT 获取指定位置方块的NBT数据
// 参数：
//   - pos: 方块位置
//   - dimension: 维度信息
//   - gameInterface: 游戏接口实例
//
// 返回：
//   - 找到的NBT数据（nil表示未找到）
//   - 可能发生的错误
func GetNBT(pos protocol.BlockPos, dimension define.Dimension, gameInterface *game_interface.GameInterface) (map[string]interface{}, error) {
	// 发送子区块请求
	subChunk, err := gameInterface.SubChunkRequest().SendSubChunkRequestWithResp(
		gameInterface.SubChunkRequest().MakeSubChunkRequestByArea(
			dimension,
			pos,
			pos,
		),
	)
	if err != nil {
		return nil, err
	}

	// 解码子区块数据获取NBT列表
	_, _, nbts, err := NEMCSubChunkDecode(
		subChunk.SubChunkEntries[0].RawPayload,
		dimension.Range(),
	)
	if err != nil {
		return nil, err
	}

	// 查找目标位置的NBT
	for _, nbt := range nbts {
		if nbt["x"] == pos.X() && nbt["y"] == pos.Y() && nbt["z"] == pos.Z() {
			return nbt, nil
		}
	}

	// 未找到NBT数据
	return nil, nil
}
