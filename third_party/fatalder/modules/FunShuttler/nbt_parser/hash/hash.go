package nbt_hash

import (
	nbt_parser_block "github.com/Yeah114/FunShuttler/nbt_parser/block"
	nbt_parser_interface "github.com/Yeah114/FunShuttler/nbt_parser/interface"

	"github.com/cespare/xxhash/v2"
)

// NBTBlockNBTHash 计算 block 的 NBT 字段的哈希校验和，
// 这意味着校验和的范围不会包含这个 NBT 方块的名称和方块状态。
//
// 如果这个方块不存在特定的 NBT 字段，则该方块不存在 NBT 字段
// 的哈希校验和。这意味着调用该函数后将返回 NBTHashNumberNotExist (0)
func NBTBlockNBTHash(block nbt_parser_interface.Block) uint64 {
	result := block.NBTStableBytes()
	if len(result) == 0 {
		return NBTHashNumberNotExist
	}
	return xxhash.Sum64(block.NBTStableBytes())
}

// NBTBlockFullHash 计算 block 的哈希校验和
func NBTBlockFullHash(block nbt_parser_interface.Block) uint64 {
	return xxhash.Sum64(block.FullStableBytes())
}

// NBTItemNBTHash 计算 item 的 NBT 哈希校验和。
// 它校验的范围不包含物品的自定义名称和附魔数据，
// 但包括物品的名称、物品的元数据、物品组件数据和
// 这个物品的一些特定 NBT 字段
func NBTItemNBTHash(item nbt_parser_interface.Item) uint64 {
	return xxhash.Sum64(item.NBTStableBytes())
}

// NBTItemTypeHash 计算 item 的种类哈希校验和。
// 这意味着，对于两种相同的物品，它们具有相同的种类哈希校验和
func NBTItemTypeHash(item nbt_parser_interface.Item) uint64 {
	return xxhash.Sum64(item.TypeStableBytes())
}

// NBTItemFullHash 计算 item 的哈希校验和
func NBTItemFullHash(item nbt_parser_interface.Item) uint64 {
	return xxhash.Sum64(item.FullStableBytes())
}

// ContainerSetHash 计算 block 的集合哈希校验和。
// ContainerSetHash 假设给定的 block 可以断言为容器。
//
// 如果提供的 block 不是容器，或容器为空，
// 则返回 SetHashNumberNotExist (0)。
// 否则，返回这个容器的集合哈希校验和。
//
// 通常地，如果两个容器装有相同种类的物品，
// 且每个种类的物品数量相等，
// 则两个容器的集合哈希校验和相等
func ContainerSetHash(block nbt_parser_interface.Block) uint64 {
	container, ok := block.(*nbt_parser_block.Container)
	if !ok {
		return SetHashNumberNotExist
	}

	setBytes := container.SetBytes()
	if len(setBytes) == 0 {
		return SetHashNumberNotExist
	}

	return xxhash.Sum64(setBytes)
}
