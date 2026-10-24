package mapping

const (
	SupportNBTBlockTypeCommandBlock uint8 = iota
	SupportNBTBlockTypeContainer
	SupportNBTBlockTypeSign
	SupportNBTBlockTypeFrame
	SupportNBTBlockTypeStructureBlock
	SupportNBTBlockTypeBanner
	SupportNBTBlockTypeLectern
	SupportNBTBlockTypeJukeBox
	SupportNBTBlockTypeBrewingStand
	SupportNBTBlockTypeCrafter
)

// 此表描述了现阶段已经支持了的方块实体。
// 键代表方块名，而值代表这种方块应该归属的类型
var SupportBlocksPool = map[string]uint8{
	// 命令方块
	"minecraft:command_block":           SupportNBTBlockTypeCommandBlock,
	"minecraft:chain_command_block":     SupportNBTBlockTypeCommandBlock,
	"minecraft:repeating_command_block": SupportNBTBlockTypeCommandBlock,
	// 容器
	"minecraft:blast_furnace":          SupportNBTBlockTypeContainer,
	"minecraft:lit_blast_furnace":      SupportNBTBlockTypeContainer,
	"minecraft:smoker":                 SupportNBTBlockTypeContainer,
	"minecraft:lit_smoker":             SupportNBTBlockTypeContainer,
	"minecraft:furnace":                SupportNBTBlockTypeContainer,
	"minecraft:lit_furnace":            SupportNBTBlockTypeContainer,
	"minecraft:chest":                  SupportNBTBlockTypeContainer,
	"minecraft:barrel":                 SupportNBTBlockTypeContainer,
	"minecraft:trapped_chest":          SupportNBTBlockTypeContainer,
	"minecraft:hopper":                 SupportNBTBlockTypeContainer,
	"minecraft:dispenser":              SupportNBTBlockTypeContainer,
	"minecraft:dropper":                SupportNBTBlockTypeContainer,
	"minecraft:undyed_shulker_box":     SupportNBTBlockTypeContainer,
	"minecraft:white_shulker_box":      SupportNBTBlockTypeContainer,
	"minecraft:orange_shulker_box":     SupportNBTBlockTypeContainer,
	"minecraft:magenta_shulker_box":    SupportNBTBlockTypeContainer,
	"minecraft:light_blue_shulker_box": SupportNBTBlockTypeContainer,
	"minecraft:yellow_shulker_box":     SupportNBTBlockTypeContainer,
	"minecraft:lime_shulker_box":       SupportNBTBlockTypeContainer,
	"minecraft:pink_shulker_box":       SupportNBTBlockTypeContainer,
	"minecraft:gray_shulker_box":       SupportNBTBlockTypeContainer,
	"minecraft:light_gray_shulker_box": SupportNBTBlockTypeContainer,
	"minecraft:cyan_shulker_box":       SupportNBTBlockTypeContainer,
	"minecraft:purple_shulker_box":     SupportNBTBlockTypeContainer,
	"minecraft:blue_shulker_box":       SupportNBTBlockTypeContainer,
	"minecraft:brown_shulker_box":      SupportNBTBlockTypeContainer,
	"minecraft:green_shulker_box":      SupportNBTBlockTypeContainer,
	"minecraft:red_shulker_box":        SupportNBTBlockTypeContainer,
	"minecraft:black_shulker_box":      SupportNBTBlockTypeContainer,
	// 告示牌
	"minecraft:standing_sign":          SupportNBTBlockTypeSign,
	"minecraft:spruce_standing_sign":   SupportNBTBlockTypeSign,
	"minecraft:birch_standing_sign":    SupportNBTBlockTypeSign,
	"minecraft:jungle_standing_sign":   SupportNBTBlockTypeSign,
	"minecraft:acacia_standing_sign":   SupportNBTBlockTypeSign,
	"minecraft:darkoak_standing_sign":  SupportNBTBlockTypeSign,
	"minecraft:mangrove_standing_sign": SupportNBTBlockTypeSign,
	"minecraft:cherry_standing_sign":   SupportNBTBlockTypeSign,
	"minecraft:bamboo_standing_sign":   SupportNBTBlockTypeSign,
	"minecraft:crimson_standing_sign":  SupportNBTBlockTypeSign,
	"minecraft:warped_standing_sign":   SupportNBTBlockTypeSign,
	"minecraft:wall_sign":              SupportNBTBlockTypeSign,
	"minecraft:spruce_wall_sign":       SupportNBTBlockTypeSign,
	"minecraft:birch_wall_sign":        SupportNBTBlockTypeSign,
	"minecraft:jungle_wall_sign":       SupportNBTBlockTypeSign,
	"minecraft:acacia_wall_sign":       SupportNBTBlockTypeSign,
	"minecraft:darkoak_wall_sign":      SupportNBTBlockTypeSign,
	"minecraft:mangrove_wall_sign":     SupportNBTBlockTypeSign,
	"minecraft:cherry_wall_sign":       SupportNBTBlockTypeSign,
	"minecraft:bamboo_wall_sign":       SupportNBTBlockTypeSign,
	"minecraft:crimson_wall_sign":      SupportNBTBlockTypeSign,
	"minecraft:warped_wall_sign":       SupportNBTBlockTypeSign,
	"minecraft:oak_hanging_sign":       SupportNBTBlockTypeSign,
	"minecraft:spruce_hanging_sign":    SupportNBTBlockTypeSign,
	"minecraft:birch_hanging_sign":     SupportNBTBlockTypeSign,
	"minecraft:jungle_hanging_sign":    SupportNBTBlockTypeSign,
	"minecraft:acacia_hanging_sign":    SupportNBTBlockTypeSign,
	"minecraft:dark_oak_hanging_sign":  SupportNBTBlockTypeSign,
	"minecraft:mangrove_hanging_sign":  SupportNBTBlockTypeSign,
	"minecraft:cherry_hanging_sign":    SupportNBTBlockTypeSign,
	"minecraft:bamboo_hanging_sign":    SupportNBTBlockTypeSign,
	"minecraft:crimson_hanging_sign":   SupportNBTBlockTypeSign,
	"minecraft:warped_hanging_sign":    SupportNBTBlockTypeSign,
	// 物品展示框
	"minecraft:frame":      SupportNBTBlockTypeFrame,
	"minecraft:glow_frame": SupportNBTBlockTypeFrame,
	// 结构方块, 旗帜, 讲台, 唱片机, 酿造台 和 合成器
	"minecraft:structure_block": SupportNBTBlockTypeStructureBlock,
	"minecraft:standing_banner": SupportNBTBlockTypeBanner,
	"minecraft:wall_banner":     SupportNBTBlockTypeBanner,
	"minecraft:lectern":         SupportNBTBlockTypeLectern,
	"minecraft:jukebox":         SupportNBTBlockTypeJukeBox,
	"minecraft:brewing_stand":   SupportNBTBlockTypeBrewingStand,
	"minecraft:crafter":         SupportNBTBlockTypeCrafter,
}
