package nbt_parser_block

import (
	"github.com/Yeah114/FunShuttler/mapping"
	nbt_parser_interface "github.com/Yeah114/FunShuttler/nbt_parser/interface"
	"github.com/Yeah114/FunShuttler/utils"
)

func init() {
	nbt_parser_interface.DeepCopyAndFixStates = DeepCopyAndFixStates
}

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
func DeepCopyAndFixStates(blockType uint8, blockName string, blockStates map[string]any) map[string]any {
	result := utils.DeepCopyNBT(blockStates)

	switch blockType {
	case mapping.SupportNBTBlockTypeContainer:
		switch blockName {
		case "minecraft:barrel":
			result["open_bit"] = byte(0)
		case "minecraft:hopper":
			result["toggle_bit"] = byte(0)
		case "minecraft:dispenser", "minecraft:dropper":
			result["triggered_bit"] = byte(0)
		}
	case mapping.SupportNBTBlockTypeFrame:
		result["item_frame_map_bit"] = byte(0)
		result["item_frame_photo_bit"] = byte(0)
	case mapping.SupportNBTBlockTypeLectern:
		result["powered_bit"] = byte(0)
	case mapping.SupportNBTBlockTypeCrafter:
		result["crafting"] = byte(0)
		result["triggered_bit"] = byte(0)
	}

	return result
}
