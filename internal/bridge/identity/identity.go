// SPDX-License-Identifier: GPL-3.0-or-later

// Package identity 维护云湖标识与 OneBot 整数标识之间的映射。
//
// 云湖的用户、群与消息标识都是字符串，而 OneBot 的多处代码要求整数。用户与群
// 标识本身是十进制数字，可以直接转换；消息标识是 32 位十六进制，必须用一张表
// 记录对应关系。
package identity

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"
	"sync"
)

// MessageStore 保存消息标识的映射关系。
type MessageStore interface {
	// MessageID 返回云湖消息标识对应的整数，没有记录时分配一个新的。
	MessageID(ctx context.Context, msgID string) (int64, error)
	// MsgID 按整数反查云湖消息标识，没有记录时第二个返回值为 false。
	MsgID(ctx context.Context, messageID int64) (string, bool, error)
}

// Memory 是内存实现。
//
// 进程退出后映射全部丢失，重启后旧消息的引用与撤回会失效。M2 会换成落盘实现，
// 这里的接口保持不变。
type Memory struct {
	mu      sync.Mutex
	next    int64
	byMsgID map[string]int64
	byID    map[int64]string
}

// NewMemory 创建内存映射表。整数从 1 开始分配，0 留作无效值。
func NewMemory() *Memory {
	return &Memory{
		next:    1,
		byMsgID: make(map[string]int64),
		byID:    make(map[int64]string),
	}
}

// MessageID 返回消息标识对应的整数，没有记录时分配一个新的。
func (m *Memory) MessageID(_ context.Context, msgID string) (int64, error) {
	if msgID == "" {
		return 0, fmt.Errorf("消息标识不能为空")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if id, ok := m.byMsgID[msgID]; ok {
		return id, nil
	}

	id := m.next
	m.next++
	m.byMsgID[msgID] = id
	m.byID[id] = msgID

	return id, nil
}

// MsgID 按整数反查消息标识。
func (m *Memory) MsgID(_ context.Context, messageID int64) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	msgID, ok := m.byID[messageID]
	return msgID, ok, nil
}

// ParseID 把云湖的数字标识转换为 OneBot 的整数标识。
//
// 云湖的用户与群标识都是十进制字符串，正常情况下直接解析即可。解析失败时退化为
// 稳定的哈希值并返回 false，调用方应当记录一次日志：这样标识仍然可比较，但已经
// 失去可读性。
func ParseID(raw string) (int64, bool) {
	if raw == "" {
		return 0, false
	}

	if id, err := strconv.ParseInt(raw, 10, 64); err == nil && id > 0 {
		return id, true
	}

	hash := fnv.New64a()
	_, _ = hash.Write([]byte(raw))

	// 取 63 位并保证为正数，避免与无效值 0 冲突。
	return int64(hash.Sum64() & 0x7FFFFFFFFFFFFFFF), false
}
