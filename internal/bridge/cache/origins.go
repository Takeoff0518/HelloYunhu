// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import "sync"

// defaultOriginSize 是消息归属缓存的默认容量。
const defaultOriginSize = 1024

// Origin 记录一条消息属于哪个会话，撤回时需要。
type Origin struct {
	// ChatID 是会话标识，群聊为群标识，私聊为用户标识。
	ChatID string
	// ChatType 是会话类型，取 group 或 bot，与事件的取值一致。
	ChatType string
}

// Origins 缓存消息标识到会话的对应关系。
//
// 撤回只需要消息标识与会话信息，不必保留整条事件。收到的消息与发出的消息都会记录，
// 进程重启后缓存为空，此时撤回返回未找到，不猜测会话。
type Origins struct {
	mu       sync.Mutex
	capacity int
	order    []int64
	items    map[int64]Origin
}

// NewOrigins 创建缓存，容量不大于 0 时使用默认值。
func NewOrigins(capacity int) *Origins {
	if capacity <= 0 {
		capacity = defaultOriginSize
	}

	return &Origins{capacity: capacity, items: make(map[int64]Origin, capacity)}
}

// Put 记录一条消息的会话归属。
func (o *Origins) Put(messageID int64, chatID, chatType string) {
	if o == nil || messageID <= 0 || chatID == "" {
		return
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	if _, exists := o.items[messageID]; exists {
		o.items[messageID] = Origin{ChatID: chatID, ChatType: chatType}
		return
	}

	if len(o.order) >= o.capacity {
		oldest := o.order[0]
		o.order = o.order[1:]
		delete(o.items, oldest)
	}

	o.order = append(o.order, messageID)
	o.items[messageID] = Origin{ChatID: chatID, ChatType: chatType}
}

// Get 返回消息的会话归属。
func (o *Origins) Get(messageID int64) (Origin, bool) {
	if o == nil || messageID <= 0 {
		return Origin{}, false
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	origin, ok := o.items[messageID]
	return origin, ok
}

// Len 返回已缓存的数量。
func (o *Origins) Len() int {
	if o == nil {
		return 0
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	return len(o.items)
}
