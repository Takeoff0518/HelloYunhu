// SPDX-License-Identifier: GPL-3.0-or-later

// Package dedup 按事件标识过滤重复事件。
//
// 同时使用订阅连接与 Webhook 时，同一条事件可能两路都到达；框架只应当收到一次。
// 这里用一个带过期时间的内存表记录最近见过的事件标识，进程重启后清空。
package dedup

import (
	"sync"
	"time"
)

// 默认参数。
const (
	defaultTTL      = 10 * time.Minute
	defaultCapacity = 4096
)

// Options 是过滤器的设置。
type Options struct {
	// TTL 是事件标识的保留时间，过期后可再次通过。
	TTL time.Duration
	// Capacity 是保留的事件数量上限。
	Capacity int
	// Now 返回当前时间，为空时使用 time.Now，测试用它模拟过期。
	Now func() time.Time
}

// Filter 是事件标识过滤器。
type Filter struct {
	ttl      time.Duration
	capacity int
	now      func() time.Time

	mu    sync.Mutex
	seen  map[string]time.Time
	order []string
}

// New 创建过滤器。TTL 或容量不为正时使用默认值。
func New(opt Options) *Filter {
	ttl := opt.TTL
	if ttl <= 0 {
		ttl = defaultTTL
	}
	capacity := opt.Capacity
	if capacity <= 0 {
		capacity = defaultCapacity
	}
	now := opt.Now
	if now == nil {
		now = time.Now
	}

	return &Filter{
		ttl:      ttl,
		capacity: capacity,
		now:      now,
		seen:     make(map[string]time.Time, capacity),
	}
}

// Seen 记录并报告事件标识是否已经出现过。
//
// 空标识一律视为未出现，避免把缺少标识的事件全部丢掉。
func (f *Filter) Seen(id string) bool {
	if id == "" {
		return false
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	now := f.now()
	f.expire(now)

	if _, ok := f.seen[id]; ok {
		return true
	}

	f.seen[id] = now
	f.order = append(f.order, id)

	for len(f.order) > f.capacity {
		oldest := f.order[0]
		f.order = f.order[1:]
		delete(f.seen, oldest)
	}

	return false
}

// expire 丢弃已经超过保留时间的记录。
func (f *Filter) expire(now time.Time) {
	cutoff := now.Add(-f.ttl)

	for len(f.order) > 0 {
		oldest := f.order[0]
		recorded, ok := f.seen[oldest]
		if ok && recorded.After(cutoff) {
			return
		}
		f.order = f.order[1:]
		delete(f.seen, oldest)
	}
}

// Len 返回当前保留的记录数量。
func (f *Filter) Len() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.seen)
}
