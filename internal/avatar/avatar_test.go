// SPDX-License-Identifier: GPL-3.0-or-later

package avatar

import (
	"bytes"
	"image/png"
	"testing"
)

func TestStorePutGet(t *testing.T) {
	store := NewStore()
	store.Put(10001, "https://chat-img.jwznb.com/a.jpg")

	url, ok := store.Get(10001)
	if !ok || url != "https://chat-img.jwznb.com/a.jpg" {
		t.Fatalf("Get 返回 %q、%v", url, ok)
	}

	if _, ok := store.Get(10002); ok {
		t.Error("没有记录时不应返回地址")
	}
}

func TestStoreIgnoresEmptyValues(t *testing.T) {
	store := NewStore()
	store.Put(0, "https://example.com/a.jpg")
	store.Put(10001, "")

	if store.Len() != 0 {
		t.Errorf("空标识与空地址都不应记录，当前条数 %d", store.Len())
	}
}

func TestStoreOverwrites(t *testing.T) {
	store := NewStore()
	store.Put(10001, "https://example.com/old.jpg")
	store.Put(10001, "https://example.com/new.jpg")

	url, _ := store.Get(10001)
	if url != "https://example.com/new.jpg" {
		t.Errorf("地址为 %q，期望被新值覆盖", url)
	}
	if store.Len() != 1 {
		t.Errorf("同一标识应只有一条记录，当前条数 %d", store.Len())
	}
}

func TestStoreLimit(t *testing.T) {
	store := NewStore()
	for index := 0; index < maxEntries+10; index++ {
		store.Put(int64(index+1), "https://example.com/a.jpg")
	}

	if store.Len() > maxEntries {
		t.Errorf("地址表超过容量上限，当前条数 %d", store.Len())
	}
}

func TestNilStore(t *testing.T) {
	var store *Store

	// 空指针上调用这三个方法都应当安全，兼容模式关闭时它们会被直接调到。
	store.Put(10001, "https://example.com/a.jpg")
	if _, ok := store.Get(10001); ok {
		t.Error("空表不应返回地址")
	}
	if store.Len() != 0 {
		t.Error("空表长度应为 0")
	}
}

func TestPlaceholderIsPNG(t *testing.T) {
	data := placeholderBytes()
	if len(data) == 0 {
		t.Fatal("占位图为空")
	}

	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Fatalf("占位图无法解码: %v", err)
	}

	// 第二次调用返回同一份内容，避免每次请求都重新编码。
	if !bytes.Equal(data, placeholderBytes()) {
		t.Error("占位图应当在首次使用时生成一次")
	}
}
