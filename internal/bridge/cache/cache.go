// SPDX-License-Identifier: GPL-3.0-or-later

// Package cache 保存最近的事件，供消息查询回放。
//
// OneBot 的 get_msg 需要返回完整消息事件，而云湖没有按消息标识查询完整事件的接口。
// 因此在内存里保留最近若干条事件，超出容量后淘汰最早的一条。进程重启后缓存为空，
// 此时返回明确的错误，不伪造数据。
package cache

import (
	"sync"
)

// 默认容量。
const defaultCapacity = 512

// Messages 是按插入顺序淘汰的消息缓存。
type Messages struct {
	mu       sync.Mutex
	capacity int
	order    []int64
	items    map[int64]any
}

// NewMessages 创建缓存。容量不大于 0 时使用默认值。
func NewMessages(capacity int) *Messages {
	if capacity <= 0 {
		capacity = defaultCapacity
	}

	return &Messages{
		capacity: capacity,
		items:    make(map[int64]any, capacity),
	}
}

// Put 保存一条事件，标识相同的旧记录会被覆盖。
func (m *Messages) Put(messageID int64, event any) {
	if messageID <= 0 || event == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.items[messageID]; exists {
		m.items[messageID] = event
		return
	}

	for len(m.order) >= m.capacity {
		oldest := m.order[0]
		m.order = m.order[1:]
		delete(m.items, oldest)
	}

	m.order = append(m.order, messageID)
	m.items[messageID] = event
}

// Get 取出一条事件，没有记录时第二个返回值为 false。
func (m *Messages) Get(messageID int64) (any, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	event, ok := m.items[messageID]
	return event, ok
}

// Len 返回当前保存的条数。
func (m *Messages) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.items)
}
