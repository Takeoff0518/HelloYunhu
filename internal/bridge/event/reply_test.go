// SPDX-License-Identifier: GPL-3.0-or-later

package event

import (
	"context"
	"encoding/json"
	"testing"

	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
)

// quotedMessage 是一条带引用的群聊消息，parentId 指向被引用的消息。
const quotedMessage = `{
  "version": "1.0",
  "header": {"eventId": "evt-quote", "eventTime": 1761936492000, "eventType": "message.receive.normal"},
  "event": {
    "sender": {"senderId": "10001", "senderNickname": "小明", "senderUserLevel": "owner"},
    "chat": {"chatId": "20001", "chatType": "group"},
    "message": {
      "msgId": "4f1e2d3c4b5a69788796a5b4c3d2e1f0",
      "parentId": "9f1e2d3c4b5a69788796a5b4c3d2e1f0",
      "sendTime": 1761936492000,
      "chatId": "20001",
      "chatType": "group",
      "contentType": "text",
      "content": {"text": "对", "at": []}
    }
  }
}`

// replyID 从转换后的事件里取出 reply 段的标识。
func replyID(t *testing.T, event *obmodel.MessageEvent) int64 {
	t.Helper()

	if len(event.Message) == 0 {
		t.Fatal("消息段为空")
	}
	if event.Message[0].Type != "reply" {
		t.Fatalf("首段为 %q，期望 reply", event.Message[0].Type)
	}

	data, ok := event.Message[0].Data.(obmodel.ReplyData)
	if !ok {
		t.Fatalf("引用段的数据为 %T", event.Message[0].Data)
	}
	return data.ID.Int64()
}

func TestQuotedMessageGetsReplySegment(t *testing.T) {
	converter := newConverter()

	got, ok, err := converter.Convert(context.Background(), parse(t, quotedMessage))
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if !ok {
		t.Fatal("事件应当发送给框架")
	}

	event := got.(*obmodel.MessageEvent)
	id := replyID(t, event)
	if id <= 0 {
		t.Errorf("引用标识为 %d", id)
	}

	// 再次转换同一条引用时，被引用消息的整数标识必须保持一致。
	again, _, err := converter.Convert(context.Background(), parse(t, quotedMessage))
	if err != nil {
		t.Fatalf("再次转换失败: %v", err)
	}
	if other := replyID(t, again.(*obmodel.MessageEvent)); other != id {
		t.Errorf("同一被引用消息的标识不稳定: %d 与 %d", id, other)
	}

	// 正文段仍在引用段之后。
	if len(event.Message) != 2 || event.Message[1].Type != "text" {
		t.Fatalf("消息段为 %+v", event.Message)
	}
}

func TestPlainMessageHasNoReplySegment(t *testing.T) {
	converter := newConverter()

	got, _, err := converter.Convert(context.Background(), parse(t, realTextMessage))
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}

	event := got.(*obmodel.MessageEvent)
	if len(event.Message) == 0 || event.Message[0].Type != "text" {
		t.Errorf("没有引用时首段应为文本: %+v", event.Message)
	}
}

func TestReplySegmentIgnoresEmptyParent(t *testing.T) {
	converter := newConverter()

	if segment := converter.replySegment(context.Background(), ""); segment != nil {
		t.Errorf("parentId 为空时不应生成引用段: %+v", segment)
	}
}

func TestReplySegmentForUnknownParentStillMaps(t *testing.T) {
	// 父消息不在映射表里时照样分配标识，框架至少能看出这条消息带了引用。
	converter := newConverter()

	segment := converter.replySegment(context.Background(), "unknown-parent-id")
	if segment == nil {
		t.Fatal("应生成引用段")
	}

	data, ok := segment.Data.(obmodel.ReplyData)
	if !ok || data.ID.Int64() <= 0 {
		t.Errorf("引用段数据为 %+v", segment.Data)
	}

	// 序列化后 id 是数字，框架按数字或字符串解析都能接受。
	raw, err := json.Marshal(segment)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if string(raw) == "" {
		t.Error("序列化结果为空")
	}
}
