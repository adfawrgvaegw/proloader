package mapping

// 不详旗帜的图案
const BannerPatternOminous = "ill"

// 此表描述了当前版本不存在，但未来版本出现的旗帜图案
var BannerPatternUnsupported = map[string]bool{}

// 此表描述了旗帜中 Pattern 字段到 旗帜图案 的映射
var BannerPatternToItemName = map[string]string{
	"cre": "creeper_banner_pattern",          // 苦力怕盾徽
	"sku": "skull_banner_pattern",            // 头颅盾徽
	"flo": "flower_banner_pattern",           // 花朵盾徽
	"moj": "mojang_banner_pattern",           // Mojang 徽标
	"bri": "field_masoned_banner_pattern",    // 砖纹
	"cbo": "bordure_indented_banner_pattern", // 波纹边
	"pig": "piglin_banner_pattern",           // 猪鼻
	"glb": "globe_banner_pattern",            // 地球
	"flw": "flow_banner_pattern",             // 涡流
	"gus": "guster_banner_pattern",           // 旋风
}

// 此表描述了旗帜中 Color 字段到 染料物品名 的映射
var BannerColorToDyeName = map[int32]string{
	15: "minecraft:white_dye",      // 白色染料
	7:  "minecraft:light_gray_dye", // 淡灰色染料
	8:  "minecraft:gray_dye",       // 灰色染料
	0:  "minecraft:black_dye",      // 黑色染料
	3:  "minecraft:brown_dye",      // 棕色染料
	1:  "minecraft:red_dye",        // 红色染料
	14: "minecraft:orange_dye",     // 橙色染料
	11: "minecraft:yellow_dye",     // 黄色染料
	10: "minecraft:lime_dye",       // 黄绿色染料
	2:  "minecraft:green_dye",      // 绿色染料
	6:  "minecraft:cyan_dye",       // 青色染料
	12: "minecraft:light_blue_dye", // 淡蓝色染料
	4:  "minecraft:blue_dye",       // 蓝色染料
	5:  "minecraft:purple_dye",     // 紫色染料
	13: "minecraft:magenta_dye",    // 品红色染料
	9:  "minecraft:pink_dye",       // 粉红色染料
}
