package base_container_cache

import (
	"github.com/Yeah114/FunShuttler/nbt_assigner/block_helper"
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_console"
	"github.com/Yeah114/FunShuttler/utils"
)

// LoadCache 加载名为 name 且方块状态为 states 的基容器。
//
// customName 指示基容器的自定义名称，它通常情况下为空。
// shulkerFacing 指示潜影盒的朝向。如果该容器不是潜影盒，
// 则可以置为默认的零值。
//
// 如果目标基容器没有找到，则尝试从已保存的结构中加载。
// 应当说明的是，基容器会在操作台的中心处被加载
func (b *BaseContainerCache) LoadCache(
	name string,
	states map[string]any,
	customName string,
	shulkerFacing uint8,
) (hit bool, err error) {
	// Compute hash number
	container := BaseContainer{
		BlockName:         name,
		BlockStatesString: utils.MarshalBlockStates(states),
		CustomeName:       customName,
		ShulkerFacing:     shulkerFacing,
	}
	hashNumber := container.Hash()

	// Try to load from internal structure record mapping
	structure, ok := b.cachedBaseContainer[hashNumber]
	if !ok {
		return false, nil
	}

	// Load structure
	err = b.console.API().StructureBackup().RevertStructure(
		structure.UniqueID,
		b.console.Center(),
	)
	if err != nil {
		return false, nil
	}

	// Update underlying container data
	newContainer := block_helper.ContainerBlockHelper{
		OpenInfo: structure.Container,
	}
	b.console.UseHelperBlock(b.uniqueID, nbt_console.ConsoleIndexCenterBlock, newContainer)

	// Return
	return true, nil
}
