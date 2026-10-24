package mapping

// 此表描述了现阶段已支持的方块实体中，
// 其物品名称到方块名的映射，
// 这将仅被用于子方块功能。
// 键代表物品名，而值代表此物品对应的方块名
var ItemNameToBlockName = map[string]string{
	// 命令方块
	"minecraft:command_block":           "minecraft:command_block",
	"minecraft:chain_command_block":     "minecraft:chain_command_block",
	"minecraft:repeating_command_block": "minecraft:repeating_command_block",
	// 容器
	"minecraft:blast_furnace":          "minecraft:blast_furnace",
	"minecraft:lit_blast_furnace":      "minecraft:lit_blast_furnace",
	"minecraft:smoker":                 "minecraft:smoker",
	"minecraft:lit_smoker":             "minecraft:lit_smoker",
	"minecraft:furnace":                "minecraft:furnace",
	"minecraft:lit_furnace":            "minecraft:lit_furnace",
	"minecraft:chest":                  "minecraft:chest",
	"minecraft:barrel":                 "minecraft:barrel",
	"minecraft:trapped_chest":          "minecraft:trapped_chest",
	"minecraft:hopper":                 "minecraft:hopper",
	"minecraft:dispenser":              "minecraft:dispenser",
	"minecraft:dropper":                "minecraft:dropper",
	"minecraft:undyed_shulker_box":     "minecraft:undyed_shulker_box",
	"minecraft:white_shulker_box":      "minecraft:white_shulker_box",
	"minecraft:orange_shulker_box":     "minecraft:orange_shulker_box",
	"minecraft:magenta_shulker_box":    "minecraft:magenta_shulker_box",
	"minecraft:light_blue_shulker_box": "minecraft:light_blue_shulker_box",
	"minecraft:yellow_shulker_box":     "minecraft:yellow_shulker_box",
	"minecraft:lime_shulker_box":       "minecraft:lime_shulker_box",
	"minecraft:pink_shulker_box":       "minecraft:pink_shulker_box",
	"minecraft:gray_shulker_box":       "minecraft:gray_shulker_box",
	"minecraft:light_gray_shulker_box": "minecraft:light_gray_shulker_box",
	"minecraft:cyan_shulker_box":       "minecraft:cyan_shulker_box",
	"minecraft:purple_shulker_box":     "minecraft:purple_shulker_box",
	"minecraft:blue_shulker_box":       "minecraft:blue_shulker_box",
	"minecraft:brown_shulker_box":      "minecraft:brown_shulker_box",
	"minecraft:green_shulker_box":      "minecraft:green_shulker_box",
	"minecraft:red_shulker_box":        "minecraft:red_shulker_box",
	"minecraft:black_shulker_box":      "minecraft:black_shulker_box",
	// 告示牌
	"minecraft:oak_sign":              "minecraft:wall_sign",
	"minecraft:spruce_sign":           "minecraft:spruce_wall_sign",
	"minecraft:birch_sign":            "minecraft:birch_wall_sign",
	"minecraft:jungle_sign":           "minecraft:jungle_wall_sign",
	"minecraft:acacia_sign":           "minecraft:acacia_wall_sign",
	"minecraft:darkoak_sign":          "minecraft:darkoak_wall_sign",
	"minecraft:mangrove_sign":         "minecraft:mangrove_wall_sign",
	"minecraft:cherry_sign":           "minecraft:cherry_wall_sign",
	"minecraft:bamboo_sign":           "minecraft:bamboo_wall_sign",
	"minecraft:crimson_sign":          "minecraft:crimson_wall_sign",
	"minecraft:warped_sign":           "minecraft:warped_wall_sign",
	"minecraft:oak_hanging_sign":      "minecraft:oak_hanging_sign",
	"minecraft:spruce_hanging_sign":   "minecraft:spruce_hanging_sign",
	"minecraft:birch_hanging_sign":    "minecraft:birch_hanging_sign",
	"minecraft:jungle_hanging_sign":   "minecraft:jungle_hanging_sign",
	"minecraft:acacia_hanging_sign":   "minecraft:acacia_hanging_sign",
	"minecraft:dark_oak_hanging_sign": "minecraft:dark_oak_hanging_sign",
	"minecraft:mangrove_hanging_sign": "minecraft:mangrove_hanging_sign",
	"minecraft:cherry_hanging_sign":   "minecraft:cherry_hanging_sign",
	"minecraft:bamboo_hanging_sign":   "minecraft:bamboo_hanging_sign",
	"minecraft:crimson_hanging_sign":  "minecraft:crimson_hanging_sign",
	"minecraft:warped_hanging_sign":   "minecraft:warped_hanging_sign",
	// 物品展示框
	"minecraft:frame":      "minecraft:frame",
	"minecraft:glow_frame": "minecraft:glow_frame",
	// 结构方块、旗帜、讲台、唱片机、酿造台 和 合成器
	"minecraft:structure_block": "minecraft:structure_block",
	"minecraft:banner":          "minecraft:wall_banner",
	"minecraft:lectern":         "minecraft:lectern",
	"minecraft:jukebox":         "minecraft:jukebox",
	"minecraft:brewing_stand":   "minecraft:brewing_stand",
	"minecraft:crafter":         "minecraft:crafter",
}

// SubBlocksPool 记载了物品制作中可以嵌套的子方块
var SubBlocksPool = map[uint8]bool{
	SupportNBTBlockTypeCommandBlock:   true,
	SupportNBTBlockTypeContainer:      true,
	SupportNBTBlockTypeSign:           true,
	SupportNBTBlockTypeStructureBlock: true,
	SupportNBTBlockTypeLectern:        true,
	SupportNBTBlockTypeJukeBox:        true,
	SupportNBTBlockTypeBrewingStand:   true,
	SupportNBTBlockTypeCrafter:        true,
}
