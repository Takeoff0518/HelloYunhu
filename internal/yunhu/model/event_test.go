// SPDX-License-Identifier: GPL-3.0-or-later

package model

import (
	"encoding/json"
	"errors"
	"testing"
)

// sampleEvent 是一条群聊文本消息，字段与调研报告第 2.3 节一致。
const sampleEvent = `{
  "version": "1.0",
  "header": {"eventId": "evt-1", "eventTime": 1761936492340, "eventType": "message.receive.normal"},
  "event": {
    "sender": {"senderId": "10001", "senderNickname": "小明", "senderUserLevel": "owner", "senderAvatarUrl": "https://cdn.example.com/a.png"},
    "chat": {"chatId": "20001", "chatType": "group"},
    "message": {
      "msgId": "0f1e2d3c4b5a69788796a5b4c3d2e1f0",
      "parentId": "",
      "sendTime": 1761936492000,
      "chatId": "20001",
      "chatType": "group",
      "contentType": "text",
      "content": {"text": "@机器人 你好", "at": ["10002"], "buttons": []}
    }
  }
}`

func TestUnmarshalEnvelope(t *testing.T) {
	var envelope Envelope
	if err := json.Unmarshal([]byte(sampleEvent), &envelope); err != nil {
		t.Fatalf("解析事件失败: %v", err)
	}

	if envelope.Header.EventType != EventTypeMessageNormal {
		t.Errorf("事件类型为 %q", envelope.Header.EventType)
	}
	if envelope.Header.EventID != "evt-1" {
		t.Errorf("事件标识为 %q", envelope.Header.EventID)
	}
	if envelope.Header.EventTime != 1761936492340 {
		t.Errorf("事件时间为 %d", envelope.Header.EventTime)
	}

	var event MessageEvent
	if err := json.Unmarshal(envelope.Event, &event); err != nil {
		t.Fatalf("解析消息事件失败: %v", err)
	}
	if event.Sender.SenderID != "10001" || event.Sender.SenderNickname != "小明" {
		t.Errorf("发送者为 %+v", event.Sender)
	}
	if event.Chat.ChatID != "20001" || event.Chat.ChatType != ChatTypeGroup {
		t.Errorf("会话为 %+v", event.Chat)
	}
	if event.Message.ContentType != ContentTypeText {
		t.Errorf("内容类型为 %q", event.Message.ContentType)
	}
}

func TestTextContent(t *testing.T) {
	var envelope Envelope
	if err := json.Unmarshal([]byte(sampleEvent), &envelope); err != nil {
		t.Fatalf("解析事件失败: %v", err)
	}

	var event MessageEvent
	if err := json.Unmarshal(envelope.Event, &event); err != nil {
		t.Fatalf("解析消息事件失败: %v", err)
	}

	content, err := event.Message.TextContent()
	if err != nil {
		t.Fatalf("解析文本内容失败: %v", err)
	}
	if content.Text != "@机器人 你好" {
		t.Errorf("正文为 %q", content.Text)
	}
	if len(content.At) != 1 || content.At[0] != "10002" {
		t.Errorf("被 @ 的用户为 %v", content.At)
	}
}

func TestTextContentRejectsOtherTypes(t *testing.T) {
	message := Message{ContentType: ContentTypeImage, Content: json.RawMessage(`{"imageUrl":"https://cdn.example.com/a.png"}`)}

	_, err := message.TextContent()
	if !errors.Is(err, ErrUnsupportedContent) {
		t.Fatalf("错误为 %v，期望 ErrUnsupportedContent", err)
	}
}

func TestSendMessageResultID(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "实测 messageInfo 结构", raw: `{"messageInfo":{"msgId":"ddd","recvId":"695367192","recvType":"group"}}`, want: "ddd"},
		{name: "msgId 字段", raw: `{"msgId":"aaa"}`, want: "aaa"},
		{name: "messageId 字段", raw: `{"messageId":"bbb"}`, want: "bbb"},
		{name: "字符串数据", raw: `"ccc"`, want: "ccc"},
		{name: "空对象", raw: `{}`, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &SendMessageResult{}
			if err := json.Unmarshal([]byte(tt.raw), result); err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if result.ID() != tt.want {
				t.Errorf("消息标识为 %q，期望 %q", result.ID(), tt.want)
			}
		})
	}
}
