package resources_control

import (
	"encoding/json"
	"sync"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/utils"
)

// SubChunkRequestCallback 处理区块请求的回调
type SubChunkRequestCallback struct {
	callback    utils.SyncMap[string, func(p *packet.SubChunk)] // 子区块请求回调
	chunkRadius int32                                           // 当前区块半径

	radiusCallbackMutex sync.Mutex  // 保护区块半径回调的互斥锁
	radiusCallback      func(int32) // 区块半径请求回调
}

// NewSubChunkRequestCallback 创建并返回一个新的 SubChunkRequestCallback
func NewSubChunkRequestCallback() *SubChunkRequestCallback {
	return &SubChunkRequestCallback{}
}

// SetRequestChunkRadiusCallback 设置区块半径请求回调
// 同一时间只允许一个有效的回调，新的回调会覆盖旧的回调
func (c *SubChunkRequestCallback) SetRequestChunkRadiusCallback(f func(radius int32)) {
	c.radiusCallbackMutex.Lock()
	defer c.radiusCallbackMutex.Unlock()

	// 清除之前的回调（如果有）
	c.radiusCallback = f
}

// ClearRequestChunkRadiusCallback 清除区块半径请求回调
func (c *SubChunkRequestCallback) ClearRequestChunkRadiusCallback() {
	c.radiusCallbackMutex.Lock()
	defer c.radiusCallbackMutex.Unlock()
	c.radiusCallback = nil
}

// SetSubChunkRequestCallback 设置子区块请求回调
func (c *SubChunkRequestCallback) SetSubChunkRequestCallback(
	request *packet.SubChunkRequest,
	f func(p *packet.SubChunk),
) {
	positionBytes, err := json.Marshal(request.Position)
	if err != nil {
		return
	}
	position := string(positionBytes)
	c.callback.Store(position, f)
}

// DeleteSubChunkRequestCallback 删除子区块请求回调
func (c *SubChunkRequestCallback) DeleteSubChunkRequestCallback(Position protocol.SubChunkPos) {
	positionBytes, err := json.Marshal(Position)
	if err != nil {
		return
	}
	position := string(positionBytes)
	c.callback.Delete(position)
}

// GetChunkRadius 获取当前区块半径
func (c *SubChunkRequestCallback) GetChunkRadius() int32 {
	return c.chunkRadius
}

// setChunkRadius 设置区块半径（内部使用）
func (c *SubChunkRequestCallback) setChunkRadius(chunkRadius int32) {
	c.chunkRadius = chunkRadius
}

// onSubChunk 处理子区块响应
func (c *SubChunkRequestCallback) onSubChunk(p *packet.SubChunk) {
	positionBytes, err := json.Marshal(p.Position)
	if err != nil {
		return
	}
	position := string(positionBytes)
	cb, ok := c.callback.LoadAndDelete(position)
	if ok {
		go cb(p)
	}
}

// onChunkRadiusUpdated 处理区块半径更新
func (c *SubChunkRequestCallback) onChunkRadiusUpdated(p *packet.ChunkRadiusUpdated) {
	// 更新区块半径
	c.setChunkRadius(p.ChunkRadius)

	// 触发并清除回调
	c.radiusCallbackMutex.Lock()
	defer c.radiusCallbackMutex.Unlock()

	if c.radiusCallback != nil {
		go c.radiusCallback(p.ChunkRadius)
		c.radiusCallback = nil // 执行后立即清除
	}
}
