package item_stack_operation

import (
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
)

// Swap 指示物品交换操作
type Swap struct {
	Source      resources_control.SlotLocation
	Destination resources_control.SlotLocation
}

func (Swap) ID() uint8 {
	return IDItemStackOperationSwap
}

func (Swap) CanInline() bool {
	return true
}

func (s Swap) Make(runtimeData MakingRuntime) []protocol.StackRequestAction {
	data := runtimeData.(SwapRuntime)
	return []protocol.StackRequestAction{
		&protocol.SwapStackRequestAction{
			Source: protocol.StackRequestSlotInfo{
				ContainerID:    data.SwapSrcContainerID,
				Slot:           byte(s.Source.SlotID),
				StackNetworkID: data.SwapSrcStackNetworkID,
			},
			Destination: protocol.StackRequestSlotInfo{
				ContainerID:    data.SwapDstContainerID,
				Slot:           byte(s.Destination.SlotID),
				StackNetworkID: data.SwapDstStackNetworkID,
			},
		},
	}
}
