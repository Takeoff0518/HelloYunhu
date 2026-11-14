// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import "testing"

func TestPutAndGet(t *testing.T) {
	messages := NewMessages(4)

	messages.Put(1, "event-1")

	event, ok := messages.Get(1)
	if !ok || event != "event-1" {
		t.Fatalf("取到的事件为 %v, %v", event, ok)
	}

	if _, ok := messages.Get(2); ok {
		t.Error("不存在的标识不应命中")
	}
}

func TestEvictsOldest(t *testing.T) {
	messages := NewMessages(2)
	messages.Put(1, "a")
	messages.Put(2, "b")
	messages.Put(3, "c")

	if messages.Len() != 2 {
		t.Errorf("缓存条数为 %d，期望 2", messages.Len())
	}
	if _, ok := messages.Get(1); ok {
		t.Error("最早的一条应被淘汰")
	}
	if _, ok := messages.Get(3); !ok {
		t.Error("最新的一条应保留")
	}
}

func TestPutOverwritesSameID(t *testing.T) {
	messages := NewMessages(4)
	messages.Put(1, "old")
	messages.Put(1, "new")

	if event, _ := messages.Get(1); event != "new" {
		t.Errorf("覆盖后的事件为 %v", event)
	}
	if messages.Len() != 1 {
		t.Errorf("缓存条数为 %d，期望 1", messages.Len())
	}
}

func TestIgnoresInvalidInput(t *testing.T) {
	messages := NewMessages(4)

	messages.Put(0, "x")
	messages.Put(-1, "x")
	messages.Put(1, nil)

	if messages.Len() != 0 {
		t.Errorf("无效输入不应写入，当前条数为 %d", messages.Len())
	}
}

func TestDefaultCapacity(t *testing.T) {
	messages := NewMessages(0)

	for i := int64(1); i <= defaultCapacity+1; i++ {
		messages.Put(i, "x")
	}

	if messages.Len() != defaultCapacity {
		t.Errorf("缓存条数为 %d，期望 %d", messages.Len(), defaultCapacity)
	}
}
