package nbt_console

import (
	"fmt"
	"math"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
)

// CanReachOrMove 判断机器人是否可以操作 targetPos 处的方块。
// 如果不能，则将机器人传送到 targetPos 所示的位置。
// 出于对机器人判定箱（碰撞箱）挤压的考虑，此处认为 7 格是最大
// 的操作距离，尽管创造模式下可以操作 12 格范围内的方块
func (c *Console) CanReachOrMove(targetPos protocol.BlockPos) error {
	deltaX := int(math.Abs(float64(targetPos[0] - c.position[0])))
	deltaY := int(math.Abs(float64(targetPos[1] - c.position[1])))
	deltaZ := int(math.Abs(float64(targetPos[2] - c.position[2])))

	if deltaX*deltaX+deltaY*deltaY+deltaZ*deltaZ > 36 {
		err := c.api.Commands().SendSettingsCommand(
			fmt.Sprintf("execute in overworld run tp %d %d %d", targetPos[0], targetPos[1], targetPos[2]),
			true,
		)
		if err != nil {
			return fmt.Errorf("CanReachOrMove: %v", err)
		}
		err = c.api.Commands().AwaitChangesGeneral()
		if err != nil {
			return fmt.Errorf("CanReachOrMove: %v", err)
		}
		c.position = targetPos
	}

	return nil
}
