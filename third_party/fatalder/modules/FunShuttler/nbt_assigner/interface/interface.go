package nbt_assigner_interface

import (
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_cache"
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_console"
	nbt_parser_interface "github.com/Yeah114/FunShuttler/nbt_parser/interface"

	"github.com/google/uuid"
)

var (
	// NBTItemIsSupported 检查 item 是否是受支持的复杂物品
	NBTItemIsSupported func(item nbt_parser_interface.Item) bool
	// MakeNBTItemMethod 根据传入的操作台、缓存命中系统和多个物品，
	// 将它们归类为每种复杂物品。对于 result 中的每个元素，可以使用
	// Make 制作它们
	MakeNBTItemMethod func(console *nbt_console.Console, cache *nbt_cache.NBTCacheSystem, multipleItems ...nbt_parser_interface.Item) []Item
	// EnchMultiple 根据操作台 console 和已放入背包的多个物品 multipleItems，
	// 将它们进行一一附魔处理。应当说明的是，这些物品应当置于非快捷栏的物品栏，
	// 并且对于无需处理的物品，应当简单的置为 nil
	EnchMultiple func(console *nbt_console.Console, multipleItems [27]*nbt_parser_interface.Item) error
	// RenameMultiple 根据操作台 console 和已放入背包的多个物品 multipleItems，
	// 将它们进行集中性物品改名处理。应当说明的是，这些物品应当置于非快捷栏的物品栏，
	// 并且对于无需处理的物品，应当简单的置为 nil
	RenameMultiple func(console *nbt_console.Console, multipleItems [27]*nbt_parser_interface.Item) error
	// EnchAndRenameMultiple 根据操作台 console 和已放入背包的多个物品 multipleItems，
	// 将它们进行集中性的物品附魔和物品改名处理。应当说明的是，这些物品应当置于非快捷栏的物品栏，
	// 并且对于无需处理的物品，应当简单的置为 nil
	EnchAndRenameMultiple func(console *nbt_console.Console, multipleItems [27]*nbt_parser_interface.Item) error
)

var (
	// NBTBlockIsSupported 检查 block 是否是受支持的 NBT 方块
	NBTBlockIsSupported func(block nbt_parser_interface.Block) bool
	// PlaceNBTBlock 根据传入的操作台和缓存命中系统，
	// 在操作台的中心方块处制作一个 NBT 方块 nbtBlock。
	//
	// canFast 指示目标方块是否可以直接通过 setblock 放置。
	//
	// 如果不能通过 setblock 放置，那么 uniqueID 指示目标
	// 方块所在结构的唯一标识，并且 offset 指示其相邻的可能
	// 的方块，例如床的尾方块相对于头方块的偏移
	PlaceNBTBlock func(
		console *nbt_console.Console,
		cache *nbt_cache.NBTCacheSystem,
		nbtBlock nbt_parser_interface.Block,
	) (
		canFast bool,
		uniqueID uuid.UUID,
		offset protocol.BlockPos,
		err error,
	)
)

// Item 是所有复杂 NBT 物品在制作时的统称
type Item interface {
	// Append 将同类的已解析的 NBT 物品加入到当前队列。
	// 应当确保所有加入队列的物品都具有唯一的 NBT Hash Number
	Append(item ...nbt_parser_interface.Item)
	// Make 试图制作多个具有不同 NBT Hash Number 的物品。
	// 并且，对于每种物品，只会生成 1 个。
	// 如果 resultSlot 为空，则没有物品可以制作。
	//
	// Make 的调用者有责任确保目标物品是有效的复杂物品，
	// 也就是它是否需要被进一步特殊处理。这意味着，如果
	// 一个物品不被认为需要特殊处理，那么该物品将不应通过
	// Make 来获得，而是应该通过简单的命令完成。
	// 您可以通过对每个物品使用 IsComplex 来确定这件事。
	//
	// resultSlot 指示物品的 NBT Hash Number 到物品生成后
	// 所在槽位的映射，可以通过它知晓物品在被制作后的位置。
	//
	// 应当确保在调用 Make 后立即使用 resultSlot 所指示的物品，
	// 否则下一次调用时，这些物品所在的物品栏可能会被重用
	Make() (resultSlot map[uint64]resources_control.SlotID, err error)
}

// Block 是所有 NBT 方块在制作时的统称
type Block interface {
	// Make 试图制作这个 NBT 方块
	Make() error
	// Offset 指示这个 NBT 方块的偏移。
	// 这对大多数 NBT 方块都是默认的零值，
	// 但对于床这种包含两个方块的情况，
	// 我们需要记录床尾相对于床头的偏移
	Offset() protocol.BlockPos
}
