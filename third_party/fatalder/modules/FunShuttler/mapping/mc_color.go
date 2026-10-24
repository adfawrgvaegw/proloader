package mapping

// 此表描述了染料 RGB 颜色值到 染料物品名 的映射
var RGBToDyeItemName map[[3]uint8]string = map[[3]uint8]string{
	{240, 240, 240}: "minecraft:white_dye",      // 白色染料
	{157, 151, 151}: "minecraft:light_gray_dye", // 淡灰色染料
	{71, 79, 82}:    "minecraft:gray_dye",       // 灰色染料
	{0, 0, 0}:       "minecraft:black_dye",      // 黑色染料
	{131, 84, 50}:   "minecraft:brown_dye",      // 棕色染料
	{176, 46, 38}:   "minecraft:red_dye",        // 红色染料
	{249, 128, 29}:  "minecraft:orange_dye",     // 橙色染料
	{254, 216, 61}:  "minecraft:yellow_dye",     // 黄色染料
	{128, 199, 31}:  "minecraft:lime_dye",       // 黄绿色染料
	{94, 124, 22}:   "minecraft:green_dye",      // 绿色染料
	{22, 156, 156}:  "minecraft:cyan_dye",       // 青色染料
	{58, 179, 218}:  "minecraft:light_blue_dye", // 淡蓝色染料
	{60, 68, 170}:   "minecraft:blue_dye",       // 蓝色染料
	{137, 50, 184}:  "minecraft:purple_dye",     // 紫色染料
	{199, 78, 189}:  "minecraft:magenta_dye",    // 品红色染料
	{243, 139, 170}: "minecraft:pink_dye",       // 粉红色染料
}

// 此表描述了 MCBE 所有原本染料的 RGB 颜色
var DefaultDyeColor [][3]uint8 = [][3]uint8{
	{240, 240, 240}, // 白色
	{157, 151, 151}, // 淡灰色
	{71, 79, 82},    // 灰色
	{0, 0, 0},       // 黑色(告示牌默认颜色)
	{131, 84, 50},   // 棕色
	{176, 46, 38},   // 红色
	{249, 128, 29},  // 橙色
	{254, 216, 61},  // 黄色
	{128, 199, 31},  // 黄绿色
	{94, 124, 22},   // 绿色
	{22, 156, 156},  // 青色
	{58, 179, 218},  // 淡蓝色
	{60, 68, 170},   // 蓝色
	{137, 50, 184},  // 紫色
	{199, 78, 189},  // 品红色
	{243, 139, 170}, // 粉红色
}
