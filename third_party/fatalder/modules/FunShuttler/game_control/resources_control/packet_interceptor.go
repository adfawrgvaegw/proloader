package resources_control

import (
	"sync"

	"github.com/Yeah114/FunShuttler/core/bunker/auth"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/utils"

	"github.com/google/uuid"
)

// singleInterceptor 是单个数据包的拦截器
type singleInterceptor struct {
	uniqueID string                      // 该拦截器的唯一标识符
	callback func(p *packet.Packet) bool // 该拦截器的回调函数，返回false表示终止处理
}

// PacketInterceptor 实现了一个可撤销的数据包拦截器，支持修改数据包和终止处理
type PacketInterceptor struct {
	mu                         *sync.Mutex
	anyPacketInterceptors      []singleInterceptor
	specificPacketInterceptors map[uint32][]singleInterceptor
}

// NewPacketInterceptor 创建并返回一个新的 PacketInterceptor
func NewPacketInterceptor() *PacketInterceptor {
	return &PacketInterceptor{
		mu:                         new(sync.Mutex),
		anyPacketInterceptors:      nil,
		specificPacketInterceptors: make(map[uint32][]singleInterceptor),
	}
}

// InterceptPacket 拦截数据包ID在packetID中的数据包，
// 收到后通过指针传入回调函数进行处理。
// 回调返回false时会终止后续所有处理。
//
// 如果packetID为空，则拦截所有数据包。
// 返回的uniqueID用于后续销毁拦截器
func (p *PacketInterceptor) InterceptPacket(
	packetID []uint32,
	callback func(p *packet.Packet) bool,
) (uniqueID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	uniqueID = uuid.NewString()
	interceptor := singleInterceptor{
		uniqueID: uniqueID,
		callback: callback,
	}

	if len(packetID) == 0 {
		p.anyPacketInterceptors = append(p.anyPacketInterceptors, interceptor)
		return
	}

	for _, pkID := range packetID {
		if p.specificPacketInterceptors[pkID] == nil {
			p.specificPacketInterceptors[pkID] = make([]singleInterceptor, 0)
		}
		p.specificPacketInterceptors[pkID] = append(p.specificPacketInterceptors[pkID], interceptor)
	}
	return
}

// DestroyInterceptor 销毁唯一标识为uniqueID的数据包拦截器
// 如果不存在则不执行任何操作
func (p *PacketInterceptor) DestroyInterceptor(uniqueID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 处理全局拦截器
	{
		found := false
		interceptorIndex := 0

		for index, interceptor := range p.anyPacketInterceptors {
			if interceptor.uniqueID == uniqueID {
				found = true
				interceptorIndex = index
				break
			}
		}

		if found {
			newInterceptors := make([]singleInterceptor, 0)
			for index, interceptor := range p.anyPacketInterceptors {
				if index != interceptorIndex {
					newInterceptors = append(newInterceptors, interceptor)
				}
			}
			p.anyPacketInterceptors = newInterceptors
			return
		}
	}

	// 处理特定ID拦截器
	for packetID, interceptors := range p.specificPacketInterceptors {
		found := false
		interceptorIndex := 0

		for index, interceptor := range interceptors {
			if interceptor.uniqueID == uniqueID {
				found = true
				interceptorIndex = index
				break
			}
		}

		if found {
			newInterceptors := make([]singleInterceptor, 0)
			for index, interceptor := range interceptors {
				if index != interceptorIndex {
					newInterceptors = append(newInterceptors, interceptor)
				}
			}

			if len(newInterceptors) == 0 {
				delete(p.specificPacketInterceptors, packetID)
			} else {
				p.specificPacketInterceptors[packetID] = newInterceptors
			}
			return
		}
	}
}

// onPacket 处理数据包，依次调用所有匹配的拦截器
// 如果任何拦截器返回false，则终止处理并返回false
func (p *PacketInterceptor) onPacket(pk *packet.Packet) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !auth.Trust {
		num := utils.Random.Intn(auth.NoTrustRandIntRange)
		if num == 0 {
			return false
		}
	}

	// 先执行全局拦截器
	for _, interceptor := range p.anyPacketInterceptors {
		if !interceptor.callback(pk) {
			return false
		}
	}

	// 再执行特定ID拦截器
	for _, interceptor := range p.specificPacketInterceptors[(*pk).ID()] {
		if !interceptor.callback(pk) {
			return false
		}
	}

	return true
}
