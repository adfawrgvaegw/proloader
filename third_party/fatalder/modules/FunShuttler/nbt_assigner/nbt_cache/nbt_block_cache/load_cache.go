package nbt_block_cache

import (
	"fmt"

	"github.com/Yeah114/FunShuttler/nbt_assigner/block_helper"
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_console"
	nbt_parser_block "github.com/Yeah114/FunShuttler/nbt_parser/block"
	nbt_hash "github.com/Yeah114/FunShuttler/nbt_parser/hash"
)

// LoadCache 尝试加载一个已缓存的 NBT 方块到操作台中心。
// 如果 hashNumber 所指示的缓存不存在，则不执行任何操作。
// 返回的 structure 指示命中的结果；
// 返回的 isSetHashHit 指示命中的缓存是否来自集合哈希校验和
func (n *NBTBlockCache) LoadCache(hashNumber nbt_hash.CompletelyHashNumber) (
	structure StructureNBTBlock,
	hit bool,
	isSetHashHit bool,
	err error,
) {
	structure, hit, isSetHashHit = n.CheckCache(hashNumber)
	if !hit && !isSetHashHit {
		return StructureNBTBlock{}, false, false, nil
	}

	err = n.console.API().StructureBackup().RevertStructure(
		structure.UniqueID,
		n.console.Center(),
	)
	if err != nil {
		return StructureNBTBlock{}, false, false, fmt.Errorf("LoadCache: %v", err)
	}

	if structure.Offset != [3]int32{0, 0, 0} {
		*n.console.NearBlockByIndex(nbt_console.ConsoleIndexCenterBlock, structure.Offset) = block_helper.ComplexBlock{
			KnownStates: true,
			Name:        structure.Block.BlockName(),
			States:      structure.Block.BlockStates(),
		}
	}

	container, ok := structure.Block.(*nbt_parser_block.Container)
	if ok {
		n.console.UseHelperBlock(
			n.uniqueID,
			nbt_console.ConsoleIndexCenterBlock,
			block_helper.ContainerBlockHelper{
				OpenInfo: block_helper.ContainerBlockOpenInfo{
					Name:                  container.BlockName(),
					States:                container.BlockStates(),
					ConsiderOpenDirection: container.ConsiderOpenDirection(),
					ShulkerFacing:         container.NBT.ShulkerFacing,
				},
			},
		)
		return structure, hit, isSetHashHit, nil
	}

	n.console.UseHelperBlock(
		n.uniqueID,
		nbt_console.ConsoleIndexCenterBlock,
		block_helper.ComplexBlock{
			KnownStates: true,
			Name:        structure.Block.BlockName(),
			States:      structure.Block.BlockStates(),
		},
	)
	return structure, hit, isSetHashHit, nil
}
