package nbt_assigner_utils

import (
	"fmt"

	"github.com/Yeah114/FunShuttler/game_control/game_interface"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_cache"
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_console"
	nbt_parser_block "github.com/Yeah114/FunShuttler/nbt_parser/block"
	nbt_hash "github.com/Yeah114/FunShuttler/nbt_parser/hash"
)

// ItemTransition 将已置于操作台中心的 srcContainer 转移为 dstContainer。
// 应当保证 dstContainer 中的物品可以只通过移动 srcContainer 中的物品得到
func ItemTransition(
	console *nbt_console.Console,
	cache *nbt_cache.NBTCacheSystem,
	srcContainer nbt_parser_block.Container,
	dstContainer nbt_parser_block.Container,
) error {
	api := console.API()

	// 清空背包
	_, err := api.Commands().SendWSCommandWithResp("clear")
	if err != nil {
		return fmt.Errorf("ItemTransition: %v", err)
	}
	console.CleanInventory()

	// 占用所有物品栏，
	// 因为我们无法确保数据匹配
	for index := range 36 {
		console.UseInventorySlot(nbt_console.RequesterUser, resources_control.SlotID(index), true)
	}

	// 打开加载好的容器
	success, err := console.OpenContainerByIndex(nbt_console.ConsoleIndexCenterBlock)
	if err != nil {
		return fmt.Errorf("ItemTransition: %v", err)
	}
	if !success {
		return fmt.Errorf("ItemTransition: Failed to open the container for %#v (stage 1)", srcContainer)
	}

	// 将该容器内的物品移动到背包 (置于第 10 到第 36 格)
	transaction := api.ItemStackOperation().OpenTransaction()
	for index, item := range srcContainer.NBT.Items {
		_ = transaction.MoveToInventory(
			resources_control.SlotID(item.Slot),
			resources_control.SlotID(index+9),
			item.Item.ItemCount(),
		)
	}

	// 提交更改
	success, _, _, err = transaction.Commit()
	if err != nil {
		_ = api.ContainerOpenAndClose().CloseContainer()
		return fmt.Errorf("ItemTransition: %v", err)
	}
	if !success {
		_ = api.ContainerOpenAndClose().CloseContainer()
		return fmt.Errorf("ItemTransition: The server rejected the stack request action")
	}

	// 关闭容器
	err = api.ContainerOpenAndClose().CloseContainer()
	if err != nil {
		return fmt.Errorf("ItemTransition: %v", err)
	}

	// 生成新容器
	err = SpawnNewEmptyBlock(
		console,
		cache,
		EmptyBlockData{
			Name:                  dstContainer.BlockName(),
			States:                dstContainer.BlockStates(),
			IsCanOpenConatiner:    true,
			ConsiderOpenDirection: dstContainer.ConsiderOpenDirection(),
			ShulkerFacing:         dstContainer.NBT.ShulkerFacing,
			BlockCustomName:       dstContainer.NBT.CustomName,
		},
	)
	if err != nil {
		return fmt.Errorf("ItemTransition: %v", err)
	}

	// 打开新容器
	success, err = console.OpenContainerByIndex(nbt_console.ConsoleIndexCenterBlock)
	if err != nil {
		return fmt.Errorf("ItemTransition: %v", err)
	}
	if !success {
		return fmt.Errorf("ItemTransition: Failed to open the container for %#v (stage 2)", srcContainer)
	}
	defer api.ContainerOpenAndClose().CloseContainer()

	// 准备
	itemTypeIndex := game_interface.ItemType(0)
	itemTypeMapping := make(map[uint64]game_interface.ItemType)
	src := make([]game_interface.ItemInfoWithSlot, 0)
	dst := make([]game_interface.ItemInfoWithSlot, 0)

	// 处理源
	for index, item := range srcContainer.NBT.Items {
		hashNumber := nbt_hash.NBTItemTypeHash(item.Item)

		if _, ok := itemTypeMapping[hashNumber]; !ok {
			itemTypeMapping[hashNumber] = itemTypeIndex
			itemTypeIndex++
		}

		src = append(src, game_interface.ItemInfoWithSlot{
			Slot: resources_control.SlotID(index + 9),
			ItemInfo: game_interface.ItemInfo{
				Count:    item.Item.ItemCount(),
				ItemType: itemTypeMapping[hashNumber],
			},
		})
	}

	// 处理目的地
	for _, item := range dstContainer.NBT.Items {
		hashNumber := nbt_hash.NBTItemTypeHash(item.Item)

		if _, ok := itemTypeMapping[hashNumber]; !ok {
			itemTypeMapping[hashNumber] = itemTypeIndex
			itemTypeIndex++
		}

		dst = append(dst, game_interface.ItemInfoWithSlot{
			Slot: resources_control.SlotID(item.Slot),
			ItemInfo: game_interface.ItemInfo{
				Count:    item.Item.ItemCount(),
				ItemType: itemTypeMapping[hashNumber],
			},
		})
	}

	// 进行物品状态转移
	success, err = api.ItemTransition().TransitionToContainer(src, dst)
	if err != nil {
		return fmt.Errorf("ItemTransition: %v", err)
	}
	if !success {
		return fmt.Errorf("ItemTransition: Failed to do transition")
	}

	return nil
}
