package nbt_assigner_utils

import (
	"fmt"
	"strings"

	"github.com/Yeah114/FunShuttler/game_control/game_interface"
	"github.com/Yeah114/FunShuttler/nbt_assigner/block_helper"
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_cache"
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_console"
	"github.com/Yeah114/FunShuttler/utils"
)

// EmptyBlockData 指示一个空的方块，
// 它可以是空的容器，如箱子或者酿造台。
//
// 当然，EmptyBlockData 也可以作用于
// 无法打开的容器，或者打开后不能操作
// 物品的容器。一些例子是讲台和唱片机
type EmptyBlockData struct {
	// 这个方块的名称
	Name string
	// 这个方块的方块状态
	States map[string]any

	// 这个方块是否是可以打开且
	// 打开后可以操作物品的容器
	IsCanOpenConatiner bool
	// 在满足 IsCanOpenConatiner 的前提下，
	// 在打开这个容器时，是否考虑其打开方向上
	// 是否存在障碍物。这似乎只对箱子和潜影盒有效
	ConsiderOpenDirection bool
	// ShulkerFacing 指示潜影盒的朝向。
	// 如果这不是一个潜影盒，则可以简单
	// 地置为默认的零只
	ShulkerFacing uint8

	// 这个方块的自定义名称
	BlockCustomName string
}

// SpawnNewEmptyBlock 在操作台中心生成 data 所指示的方块。
// 它可以是空的容器，如箱子或者酿造台。
// SpawnNewEmptyBlock 也可以被用于那些无法被打开的容器，
// 或者打开后不能操作物品的容器，如讲台和唱片机。
//
// 确保这个方块的自定义物品名称也被考虑在内。
//
// SpawnNewEmptyBlock 因为可能需要通过点击来完成方块的放置，
// 因此快捷栏的槽位可能会被重用。这意味着，调用者有责任确保快
// 捷栏的物品不会因此而被意外使用
func SpawnNewEmptyBlock(
	console *nbt_console.Console,
	cache *nbt_cache.NBTCacheSystem,
	data EmptyBlockData,
) error {
	// 准备
	api := console.API()
	useCommandToPlaceBlock := true

	// successFunc 指示所有操作成功后，
	// 应当执行的函数。它用于同步已放置
	// 方块的数据到底层操作台
	successFunc := func() {
		console.UseHelperBlock(nbt_console.RequesterUser, nbt_console.ConsoleIndexCenterBlock, block_helper.ContainerBlockHelper{
			OpenInfo: block_helper.ContainerBlockOpenInfo{
				Name:                  data.Name,
				States:                data.States,
				ConsiderOpenDirection: data.ConsiderOpenDirection,
				ShulkerFacing:         data.ShulkerFacing,
			},
		})
	}
	if !data.IsCanOpenConatiner {
		successFunc = func() {
			console.UseHelperBlock(nbt_console.RequesterUser, nbt_console.ConsoleIndexCenterBlock, block_helper.ComplexBlock{
				KnownStates: true,
				Name:        data.Name,
				States:      data.States,
			})
		}
	}

	// 如果这是一个可以打开且可以操作物品的容器，则尝试基容器缓存
	if data.IsCanOpenConatiner {
		hit, err := cache.BaseContainerCache().LoadCache(
			data.Name,
			data.States,
			data.BlockCustomName,
			data.ShulkerFacing,
		)
		if err != nil {
			return fmt.Errorf("SpawnNewEmptyBlock: %v", err)
		}
		if hit {
			return nil
		}
	}

	// 先将目标位置替换为空气
	err := api.SetBlock().SetBlock(console.Center(), "minecraft:air", "[]")
	if err != nil {
		return fmt.Errorf("SpawnNewEmptyBlock: %v", err)
	}
	console.UseHelperBlock(nbt_console.RequesterUser, nbt_console.ConsoleIndexCenterBlock, block_helper.Air{})

	// 检查是否需要复杂的工序来放置目标方块
	if len(data.BlockCustomName) > 0 {
		useCommandToPlaceBlock = false
	}
	if data.IsCanOpenConatiner && strings.Contains(data.Name, "shulker") && data.ShulkerFacing != 1 {
		useCommandToPlaceBlock = false
	}

	// 如果需要复杂的工序
	if !useCommandToPlaceBlock {
		// 先把目标物品获取到物品栏
		err := api.Replaceitem().ReplaceitemInInventory(
			"@s",
			game_interface.ReplacePathHotbarOnly,
			game_interface.ReplaceitemInfo{
				Name:     data.Name,
				Count:    1,
				MetaData: 0,
				Slot:     console.HotbarSlotID(),
			},
			"",
			true,
		)
		if err != nil {
			return fmt.Errorf("SpawnNewEmptyBlock: %v", err)
		}
		console.UseInventorySlot(nbt_console.RequesterUser, console.HotbarSlotID(), true)

		// 这个方块具有自定义的物品名称，需要进一步特殊处理
		if len(data.BlockCustomName) > 0 {
			index, err := console.FindOrGenerateNewAnvil()
			if err != nil {
				return fmt.Errorf("SpawnNewEmptyBlock: %v", err)
			}

			success, err := console.OpenContainerByIndex(index)
			if err != nil {
				return fmt.Errorf("SpawnNewEmptyBlock: %v", err)
			}
			if !success {
				return fmt.Errorf("SpawnNewEmptyBlock: Failed to open the anvil to rename container")
			}

			success, _, _, err = api.ItemStackOperation().OpenTransaction().
				RenameInventoryItem(console.HotbarSlotID(), data.BlockCustomName).
				Commit()
			if err != nil {
				_ = api.ContainerOpenAndClose().CloseContainer()
				return fmt.Errorf("SpawnNewEmptyBlock: %v", err)
			}
			if !success {
				_ = api.ContainerOpenAndClose().CloseContainer()
				return fmt.Errorf("SpawnNewEmptyBlock: The server rejected the container rename operation")
			}

			err = api.ContainerOpenAndClose().CloseContainer()
			if err != nil {
				return fmt.Errorf("SpawnNewEmptyBlock: %v", err)
			}
		}

		// 确定放置目标容器时所使用的朝向
		var facing uint8 = 1
		if data.IsCanOpenConatiner && strings.Contains(data.Name, "shulker") {
			facing = data.ShulkerFacing
		}

		// 移动机器人到操作台中心
		err = console.CanReachOrMove(console.Center())
		if err != nil {
			return fmt.Errorf("makeNormal: %v", err)
		}

		// 放置目标方块
		_, offsetPos, err := api.BotClick().PlaceBlockHighLevel(console.Center(), console.Position(), console.HotbarSlotID(), facing)
		if err != nil {
			return fmt.Errorf("makeNormal: %v", err)
		}
		console.UseHelperBlock(nbt_console.RequesterUser, nbt_console.ConsoleIndexCenterBlock, block_helper.ComplexBlock{
			KnownStates: false,
			Name:        data.Name,
		})
		*console.NearBlockByIndex(nbt_console.ConsoleIndexCenterBlock, offsetPos) = block_helper.NearBlock{
			Name: game_interface.BasePlaceBlock,
		}

		// 覆写容器的方块状态
		err = api.SetBlock().SetBlock(console.Center(), data.Name, utils.MarshalBlockStates(data.States))
		if err != nil {
			return fmt.Errorf("makeNormal: %v", err)
		}
		successFunc()

		// 如果这个容器可以打开且可以操作物品，则将其保存到基容器缓存命中系统
		if data.IsCanOpenConatiner {
			err = cache.BaseContainerCache().StoreCache(data.BlockCustomName, data.ShulkerFacing)
			if err != nil {
				return fmt.Errorf("makeNormal: %v", err)
			}
		}

		// 返回值
		return nil
	}

	// 目标方块可以直接通过简单的 setblock 放置
	err = api.SetBlock().SetBlock(console.Center(), data.Name, utils.MarshalBlockStates(data.States))
	if err != nil {
		return fmt.Errorf("SpawnNewEmptyBlock: %v", err)
	}
	successFunc()

	return nil
}
