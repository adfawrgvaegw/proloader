package nbt_block_cache

import (
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	nbt_hash "github.com/Yeah114/FunShuttler/nbt_parser/hash"
	nbt_parser_interface "github.com/Yeah114/FunShuttler/nbt_parser/interface"

	"github.com/google/uuid"
)

// StructureNBTBlock 指示了一个保存在结构中的 NBT 方块
type StructureNBTBlock struct {
	// UniqueID 是这个方块的唯一标识符
	UniqueID uuid.UUID
	// HashNumber 是这个方块的哈希校验和
	HashNumber nbt_hash.CompletelyHashNumber
	// Offset 用于该 NBT 方块的偏移，
	// 例如床的尾方块相对于头的偏移
	Offset protocol.BlockPos
	// Block 是这个结构储存的方块实体
	Block nbt_parser_interface.Block
}
