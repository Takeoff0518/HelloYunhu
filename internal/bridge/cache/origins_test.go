// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import "testing"

func TestOrigins(t *testing.T) {
	origins := NewOrigins(2)

	if _, ok := origins.Get(1); ok {
		t.Fatal("空缓存不应命中")
	}

	origins.Put(1, "695367192", "group")
	if got, ok := origins.Get(1); !ok || got.ChatID != "695367192" || got.ChatType != "group" {
		t.Errorf("归属为 %+v，命中为 %v", got, ok)
	}

	// 同一标识重复写入时覆盖旧值。
	origins.Put(1, "2272345", "bot")
	if got, _ := origins.Get(1); got.ChatID != "2272345" || got.ChatType != "bot" {
		t.Errorf("归属为 %+v", got)
	}
	if origins.Len() != 1 {
		t.Errorf("数量为 %d", origins.Len())
	}
}

func TestOriginsEvictsOldest(t *testing.T) {
	origins := NewOrigins(2)

	origins.Put(1, "a", "group")
	origins.Put(2, "b", "group")
	origins.Put(3, "c", "group")

	if _, ok := origins.Get(1); ok {
		t.Error("最早的一条应被淘汰")
	}
	if _, ok := origins.Get(3); !ok {
		t.Error("最新的一条应命中")
	}
	if origins.Len() != 2 {
		t.Errorf("数量为 %d", origins.Len())
	}
}

func TestOriginsNilSafe(t *testing.T) {
	var origins *Origins

	origins.Put(1, "a", "group")
	if _, ok := origins.Get(1); ok {
		t.Error("空指针不应命中")
	}
	if origins.Len() != 0 {
		t.Errorf("数量为 %d", origins.Len())
	}
}
