package item_stack_operation

import (
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
)

// Renaming 指示基于铁砧的物品重命名操作
type Renaming struct {
	Path    resources_control.SlotLocation
	NewName string
}

func (Renaming) ID() uint8 {
	return IDItemStackOperationHighLevelRenaming
}

func (Renaming) CanInline() bool {
	return false
}

func (r Renaming) Make(runtimeData MakingRuntime) []protocol.StackRequestAction {
	data := runtimeData.(RenamingRuntime)

	move := protocol.TakeStackRequestAction{}
	move.Count = data.ItemCount
	move.Source = protocol.StackRequestSlotInfo{
		ContainerID:    data.SrcContainerID,
		Slot:           byte(r.Path.SlotID),
		StackNetworkID: data.SrcStackNetworkID,
	}
	move.Destination = protocol.StackRequestSlotInfo{
		ContainerID:    0,
		Slot:           1,
		StackNetworkID: data.AnvilSlotStackNetworkID,
	}

	moveBack := protocol.PlaceStackRequestAction{}
	moveBack.Count = data.ItemCount
	moveBack.Source = protocol.StackRequestSlotInfo{
		ContainerID:    protocol.ContainerCreatedOutput,
		Slot:           0x32,
		StackNetworkID: data.RequestID,
	}
	moveBack.Destination = protocol.StackRequestSlotInfo{
		ContainerID:    data.SrcContainerID,
		Slot:           byte(r.Path.SlotID),
		StackNetworkID: data.RequestID,
	}

	return []protocol.StackRequestAction{
		&move,
		&protocol.CraftRecipeOptionalStackRequestAction{
			RecipeNetworkID:   0,
			FilterStringIndex: 0,
		},
		&protocol.ConsumeStackRequestAction{
			DestroyStackRequestAction: protocol.DestroyStackRequestAction{
				Count: data.ItemCount,
				Source: protocol.StackRequestSlotInfo{
					ContainerID:    0,
					Slot:           1,
					StackNetworkID: data.RequestID,
				},
			},
		},
		&moveBack,
	}
}
