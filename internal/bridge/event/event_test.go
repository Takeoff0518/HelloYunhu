// SPDX-License-Identifier: GPL-3.0-or-later

package event

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Takeoff0518/HelloYunhu/internal/bridge/identity"
	"github.com/Takeoff0518/HelloYunhu/internal/bridge/media"
	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

// groupMessage 是一条群聊文本消息事件。
const groupMessage = `{
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

// privateMessage 是一条私聊文本消息事件。
const privateMessage = `{
  "version": "1.0",
  "header": {"eventId": "evt-2", "eventTime": 1761936492000, "eventType": "message.receive.normal"},
  "event": {
    "sender": {"senderId": "10001", "senderNickname": "小明"},
    "chat": {"chatId": "1001", "chatType": "bot"},
    "message": {
      "msgId": "1f1e2d3c4b5a69788796a5b4c3d2e1f0",
      "chatId": "1001",
      "chatType": "bot",
      "contentType": "text",
      "content": {"text": "在吗", "at": []}
    }
  }
}`

// imageMessage 是一条图片消息，用于验证降级。
const imageMessage = `{
  "version": "1.0",
  "header": {"eventId": "evt-3", "eventTime": 1761936492000, "eventType": "message.receive.normal"},
  "event": {
    "sender": {"senderId": "10001", "senderNickname": "小明"},
    "chat": {"chatId": "20001", "chatType": "group"},
    "message": {
      "msgId": "2f1e2d3c4b5a69788796a5b4c3d2e1f0",
      "chatId": "20001",
      "chatType": "group",
      "contentType": "image",
      "content": {"imageUrl": "https://cdn.example.com/a.png"}
    }
  }
}`

// newConverter 创建转换器。
func newConverter() *Converter {
	return &Converter{SelfID: 1001, BotID: "1001", Messages: identity.NewMemory()}
}

// parse 解析事件外壳。
func parse(t *testing.T, raw string) model.Envelope {
	t.Helper()

	var envelope model.Envelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("解析事件失败: %v", err)
	}
	return envelope
}

// realTextMessage 是真实服务端推送的群聊文本事件，用于锁定字段类型。
//
// 两个容易看错的地方：senderUserLevel 是 "owner" 这类字符串，不是等级数字；
// instructionId 与 commandId 位于 message 内，且是数字。
const realTextMessage = `{
  "version": "1.0",
  "header": {"eventId": "evt-real", "eventTime": 1794708523781, "eventType": "message.receive.normal"},
  "event": {
    "sender": {"senderId": "2272345", "senderType": "user", "senderUserLevel": "owner", "senderNickname": "洛雨辰", "senderAvatarUrl": "https://cdn.example.com/a.webp"},
    "chat": {"chatId": "695367192", "chatType": "group"},
    "message": {
      "msgId": "def103e3220742d79a3f6dd6ddb98309",
      "parentId": "",
      "sendTime": 1794708523770,
      "chatId": "695367192",
      "chatType": "group",
      "contentType": "text",
      "content": {"text": "12345"},
      "instructionId": 0,
      "instructionName": "",
      "commandId": 0,
      "commandName": ""
    }
  }
}`

func TestConvertRealTextMessage(t *testing.T) {
	converter := newConverter()

	got, ok, err := converter.Convert(context.Background(), parse(t, realTextMessage))
	if err != nil {
		t.Fatalf("真实事件解析失败: %v", err)
	}
	if !ok {
		t.Fatal("真实事件应当发送给框架")
	}

	event := got.(*obmodel.MessageEvent)
	if event.GroupID != 695367192 || event.UserID != 2272345 {
		t.Errorf("标识为 %d/%d", event.GroupID, event.UserID)
	}
	if event.Sender.Role != "owner" {
		t.Errorf("角色为 %q，期望 owner", event.Sender.Role)
	}
	if event.RawMessage != "12345" {
		t.Errorf("原始消息为 %q", event.RawMessage)
	}
	if len(event.Message) != 1 || event.Message[0].Type != "text" {
		t.Errorf("消息段为 %+v", event.Message)
	}
}

// realInstructionMessage 是云湖客户端用 /quote 指令发消息时推送的事件，正文只有参数。
const realInstructionMessage = `{
  "version": "1.0",
  "header": {"eventId": "evt-instr", "eventTime": 1794710180623, "eventType": "message.receive.instruction"},
  "event": {
    "sender": {"senderId": "2272345", "senderType": "user", "senderUserLevel": "owner", "senderNickname": "洛雨辰"},
    "chat": {"chatId": "695367192", "chatType": "group"},
    "message": {
      "msgId": "9580332887de4380bc9a808030f9a8a2",
      "chatId": "695367192",
      "chatType": "group",
      "contentType": "text",
      "content": {"text": "test"},
      "instructionId": 2755,
      "instructionName": "/quote",
      "commandId": 2755,
      "commandName": "/quote"
    }
  }
}`

func TestConvertInstructionMessage(t *testing.T) {
	converter := newConverter()

	got, ok, err := converter.Convert(context.Background(), parse(t, realInstructionMessage))
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if !ok {
		t.Fatal("指令消息应当发送给框架")
	}

	event := got.(*obmodel.MessageEvent)

	// 指令名拼回正文，框架才能按正文匹配命令。
	if event.RawMessage != "/quote test" {
		t.Errorf("原始消息为 %q，期望 /quote test", event.RawMessage)
	}
	data, isText := event.Message[0].Data.(obmodel.TextData)
	if !isText || data.Text != "/quote test" {
		t.Errorf("消息段为 %#v", event.Message[0].Data)
	}
	if event.Yunhu == nil || event.Yunhu.Instruction != "/quote" {
		t.Errorf("扩展字段为 %+v", event.Yunhu)
	}
}

func TestConvertGroupMessage(t *testing.T) {
	converter := newConverter()

	got, ok, err := converter.Convert(context.Background(), parse(t, groupMessage))
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if !ok {
		t.Fatal("群消息应当发送给框架")
	}

	event, isMessage := got.(*obmodel.MessageEvent)
	if !isMessage {
		t.Fatalf("事件类型为 %T", got)
	}

	if event.PostType != obmodel.PostTypeMessage || event.MessageType != obmodel.MessageTypeGroup {
		t.Errorf("事件类型为 %s/%s", event.PostType, event.MessageType)
	}
	if event.GroupID != 20001 || event.UserID != 10001 {
		t.Errorf("群标识为 %d，用户标识为 %d", event.GroupID, event.UserID)
	}
	if event.MessageID != 1 {
		t.Errorf("消息标识为 %d，期望 1", event.MessageID)
	}
	if event.Time != 1761936492 {
		t.Errorf("事件时间为 %d", event.Time)
	}
	if event.RawMessage != "@机器人 你好" {
		t.Errorf("原始消息为 %q", event.RawMessage)
	}
	if event.Sender.Nickname != "小明" || event.Sender.UserID != 10001 {
		t.Errorf("发送者为 %+v", event.Sender)
	}
	if len(event.Message) != 2 {
		t.Fatalf("消息段数量为 %d", len(event.Message))
	}
	if event.Message[0].Type != "at" {
		t.Errorf("第一个消息段为 %q", event.Message[0].Type)
	}
	if event.Yunhu == nil || event.Yunhu.MsgID != "0f1e2d3c4b5a69788796a5b4c3d2e1f0" {
		t.Errorf("扩展字段为 %+v", event.Yunhu)
	}
}

func TestConvertPrivateMessage(t *testing.T) {
	converter := newConverter()

	got, ok, err := converter.Convert(context.Background(), parse(t, privateMessage))
	if err != nil || !ok {
		t.Fatalf("转换结果为 %v, %v", ok, err)
	}

	event := got.(*obmodel.MessageEvent)
	if event.MessageType != obmodel.MessageTypePrivate || event.SubType != "friend" {
		t.Errorf("事件类型为 %s/%s", event.MessageType, event.SubType)
	}
	if event.GroupID != 0 {
		t.Errorf("私聊事件不应带群标识，实际为 %d", event.GroupID)
	}
	if event.UserID != 10001 {
		t.Errorf("用户标识为 %d", event.UserID)
	}
}

// TestConvertImageWithoutLoader 说明没有媒体读取器时图片段直接给出网络地址。
func TestConvertImageWithoutLoader(t *testing.T) {
	converter := newConverter()

	got, ok, err := converter.Convert(context.Background(), parse(t, imageMessage))
	if err != nil || !ok {
		t.Fatalf("转换结果为 %v, %v", ok, err)
	}

	event := got.(*obmodel.MessageEvent)
	if len(event.Message) != 1 {
		t.Fatalf("消息段数量为 %d", len(event.Message))
	}
	data, isImage := event.Message[0].Data.(obmodel.ImageData)
	if !isImage {
		t.Fatalf("消息段为 %T", event.Message[0].Data)
	}
	if data.File != "https://cdn.example.com/a.png" || data.URL != "https://cdn.example.com/a.png" {
		t.Errorf("图片段为 %+v", data)
	}
	if event.RawMessage != "[图片]" {
		t.Errorf("原始消息为 %q", event.RawMessage)
	}
}

// TestConvertImageInlinesBase64 说明配置了媒体读取器时图片会下载并转成 base64。
func TestConvertImageInlinesBase64(t *testing.T) {
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("image-bytes"))
	}))
	defer imageServer.Close()

	payload := strings.ReplaceAll(imageMessage, "https://cdn.example.com/a.png", imageServer.URL+"/a.png")

	converter := newConverter()
	converter.Media = &media.Loader{HTTPClient: imageServer.Client()}

	got, ok, err := converter.Convert(context.Background(), parse(t, payload))
	if err != nil || !ok {
		t.Fatalf("转换结果为 %v, %v", ok, err)
	}

	event := got.(*obmodel.MessageEvent)
	data, isImage := event.Message[0].Data.(obmodel.ImageData)
	if !isImage {
		t.Fatalf("消息段为 %T", event.Message[0].Data)
	}
	if data.File != "base64://"+base64.StdEncoding.EncodeToString([]byte("image-bytes")) {
		t.Errorf("图片段为 %+v", data)
	}
	if data.URL != imageServer.URL+"/a.png" {
		t.Errorf("原始地址为 %q", data.URL)
	}
}

// TestConvertImageFallbackToURL 说明下载失败时退化为地址而不是丢掉消息。
func TestConvertImageFallbackToURL(t *testing.T) {
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer imageServer.Close()

	payload := strings.ReplaceAll(imageMessage, "https://cdn.example.com/a.png", imageServer.URL+"/a.png")

	converter := newConverter()
	converter.Media = &media.Loader{HTTPClient: imageServer.Client()}

	got, ok, err := converter.Convert(context.Background(), parse(t, payload))
	if err != nil || !ok {
		t.Fatalf("转换结果为 %v, %v", ok, err)
	}

	event := got.(*obmodel.MessageEvent)
	data, isImage := event.Message[0].Data.(obmodel.ImageData)
	if !isImage {
		t.Fatalf("消息段为 %T", event.Message[0].Data)
	}
	if data.File != imageServer.URL+"/a.png" {
		t.Errorf("图片段为 %+v", data)
	}
}

func TestConvertMediaSegments(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		wantType    string
		wantFileURL string
	}{
		{
			name:        "文件",
			payload:     `{"fileUrl":"https://cdn.example.com/a.pdf","fileName":"a.pdf"}`,
			wantType:    "file",
			wantFileURL: "https://cdn.example.com/a.pdf",
		},
		{
			name:        "视频",
			payload:     `{"videoUrl":"https://cdn.example.com/a.mp4"}`,
			wantType:    "video",
			wantFileURL: "https://cdn.example.com/a.mp4",
		},
		{
			name:        "语音",
			payload:     `{"audioUrl":"https://cdn.example.com/a.amr"}`,
			wantType:    "record",
			wantFileURL: "https://cdn.example.com/a.amr",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contentType := map[string]string{
				"文件": "file",
				"视频": "video",
				"语音": "audio",
			}[tt.name]

			raw := `{
				"version": "1.0",
				"header": {"eventId": "evt-media", "eventTime": 1761936492000, "eventType": "message.receive.normal"},
				"event": {
					"sender": {"senderId": "10001", "senderNickname": "小明"},
					"chat": {"chatId": "20001", "chatType": "group"},
					"message": {
						"msgId": "9f1e2d3c4b5a69788796a5b4c3d2e1f0",
						"chatId": "20001",
						"chatType": "group",
						"contentType": "` + contentType + `",
						"content": ` + tt.payload + `
					}
				}
			}`

			converter := newConverter()
			got, ok, err := converter.Convert(context.Background(), parse(t, raw))
			if err != nil || !ok {
				t.Fatalf("转换结果为 %v, %v", ok, err)
			}

			event := got.(*obmodel.MessageEvent)
			if len(event.Message) != 1 || event.Message[0].Type != tt.wantType {
				t.Fatalf("消息段为 %+v", event.Message)
			}

			var file string
			switch data := event.Message[0].Data.(type) {
			case obmodel.FileData:
				file = data.File
			case obmodel.VideoData:
				file = data.File
			case obmodel.RecordData:
				file = data.File
			default:
				t.Fatalf("消息段数据为 %T", event.Message[0].Data)
			}
			if file != tt.wantFileURL {
				t.Errorf("文件为 %q，期望 %q", file, tt.wantFileURL)
			}
		})
	}
}

func TestConvertSkipsSelfMessage(t *testing.T) {
	converter := newConverter()
	converter.BotID = "10001"

	_, ok, err := converter.Convert(context.Background(), parse(t, groupMessage))
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if ok {
		t.Fatal("机器人自己发出的消息应当跳过")
	}
}

func TestConvertSkipsNoticeForNow(t *testing.T) {
	converter := newConverter()

	envelope := parse(t, `{"header":{"eventType":"bot.followed"},"event":{"userId":"10001"}}`)
	_, ok, err := converter.Convert(context.Background(), envelope)
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if ok {
		t.Fatal("尚未实现的通知事件应当跳过")
	}
}

func TestConvertBrokenPayload(t *testing.T) {
	converter := newConverter()

	envelope := parse(t, `{"header":{"eventType":"message.receive.normal"},"event":{}}`)
	_, ok, err := converter.Convert(context.Background(), envelope)
	if err == nil {
		t.Fatal("缺少消息字段时应返回错误")
	}
	if ok {
		t.Fatal("解析失败时不应发送事件")
	}
}

func TestMessageIDsAreStable(t *testing.T) {
	converter := newConverter()
	ctx := context.Background()

	first, _, err := converter.Convert(ctx, parse(t, groupMessage))
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	second, _, err := converter.Convert(ctx, parse(t, groupMessage))
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}

	firstID := first.(*obmodel.MessageEvent).MessageID
	secondID := second.(*obmodel.MessageEvent).MessageID
	if firstID != secondID {
		t.Errorf("同一条消息得到了不同的整数: %d 与 %d", firstID, secondID)
	}
}

func TestLifecycle(t *testing.T) {
	now := time.Unix(1761936492, 0)
	event := Lifecycle(1001, now)

	if event.PostType != obmodel.PostTypeMetaEvent || event.MetaEventType != obmodel.MetaEventTypeLifecycle {
		t.Errorf("事件为 %+v", event)
	}
	if event.SubType != "connect" {
		t.Errorf("子类型为 %q", event.SubType)
	}
	if event.Time != now.Unix() || event.SelfID != 1001 {
		t.Errorf("时间为 %d，标识为 %d", event.Time, event.SelfID)
	}
}
