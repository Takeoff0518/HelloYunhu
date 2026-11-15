// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import "sync"

// defaultNicknameSize 是昵称缓存的默认容量。
const defaultNicknameSize = 2048

// Nicknames 缓存用户标识与昵称的对应关系。
//
// 云湖的 at 数组只给用户标识，发送时的正文却需要昵称才能显示成提及，因此把见到的
// 昵称留在内存里。数据只用于展示，丢失时退化为标识，不写数据库。
type Nicknames struct {
	mu    sync.RWMutex
	items map[string]string
	max   int
}

// NewNicknames 创建昵称缓存，max 小于等于 0 时使用默认容量。
func NewNicknames(max int) *Nicknames {
	if max <= 0 {
		max = defaultNicknameSize
	}

	return &Nicknames{items: make(map[string]string), max: max}
}

// Set 记录一个昵称，空昵称与空标识会被忽略。
//
// 容量用尽时整体清空后再写入。昵称只影响展示，清空的代价是短时间退化为标识，
// 比起定期扫描更简单。
func (n *Nicknames) Set(id, nickname string) {
	if n == nil || id == "" || nickname == "" {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if len(n.items) >= n.max {
		n.items = make(map[string]string, n.max)
	}
	n.items[id] = nickname
}

// Get 返回用户标识对应的昵称。
func (n *Nicknames) Get(id string) (string, bool) {
	if n == nil || id == "" {
		return "", false
	}

	n.mu.RLock()
	defer n.mu.RUnlock()

	nickname, ok := n.items[id]
	return nickname, ok
}

// Len 返回已缓存的昵称数量，供测试与诊断使用。
func (n *Nicknames) Len() int {
	if n == nil {
		return 0
	}

	n.mu.RLock()
	defer n.mu.RUnlock()

	return len(n.items)
}
