package nbt_parser_interface

import "github.com/Yeah114/FunShuttler/core/minecraft/protocol"

var (
	// ParseNBTBlock 从方块实体数据 blockNBT 解析一个方块实体。
	// blockName 和 blockStates 分别指示这个方块实体的名称和方块状态。
	//
	// nameChecker 是一个可选的函数，用于检查 name 所指示的物品名称是
	// 否可通过指令获取。如果不能，则 nameChecker 返回假。
	//
	// nameChecker 对于大多数方块的解析可能没有帮助，但它可以帮助验证
	// 容器内的物品是否是可以通过指令获取的物品。
	//
	// 另外，如果没有这样的 nameChecker 函数，则可以将其简单的置为 nil
	ParseBlock func(
		nameChecker func(name string) bool,
		blockName string,
		blockStates map[string]any,
		blockNBT map[string]any,
	) (
		block Block,
		err error,
	)
	// ParseItemNormal 从 nbtMap 解析一个 NBT 物品。
	// nbtMap 是含有这个物品 tag 标签的父复合标签。
	//
	// nameChecker 是一个可选的函数，用于检查 name 所
	// 指示的物品名称是否可通过指令获取。如果不能，则返
	// 回的 canGetByCommand 为假。
	//
	// 无论 canGetByCommand 的值是多少，如果解析没有发
	// 生错误，则 item 不会为空。
	//
	// 另外，如果没有这样的 nameChecker 函数，则可以将其
	// 简单的置为 nil
	ParseItemNormal func(
		nameChecker func(name string) bool,
		nbtMap map[string]any,
	) (
		item Item,
		canGetByCommand bool,
		err error,
	)
	// ParseItemNetwork 解析网络传输上的物品堆栈实例 item。
	// itemName 是这个物品堆栈实例的名称
	ParseItemNetwork func(
		itemStack protocol.ItemStack,
		itemName string,
	) (
		item Item,
		err error,
	)
)

var (
	// DeepCopyAndFixStates 先深拷贝 blockStates，然后修复类型为 blockType，
	// 方块名称为 blockName 且方块状态为 blockStates 的方块的方块状态。
	//
	// 这主要用于解决导入时产生的不可能问题，即用户提供的方块状态可能包含
	// 一些不可能抵达的成分，例如一些方块状态字段指示了这个方块是否被红石
	// 激活等。
	// 在实际导入时，我们并不会提供红石信号，这意味着放置的方块在很大程度上，
	// 其方块状态会被纠正 (例如改变为没有红石激活的情况)。
	//
	// 基于此，我们需要结合导入的实际环境，修正传入方块的方块状态。
	// DeepCopyAndFixStates 在实现上是深拷贝的，这意味着使用者可以安全的修改返回值
	DeepCopyAndFixStates func(blockType uint8, blockName string, blockStates map[string]any) map[string]any
	// SetItemCount 设置 item 的物品数量为 count。
	// 它目前是对酿造台中烈焰粉所在槽位的特殊处理
	SetItemCount func(item Item, count uint8)
)

// Block 是所有已实现的 NBT 方块的统称
type Block interface {
	// BlockName 返回这个方块的名称
	BlockName() string
	// BlockStates 返回这个方块的方块状态
	BlockStates() map[string]any
	// BlockStatesString 返回这个方块的方块状态的字符串表示
	BlockStatesString() string
	// Format 将这个 NBT 方块格式化为中文的字符串表示。
	// prefix 是格式化时所使用的前缀字符
	Format(prefix string) string
	// Parse 从 nbtMap 解析一个方块实体，
	// nbtMap 是这个方块的方块实体数据
	Parse(nbtMap map[string]any) error
	// NeedSpecialHandle 指示在导入这个
	// 方块实体是否需要进行特殊处理。
	// 如果不需要，则方块直接使用命令放置
	NeedSpecialHandle() bool
	// NeedCheckCompletely 指示在完成这个
	// 方块的导入后是否需要检查其完整性。
	// 如果 NeedSpecialHandle 为假，
	// 则 NeedCheckCompletely 不应被使用
	NeedCheckCompletely() bool
	// NBTStableBytes 返回这个方块实体在特定
	// 字段上的稳定唯一表示
	NBTStableBytes() []byte
	// FullStableBytes 返回这个方块实体的数据
	// 的稳定唯一表示。其与 NBTStableBytes 的
	// 区别在于它还会考虑方块的名称和方块状态
	FullStableBytes() []byte
}

// Item 是所有已实现的 NBT 物品的统称
type Item interface {
	// ItemName 返回这个物品的名称
	ItemName() string
	// ItemCount 返回这个物品的数量
	ItemCount() uint8
	// ItemMetadata 返回这个物品的元数据
	ItemMetadata() int16
	// Format 将这个 NBT 物品格式化为中文的字符串表示。
	// prefix 是格式化时所使用的前缀字符
	Format(prefix string) string
	// ParseNetwork 解析网络传输上的物品堆栈实例 item。
	// itemName 是这个物品的名称
	ParseNetwork(item protocol.ItemStack, itemName string) error
	// ParseNormal 从 nbtMap 解析一个 NBT 物品。
	// nbtMap 是含有这个物品 tag 标签的父复合标签
	ParseNormal(nbtMap map[string]any) error
	// UnderlyingItem 返回这个物品的底层实现，
	// 这意味着返回值可以被断言为 DefaultItem
	UnderlyingItem() Item
	// NeedEnchOrRename 指示在导入这个
	// NBT 物品时是否需要附魔或重命名
	NeedEnchOrRename() bool
	// IsComplex 指示这个物品是否
	// 需要进一步的特殊处理才能得到
	IsComplex() bool
	// NBTStableBytes 返回该物品在 NBT 部分的校验和。
	// NBT 的部分不包含物品的自定义名称和附魔数据，
	// 但包括物品的名称、物品的元数据、物品组件数据和
	// 这个物品的一些特定 NBT 字段
	NBTStableBytes() []byte
	// TypeStableBytes 返回该种物品的种类哈希校验和。
	// 这意味着，同种的物品具有一致的种类哈希校验和
	TypeStableBytes() []byte
	// FullStableBytes 返回这个物品的哈希校验和
	FullStableBytes() []byte
}
