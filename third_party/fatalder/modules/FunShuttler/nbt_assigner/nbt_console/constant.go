package nbt_console

import (
	"time"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
)

const (
	// BaseBackground 是操作台地板的构成方块
	BaseBackground = "minecraft:sea_lantern"
	// DefaultHotbarSlot 是机器人默认的手持物品栏
	DefaultHotbarSlot = 5
	// DefaultTimeoutInitConsole 是抵达操作台目标区域的最长等待期限
	DefaultTimeoutInitConsole = time.Second * 30
)

const (
	// RequesterSystemCall 指示请求提及者是操作台本身
	RequesterSystemCall = "System Call"
	// RequesterUser 指示请求提及者是缓存命中系统的使用者
	RequesterUser = "User"
)

// ConsoleIndex 描述操作台中心方块
// 及 8 个帮助方块的索引
const (
	ConsoleIndexCenterBlock int = iota
	ConsoleIndexFirstHelperBlock
	ConsoleIndexSecondHelperBlock
	ConsoleIndexThirdHelperBlock
	ConsoleIndexForthHelperBlock
	ConsoleIndexFifthHelperBlock
	ConsoleIndexSixthHelperBlock
	ConsoleIndexSeventhHelperBlock
	ConsoleIndexEighthHelperBlock
)

var (
	// nearBlockMapping ..
	nearBlockMapping = []protocol.BlockPos{
		[3]int32{-1, 0, 0},
		[3]int32{1, 0, 0},
		[3]int32{0, -1, 0},
		[3]int32{0, 1, 0},
		[3]int32{0, 0, 1},
		[3]int32{0, 0, -1},
	}
	// helperBlockMapping ..
	helperBlockMapping = []protocol.BlockPos{
		[3]int32{0, 0, 0},
		[3]int32{-3, 0, -3},
		[3]int32{-3, 0, 3},
		[3]int32{3, 0, 3},
		[3]int32{3, 0, -3},
		[3]int32{-3, 0, 0},
		[3]int32{3, 0, 0},
		[3]int32{0, 0, 3},
		[3]int32{0, 0, -3},
	}
	// nearBlockMappingInv ..
	nearBlockMappingInv map[protocol.BlockPos]int
	// nearBlockMappingInv ..
	helperBlockMappingInv map[protocol.BlockPos]int
)

func init() {
	nearBlockMappingInv = make(map[protocol.BlockPos]int)
	for key, value := range nearBlockMapping {
		nearBlockMappingInv[value] = key
	}
	helperBlockMappingInv = make(map[protocol.BlockPos]int)
	for key, value := range helperBlockMapping {
		helperBlockMappingInv[value] = key
	}
}
