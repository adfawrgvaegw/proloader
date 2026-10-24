package nbt_console

import (
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
	"github.com/Yeah114/FunShuttler/nbt_assigner/block_helper"
)

// SetSlotUseCallback 设置当特定的背包物品栏被使用时应当执行的回调函数。
// requester 指示每个请求提起者的唯一标识；
// slotID 指示每个请求中涉及的背包槽位索引
func (c *Console) SetSlotUseCallback(
	f func(requester string, slotID resources_control.SlotID),
) {
	c.inventoryUseCallback = append(c.inventoryUseCallback, f)
}

// SetHelperUseCallback 设置当操作台中心方
// 块或帮助方块被使用时应当执行的回调函数。
// requester 指示每个请求提起者的唯一标识；
// index 是这个方块在操作台上的索引
func (c *Console) SetHelperUseCallback(
	f func(requester string, index int),
) {
	c.blocksUseCallback = append(c.blocksUseCallback, f)
}

// UseInventorySlot 将背包 slotID 处的物品设置为 notAir，
// 并将该更改广播到依赖于这个数据的所有人。
// requester 指示 UseInventorySlot 调用者的唯一标识
func (c *Console) UseInventorySlot(requester string, slot resources_control.SlotID, notAir bool) {
	c.airSlotInInventory[slot] = notAir
	for _, f := range c.inventoryUseCallback {
		f(requester, slot)
	}
}

// UseHelperBlock 将操作台 index 处的帮助方块 (或中心方块)
// 更新为 newBlock，并将该更改广播到依赖于这个数据的所有人。
// requester 指示 UseHelperBlock 调用者的唯一标识
func (c *Console) UseHelperBlock(requester string, index int, newBlock block_helper.BlockHelper) {
	*c.helperBlocks[index] = newBlock
	for _, f := range c.blocksUseCallback {
		f(requester, index)
	}
}
