package main

import (
	"fmt"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	standard_protocol "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol"
)

func S2NStackRequestSlotInfo(stackRequestSlotInfo *protocol.StackRequestSlotInfo) {
	if stackRequestSlotInfo.ContainerID > 17 {
		stackRequestSlotInfo.ContainerID++
	}
}

func S2NItemStackRequest(standardItemStackRequest standard_protocol.ItemStackRequest) (protocol.ItemStackRequest, error) {
	itemStackRequest := protocol.ItemStackRequest{
		RequestID: standardItemStackRequest.RequestID,
		FilterStrings: standardItemStackRequest.FilterStrings,
		FilterCause: standardItemStackRequest.FilterCause,
	}
	actions := make([]protocol.StackRequestAction, 0)
	for _, standardAction := range standardItemStackRequest.Actions {
		switch val := standardAction.(type) {
		case *standard_protocol.TakeStackRequestAction:
			action := &protocol.TakeStackRequestAction{}
			if err := ConvertStruct(val, action); err != nil {
				return itemStackRequest, err
			}
			S2NStackRequestSlotInfo(&action.Source)
			S2NStackRequestSlotInfo(&action.Destination)
			actions = append(actions, action)
		case *standard_protocol.PlaceStackRequestAction:
			action := &protocol.PlaceStackRequestAction{}
			if err := ConvertStruct(val, action); err != nil {
				return itemStackRequest, err
			}
			S2NStackRequestSlotInfo(&action.Source)
			S2NStackRequestSlotInfo(&action.Destination)
			actions = append(actions, action)
		case *standard_protocol.SwapStackRequestAction:
			action := &protocol.SwapStackRequestAction{}
			if err := ConvertStruct(val, action); err != nil {
				return itemStackRequest, err
			}
			S2NStackRequestSlotInfo(&action.Source)
			S2NStackRequestSlotInfo(&action.Destination)
			actions = append(actions, action)
		case *standard_protocol.DropStackRequestAction:
			action := &protocol.DropStackRequestAction{}
			if err := ConvertStruct(val, action); err != nil {
				return itemStackRequest, err
			}
			S2NStackRequestSlotInfo(&action.Source)
			actions = append(actions, action)
		case *standard_protocol.DestroyStackRequestAction:
			action := &protocol.DestroyStackRequestAction{}
			if err := ConvertStruct(val, action); err != nil {
				return itemStackRequest, err
			}
			S2NStackRequestSlotInfo(&action.Source)
			actions = append(actions, action)
		case *standard_protocol.ConsumeStackRequestAction:
			action := &protocol.ConsumeStackRequestAction{}
			if err := ConvertStruct(val, action); err != nil {
				return itemStackRequest, err
			}
			S2NStackRequestSlotInfo(&action.Source)
			actions = append(actions, action)
		case *standard_protocol.CreateStackRequestAction:
			action := &protocol.CreateStackRequestAction{}
			if err := ConvertStruct(val, action); err != nil {
				return itemStackRequest, err
			}
			actions = append(actions, action)
		case *standard_protocol.LabTableCombineStackRequestAction:
			action := &protocol.LabTableCombineStackRequestAction{}
			if err := ConvertStruct(val, action); err != nil {
				return itemStackRequest, err
			}
			actions = append(actions, action)
		case *standard_protocol.BeaconPaymentStackRequestAction:
			action := &protocol.BeaconPaymentStackRequestAction{}
			if err := ConvertStruct(val, action); err != nil {
				return itemStackRequest, err
			}
			actions = append(actions, action)
		case *standard_protocol.MineBlockStackRequestAction:
			action := &protocol.MineBlockStackRequestAction{}
			if err := ConvertStruct(val, action); err != nil {
				return itemStackRequest, err
			}
			actions = append(actions, action)
		case *standard_protocol.CraftRecipeStackRequestAction:
			action := &protocol.CraftRecipeStackRequestAction{}
			if err := ConvertStruct(val, action); err != nil {
				return itemStackRequest, err
			}
			actions = append(actions, action)
		case *standard_protocol.AutoCraftRecipeStackRequestAction:
			action := &protocol.AutoCraftRecipeStackRequestAction{}
			if err := ConvertStruct(val, action); err != nil {
				return itemStackRequest, err
			}
			actions = append(actions, action)
		case *standard_protocol.CraftCreativeStackRequestAction:
			action := &protocol.CraftCreativeStackRequestAction{}
			if err := ConvertStruct(val, action); err != nil {
				return itemStackRequest, err
			}
			actions = append(actions, action)
		case *standard_protocol.CraftRecipeOptionalStackRequestAction:
			action := &protocol.CraftRecipeOptionalStackRequestAction{}
			if err := ConvertStruct(val, action); err != nil {
				return itemStackRequest, err
			}
			actions = append(actions, action)
		case *standard_protocol.CraftGrindstoneRecipeStackRequestAction:
			action := &protocol.CraftGrindstoneRecipeStackRequestAction{}
			if err := ConvertStruct(val, action); err != nil {
				return itemStackRequest, err
			}
			actions = append(actions, action)
		case *standard_protocol.CraftLoomRecipeStackRequestAction:
			action := &protocol.CraftLoomRecipeStackRequestAction{}
			if err := ConvertStruct(val, action); err != nil {
				return itemStackRequest, err
			}
			actions = append(actions, action)
		case *standard_protocol.CraftNonImplementedStackRequestAction:
			action := &protocol.CraftNonImplementedStackRequestAction{}
			if err := ConvertStruct(val, action); err != nil {
				return itemStackRequest, err
			}
			actions = append(actions, action)
		case *standard_protocol.CraftResultsDeprecatedStackRequestAction:
			action := &protocol.CraftResultsDeprecatedStackRequestAction{}
			if err := ConvertStruct(val, action); err != nil {
				return itemStackRequest, err
			}
			actions = append(actions, action)
		default:
			return itemStackRequest, fmt.Errorf("unknown stack request action type: %T", val)
		}
	}
	itemStackRequest.Actions = actions
	return itemStackRequest, nil
}

