package game_interface

import (
	"fmt"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
)

// ItemTransition 是基于 ResourcesWrapper 和 ItemStackOperation 实现的物品状态转移
type ItemTransition struct {
	wrapper *ResourcesWrapper
	api     *ItemStackOperation
}

// NewItemTransition 根据 wrapper 和 api 创建并返回一个新的 ItemTransition
func NewItemTransition(wrapper *ResourcesWrapper, api *ItemStackOperation) *ItemTransition {
	return &ItemTransition{
		wrapper: wrapper,
		api:     api,
	}
}

// Transition 将库存 srcWindowID 处 src 所指示的物品状
// 态转移到库存 dstWindowID 中，并且指定最终状态是 dst。
//
// 应当保证 dst 的物品可以完全从 src 中获得，
// 并且 src 和 dst 均不包括空气。
// 另外，实现假设状态转移前 dst 所指示的所有槽位都是空气。
//
// 底层实现不保证最终提交的操作是最简的，
// 这是因为目前使用的是基本的基线算法实现
func (h *ItemTransition) Transition(
	src []ItemInfoWithSlot,
	dst []ItemInfoWithSlot,
	srcWindowID resources_control.WindowID,
	dstWindowID resources_control.WindowID,
) (success bool, err error) {
	if len(dst) == 0 {
		return true, nil
	}
	if len(src) == 0 {
		return false, fmt.Errorf("Transition: Given src is empty but dst is not; src = %#v, dst = %#v", src, dst)
	}

	srcMapping := make(map[ItemType][]ItemInfoWithSlot)
	for _, item := range src {
		srcMapping[item.ItemInfo.ItemType] = append(
			srcMapping[item.ItemInfo.ItemType],
			item,
		)
	}

	transaction := h.api.OpenTransaction()
	for index, dstItem := range dst {
		requireCount := dstItem.ItemInfo.Count

		for idx, srcItem := range srcMapping[dstItem.ItemInfo.ItemType] {
			// If the have count is 0, then this src item is used from other dst slot
			haveCount := srcItem.ItemInfo.Count
			if haveCount == 0 {
				continue
			}
			// If have count is more than require count, then we just need the one we needed
			if haveCount > requireCount {
				_ = transaction.MoveItem(
					resources_control.SlotLocation{
						WindowID: srcWindowID,
						SlotID:   srcItem.Slot,
					},
					resources_control.SlotLocation{
						WindowID: dstWindowID,
						SlotID:   dstItem.Slot,
					},
					requireCount,
				)
				srcMapping[srcItem.ItemInfo.ItemType][idx].ItemInfo.Count -= requireCount
				requireCount = 0
				break
			}
			// Or, haveCount <= requireCount,
			// then we only move haveCount
			_ = transaction.MoveItem(
				resources_control.SlotLocation{
					WindowID: srcWindowID,
					SlotID:   srcItem.Slot,
				},
				resources_control.SlotLocation{
					WindowID: dstWindowID,
					SlotID:   dstItem.Slot,
				},
				haveCount,
			)
			srcMapping[srcItem.ItemInfo.ItemType][idx].ItemInfo.Count = 0
			requireCount -= haveCount
			// If haveCount is equal to requireCount,
			// then the requireCount will equal to 0
			// here, and we need to break this loop
			if requireCount == 0 {
				break
			}
		}

		if requireCount > 0 {
			return false, fmt.Errorf(
				"Transition: dst can't move from src because the given src is not enough; dst[%d] = %#v; src = %#v; dst = %#v",
				index, dstItem, src, dst,
			)
		}
	}

	success, _, _, err = transaction.Commit()
	if err != nil {
		return false, fmt.Errorf("Transition: %v", err)
	}

	return
}

// TransitionBetweenInventory 将背包中 src 所
// 指示的物品进行状态转移，并且指定最终状态是 dst。
//
// 应当保证 dst 的物品可以完全从 src 中获得，
// 并且 src 和 dst 均不包括空气。
// 另外，TransitionBetweenInventory 假设状态转移
// 前 dst 所指示的所有槽位
// 都是空气。

// 底层实现不保证最终提交的操作是最简的，
// 这是因为目前使用的是基本的基线算法实现。
//
// 此操作需要保证背包已被打开，
// 或者已打开的容器中可以在背包中移动物品
func (h *ItemTransition) TransitionBetweenInventory(src []ItemInfoWithSlot, dst []ItemInfoWithSlot) (
	success bool,
	err error,
) {
	success, err = h.Transition(src, dst, protocol.WindowIDInventory, protocol.WindowIDInventory)
	if err != nil {
		return false, fmt.Errorf("TransitionBetweenInventory: %v", err)
	}
	return
}

// TransitionBetweenContainer 将已打开容器中 src
// 所指示的物品进行状态转移，并且指定最终状态是 dst。
//
// 应当保证 dst 的物品可以完全从 src 中获得，
// 并且 src 和 dst 均不包括空气。
// 另外，TransitionBetweenContainer 假设状态转移前
// dst 所指示的所有槽位都是空气。
//
// 底层实现不保证最终提交的操作是最简的，
// 这是因为目前使用的是基本的基线算法实现。
//
// 此操作需要保证目前已经打开了一个容器，
// 否则效果将会与 TransitionBetweenInventory 等同
func (h *ItemTransition) TransitionBetweenContainer(src []ItemInfoWithSlot, dst []ItemInfoWithSlot) (
	success bool,
	err error,
) {
	data, _, _ := h.wrapper.Container().ContainerData()
	windowID := resources_control.WindowID(data.WindowID)

	success, err = h.Transition(src, dst, windowID, windowID)
	if err != nil {
		return false, fmt.Errorf("TransitionBetweenContainer: %v", err)
	}

	return
}

// TransitionToContainer 将背包中 src 所指示的物
// 品状态转移到已打开容器中，并且指定最终状态是 dst。
//
// 应当保证 dst 的物品可以完全从 src 中获得，
// 并且 src 和 dst 均不包括空气。
// 另外，实现TransitionToContainer 假设状态转移前
// dst 所指示的所有槽位都是空气。
//
// 底层实现不保证最终提交的操作是最简的，
// 目前使用的是基本的基线算法实现。
//
// 此操作需要保证目前已经打开了一个容器，
// 否则效果将会与 TransitionBetweenInventory 等同
func (h *ItemTransition) TransitionToContainer(src []ItemInfoWithSlot, dst []ItemInfoWithSlot) (
	success bool,
	err error,
) {
	data, _, _ := h.wrapper.Container().ContainerData()
	windowID := resources_control.WindowID(data.WindowID)

	success, err = h.Transition(src, dst, protocol.WindowIDInventory, windowID)
	if err != nil {
		return false, fmt.Errorf("TransitionToContainer: %v", err)
	}

	return
}

// TransitionToInventory 将已打开容器中 src 所指
// 示的物品状态转移到背包中，并且指定最终状态是 dst。
//
// 应当保证 dst 的物品可以完全从 src 中获得，
// 并且 src 和 dst 均不包括空气。
// 另外，TransitionToInventory 假设状态转移前 dst
// 所指示的所有槽位都是空气。
//
// 底层实现不保证最终提交的操作是最简的，
// 这是因为目前使用的是基本的基线算法实现。
//
// 此操作需要保证目前已经打开了一个容器，
// 否则效果将会与 TransitionBetweenInventory 等同
func (h *ItemTransition) TransitionToInventory(src []ItemInfoWithSlot, dst []ItemInfoWithSlot) (
	success bool,
	err error,
) {
	data, _, _ := h.wrapper.Container().ContainerData()
	windowID := resources_control.WindowID(data.WindowID)

	success, err = h.Transition(src, dst, windowID, protocol.WindowIDInventory)
	if err != nil {
		return false, fmt.Errorf("TransitionToInventory: %v", err)
	}

	return
}
