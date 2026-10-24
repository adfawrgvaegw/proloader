package game_interface

import (
	"github.com/Yeah114/FunShuttler/game_control/game_interface/item_stack_transaction"
)

// ItemStackOperation 是物品操作请求的包装实现
type ItemStackOperation struct {
	api *ResourcesWrapper
}

// NewItemStackOperation 基于 api 创建并返回一个新的 ItemStackOperation
func NewItemStackOperation(api *ResourcesWrapper) *ItemStackOperation {
	return &ItemStackOperation{api: api}
}

// OpenTransaction 打开一个新的物品堆栈操作事务。
//
// 多个事务可以被同时打开，但各个事务的操作内容不
// 应该发生重叠，否则操作的结果是未定义的。
//
// 另外，同一个事务应当只能被同一个 go 惯例所使用，
// 这意味着同时并发使用同一个事务不保证线程安全性
func (i *ItemStackOperation) OpenTransaction() *item_stack_transaction.ItemStackTransaction {
	return item_stack_transaction.NewItemStackTransaction(i.api.Resources)
}
