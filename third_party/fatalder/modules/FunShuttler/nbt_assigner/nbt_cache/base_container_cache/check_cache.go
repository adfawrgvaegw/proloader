package base_container_cache

import (
	"github.com/Yeah114/FunShuttler/utils"
)

// CheckCache 检索整个缓存命中系统，查询名称为
// name 且方块状态为 states 的基容器是否存在
func (b *BaseContainerCache) CheckCache(name string, states map[string]any) (hit bool) {
	container := BaseContainer{
		BlockName:         name,
		BlockStatesString: utils.MarshalBlockStates(states),
	}
	hashNumber := container.Hash()
	_, hit = b.cachedBaseContainer[hashNumber]
	return
}
