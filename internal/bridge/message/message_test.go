// SPDX-License-Identifier: GPL-3.0-or-later

package message

import (
	"encoding/json"
	"strings"
	"testing"

	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

func TestIncomingText(t *testing.T) {
	content := &model.TextContent{Text: "@机器人 你好", At: []string{"10002"}}

	segments := IncomingText(content)
	if len(segments) != 2 {
		t.Fatalf("消息段数量为 %d，期望 2", len(segments))
	}
	if segments[0].Type != "at" {
		t.Errorf("第一个消息段为 %q", segments[0].Type)
	}
	at, ok := segments[0].Data.(obmodel.AtData)
	if !ok || at.QQ != 10002 {
		t.Errorf("@ 段为 %#v", segments[0].Data)
	}
	text, ok := segments[1].Data.(obmodel.TextData)
	if !ok || text.Text != "@机器人 你好" {
		t.Errorf("文本段为 %#v", segments[1].Data)
	}
}

func TestIncomingImage(t *testing.T) {
	segment := IncomingImage("base64://abc", "https://cdn.example.com/a.png", "a.png")

	if segment.Type != "image" {
		t.Fatalf("消息段类型为 %q", segment.Type)
	}
	data, ok := segment.Data.(obmodel.ImageData)
	if !ok {
		t.Fatalf("消息段数据为 %T", segment.Data)
	}
	if data.File != "base64://abc" || data.URL != "https://cdn.example.com/a.png" || data.Name != "a.png" {
		t.Errorf("图片段为 %+v", data)
	}
}

func TestIncomingPlaceholder(t *testing.T) {
	tests := []struct {
		contentType string
		want        string
	}{
		{contentType: model.ContentTypeImage, want: PlaceholderImage},
		{contentType: model.ContentTypeFile, want: PlaceholderFile},
		{contentType: model.ContentTypeVideo, want: PlaceholderVideo},
		{contentType: model.ContentTypeAudio, want: PlaceholderAudio},
		{contentType: "form", want: "[暂不支持的消息类型: form]"},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			segments := IncomingPlaceholder(tt.contentType)
			if len(segments) != 1 {
				t.Fatalf("消息段数量为 %d", len(segments))
			}
			data, ok := segments[0].Data.(obmodel.TextData)
			if !ok || data.Text != tt.want {
				t.Errorf("占位文本为 %#v，期望 %q", segments[0].Data, tt.want)
			}
		})
	}
}

// segment 构造测试用的收到消息段。
func segment(t *testing.T, typ string, data any) obmodel.InSegment {
	t.Helper()

	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("编码消息段失败: %v", err)
	}
	return obmodel.InSegment{Type: typ, Data: raw}
}

func TestSplitTextAtReply(t *testing.T) {
	parts, err := Split([]obmodel.InSegment{
		segment(t, "reply", obmodel.ReplyData{ID: 42}),
		segment(t, "at", obmodel.AtData{QQ: 10001}),
		segment(t, "text", obmodel.TextData{Text: "你好"}),
	})
	if err != nil {
		t.Fatalf("拆分失败: %v", err)
	}

	if len(parts) != 1 {
		t.Fatalf("段数为 %d，期望 1", len(parts))
	}
	if parts[0].ReplyMessageID != 42 {
		t.Errorf("引用目标为 %d", parts[0].ReplyMessageID)
	}
	if len(parts[0].At) != 1 || parts[0].At[0] != "10001" {
		t.Errorf("@ 列表为 %v", parts[0].At)
	}
	if parts[0].Text != "@10001 你好" {
		t.Errorf("正文为 %q", parts[0].Text)
	}
}

func TestSplitSplitsAroundImage(t *testing.T) {
	parts, err := Split([]obmodel.InSegment{
		segment(t, "reply", obmodel.ReplyData{ID: 7}),
		segment(t, "text", obmodel.TextData{Text: "看图"}),
		segment(t, "image", obmodel.ImageData{File: "https://example.com/a.png", Name: "a.png"}),
		segment(t, "text", obmodel.TextData{Text: "好看吗"}),
	})
	if err != nil {
		t.Fatalf("拆分失败: %v", err)
	}

	if len(parts) != 3 {
		t.Fatalf("段数为 %d，期望 3", len(parts))
	}

	if parts[0].Text != "看图" || parts[0].ReplyMessageID != 7 || parts[0].Media != nil {
		t.Errorf("第一段为 %+v", parts[0])
	}
	if parts[1].Media == nil {
		t.Fatalf("第二段应为媒体: %+v", parts[1])
	}
	if parts[1].Media.Kind != "image" || parts[1].Media.Source != "https://example.com/a.png" {
		t.Errorf("媒体段为 %+v", parts[1].Media)
	}
	if parts[2].Text != "好看吗" {
		t.Errorf("第三段正文为 %q", parts[2].Text)
	}
}

func TestSplitImageOnly(t *testing.T) {
	parts, err := Split([]obmodel.InSegment{
		segment(t, "image", obmodel.ImageData{File: "base64://abc"}),
	})
	if err != nil {
		t.Fatalf("拆分失败: %v", err)
	}
	if len(parts) != 1 || parts[0].Media == nil {
		t.Fatalf("段为 %+v", parts)
	}
	if parts[0].Text != "" || len(parts[0].At) != 0 {
		t.Errorf("媒体段不应带文本: %+v", parts[0])
	}
}

func TestSplitAtAsStringAndAll(t *testing.T) {
	parts, err := Split([]obmodel.InSegment{
		{Type: "at", Data: json.RawMessage(`{"qq":"10001"}`)},
		{Type: "at", Data: json.RawMessage(`{"qq":"all"}`)},
		segment(t, "text", obmodel.TextData{Text: "开会"}),
	})
	if err != nil {
		t.Fatalf("拆分失败: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("段数为 %d", len(parts))
	}
	if len(parts[0].At) != 1 || parts[0].At[0] != "10001" {
		t.Errorf("@ 列表为 %v", parts[0].At)
	}
	if !strings.Contains(parts[0].Text, "全体成员") {
		t.Errorf("正文为 %q", parts[0].Text)
	}
}

func TestSplitPlaceholders(t *testing.T) {
	parts, err := Split([]obmodel.InSegment{
		{Type: "file", Data: json.RawMessage(`{"file":"report.pdf"}`)},
		{Type: "face", Data: json.RawMessage(`{"id":"1"}`)},
		{Type: "node", Data: json.RawMessage(`{}`)},
	})
	if err != nil {
		t.Fatalf("拆分失败: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("段数为 %d", len(parts))
	}
	for _, want := range []string{PlaceholderFile, PlaceholderFace, "暂不支持的消息段: node"} {
		if !strings.Contains(parts[0].Text, want) {
			t.Errorf("正文 %q 中缺少 %q", parts[0].Text, want)
		}
	}
}

func TestSplitErrors(t *testing.T) {
	tests := []struct {
		name     string
		segments []obmodel.InSegment
	}{
		{name: "空消息", segments: nil},
		{name: "文本段损坏", segments: []obmodel.InSegment{{Type: "text", Data: json.RawMessage(`{"text":`)}}},
		{name: "@ 段缺少标识", segments: []obmodel.InSegment{{Type: "at", Data: json.RawMessage(`{}`)}}},
		{name: "图片段缺少 file", segments: []obmodel.InSegment{{Type: "image", Data: json.RawMessage(`{"name":"a.png"}`)}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Split(tt.segments); err == nil {
				t.Fatal("期望返回错误")
			}
		})
	}
}
