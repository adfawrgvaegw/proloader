package game_interface

import (
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/go-gl/mathgl/mgl32"
)

type BotAction struct {
	api *GameInterface
}

// NewBotAction 基于 api 创建并返回一个新的 BotAction
func NewBotAction(api *GameInterface) *BotAction {
	return &BotAction{api: api}
}

func (b *BotAction) MoveToPosition(position mgl32.Vec3, facing mgl32.Vec3) (err error) {
	for j := 0; j < 10; j++ {
		err = b.api.Resources().WritePacket(&packet.PlayerAuthInput{
			Pitch:            facing[0],
			Yaw:              facing[1],
			HeadYaw:          facing[2],
			InputData:        packet.InputFlagPersistSneak | packet.InputFlagPaddlingRight,
			InputMode:        packet.InputModeTouch,
			PlayMode:         packet.PlayModeScreen,
			InteractionModel: packet.InteractionModelCrosshair,
			Position:         position,
		})
		if err != nil {
			return
		}
	}
	return
}
