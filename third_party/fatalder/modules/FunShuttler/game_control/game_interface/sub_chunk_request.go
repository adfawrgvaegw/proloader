package game_interface

import (
	"fmt"
	"time"

	"github.com/TriM-Organization/bedrock-world-operator/define"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
)

// SubChunkRequest 是基于 ResourcesWrapper 实现的子区块请求器
type SubChunkRequest struct {
	api *ResourcesWrapper
}

// NewSubChunkRequest 基于 api 创建并返回一个新的 SubChunkRequest
func NewSubChunkRequest(api *ResourcesWrapper) *SubChunkRequest {
	return &SubChunkRequest{api: api}
}

// SendSubChunkRequestWithResp 是用于
// 请求 request 代表的子区块请求并获取与之对应的响应体
func (s *SubChunkRequest) SendSubChunkRequestWithResp(request *packet.SubChunkRequest) (
	resp *packet.SubChunk,
	err error,
) {
	var isTimeout bool
	resp, isTimeout, err = s.SendSubChunkRequestWithRespTimeout(request, 0)
	if err != nil {
		return nil, fmt.Errorf("SendSubChunkRequestWithResp: %v", err)
	}
	if isTimeout {
		return nil, fmt.Errorf("SendSubChunkRequestWithResp: request timed out")
	}
	return resp, nil
}

// SendSubChunkRequestWithRespTimeout 请求子区块并在超时前等待响应
func (s *SubChunkRequest) SendSubChunkRequestWithRespTimeout(
	request *packet.SubChunkRequest,
	timeout time.Duration,
) (
	resp *packet.SubChunk,
	isTimeout bool,
	err error,
) {
	api := s.api
	responseChan := make(chan *packet.SubChunk, 1)

	api.Resources.SubChunkRequest().SetSubChunkRequestCallback(
		request,
		func(p *packet.SubChunk) {
			select {
			case responseChan <- p:
			default:
			}
		},
	)
	if err = api.WritePacket(request); err != nil {
		api.Resources.SubChunkRequest().DeleteSubChunkRequestCallback(request.Position)
		return nil, false, fmt.Errorf("write packet: %w", err)
	}

	var (
		timer     *time.Timer
		timeoutCh <-chan time.Time
	)
	if timeout > 0 {
		timer = time.NewTimer(timeout)
		timeoutCh = timer.C
	}

	select {
	case resp = <-responseChan:
		if timer != nil && !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		return resp, false, nil
	case <-timeoutCh:
		api.Resources.SubChunkRequest().DeleteSubChunkRequestCallback(request.Position)
		return nil, true, nil
	}
}

// MakeSubChunkRequestByArea 制作一个两点坐标之间的获取所有子区块请求
func (s *SubChunkRequest) MakeSubChunkRequestByArea(dimension define.Dimension, start, end protocol.BlockPos) (request *packet.SubChunkRequest) {
	// 确定Y坐标范围
	yRange := dimension.Range()

	// 将方块坐标转换为区块坐标
	minChunkX := min(start[0], end[0]) >> 4
	maxChunkX := max(start[0], end[0]) >> 4
	minChunkZ := min(start[2], end[2]) >> 4
	maxChunkZ := max(start[2], end[2]) >> 4

	// 计算子区块Y范围
	minSubY := max(int8(start[1]>>4), int8(yRange[0]/16))
	maxSubY := min(int8(end[1]>>4), int8(yRange[1]/16))

	// 选择中心区块（通常是起始区块）
	centerChunkX := minChunkX
	centerChunkZ := minChunkZ

	// 准备所有偏移量
	var offsets []protocol.SubChunkOffset

	// 遍历所有区块和子区块
	for x := minChunkX; x <= maxChunkX; x++ {
		for z := minChunkZ; z <= maxChunkZ; z++ {
			// 计算相对于中心区块的偏移
			offsetX := int8(x - centerChunkX)
			offsetZ := int8(z - centerChunkZ)

			for y := minSubY; y <= maxSubY; y++ {
				offsets = append(offsets, protocol.SubChunkOffset{offsetX, y, offsetZ})
			}
		}
	}

	// 创建子区块请求
	request = &packet.SubChunkRequest{
		Dimension: int32(dimension),
		Position:  protocol.SubChunkPos{centerChunkX, 0, centerChunkZ},
		Offsets:   offsets,
	}
	return
}

// MakeSubChunkRequestBySubChunkPos 制作一个基于子区块绝对坐标获取所有子区块请求
func (s *SubChunkRequest) MakeSubChunkRequestBySubChunkPos(dimension define.Dimension, subChunksPos []protocol.SubChunkPos) (request *packet.SubChunkRequest) {
	// 初始化偏移量切片
	offsets := make([]protocol.SubChunkOffset, 0)

	// 确定中心区块（取第一个子区块坐标作为基准）
	centerChunkX := subChunksPos[0].X()
	centerChunkZ := subChunksPos[0].Z()

	// 遍历所有子区块坐标
	for _, pos := range subChunksPos {
		// 计算相对于中心区块的偏移量
		offsetX := int8(pos.X() - centerChunkX)
		offsetZ := int8(pos.Z() - centerChunkZ)
		offsetY := int8(pos.Y()) // 子区块Y坐标直接作为偏移量

		// 添加到偏移量列表
		offsets = append(offsets, protocol.SubChunkOffset{offsetX, offsetY, offsetZ})
	}

	// 创建子区块请求
	request = &packet.SubChunkRequest{
		Dimension: int32(dimension),
		Position:  protocol.SubChunkPos{centerChunkX, 0, centerChunkZ},
		Offsets:   offsets,
	}
	return
}

// GetSubChunksInArea 获取两点坐标之间的所有子区块
func (s *SubChunkRequest) GetSubChunksInArea(dimension define.Dimension, start, end protocol.BlockPos) (*packet.SubChunk, error) {
	// 发送请求并获取响应
	resp, err := s.SendSubChunkRequestWithResp(s.MakeSubChunkRequestByArea(
		dimension,
		start,
		end,
	))
	if err != nil {
		return nil, fmt.Errorf("GetSubChunksInArea: %v", err)
	}

	return resp, nil
}

// GetSubChunksInChunk 获取指定区块内的所有子区块
func (s *SubChunkRequest) GetSubChunksInChunk(dimension define.Dimension, position protocol.ChunkPos) (*packet.SubChunk, error) {
	// 确定Y坐标范围
	yRange := dimension.Range()

	// 将区块坐标转换为方块坐标范围
	blockX := position[0] << 4
	blockZ := position[1] << 4

	// 创建起点和终点坐标
	start := protocol.BlockPos{blockX, int32(yRange[0]), blockZ}
	end := protocol.BlockPos{blockX + 15, int32(yRange[1]), blockZ + 15} // 区块包含16x16方块

	// 使用区域请求函数获取整个区块的子区块
	return s.GetSubChunksInArea(dimension, start, end)
}

// SetChunkRadius 是用于设置区块可视半径
func (s *SubChunkRequest) SetChunkRadius(chunkRadius int32) (
	resp int32,
	err error,
) {
	api := s.api
	channel := make(chan struct{})

	api.Resources.SubChunkRequest().SetRequestChunkRadiusCallback(
		func(c int32) {
			resp = c
			close(channel)
		},
	)
	request := &packet.RequestChunkRadius{
		ChunkRadius:    chunkRadius,
		MaxChunkRadius: chunkRadius,
	}
	err = api.WritePacket(request)
	if err != nil {
		return -1, fmt.Errorf("SetChunkRadius: %v", err)
	}

	<-channel
	return resp, nil
}
