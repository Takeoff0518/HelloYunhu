// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLimiterSpacesCalls(t *testing.T) {
	// 每秒 50 次即 20 毫秒一次，连续三次至少要等 40 毫秒。
	limiter := newLimiter(50)
	start := time.Now()

	for i := 0; i < 3; i++ {
		if err := limiter.wait(context.Background()); err != nil {
			t.Fatalf("等待失败: %v", err)
		}
	}

	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Errorf("三次调用只用了 %s", elapsed)
	}
}

func TestLimiterDisabled(t *testing.T) {
	limiter := newLimiter(0)
	if limiter != nil {
		t.Fatal("每秒 0 次应当不限流")
	}

	start := time.Now()
	for i := 0; i < 100; i++ {
		if err := limiter.wait(context.Background()); err != nil {
			t.Fatalf("等待失败: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		t.Errorf("不限流时耗时 %s", elapsed)
	}
}

func TestLimiterHonoursContext(t *testing.T) {
	limiter := newLimiter(1)
	if err := limiter.wait(context.Background()); err != nil {
		t.Fatalf("首次等待失败: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	if err := limiter.wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("错误为 %v", err)
	}
}

func TestAPIErrorMatchesRateLimited(t *testing.T) {
	err := &APIError{Code: CodeRateLimited, Op: "POST /bot/send"}
	if !errors.Is(err, ErrRateLimited) {
		t.Error("1007 应当匹配频率超限")
	}

	other := &APIError{Code: 1003, Op: "GET /bot/info"}
	if errors.Is(other, ErrRateLimited) {
		t.Error("1003 不应匹配频率超限")
	}
}
