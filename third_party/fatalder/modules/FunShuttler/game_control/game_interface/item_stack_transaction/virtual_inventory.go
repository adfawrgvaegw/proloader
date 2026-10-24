package item_stack_transaction

import (
	"fmt"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
	"github.com/Yeah114/FunShuttler/utils"
)

// ------------------------- Define -------------------------

// virtualInventories 是虚拟库存实现，
// 它是一个内部实现细节，不应被其他人使用
type virtualInventories struct {
	api   *resources_control.Inventories
	items map[resources_control.SlotLocation]protocol.ItemInstance
}

// newVirtualInventories 基于 api 创建一个新的 newVirtualInventories
func newVirtualInventories(api *resources_control.Inventories) *virtualInventories {
	return &virtualInventories{
		api:   api,
		items: make(map[resources_control.SlotLocation]protocol.ItemInstance),
	}
}

// ------------------------- Basic item set and load -------------------------

// allItemInstances 列出窗口 ID 为 windowID 的库存中所有物品的网络物品堆栈信息。
// 保证 allItemInstances 在实现上是深拷贝的，这意味着使用者可以安全的修改返回值
func (v *virtualInventories) allItemInstances(windowID resources_control.WindowID) map[resources_control.SlotID]protocol.ItemInstance {
	result := make(map[resources_control.SlotID]protocol.ItemInstance)

	mapping, inventoryExisted := v.api.GetAllItemStack(windowID)
	if !inventoryExisted {
		return nil
	}
	for slotID, item := range mapping {
		result[slotID] = utils.DeepCopyItemInstance(*item)
	}

	for location, item := range v.items {
		if location.WindowID != windowID {
			continue
		}
		result[location.SlotID] = utils.DeepCopyItemInstance(item)
	}

	return result
}

// loadItemStack 从虚拟库存加载 slotLocation 处的物品。
// 如果虚拟库存中不存在，则试图从主库存中加载这个物品
func (v *virtualInventories) loadItemStack(slotLocation resources_control.SlotLocation) (result protocol.ItemStack, err error) {
	if item, ok := v.items[slotLocation]; ok {
		return utils.DeepCopyItemStack(item.Stack), nil
	}

	item, inventoryExisted := v.api.GetItemStack(slotLocation.WindowID, slotLocation.SlotID)
	if !inventoryExisted {
		return protocol.ItemStack{}, fmt.Errorf("loadItemStack: Can not find the item whose at %#v", slotLocation)
	}
	v.items[slotLocation] = utils.DeepCopyItemInstance(*item)

	return utils.DeepCopyItemStack(item.Stack), nil
}

// setItemStack 设置 slotLocation 处的物品为 itemStack。
// 保证 setItemStack 不会更改该物品的运行时 ID，并且调用
// 结束后，调用者可以安全的继续修改 itemStack 中的有关数据
func (v *virtualInventories) setItemStack(
	slotLocation resources_control.SlotLocation,
	itemStack protocol.ItemStack,
) error {
	currentItem, ok := v.items[slotLocation]
	if !ok {
		if _, err := v.loadItemStack(slotLocation); err != nil {
			return fmt.Errorf("setItemStack: %v", err)
		}
		currentItem = v.items[slotLocation]
	}

	currentItem = protocol.ItemInstance{
		StackNetworkID: currentItem.StackNetworkID,
		Stack:          utils.DeepCopyItemStack(itemStack),
	}

	v.items[slotLocation] = currentItem
	return nil
}

// setAir 将 slotLocation 处的物品改变为空气。
// 保证 setAir 不会更改该物品的运行时 ID
func (v *virtualInventories) setAir(slotLocation resources_control.SlotLocation) error {
	err := v.setItemStack(slotLocation, *resources_control.NewAirItemStack())
	if err != nil {
		return fmt.Errorf("setAir: %v", err)
	}
	return nil
}

// ------------------------- Item updaters -------------------------

// dumpToUpdaters 将虚拟库存中已记录的所有物品数据导出为 updaters
func (v *virtualInventories) dumpToUpdaters() map[resources_control.SlotLocation]resources_control.ExpectedNewItem {
	result := make(map[resources_control.SlotLocation]resources_control.ExpectedNewItem)

	for location, item := range v.items {
		newCanPlaceOn := make([]string, len(item.Stack.CanBePlacedOn))
		newCanDestroy := make([]string, len(item.Stack.CanBreak))

		copy(newCanPlaceOn, item.Stack.CanBePlacedOn)
		copy(newCanDestroy, item.Stack.CanBreak)

		result[location] = resources_control.ExpectedNewItem{
			ItemType: resources_control.ItemNewType{
				UseNetworkID: true,
				NetworkID:    item.Stack.ItemType.NetworkID,
				UseMetadata:  true,
				Metadata:     item.Stack.MetadataValue,
			},
			BlockRuntimeID: resources_control.ItemNewBlockRuntimeID{
				UseBlockRuntimeID: true,
				BlockRuntimeID:    item.Stack.BlockRuntimeID,
			},
			NBT: resources_control.ItemNewNBTData{
				UseNBTData:       true,
				UseOriginDamage:  false,
				NBTData:          utils.DeepCopyNBT(item.Stack.NBTData),
				ChangeRepairCost: false,
				ChangeDamage:     false,
			},
			Component: resources_control.ItemNewComponent{
				UseCanPlaceOn: true,
				CanPlaceOn:    newCanPlaceOn,
				UseCanDestroy: true,
				CanDestroy:    newCanDestroy,
			},
		}
	}

	return result
}

// updateFromUpdater 根据 slotLocation 和 clientExpected 设置相应物品的新值
func (v *virtualInventories) updateFromUpdater(
	slotLocation resources_control.SlotLocation,
	clientExpected resources_control.ExpectedNewItem,
) error {
	_, err := v.loadItemStack(slotLocation)
	if err != nil {
		return fmt.Errorf("updateFromUpdater: %v", err)
	}

	currentItem := v.items[slotLocation]
	resources_control.UpdateItemClientSide(&currentItem, slotLocation, clientExpected)
	v.items[slotLocation] = currentItem

	return nil
}

// ------------------------- Stack Network ID -------------------------

// loadStackNetworkID 加载 slotLocation 处的物品堆栈网络 ID
func (v *virtualInventories) loadStackNetworkID(slotLocation resources_control.SlotLocation) (result int32, err error) {
	_, err = v.loadItemStack(slotLocation)
	if err != nil {
		return 0, fmt.Errorf("loadStackNetworkID: %v", err)
	}
	return v.items[slotLocation].StackNetworkID, nil
}

// setStackNetworkID 设置 slotLocation 处的物品堆栈网络 ID 为 requestID
func (v *virtualInventories) setStackNetworkID(
	slotLocation resources_control.SlotLocation,
	requestID resources_control.ItemStackRequestID,
) error {
	if _, ok := v.items[slotLocation]; !ok {
		if _, err := v.loadItemStack(slotLocation); err != nil {
			return fmt.Errorf("setStackNetworkID: %v", err)
		}
	}

	v.items[slotLocation] = protocol.ItemInstance{
		StackNetworkID: int32(requestID),
		Stack:          v.items[slotLocation].Stack,
	}

	return nil
}

// loadAndSetStackNetworkID 加载 slotLocation 处的物品堆栈网络 ID，
// 并将 slotLocation 处的物品堆栈网络 ID 更新为 requestID
func (v *virtualInventories) loadAndSetStackNetworkID(
	slotLocation resources_control.SlotLocation,
	requestID resources_control.ItemStackRequestID,
) (result int32, err error) {
	result, err = v.loadStackNetworkID(slotLocation)
	if err != nil {
		return 0, fmt.Errorf("loadAndSetStackNetworkID: %v", err)
	}

	err = v.setStackNetworkID(slotLocation, requestID)
	if err != nil {
		return 0, fmt.Errorf("loadAndSetStackNetworkID: %v", err)
	}

	return
}

// ------------------------- Item Count -------------------------

// loadItemCount 加载 slotLocation 处的物品数量
func (v *virtualInventories) loadItemCount(slotLocation resources_control.SlotLocation) (result uint8, err error) {
	_, err = v.loadItemStack(slotLocation)
	if err != nil {
		return 0, fmt.Errorf("loadStackNetworkID: %v", err)
	}
	return uint8(v.items[slotLocation].Stack.Count), nil
}

// addItemCount 将 slotLocation 处的物品数量添加 delta。
// 另外，delta 可以是负数。
// allowNoChange 指示是否允许 delta 为 0。如果填写假且
// delta 为 0，那么返回错误
func (v *virtualInventories) addItemCount(
	slotLocation resources_control.SlotLocation,
	delta int8,
	allowNoChange bool,
) error {
	_, ok := v.items[slotLocation]
	if !ok {
		if _, err := v.loadItemStack(slotLocation); err != nil {
			return fmt.Errorf("addItemCount: %v", err)
		}
	}

	if delta == 0 && !allowNoChange {
		return fmt.Errorf("addItemCount: Item count no change when not allow no change")
	}

	resultCount := int8(v.items[slotLocation].Stack.Count) + delta
	if resultCount < 0 {
		return fmt.Errorf(
			"addItemCount: Invalid result count (origin count = %d, delta = %d, result count = %d)",
			v.items[slotLocation].Stack.Count, delta, resultCount,
		)
	}

	currentItem := v.items[slotLocation]
	currentItem.Stack.Count = uint16(resultCount)
	v.items[slotLocation] = currentItem

	return nil
}

// addItemCount 将 slotLocation 处的物品数量设置为 count
func (v *virtualInventories) setItemCount(slotLocation resources_control.SlotLocation, count uint8) error {
	_, ok := v.items[slotLocation]
	if !ok {
		if _, err := v.loadItemStack(slotLocation); err != nil {
			return fmt.Errorf("setItemCount: %v", err)
		}
	}

	if count > 64 {
		return fmt.Errorf("setItemCount: Invalid count %d because it more than 64", count)
	}

	currentItem := v.items[slotLocation]
	currentItem.Stack.Count = uint16(count)
	v.items[slotLocation] = currentItem

	return nil
}

// loadAndAddItemCount 加载 slotLocation 处的物品数量，
// 并将该数量添加 delta。
// allowNoChange 指示是否允许 delta 为 0。如果填写假且
// delta 为 0，那么返回错误
func (v *virtualInventories) loadAndAddItemCount(
	slotLocation resources_control.SlotLocation,
	delta int8,
	allowNoChange bool,
) (result uint8, err error) {
	result, err = v.loadItemCount(slotLocation)
	if err != nil {
		return 0, fmt.Errorf("loadAndAddItemCount: %v", err)
	}
	err = v.addItemCount(slotLocation, delta, allowNoChange)
	if err != nil {
		return 0, fmt.Errorf("loadAndAddItemCount: %v", err)
	}
	return
}

// loadAndSetItemCount 加载 slotLocation 处的物品数量，
// 并将该数量设置为 count
func (v *virtualInventories) loadAndSetItemCount(
	slotLocation resources_control.SlotLocation,
	count uint8,
) (result uint8, err error) {
	result, err = v.loadItemCount(slotLocation)
	if err != nil {
		return 0, fmt.Errorf("loadAndSetItemCount: %v", err)
	}
	err = v.setItemCount(slotLocation, count)
	if err != nil {
		return 0, fmt.Errorf("loadAndAddItemCount: %v", err)
	}
	return
}

// ------------------------- End -------------------------
