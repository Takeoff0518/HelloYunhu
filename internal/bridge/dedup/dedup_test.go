// SPDX-License-Identifier: GPL-3.0-or-later

package dedup

import (
	"testing"
	"time"
)

// fakeClock 是可手动推进的时钟。
type fakeClock struct {
	at time.Time
}

// now 返回当前模拟时间。
func (c *fakeClock) now() time.Time {
	return c.at
}

func TestSeenReportsDuplicates(t *testing.T) {
	filter := New(Options{})

	if filter.Seen("evt-1") {
		t.Fatal("第一次出现不应算重复")
	}
	if !filter.Seen("evt-1") {
		t.Fatal("第二次出现应算重复")
	}
	if filter.Seen("evt-2") {
		t.Fatal("不同标识不应算重复")
	}
}

func TestEmptyIdentifierIsNeverDuplicate(t *testing.T) {
	filter := New(Options{})

	if filter.Seen("") {
		t.Error("空标识不应被判定为重复")
	}
	if filter.Seen("") {
		t.Error("空标识重复出现也不应被判定为重复")
	}
	if filter.Len() != 0 {
		t.Errorf("空标识不应计入记录，当前为 %d", filter.Len())
	}
}

func TestExpiryAllowsAgain(t *testing.T) {
	clock := &fakeClock{at: time.Date(2026, time.November, 15, 12, 0, 0, 0, time.Local)}
	filter := New(Options{TTL: time.Minute, Now: clock.now})

	if filter.Seen("evt-1") {
		t.Fatal("第一次出现不应算重复")
	}

	clock.at = clock.at.Add(2 * time.Minute)
	if filter.Seen("evt-1") {
		t.Error("超过保留时间后应重新接受")
	}
}

func TestCapacityEvictsOldest(t *testing.T) {
	filter := New(Options{Capacity: 2})

	filter.Seen("a")
	filter.Seen("b")
	filter.Seen("c")

	if filter.Seen("a") {
		t.Error("最早的记录应被淘汰")
	}
	if !filter.Seen("c") {
		t.Error("最近的记录应仍被视为重复")
	}
}
