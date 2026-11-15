// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"sync"
	"time"
)

// limiter 按固定间隔放行调用。
//
// 云湖对每个接口按每秒十次限流，超限返回错误码 1007。这里不排队堆积，只把相邻两次
// 调用之间的间隔拉到 1/rate 秒，余量足够时立即放行。等待受上下文控制，取消时返回
// 错误而不是继续发请求。
type limiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
}

// newLimiter 创建限流器，perSecond 不大于 0 时返回 nil 表示不限流。
func newLimiter(perSecond int) *limiter {
	if perSecond <= 0 {
		return nil
	}
	if perSecond > 1000 {
		perSecond = 1000
	}

	return &limiter{interval: time.Second / time.Duration(perSecond)}
}

// wait 等到可以发起下一次调用。
func (l *limiter) wait(ctx context.Context) error {
	if l == nil {
		return nil
	}

	l.mu.Lock()
	now := time.Now()
	if l.next.Before(now) {
		l.next = now
	}
	delay := l.next.Sub(now)
	l.next = l.next.Add(l.interval)
	l.mu.Unlock()

	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
