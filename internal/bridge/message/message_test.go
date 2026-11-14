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

func TestFromOneBotTextAtReply(t *testing.T) {
	segments := []obmodel.InSegment{
		segment(t, "reply", obmodel.ReplyData{ID: 42}),
		segment(t, "at", obmodel.AtData{QQ: 10001}),
		segment(t, "text", obmodel.TextData{Text: "你好"}),
	}

	out, err := FromOneBot(segments)
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if out.ReplyMessageID != 42 {
		t.Errorf("引用目标为 %d，期望 42", out.ReplyMessageID)
	}
	if len(out.At) != 1 || out.At[0] != "10001" {
		t.Errorf("@ 列表为 %v", out.At)
	}
	if out.Text != "@10001 你好" {
		t.Errorf("正文为 %q", out.Text)
	}
}

func TestFromOneBotAtAsString(t *testing.T) {
	segments := []obmodel.InSegment{
		{Type: "at", Data: json.RawMessage(`{"qq":"10001"}`)},
		segment(t, "text", obmodel.TextData{Text: "你好"}),
	}

	out, err := FromOneBot(segments)
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if len(out.At) != 1 || out.At[0] != "10001" {
		t.Errorf("@ 列表为 %v", out.At)
	}
}

func TestFromOneBotAtAll(t *testing.T) {
	segments := []obmodel.InSegment{
		{Type: "at", Data: json.RawMessage(`{"qq":"all"}`)},
		segment(t, "text", obmodel.TextData{Text: "开会了"}),
	}

	out, err := FromOneBot(segments)
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if len(out.At) != 0 {
		t.Errorf("@ 列表应为空，实际为 %v", out.At)
	}
	if !strings.Contains(out.Text, "全体成员") {
		t.Errorf("正文为 %q，期望包含全体成员", out.Text)
	}
}

func TestFromOneBotPlaceholders(t *testing.T) {
	segments := []obmodel.InSegment{
		{Type: "image", Data: json.RawMessage(`{"file":"base64://..."}`)},
		{Type: "file", Data: json.RawMessage(`{"file":"report.pdf"}`)},
		{Type: "face", Data: json.RawMessage(`{"id":"1"}`)},
		{Type: "node", Data: json.RawMessage(`{}`)},
	}

	out, err := FromOneBot(segments)
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	for _, want := range []string{PlaceholderImage, PlaceholderFile, PlaceholderFace, "暂不支持的消息段: node"} {
		if !strings.Contains(out.Text, want) {
			t.Errorf("正文 %q 中缺少 %q", out.Text, want)
		}
	}
}

func TestFromOneBotErrors(t *testing.T) {
	tests := []struct {
		name     string
		segments []obmodel.InSegment
	}{
		{
			name:     "空消息",
			segments: nil,
		},
		{
			name:     "文本段损坏",
			segments: []obmodel.InSegment{{Type: "text", Data: json.RawMessage(`{"text":`)}},
		},
		{
			name:     "@ 段缺少标识",
			segments: []obmodel.InSegment{{Type: "at", Data: json.RawMessage(`{}`)}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := FromOneBot(tt.segments); err == nil {
				t.Fatal("期望返回错误")
			}
		})
	}
}
