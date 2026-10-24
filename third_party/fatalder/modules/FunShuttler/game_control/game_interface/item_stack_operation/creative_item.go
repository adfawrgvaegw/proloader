package item_stack_operation

import (
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
)

// CreativeItem 指示创造物品获取操作
type CreativeItem struct {
	CINI  uint32 // CreativeItemNetworkID
	Path  resources_control.SlotLocation
	Count uint8
}

func (CreativeItem) ID() uint8 {
	return IDItemStackOperationCreativeItem
}

func (CreativeItem) CanInline() bool {
	return false
}

func (d CreativeItem) Make(runtimeData MakingRuntime) []protocol.StackRequestAction {
	data := runtimeData.(CreativeItemRuntime)

	move := protocol.PlaceStackRequestAction{}
	move.Count = d.Count
	move.Source = protocol.StackRequestSlotInfo{
		ContainerID:    protocol.ContainerCreatedOutput,
		Slot:           0x32,
		StackNetworkID: data.RequestID,
	}
	move.Destination = protocol.StackRequestSlotInfo{
		ContainerID:    data.DstContainerID,
		Slot:           byte(d.Path.SlotID),
		StackNetworkID: data.DstItemStackID,
	}

	return []protocol.StackRequestAction{
		&protocol.CraftCreativeStackRequestAction{CreativeItemNetworkID: data.CreativeItemNetworkID},
		&move,
	}
}
