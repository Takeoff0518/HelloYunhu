// SPDX-License-Identifier: GPL-3.0-or-later

package event

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Takeoff0518/HelloYunhu/internal/bridge/identity"
	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

// 真实事件：群里 @机器人 发 /diag，正文里提及后面带一个零宽空格。
const atMentionEvent = `{"version":"1.0","header":{"eventId":"0833235f32ee4cb9ab1b997ee7e886e4","eventType":"message.receive.normal","eventTime":1794713482041},"event":{"sender":{"senderId":"2272345","senderType":"user","senderUserLevel":"owner","senderNickname":"洛雨辰"},"chat":{"chatId":"695367192","chatType":"group"},"message":{"msgId":"dcfc49709b6745819d1bb3d9e260f57c","parentId":"","sendTime":1794713482029,"chatId":"695367192","chatType":"group","contentType":"text","content":{"text":"@乐正绫TwinkLing ​/diag","at":["11464576"]},"instructionId":0,"instructionName":"","commandId":0,"commandName":""}}}`

func TestAtMentionIsSplitOut(t *testing.T) {
	converter := &Converter{SelfID: 11464576, BotID: "11464576", Messages: identity.NewMemory()}

	var envelope model.Envelope
	if err := json.Unmarshal([]byte(atMentionEvent), &envelope); err != nil {
		t.Fatalf("解析事件失败: %v", err)
	}

	converted, ok, err := converter.Convert(context.Background(), envelope)
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if !ok {
		t.Fatal("事件应发给框架")
	}

	message, isMessage := converted.(*obmodel.MessageEvent)
	if !isMessage {
		t.Fatalf("事件类型为 %T", converted)
	}

	if len(message.Message) != 2 {
		t.Fatalf("消息段数量为 %d: %#v", len(message.Message), message.Message)
	}
	if message.Message[0].Type != "at" {
		t.Errorf("第一个消息段为 %q", message.Message[0].Type)
	}
	if text, isText := message.Message[1].Data.(obmodel.TextData); !isText || text.Text != "/diag" {
		t.Errorf("第二个消息段为 %#v", message.Message[1].Data)
	}

	raw, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("编码事件失败: %v", err)
	}
	t.Logf("事件 JSON: %s", raw)
}
