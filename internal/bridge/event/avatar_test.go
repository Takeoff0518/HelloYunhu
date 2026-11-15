// SPDX-License-Identifier: GPL-3.0-or-later

package event

import (
	"context"
	"testing"

	"github.com/Takeoff0518/HelloYunhu/internal/avatar"
)

func TestRecordsSenderAvatar(t *testing.T) {
	store := avatar.NewStore()
	converter := newConverter()
	converter.Avatars = store

	if _, _, err := converter.Convert(context.Background(), parse(t, realTextMessage)); err != nil {
		t.Fatalf("转换失败: %v", err)
	}

	url, ok := store.Get(2272345)
	if !ok {
		t.Fatal("应记录发送者的头像地址")
	}
	if url != "https://cdn.example.com/a.webp" {
		t.Errorf("头像地址为 %q", url)
	}
}

func TestRecordsFollowAvatar(t *testing.T) {
	const followed = `{
  "version": "1.0",
  "header": {"eventId": "evt-follow", "eventTime": 1761936492000, "eventType": "bot.followed"},
  "event": {"userId": "10001", "nickname": "小明", "avatarUrl": "https://cdn.example.com/follow.png"}
}`

	store := avatar.NewStore()
	converter := newConverter()
	converter.Avatars = store

	if _, _, err := converter.Convert(context.Background(), parse(t, followed)); err != nil {
		t.Fatalf("转换失败: %v", err)
	}

	url, ok := store.Get(10001)
	if !ok || url != "https://cdn.example.com/follow.png" {
		t.Fatalf("记录的头像地址为 %q、%v", url, ok)
	}
}

func TestAvatarRecordingWithoutStore(t *testing.T) {
	// 兼容模式关闭时没有地址表，转换过程不应受影响。
	converter := newConverter()

	got, ok, err := converter.Convert(context.Background(), parse(t, realTextMessage))
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if !ok || got == nil {
		t.Fatal("事件应当照常发出")
	}
}
