package nbt_console

import "github.com/Yeah114/FunShuttler/core/minecraft/protocol"

// BlockPosByIndex 按 index 查找操作台上目标方块的绝对坐标。
// index 为 0 将查找操作台中心处方块的绝对坐标，
// index 为 i (i>0) 将查找第 i-1 个帮助方块的绝对坐标
func (c Console) BlockPosByIndex(index int) protocol.BlockPos {
	offset := helperBlockMapping[index]
	return protocol.BlockPos{
		c.center[0] + offset[0],
		c.center[1] + offset[1],
		c.center[2] + offset[2],
	}
}

// BlockPosByOffset 按坐标偏移量查找操作台上目标方块的绝对坐标。
// 如果给出的偏移量不能对应操作台上的方块，则返回操作台中心坐标
func (c Console) BlockPosByOffset(offset protocol.BlockPos) protocol.BlockPos {
	return protocol.BlockPos{
		c.center[0] + offset[0],
		c.center[1] + offset[1],
		c.center[2] + offset[2],
	}
}

// NearBlockPosByIndex 按 index 查找操作台上帮助方块
// (或中心处方块)的相邻方块的绝对坐标。
//
// index 为 0 将查找操作台中心处相邻方块的绝对坐标，
// index 为 i (i>0) 将查找第 i-1 个帮助方块处相邻方
// 块的绝对坐标。
//
// nearOffset 指示在根据 index 找到目标帮助方块(或中心
// 方块)后，相邻方块相对于这个方块本身的坐标偏移。然后，
// 我们偏移到该方块上并返回该方块的绝对坐标
func (c Console) NearBlockPosByIndex(index int, nearOffset protocol.BlockPos) protocol.BlockPos {
	pos := c.BlockPosByIndex(index)
	return protocol.BlockPos{
		pos[0] + nearOffset[0],
		pos[1] + nearOffset[1],
		pos[2] + nearOffset[2],
	}
}

// NearBlockByOffset 按 offset 查找操作台上帮助方块
// (或中心处方块)的相邻方块的绝对坐标。
//
// 如果给出的偏移量不能对应操作台上的方块，则返回操作台
// 中心坐标偏移 nearOffset 后的绝对坐标。
//
// nearOffset 指示在根据 offset 找到目标帮助方块(或中心
// 方块)后，相邻方块相对于这个方块本身的坐标偏移。然后，我
// 们偏移到该方块上并返回该方块的绝对坐标
func (c Console) NearBlockPosByOffset(offset protocol.BlockPos, nearOffset protocol.BlockPos) protocol.BlockPos {
	pos := c.BlockPosByOffset(offset)
	return protocol.BlockPos{
		pos[0] + nearOffset[0],
		pos[1] + nearOffset[1],
		pos[2] + nearOffset[2],
	}
}
