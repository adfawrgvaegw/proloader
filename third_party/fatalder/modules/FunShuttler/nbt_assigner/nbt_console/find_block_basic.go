package nbt_console

import (
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/nbt_assigner/block_helper"
)

// BlockByIndex 按 index 查找操作台上的方块。
// index 为 0 将查找操作台中心的方块，
// index 为 i (i>0) 将查找第 i-1 个帮助方块
func (c Console) BlockByIndex(index int) (result *block_helper.BlockHelper) {
	return c.helperBlocks[index]
}

// BlockByOffset 按坐标偏移量查找操作台上的方块。
// 如果给出的偏移量不能对应操作台上的方块，则返回操作台中心处的方块
func (c Console) BlockByOffset(offset protocol.BlockPos) (result *block_helper.BlockHelper) {
	return c.helperBlocks[helperBlockMappingInv[offset]]
}

// NearBlockByIndex 按 index 先找到操作台上的帮助方块，
// 然后按 nearOffset 偏移到目标方块并返回这个方块。
//
// index 为 0 将查找操作台中心处方块的相邻方块，
// index 为 i (i>0) 将查找第 i-1 个帮助方块的相邻方块
func (c Console) NearBlockByIndex(index int, nearOffset protocol.BlockPos) (result *block_helper.BlockHelper) {
	return c.nearBlocks[index][nearBlockMappingInv[nearOffset]]
}

// NearBlockByOffset 按 offset 先找到操作台上的帮助方块，
// 然后按 nearOffset 偏移到目标方块并返回这个方块。
//
// 如果给出的偏移量不能对应操作台上的方块，则返回操作台中心
// 处方块的相邻方块
func (c Console) NearBlockByOffset(offset protocol.BlockPos, nearOffset protocol.BlockPos) (result *block_helper.BlockHelper) {
	return c.nearBlocks[helperBlockMappingInv[offset]][nearBlockMappingInv[offset]]
}
