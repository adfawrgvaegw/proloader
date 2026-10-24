package base_container_cache

import (
	"fmt"

	"github.com/Yeah114/FunShuttler/nbt_assigner/block_helper"
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_console"
	"github.com/Yeah114/FunShuttler/utils"
)

// StoreCache 将操作台中心处的方块保存到当前缓存命中系统。
// StoreCache 将会假定操作台中心处的方块是一个空容器。
//
// customName 指示这个容器的自定义名称，通常情况下可以置空；
// shulkerFacing 指示潜影盒的朝向，如果该容器不是潜影盒，则
// 可以置为默认的零值
func (b *BaseContainerCache) StoreCache(customName string, shulkerFacing uint8) error {
	block := b.console.BlockByIndex(nbt_console.ConsoleIndexCenterBlock)
	container, ok := (*block).(block_helper.ContainerBlockHelper)
	if !ok {
		return fmt.Errorf("StoreCache: The center of the console is not a container; *block = %#v", *block)
	}

	c := BaseContainer{
		BlockName:         container.OpenInfo.Name,
		BlockStatesString: utils.MarshalBlockStates(container.OpenInfo.States),
		CustomeName:       customName,
		ShulkerFacing:     shulkerFacing,
	}
	hashNumber := c.Hash()

	if _, ok := b.cachedBaseContainer[hashNumber]; ok {
		return nil
	}

	uniqueID, err := b.console.API().StructureBackup().BackupStructure(
		b.console.Center(),
	)
	if err != nil {
		return fmt.Errorf("StoreCache: %v", err)
	}

	b.cachedBaseContainer[hashNumber] = StructureBaseContainer{
		UniqueID:  uniqueID,
		Container: container.OpenInfo,
	}
	return nil
}

// CleanCache 清除该缓存命中系统中已有的全部缓存
func (b *BaseContainerCache) CleanCache() {
	api := b.console.API().StructureBackup()

	for _, value := range b.cachedBaseContainer {
		_ = api.DeleteStructure(value.UniqueID)
	}

	b.cachedBaseContainer = make(map[uint64]StructureBaseContainer)
}
