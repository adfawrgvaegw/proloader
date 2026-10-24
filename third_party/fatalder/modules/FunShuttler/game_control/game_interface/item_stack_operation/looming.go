package item_stack_operation

import (
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
)

// Looming 指示织布机操作
type Looming struct {
	UsePattern  bool
	PatternName string

	PatternPath resources_control.SlotLocation
	BannerPath  resources_control.SlotLocation
	DyePath     resources_control.SlotLocation

	ResultItem resources_control.ExpectedNewItem
}

func (Looming) ID() uint8 {
	return IDItemStackOperationHighLevelLooming
}

func (Looming) CanInline() bool {
	return false
}

func (l Looming) Make(runtiemData MakingRuntime) []protocol.StackRequestAction {
	data := runtiemData.(LoomingRuntime)

	requestID := data.RequestID
	movePattern := protocol.PlaceStackRequestAction{}
	moveBackPattern := protocol.PlaceStackRequestAction{}
	moveBanner := protocol.PlaceStackRequestAction{}
	moveDye := protocol.PlaceStackRequestAction{}
	moveResult := protocol.TakeStackRequestAction{}

	if l.UsePattern {
		movePattern.Count = 1
		movePattern.Source = protocol.StackRequestSlotInfo{
			ContainerID:    data.MovePatternSrcContainerID,
			Slot:           byte(l.PatternPath.SlotID),
			StackNetworkID: data.MovePatternSrcStackNetworkID,
		}
		movePattern.Destination = protocol.StackRequestSlotInfo{
			ContainerID:    protocol.ContainerLoomMaterial,
			Slot:           11,
			StackNetworkID: data.LoomPatternStackNetworkID,
		}

		moveBackPattern.Count = 1
		moveBackPattern.Source = protocol.StackRequestSlotInfo{
			ContainerID:    protocol.ContainerLoomMaterial,
			Slot:           11,
			StackNetworkID: data.RequestID,
		}
		moveBackPattern.Destination = protocol.StackRequestSlotInfo{
			ContainerID:    data.MovePatternSrcContainerID,
			Slot:           byte(l.PatternPath.SlotID),
			StackNetworkID: data.RequestID,
		}
	}

	{
		moveBanner.Count = 1
		moveBanner.Source = protocol.StackRequestSlotInfo{
			ContainerID:    data.MoveBannerSrcContainerID,
			Slot:           byte(l.BannerPath.SlotID),
			StackNetworkID: data.MoveBannerSrcStackNetworkID,
		}
		moveBanner.Destination = protocol.StackRequestSlotInfo{
			ContainerID:    protocol.ContainerLoomInput,
			Slot:           9,
			StackNetworkID: data.LoomBannerStackNetworkID,
		}

		moveDye.Count = 1
		moveDye.Source = protocol.StackRequestSlotInfo{
			ContainerID:    data.MoveDyeSrcContainerID,
			Slot:           byte(l.DyePath.SlotID),
			StackNetworkID: data.MoveDyeSrcStackNetworkID,
		}
		moveDye.Destination = protocol.StackRequestSlotInfo{
			ContainerID:    protocol.ContainerLoomDye,
			Slot:           10,
			StackNetworkID: data.LoomDyeStackNetworkID,
		}

		moveResult.Count = 1
		moveResult.Source = protocol.StackRequestSlotInfo{
			ContainerID:    protocol.ContainerCreatedOutput, // [NEMC 1.20.10] 60 -> 61 (Added by Happy2018new)
			Slot:           0x32,
			StackNetworkID: requestID,
		}
		moveResult.Destination = protocol.StackRequestSlotInfo{
			ContainerID:    data.MoveBannerSrcContainerID,
			Slot:           byte(l.BannerPath.SlotID),
			StackNetworkID: data.RequestID,
		}
	}

	result := []protocol.StackRequestAction{
		&moveBanner,
		&moveDye,
		&protocol.CraftLoomRecipeStackRequestAction{Pattern: l.PatternName},
		&protocol.ConsumeStackRequestAction{
			DestroyStackRequestAction: protocol.DestroyStackRequestAction{
				Count: 1,
				Source: protocol.StackRequestSlotInfo{
					ContainerID:    protocol.ContainerLoomInput,
					Slot:           9,
					StackNetworkID: requestID,
				},
			},
		},
		&protocol.ConsumeStackRequestAction{
			DestroyStackRequestAction: protocol.DestroyStackRequestAction{
				Count: 1,
				Source: protocol.StackRequestSlotInfo{
					ContainerID:    protocol.ContainerLoomDye,
					Slot:           10,
					StackNetworkID: requestID,
				},
			},
		},
		&moveResult,
	}

	if l.UsePattern {
		result = append([]protocol.StackRequestAction{&movePattern}, result...)
		result = append(result, &moveBackPattern)
	}

	return result
}
