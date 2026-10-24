package nbt_block_cache

import (
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_console"

	"github.com/google/uuid"
)

// NBTBlockCache 是基于操作台实现的 NBT 方块缓存命中系统
type NBTBlockCache struct {
	// uniqueID 是当前缓存命中系统的唯一标识
	uniqueID string
	// console 是机器人使用的操作台
	console *nbt_console.Console
	// completelyCache 记载了已缓存的所有 NBT 方块，
	// 它指示 NBT 方块的完整哈希校验和到缓存数据结构
	// 的映射
	completelyCache map[uint64]*StructureNBTBlock
	// setHashCache 与 completelyCache 的不同之处在
	// 于 setHashCache 使用的是 NBT 方块的集合哈希校
	// 验和到缓存数据结构的映射。目前只有容器使用它
	setHashCache map[uint64]*StructureNBTBlock
}

// NewNBTBlockCache 基于操作台 console 创建并返回一个新的 NBT 方块缓存命中系统
func NewNBTBlockCache(console *nbt_console.Console) *NBTBlockCache {
	return &NBTBlockCache{
		uniqueID:        uuid.NewString(),
		console:         console,
		completelyCache: make(map[uint64]*StructureNBTBlock),
		setHashCache:    make(map[uint64]*StructureNBTBlock),
	}
}
