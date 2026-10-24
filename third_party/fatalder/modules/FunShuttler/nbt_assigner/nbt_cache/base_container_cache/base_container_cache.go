package base_container_cache

import (
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_console"

	"github.com/google/uuid"
)

// BaseContainerCache 是基于操作台实现的基容器缓存命中系统
type BaseContainerCache struct {
	// uniqueID 是当前缓存命中系统的唯一标识
	uniqueID string
	// console 是机器人使用的操作台
	console *nbt_console.Console
	// cachedBaseContainer 记载了已缓存的所有基容器
	cachedBaseContainer map[uint64]StructureBaseContainer
}

// NewBaseContainerCache 基于操作台 console 创建并返回一个新的基容器缓存命中系统
func NewBaseContainerCache(console *nbt_console.Console) *BaseContainerCache {
	return &BaseContainerCache{
		uniqueID:            uuid.NewString(),
		console:             console,
		cachedBaseContainer: make(map[uint64]StructureBaseContainer),
	}
}
