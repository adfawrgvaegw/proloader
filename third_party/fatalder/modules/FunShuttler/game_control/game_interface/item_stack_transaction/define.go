package item_stack_transaction

import (
	"github.com/Yeah114/FunShuttler/game_control/game_interface/item_stack_operation"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
)

// DebugPrintFailedRename 指示命名操作成功，
// 但物品新名称和要求的值不匹配时是否应该打印。
// 这可能发生在使用者提供的名称是网易屏蔽词时
const DebugPrintFailedRename = true

// ItemStackTransaction 是单个物品操作事务，
// 它希望使用者尽可能多的将物品堆栈请求内联在一个数据包中，
// 这样可以有效的节省操作的时间消耗
type ItemStackTransaction struct {
	api            *resources_control.Resources
	operations     []item_stack_operation.ItemStackOperation
	stackNetworkID map[resources_control.SlotLocation]int32
}

// NewItemStackTransaction 基于 api 创建并返回一个新的 ItemStackTransaction
func NewItemStackTransaction(api *resources_control.Resources) *ItemStackTransaction {
	return &ItemStackTransaction{
		api:            api,
		operations:     nil,
		stackNetworkID: make(map[resources_control.SlotLocation]int32),
	}
}
