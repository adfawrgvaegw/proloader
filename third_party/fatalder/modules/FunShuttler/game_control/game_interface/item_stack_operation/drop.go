package item_stack_operation

import (
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
)

// Drop 指示物品丢弃操作
type Drop struct {
	Path  resources_control.SlotLocation
	Count uint8
}

func (Drop) ID() uint8 {
	return IDItemStackOperationDrop
}

func (Drop) CanInline() bool {
	return true
}

func (d Drop) Make(runtimeData MakingRuntime) []protocol.StackRequestAction {
	data := runtimeData.(DropRuntime)
	return []protocol.StackRequestAction{
		&protocol.DropStackRequestAction{
			Count: d.Count,
			Source: protocol.StackRequestSlotInfo{
				ContainerID:    data.DropSrcContainerID,
				Slot:           byte(d.Path.SlotID),
				StackNetworkID: data.DropSrcStackNetworkID,
			},
			Randomly: data.Randomly,
		},
	}
}
