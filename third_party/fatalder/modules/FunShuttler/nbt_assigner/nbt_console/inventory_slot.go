package nbt_console

import (
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
)

// FindInventorySlot 从背包查找一个空气物品。
// 如果背包已满，返回一个不被 exclusion 包含
// 在内的一个物品栏
func (c Console) FindInventorySlot(exclusion []resources_control.SlotID) resources_control.SlotID {
	exclusionMapping := make(map[int]bool)
	for _, slotID := range exclusion {
		exclusionMapping[int(slotID)] = true
	}

	for index, notAir := range c.airSlotInInventory {
		if !notAir {
			return resources_control.SlotID(index)
		}
	}

	for index := range c.airSlotInInventory {
		if !exclusionMapping[index] {
			return resources_control.SlotID(index)
		}
	}

	panic("FindInventorySlot: Impossible to find a available slot when exclusion contains the whole inventory")
}

// InventorySlotIsNonAir 返回背包 slotID 处的物品是否不是空气
func (c Console) InventorySlotIsNonAir(slotID resources_control.SlotID) (notAir bool) {
	return c.airSlotInInventory[slotID]
}

// CleanInventory 将背包中的所有物品标记为空气
func (c *Console) CleanInventory() {
	for index := range 36 {
		c.UseInventorySlot(
			RequesterSystemCall,
			resources_control.SlotID(index),
			false,
		)
	}
}
