package nbt_block_cache

import (
	"fmt"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	nbt_hash "github.com/Yeah114/FunShuttler/nbt_parser/hash"
	nbt_parser_interface "github.com/Yeah114/FunShuttler/nbt_parser/interface"
)

// StoreCache 将操作台中心处的 NBT 方块保存到当前的缓存命中系统。
// block 是操作台中心处的 NBT 方块数据；
// offset 是相对于这个 NBT 方块的偏移，例如床尾相对于床头的偏移
func (n *NBTBlockCache) StoreCache(block nbt_parser_interface.Block, offset protocol.BlockPos) error {
	var err error

	structure := StructureNBTBlock{
		HashNumber: nbt_hash.CompletelyHashNumber{
			HashNumber:    nbt_hash.NBTBlockFullHash(block),
			SetHashNumber: nbt_hash.ContainerSetHash(block),
		},
		Offset: offset,
		Block:  block,
	}

	_, ok := n.completelyCache[structure.HashNumber.HashNumber]
	if ok {
		return nil
	}

	structure.UniqueID, err = n.console.API().StructureBackup().BackupOffset(
		n.console.Center(),
		structure.Offset,
	)
	if err != nil {
		return fmt.Errorf("StoreCache: %v", err)
	}

	n.completelyCache[structure.HashNumber.HashNumber] = &structure
	if structure.HashNumber.SetHashNumber == nbt_hash.SetHashNumberNotExist {
		return nil
	}

	if _, ok := n.setHashCache[structure.HashNumber.SetHashNumber]; !ok {
		n.setHashCache[structure.HashNumber.SetHashNumber] = &structure
	}
	return nil
}

// CleanCache 清除该缓存命中系统中已有的全部缓存
func (n *NBTBlockCache) CleanCache() {
	api := n.console.API().StructureBackup()

	for _, value := range n.completelyCache {
		_ = api.DeleteStructure(value.UniqueID)
	}

	n.completelyCache = make(map[uint64]*StructureNBTBlock)
	n.setHashCache = make(map[uint64]*StructureNBTBlock)
}
