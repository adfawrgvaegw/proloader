package nbt_hash

const (
	// SetHashNumberNotExist 为 0 指
	// 示相应的集合哈希校验和是不存在的
	SetHashNumberNotExist uint64 = 0
	// NBTHashNumberNotExist 为 0 指示相应
	// NBT 方块的 NBT 哈希校验和是不存在的
	NBTHashNumberNotExist uint64 = 0
)

// CompletelyHashNumber 描述一个物品或方块的哈希校验和
type CompletelyHashNumber struct {
	// HashNumber 是这个物品或方块的完整哈希校验和
	HashNumber uint64
	// SetHashNumber 指示集合哈希校验和，
	// 这只对容器方块或容器物品有效。
	// 这意味着它描述的是容器中物品集合的校验和。
	//
	// 通常的说，如果两个容器中物品的种类数相同，
	// 并且各个种类的物品数量也完全一致，
	// 那么无论这两个容器中的物品是如何分布的，
	// SetHashNumber 对于这两个容器的校验和总是相同。
	//
	// 对于非容器或空的容器，可以简单的将其置为默认零值。
	// 当然，我们更推荐置为 SetHashNumberNotExist
	SetHashNumber uint64
}
