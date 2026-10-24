package resources_control

import (
	"github.com/Yeah114/FunShuttler/client"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
)

// BotInfo 记载机器人的基本信息
type BotInfo struct {
	BotName         string // 机器人名称
	XUID            string // 机器人 XUID
	EntityUniqueID  int64  // 机器人唯一 ID
	EntityRuntimeID uint64 // 机器人运行时 ID
}

type Resources struct {
	// client 是连接到租赁服的基本客户端
	client *client.Client
	// commands 存放所有命令请求的回调函数
	commands *CommandRequestCallback
	// inventory 持有机器人已经拥有或打开的库存
	inventory *Inventories
	// itemStack 管理物品堆栈操作请求
	itemStack *ItemStackOperationManager
	// container 维护机器人的容器资源，
	// 处理其占用和释放，以及一些持久化数据
	container *ContainerManager
	// listener 是一个可撤销的简单数据包监听器实现
	listener *PacketListener
	// constant 是常量数据包的简要记录实现
	constant *ConstantPacket
	// interceptor 是一个可撤销的拦截器实现
	interceptor *PacketInterceptor
	// uqholder 是简单的 MicroUQHolder 维护器
	uqholder *UQHolder
	// structureRequest 存放所有结构请求的回调函数
	structureRequest *StructureRequestCallback
	// subChunkRequest 存放所有子区块请求的回调函数
	subChunkRequest *SubChunkRequestCallback

	allPacket chan packet.Packet
}

// NewResourcesControl 基于 client 创建一个新的资源中心。
// 它应当在机器人连接到租赁服后立即被调用，且最多调用一次。
//
// 需要注意的是，client.Conn().ReadPacket 不应继续被使用，
// 否则可能会出现未知的竞态条件问题，因为资源管理器本身也会
// 不断的读取数据包并依此更新其自身的资源数据
func NewResourcesControl(client *client.Client) *Resources {
	clientCtx := client.Conn().Context()
	resourcesControl := &Resources{
		client:           client,
		commands:         NewCommandRequestCallback(clientCtx),
		itemStack:        NewItemStackOperationManager(clientCtx),
		container:        NewContainerManager(clientCtx),
		listener:         NewPacketListener(clientCtx),
		interceptor:      NewPacketInterceptor(),
		uqholder:         NewUQHolder(client.Conn()),
		structureRequest: NewStructureRequestCallback(),
		subChunkRequest:  NewSubChunkRequestCallback(),
		allPacket:        make(chan packet.Packet, 32767),
	}

	inventory := NewInventories()
	inventory.createInventory(protocol.WindowIDCrafting)
	resourcesControl.inventory = inventory

	constantPacket := NewConstantPacket()
	constantPacket.updateByGameData(client.Conn().GameData())
	resourcesControl.constant = constantPacket

	for {
		pk := <-resourcesControl.client.CachedPacket()
		if pk == nil {
			break
		}
		resourcesControl.handlePacket(pk)
	}
	go resourcesControl.listenPacket()

	return resourcesControl
}

func (r *Resources) AllPacket() chan packet.Packet {
	return r.allPacket
}

// listenPacket ..
func (r *Resources) listenPacket() {
	for {
		pk, err := r.client.Conn().ReadPacket()
		if err != nil {
			r.handleConnClose(err)
			return
		}
		r.handlePacket(pk)
	}
}

// BotInfo 返回机器人的基本信息
func (r *Resources) BotInfo() BotInfo {
	return BotInfo{
		BotName:         r.client.Conn().IdentityData().DisplayName,
		XUID:            r.client.Conn().IdentityData().XUID,
		EntityUniqueID:  r.client.Conn().GameData().EntityUniqueID,
		EntityRuntimeID: r.client.Conn().GameData().EntityRuntimeID,
	}
}

// WritePacket 用于向租赁服发送数据包 p
func (r *Resources) WritePacket(p packet.Packet) error {
	return r.client.Conn().WritePacket(p)
}

// Commands 返回命令请求的相关资源
func (r *Resources) Commands() *CommandRequestCallback {
	return r.commands
}

// Inventories 返回库存的相关资源
func (r *Resources) Inventories() *Inventories {
	return r.inventory
}

// ItemStackOperation 返回物品堆栈操作请求的相关资源
func (r *Resources) ItemStackOperation() *ItemStackOperationManager {
	return r.itemStack
}

// Container 返回容器的相关资源
func (r *Resources) Container() *ContainerManager {
	return r.container
}

// PacketListener 返回数据包监听的有关实现
func (r *Resources) PacketListener() *PacketListener {
	return r.listener
}

// ConstantPacket 返回常量数据包的有关实现
func (r *Resources) ConstantPacket() *ConstantPacket {
	return r.constant
}

// PacketInterceptor 返回数据包拦截器的有关实现
func (r *Resources) PacketInterceptor() *PacketInterceptor {
	return r.interceptor
}

// UQHolder 返回 MicroUQHolder 的有关实现
func (r *Resources) UQHolder() *UQHolder {
	return r.uqholder
}

// StructureRequest 返回结构请求的相关资源
func (r *Resources) StructureRequest() *StructureRequestCallback {
	return r.structureRequest
}

// SubChunkRequest 返回子区块请求的相关资源
func (r *Resources) SubChunkRequest() *SubChunkRequestCallback {
	return r.subChunkRequest
}

// Client 返回底层的客户端实现
func (r *Resources) Client() *client.Client {
	return r.client
}

// HandlePacket 对上层暴露处理数据包的能力，主要用于历史兼容
func (r *Resources) HandlePacket(pkt *packet.Packet) bool {
	if pkt == nil {
		return false
	}
	if !r.interceptor.onPacket(pkt) {
		return false
	}
	r.processPacket(*pkt)
	return true
}
