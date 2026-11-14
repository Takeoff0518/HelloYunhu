// SPDX-License-Identifier: GPL-3.0-or-later

package cqcode

import (
	"reflect"
	"testing"
)

func TestParsePlainText(t *testing.T) {
	segments := Parse("你好，世界")

	if len(segments) != 1 || segments[0].Type != TextType || segments[0].Data["text"] != "你好，世界" {
		t.Fatalf("解析结果为 %+v", segments)
	}
}

func TestParseMixed(t *testing.T) {
	segments := Parse("[CQ:at,qq=123] 你好[CQ:image,file=a.png]")

	want := []Segment{
		{Type: "at", Data: map[string]string{"qq": "123"}},
		Text(" 你好"),
		{Type: "image", Data: map[string]string{"file": "a.png"}},
	}
	if !reflect.DeepEqual(segments, want) {
		t.Fatalf("解析结果为 %+v，期望 %+v", segments, want)
	}
}

func TestParseEscapedBracketsAsText(t *testing.T) {
	segments := Parse("&#91;CQ:at,qq=1&#93; 是转义后的文本")

	if len(segments) != 1 {
		t.Fatalf("解析结果为 %+v", segments)
	}
	if got := segments[0].Data["text"]; got != "[CQ:at,qq=1] 是转义后的文本" {
		t.Errorf("文本为 %q", got)
	}
}

func TestParseEscapedCommaInValue(t *testing.T) {
	segments := Parse("[CQ:image,file=a&#44;b.png,url=https://example.com/a.png]")

	if len(segments) != 1 || segments[0].Type != "image" {
		t.Fatalf("解析结果为 %+v", segments)
	}
	if got := segments[0].Data["file"]; got != "a,b.png" {
		t.Errorf("file 参数为 %q", got)
	}
}

func TestParseUnclosedBracketStaysText(t *testing.T) {
	segments := Parse("[CQ:at,qq=123 没有闭合")

	if len(segments) != 1 || segments[0].Type != TextType {
		t.Fatalf("解析结果为 %+v", segments)
	}
	if got := segments[0].Data["text"]; got != "[CQ:at,qq=123 没有闭合" {
		t.Errorf("文本为 %q", got)
	}
}

func TestParseEmpty(t *testing.T) {
	segments := Parse("")

	if len(segments) != 1 || segments[0].Type != TextType || segments[0].Data["text"] != "" {
		t.Fatalf("解析结果为 %+v", segments)
	}
}

func TestParseMissingEquals(t *testing.T) {
	segments := Parse("[CQ:face,id]")

	if len(segments) != 1 || segments[0].Type != "face" {
		t.Fatalf("解析结果为 %+v", segments)
	}
	if value, ok := segments[0].Data["id"]; !ok || value != "" {
		t.Errorf("参数为 %+v", segments[0].Data)
	}
}

func TestEncode(t *testing.T) {
	tests := []struct {
		name     string
		segments []Segment
		want     string
	}{
		{
			name:     "纯文本",
			segments: []Segment{Text("你好")},
			want:     "你好",
		},
		{
			name:     "文本与 @",
			segments: []Segment{Text("你好"), {Type: "at", Data: map[string]string{"qq": "123"}}},
			want:     "你好[CQ:at,qq=123]",
		},
		{
			name:     "参数按字母序",
			segments: []Segment{{Type: "image", Data: map[string]string{"url": "https://example.com/a.png", "file": "a.png"}}},
			want:     "[CQ:image,file=a.png,url=https://example.com/a.png]",
		},
		{
			name:     "转义特殊字符",
			segments: []Segment{Text("[CQ:at,qq=1] & 逗号,")},
			// 逗号在文本里也一并转义，这样同一套规则可以用于正文与参数。
			want: "&#91;CQ:at&#44;qq=1&#93; &amp; 逗号&#44;",
		},
		{
			name:     "没有参数",
			segments: []Segment{{Type: "face"}},
			want:     "[CQ:face]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Encode(tt.segments); got != tt.want {
				t.Errorf("编码结果为 %q，期望 %q", got, tt.want)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	original := "[CQ:at,qq=123] 你好[CQ:image,file=a.png]"

	if got := Encode(Parse(original)); got != original {
		t.Errorf("往返结果为 %q，期望 %q", got, original)
	}
}

func TestUnescapeOrder(t *testing.T) {
	// 先还原 &#44; 再还原 &amp;，否则 &amp;#44; 会被二次还原。
	if got := Unescape("&amp;&#44;"); got != "&," {
		t.Errorf("还原结果为 %q", got)
	}
	if got := Escape("&[,]"); got != "&amp;&#91;&#44;&#93;" {
		t.Errorf("转义结果为 %q", got)
	}
}
