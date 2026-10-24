package item_stack_operation

import (
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
)

// Move 指示物品移动操作
type Move struct {
	Source      resources_control.SlotLocation
	Destination resources_control.SlotLocation
	Count       int32
}

func (Move) ID() uint8 {
	return IDItemStackOperationMove
}

func (Move) CanInline() bool {
	return true
}

func (m Move) Make(runtimeData MakingRuntime) []protocol.StackRequestAction {
	data := runtimeData.(MoveRuntime)
	move := protocol.TakeStackRequestAction{}

	move.Count = byte(m.Count)
	move.Source = protocol.StackRequestSlotInfo{
		ContainerID:    data.MoveSrcContainerID,
		Slot:           byte(m.Source.SlotID),
		StackNetworkID: data.MoveSrcStackNetworkID,
	}
	move.Destination = protocol.StackRequestSlotInfo{
		ContainerID:    data.MoveDstContainerID,
		Slot:           byte(m.Destination.SlotID),
		StackNetworkID: data.MoveDstStackNetworkID,
	}

	return []protocol.StackRequestAction{
		&move,
	}
}
