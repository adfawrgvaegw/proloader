package mapping

const (
	SupportNBTItemTypeBook uint8 = iota
	SupportNBTItemTypeBanner
	SupportNBTItemTypeShield
)

// 此表描述了现阶段已经支持了的特殊物品，如烟花等物品。
// 键代表物品名，而值代表这种物品应该归属的类型
var SupportItemsPool = map[string]uint8{
	// 成书
	"minecraft:writable_book": SupportNBTItemTypeBook,
	"minecraft:written_book":  SupportNBTItemTypeBook,
	// 旗帜
	"minecraft:banner": SupportNBTItemTypeBanner,
	// 盾牌
	"minecraft:shield": SupportNBTItemTypeShield,
}
