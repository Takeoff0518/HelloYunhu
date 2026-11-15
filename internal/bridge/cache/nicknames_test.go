// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import "testing"

func TestNicknames(t *testing.T) {
	nicknames := NewNicknames(0)

	if _, ok := nicknames.Get("1"); ok {
		t.Fatal("空缓存不应命中")
	}

	nicknames.Set("1", "甲")
	if got, ok := nicknames.Get("1"); !ok || got != "甲" {
		t.Errorf("昵称为 %q，命中为 %v", got, ok)
	}

	// 空昵称与空标识不写入。
	nicknames.Set("2", "")
	nicknames.Set("", "乙")
	if nicknames.Len() != 1 {
		t.Errorf("缓存数量为 %d", nicknames.Len())
	}
}

func TestNicknamesOverwriteAndEvict(t *testing.T) {
	nicknames := NewNicknames(2)

	nicknames.Set("1", "甲")
	nicknames.Set("1", "甲二")
	if got, _ := nicknames.Get("1"); got != "甲二" {
		t.Errorf("昵称为 %q", got)
	}

	// 容量用尽后整体清空再写入，旧昵称不再命中。
	nicknames.Set("2", "乙")
	nicknames.Set("3", "丙")
	if _, ok := nicknames.Get("1"); ok {
		t.Error("清空后不应命中旧昵称")
	}
	if got, ok := nicknames.Get("3"); !ok || got != "丙" {
		t.Errorf("昵称为 %q，命中为 %v", got, ok)
	}
}

func TestNicknamesNilSafe(t *testing.T) {
	var nicknames *Nicknames

	nicknames.Set("1", "甲")
	if _, ok := nicknames.Get("1"); ok {
		t.Error("空指针不应命中")
	}
	if nicknames.Len() != 0 {
		t.Errorf("空指针数量为 %d", nicknames.Len())
	}
}
