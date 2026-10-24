package mapping

// ContainerStorageKey 描述了现已支持的容器中，
// 用于储存物品所使用的复合标签或列表的名字
var ContainerStorageKey map[string]string = map[string]string{
	// 高炉(发光的高炉)
	"minecraft:blast_furnace":     "Items",
	"minecraft:lit_blast_furnace": "Items",
	// 烟熏炉(发光的烟熏炉)
	"minecraft:smoker":     "Items",
	"minecraft:lit_smoker": "Items",
	// 熔炉(发光的熔炉)
	"minecraft:furnace":     "Items",
	"minecraft:lit_furnace": "Items",
	// 箱子、陷阱箱、漏斗、发射器、投掷器、唱片机、木桶
	"minecraft:chest":         "Items",
	"minecraft:trapped_chest": "Items",
	"minecraft:hopper":        "Items",
	"minecraft:dispenser":     "Items",
	"minecraft:dropper":       "Items",
	"minecraft:barrel":        "Items",
	// 各种颜色或未被染色的潜影盒
	"minecraft:undyed_shulker_box":     "Items",
	"minecraft:white_shulker_box":      "Items",
	"minecraft:orange_shulker_box":     "Items",
	"minecraft:magenta_shulker_box":    "Items",
	"minecraft:light_blue_shulker_box": "Items",
	"minecraft:yellow_shulker_box":     "Items",
	"minecraft:lime_shulker_box":       "Items",
	"minecraft:pink_shulker_box":       "Items",
	"minecraft:gray_shulker_box":       "Items",
	"minecraft:light_gray_shulker_box": "Items",
	"minecraft:cyan_shulker_box":       "Items",
	"minecraft:purple_shulker_box":     "Items",
	"minecraft:blue_shulker_box":       "Items",
	"minecraft:brown_shulker_box":      "Items",
	"minecraft:green_shulker_box":      "Items",
	"minecraft:red_shulker_box":        "Items",
	"minecraft:black_shulker_box":      "Items",
	// 合成器
	"minecraft:crafter": "Items",
}
