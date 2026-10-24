package item_stack_operation

import (
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
)

// CraftingConsume 是合成操作中单个物品消耗动作。
// 如果操作成功，目标物品应当完全消耗，并且没有残留
type CraftingConsume struct {
	Slot           resources_control.SlotID // 被消耗物品所在的槽位
	StackNetworkID int32                    // 被消耗物品的运行时 ID
	Count          uint8                    // 消耗的物品数量
}

// Crafting 指示合成操作
type Crafting struct {
	RecipeNetworkID uint32
	ResultSlotID    resources_control.SlotID
	ResultCount     uint8
	ResultItem      resources_control.ExpectedNewItem
}

func (Crafting) ID() uint8 {
	return IDItemStackOperationHighLevelCrafting
}

func (Crafting) CanInline() bool {
	return false
}

func (d Crafting) Make(runtimeData MakingRuntime) []protocol.StackRequestAction {
	data := runtimeData.(CraftingRuntime)

	consumeActions := make([]protocol.StackRequestAction, 0)
	for _, value := range data.Consumes {
		consumeActions = append(
			consumeActions,
			&protocol.ConsumeStackRequestAction{
				DestroyStackRequestAction: protocol.DestroyStackRequestAction{
					Count: value.Count,
					Source: protocol.StackRequestSlotInfo{
						ContainerID:    protocol.ContainerCraftingInput,
						Slot:           byte(value.Slot),
						StackNetworkID: value.StackNetworkID,
					},
				},
			},
		)
	}

	moveBack := protocol.TakeStackRequestAction{}
	moveBack.Count = d.ResultCount
	moveBack.Source = protocol.StackRequestSlotInfo{
		ContainerID:    protocol.ContainerCreatedOutput, // [NEMC 1.20.10] 60 -> 61 (Added by Happy2018new)
		Slot:           0x32,
		StackNetworkID: data.RequestID,
	}
	moveBack.Destination = protocol.StackRequestSlotInfo{
		ContainerID:    protocol.ContainerCombinedHotBarAndInventory,
		Slot:           byte(d.ResultSlotID),
		StackNetworkID: data.ResultStackNetworkID,
	}

	actions := append(
		[]protocol.StackRequestAction{
			&protocol.CraftRecipeStackRequestAction{RecipeNetworkID: d.RecipeNetworkID},
		},
		consumeActions...,
	)
	actions = append(actions, &moveBack)

	return actions
}
