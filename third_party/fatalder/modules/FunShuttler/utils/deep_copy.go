package utils

import (
	"github.com/Yeah114/FunShuttler/core/minecraft/nbt"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
)

// DeepCopyNBT 深拷贝 src 所指示的 NBT 数据，
// 并返回深拷贝产物 dst
func DeepCopyNBT(src map[string]any) (dst map[string]any) {
	srcBytes, _ := nbt.MarshalEncoding(src, nbt.LittleEndian)
	nbt.UnmarshalEncoding(srcBytes, &dst, nbt.LittleEndian)
	if dst == nil {
		dst = make(map[string]any)
	}
	return
}

// DeepCopyItemStack 深拷贝 src 所指示的物品堆栈数据，
// 并返回深拷贝产物 dst
func DeepCopyItemStack(src protocol.ItemStack) (dst protocol.ItemStack) {
	dst = protocol.ItemStack{
		ItemType: protocol.ItemType{
			NetworkID:     src.NetworkID,
			MetadataValue: src.MetadataValue,
		},
		BlockRuntimeID: src.BlockRuntimeID,
		Count:          src.Count,
		NBTData:        DeepCopyNBT(src.NBTData),
		CanBePlacedOn:  make([]string, len(src.CanBePlacedOn)),
		CanBreak:       make([]string, len(src.CanBreak)),
		HasNetworkID:   src.HasNetworkID,
	}

	copy(dst.CanBePlacedOn, src.CanBePlacedOn)
	copy(dst.CanBreak, src.CanBreak)

	return
}

// DeepCopyItemInstance 深拷贝 src 所指示的网络物品堆栈实例，
// 并返回深拷贝产物 dst
func DeepCopyItemInstance(src protocol.ItemInstance) (dst protocol.ItemInstance) {
	return protocol.ItemInstance{
		StackNetworkID: src.StackNetworkID,
		Stack:          DeepCopyItemStack(src.Stack),
	}
}
