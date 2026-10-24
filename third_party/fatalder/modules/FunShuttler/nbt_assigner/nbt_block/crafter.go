package nbt_block

import (
	"fmt"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_cache"
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_console"
	nbt_assigner_utils "github.com/Yeah114/FunShuttler/nbt_assigner/utils"
	nbt_parser_block "github.com/Yeah114/FunShuttler/nbt_parser/block"
)

// 合成器
type Crafter struct {
	console *nbt_console.Console
	cache   *nbt_cache.NBTCacheSystem
	data    nbt_parser_block.Crafter
}

func (Crafter) Offset() protocol.BlockPos {
	return protocol.BlockPos{0, 0, 0}
}

func (c *Crafter) Make() error {
	api := c.console.API()
	center := c.console.Center()

	// 处理容器内的物品。如果无需处理，则说明这是一个空的合成台，
	// 但内部禁用了一些物品栏，于是我们需要手动地生成新的合成台
	if c.data.AsContainer().NeedSpecialHandle() {
		container := Container{
			console: c.console,
			cache:   c.cache,
			data:    *c.data.AsContainer(),
		}
		err := container.Make()
		if err != nil {
			return fmt.Errorf("Make: %v", err)
		}
	} else {
		err := nbt_assigner_utils.SpawnNewEmptyBlock(
			c.console,
			c.cache,
			nbt_assigner_utils.EmptyBlockData{
				Name:                  c.data.BlockName(),
				States:                c.data.BlockStates(),
				IsCanOpenConatiner:    true,
				ConsiderOpenDirection: false,
				BlockCustomName:       c.data.NBT.ContainerInfo.CustomName,
			},
		)
		if err != nil {
			return fmt.Errorf("Make: %v", err)
		}
	}

	// 如果这个合成台没有被禁用的物品栏，
	// 则可以直接返回值
	if c.data.NBT.DisabledSlots == 0 {
		return nil
	}

	// 打开合成器
	success, err := c.console.OpenContainerByIndex(nbt_console.ConsoleIndexCenterBlock)
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}
	if !success {
		return fmt.Errorf("Make: Failed to open the crafter when set its disabled slot")
	}
	defer api.ContainerOpenAndClose().CloseContainer()

	// 否则，开始设置被禁用的物品栏
	for index := range 9 {
		if c.data.NBT.DisabledSlots&int16(1<<index) == 0 {
			continue
		}
		err := api.Resources().WritePacket(&packet.PlayerToggleCrafterSlotRequest{
			PosX:     center[0],
			PosY:     center[1],
			PosZ:     center[2],
			Slot:     byte(index),
			Disabled: true,
		})
		if err != nil {
			return fmt.Errorf("Make: %v", err)
		}
	}

	// 等待更改
	err = api.Commands().AwaitChangesGeneral()
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}

	return nil
}
