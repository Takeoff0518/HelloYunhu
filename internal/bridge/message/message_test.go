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
	// 提及文字已从正文剪掉，剩下的部分才能被框架当成命令。
	text, ok := segments[1].Data.(obmodel.TextData)
	if !ok || text.Text != "你好" {
		t.Errorf("文本段为 %#v", segments[1].Data)
	}
}

func TestIncomingTextMentionPosition(t *testing.T) {
	// 真实事件的正文形如「@机器人​/m2check」，提及后面会跟一个零宽空格。
	content := &model.TextContent{Text: "请看 @某人 这条", At: []string{"10003"}}

	segments := IncomingText(content)
	want := []string{"text", "at", "text"}
	if len(segments) != len(want) {
		t.Fatalf("消息段数量为 %d，期望 %d", len(segments), len(want))
	}
	for index, kind := range want {
		if segments[index].Type != kind {
			t.Errorf("第 %d 个消息段为 %q，期望 %q", index, segments[index].Type, kind)
		}
	}
	if text, _ := segments[0].Data.(obmodel.TextData); text.Text != "请看 " {
		t.Errorf("前置正文为 %q", text.Text)
	}
	if text, _ := segments[2].Data.(obmodel.TextData); text.Text != "这条" {
		t.Errorf("后置正文为 %q", text.Text)
	}
}

func TestIncomingTextKeepsTextWhenCountMismatch(t *testing.T) {
	// 正文里的 @ 数量与 at 数组对不上时，退回成 @ 段在前、正文原样保留。
	content := &model.TextContent{Text: "邮箱 a@b.com 与 @某人", At: []string{"10003"}}

	segments := IncomingText(content)
	if len(segments) != 2 || segments[0].Type != "at" || segments[1].Type != "text" {
		t.Fatalf("消息段为 %#v", segments)
	}
	if text, _ := segments[1].Data.(obmodel.TextData); text.Text != "邮箱 a@b.com 与 @某人" {
		t.Errorf("正文为 %q", text.Text)
	}
}

func TestMentionNames(t *testing.T) {
	content := &model.TextContent{Text: "@甲 与 @乙 都在", At: []string{"1", "2"}}

	pairs := MentionNames(content)
	if len(pairs) != 2 {
		t.Fatalf("昵称数量为 %d", len(pairs))
	}
	if pairs[0] != [2]string{"1", "甲"} || pairs[1] != [2]string{"2", "乙"} {
		t.Errorf("昵称为 %#v", pairs)
	}

	if got := MentionNames(&model.TextContent{Text: "没有提及", At: []string{"1"}}); got != nil {
		t.Errorf("没有提及时应返回空，实际为 %#v", got)
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
	// 内容已经内联，地址不再输出：客户端拿地址去下载会得到 403。
	if data.File != "base64://abc" || data.URL != "" || data.Name != "a.png" {
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
		segment(t, "reply", obmodel.ReplyData{ID: obmodel.IDValue(42)}),
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

func TestSplitReplyIDAsString(t *testing.T) {
	// NoneBot2 会把引用段的 id 序列化成字符串。
	parts, err := Split([]obmodel.InSegment{
		{Type: "reply", Data: json.RawMessage(`{"id":"42"}`)},
		segment(t, "text", obmodel.TextData{Text: "引用"}),
	})
	if err != nil {
		t.Fatalf("拆分失败: %v", err)
	}
	if parts[0].ReplyMessageID != 42 {
		t.Errorf("引用目标为 %d，期望 42", parts[0].ReplyMessageID)
	}
}

func TestSplitSplitsAroundImage(t *testing.T) {
	parts, err := Split([]obmodel.InSegment{
		segment(t, "reply", obmodel.ReplyData{ID: obmodel.IDValue(7)}),
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

func TestSplitImageWithReply(t *testing.T) {
	// 框架经常把引用与图片放在同一条消息里，例如 PatPat 的回复。
	// 只有引用没有正文时，引用应当挂到图片消息上，而不是被丢掉。
	parts, err := Split([]obmodel.InSegment{
		segment(t, "reply", obmodel.ReplyData{ID: obmodel.IDValue(41)}),
		segment(t, "image", obmodel.ImageData{File: "base64://abc"}),
	})
	if err != nil {
		t.Fatalf("拆分失败: %v", err)
	}

	if len(parts) != 1 {
		t.Fatalf("段数为 %d，期望 1", len(parts))
	}
	if parts[0].Media == nil {
		t.Fatalf("段应为媒体: %+v", parts[0])
	}
	if parts[0].ReplyMessageID != 41 {
		t.Errorf("引用目标为 %d，期望 41", parts[0].ReplyMessageID)
	}
	if parts[0].Text != "" {
		t.Errorf("媒体段不应带文本: %+v", parts[0])
	}
}

func TestSplitReplyStaysOnText(t *testing.T) {
	// 有正文时引用仍然挂到第一条文本消息上，图片不重复携带。
	parts, err := Split([]obmodel.InSegment{
		segment(t, "reply", obmodel.ReplyData{ID: obmodel.IDValue(41)}),
		segment(t, "text", obmodel.TextData{Text: "看图"}),
		segment(t, "image", obmodel.ImageData{File: "base64://abc"}),
	})
	if err != nil {
		t.Fatalf("拆分失败: %v", err)
	}

	if len(parts) != 2 {
		t.Fatalf("段数为 %d，期望 2", len(parts))
	}
	if parts[0].ReplyMessageID != 41 {
		t.Errorf("文本段的引用目标为 %d", parts[0].ReplyMessageID)
	}
	if parts[1].ReplyMessageID != 0 {
		t.Errorf("图片段不应再带引用，当前为 %d", parts[1].ReplyMessageID)
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
		{Type: "face", Data: json.RawMessage(`{"id":"1"}`)},
		{Type: "poke", Data: json.RawMessage(`{}`)},
	})
	if err != nil {
		t.Fatalf("拆分失败: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("段数为 %d", len(parts))
	}
	for _, want := range []string{PlaceholderFace, "暂不支持的消息段: poke"} {
		if !strings.Contains(parts[0].Text, want) {
			t.Errorf("正文 %q 中缺少 %q", parts[0].Text, want)
		}
	}
}

func TestSplitFileVideoRecord(t *testing.T) {
	parts, err := Split([]obmodel.InSegment{
		{Type: "file", Data: json.RawMessage(`{"file":"https://example.com/a.pdf","name":"a.pdf"}`)},
		{Type: "video", Data: json.RawMessage(`{"file":"base64://dmlkZW8="}`)},
		{Type: "record", Data: json.RawMessage(`{"file":"https://example.com/a.amr"}`)},
	})
	if err != nil {
		t.Fatalf("拆分失败: %v", err)
	}
	if len(parts) != 3 {
		t.Fatalf("段数为 %d，期望 3", len(parts))
	}

	want := []struct {
		kind   string
		source string
		name   string
	}{
		{kind: "file", source: "https://example.com/a.pdf", name: "a.pdf"},
		{kind: "video", source: "base64://dmlkZW8="},
		{kind: "record", source: "https://example.com/a.amr"},
	}

	for index, item := range want {
		media := parts[index].Media
		if media == nil {
			t.Fatalf("第 %d 段不是媒体: %+v", index, parts[index])
		}
		if media.Kind != item.kind || media.Source != item.source || media.Name != item.name {
			t.Errorf("第 %d 段为 %+v", index, *media)
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
