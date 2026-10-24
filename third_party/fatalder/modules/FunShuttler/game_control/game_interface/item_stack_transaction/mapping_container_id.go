package item_stack_transaction

import (
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
	"github.com/Yeah114/FunShuttler/mapping"
)

// slotLocationToContainerID 根据 slotLocation 查找本地持有(或打开)的库存，
// 并找到对应 slotLocation 的容器 ID。api 指示容器资源的管理器
func slotLocationToContainerID(
	api *resources_control.ContainerManager,
	slotLocation resources_control.SlotLocation,
) (
	result resources_control.ContainerID,
	found bool,
) {
	switch slotLocation.WindowID {
	case protocol.WindowIDInventory:
		return protocol.ContainerCombinedHotBarAndInventory, true
	case protocol.WindowIDOffHand:
		return protocol.ContainerOffhand, true
	case protocol.WindowIDArmour:
		return protocol.ContainerArmor, true
	case protocol.WindowIDCrafting:
		return protocol.ContainerCraftingInput, true
	case protocol.WindowIDUI:
		return 0, false // TODO: Figure out what WindowIDUI means
	}

	containerData, containerID, existed := api.ContainerData()
	if !existed {
		return 0, false
	}
	if containerData.WindowID != byte(slotLocation.WindowID) {
		return 0, false
	}
	if containerID != mapping.ContainerIDUnknown {
		return containerID, true
	}

	containerTypeWithSlot := mapping.ContainerTypeWithSlot{
		ContainerType: int(containerData.ContainerType),
	}
	if mapping.ContainerNeedSlotIDMapping[containerTypeWithSlot.ContainerType] {
		containerTypeWithSlot.SlotID = uint8(slotLocation.SlotID)
	}
	if result, ok := mapping.ContainerIDMapping[containerTypeWithSlot]; ok {
		if result == mapping.ContainerIDUnknown || result == mapping.ContainerIDCanNotOpen {
			return 0, false
		}
		return resources_control.ContainerID(result), true
	}

	return 0, false
}
