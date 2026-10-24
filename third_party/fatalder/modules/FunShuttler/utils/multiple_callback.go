package utils

import (
	"sync"

	"github.com/google/uuid"
)

// callbackWrapper 是单个回调的包装
type callbackWrapper[T any] struct {
	uniqueID string
	callback func(data T)
}

// MultipleCallback 包装在同一个事件上的多个回调函数
type MultipleCallback[T any] struct {
	mu        *sync.Mutex
	callbacks []callbackWrapper[T]
}

// NewMultipleCallback 创建一个新的 MultipleCallback
func NewMultipleCallback[T any]() *MultipleCallback[T] {
	return &MultipleCallback[T]{
		mu:        new(sync.Mutex),
		callbacks: nil,
	}
}

// Append 将一个新的回调函数加入底层切片，
// 并返回该回调函数对应的唯一标识。
// 您可使用 Destory 撤销该回调函数
func (m *MultipleCallback[T]) Append(f func(data T)) (uniqueID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	uniqueID = uuid.NewString()
	m.callbacks = append(
		m.callbacks,
		callbackWrapper[T]{
			uniqueID: uniqueID,
			callback: f,
		},
	)
	return
}

// Destory 撤销唯一标识为 uniqueID 的回调函数。
// 如果不存在，则不进行任何操作
func (m *MultipleCallback[T]) Destory(uniqueID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	newCallbacks := make([]callbackWrapper[T], 0)
	for _, wrapper := range m.callbacks {
		if wrapper.uniqueID == uniqueID {
			continue
		}
		newCallbacks = append(newCallbacks, wrapper)
	}
	m.callbacks = newCallbacks
}

// FinishAll 执行底层切片的所有回调函数，并将切片清空
func (m *MultipleCallback[T]) FinishAll(data T) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, wrapper := range m.callbacks {
		go wrapper.callback(data)
	}
	m.callbacks = nil
}
