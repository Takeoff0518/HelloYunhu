// SPDX-License-Identifier: GPL-3.0-or-later

// Package cqcode 实现 CQ 码与消息段之间的相互转换。
//
// CQ 码是 OneBot v11 的字符串消息格式，例如 [CQ:at,qq=123]。字符串里出现特殊
// 字符时用实体转义：& 写成 &amp;，[ 写成 &#91;，] 写成 &#93;，逗号写成 &#44;。
// 数组格式与字符串格式不能混用，转换只在入口与出口各做一次。
package cqcode

import (
	"sort"
	"strings"
)

// TextType 是文本段的类型名。
const TextType = "text"

// 转义用的实体。
const (
	amp   = "&amp;"
	lb    = "&#91;"
	rb    = "&#93;"
	comma = "&#44;"
)

// prefix 是 CQ 码的固定前缀。
const prefix = "[CQ:"

// Segment 是解析后的消息段，数据用键值对表示。
type Segment struct {
	// Type 是消息段类型，例如 text、at、image。
	Type string
	// Data 是消息段的参数，文本段只有 text 一个键。
	Data map[string]string
}

// Text 构造文本段。
func Text(text string) Segment {
	return Segment{Type: TextType, Data: map[string]string{"text": text}}
}

// Parse 把字符串消息解析为消息段。
//
// 合法的 CQ 码转成消息段，其余内容按文本处理；没有闭合方括号的片段也当作文本，
// 不会吞掉用户输入。
func Parse(message string) []Segment {
	var (
		segments []Segment
		text     strings.Builder
	)

	flush := func() {
		if text.Len() == 0 {
			return
		}
		segments = append(segments, Text(Unescape(text.String())))
		text.Reset()
	}

	for index := 0; index < len(message); {
		if strings.HasPrefix(message[index:], prefix) {
			if end := strings.IndexByte(message[index+len(prefix):], ']'); end >= 0 {
				flush()
				body := message[index+len(prefix) : index+len(prefix)+end]
				segments = append(segments, parseCQ(body))
				index += len(prefix) + end + 1
				continue
			}
		}

		text.WriteByte(message[index])
		index++
	}

	flush()

	if len(segments) == 0 {
		return []Segment{Text("")}
	}

	return segments
}

// Encode 把消息段编码为字符串消息。
func Encode(segments []Segment) string {
	var builder strings.Builder

	for _, segment := range segments {
		if segment.Type == TextType || segment.Type == "" {
			builder.WriteString(Escape(segment.Data["text"]))
			continue
		}

		builder.WriteString(prefix)
		builder.WriteString(segment.Type)

		keys := make([]string, 0, len(segment.Data))
		for key := range segment.Data {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			builder.WriteByte(',')
			builder.WriteString(Escape(key))
			builder.WriteByte('=')
			builder.WriteString(Escape(segment.Data[key]))
		}

		builder.WriteByte(']')
	}

	return builder.String()
}

// Escape 转义字符串中的特殊字符。
func Escape(text string) string {
	replacer := strings.NewReplacer(
		"&", amp,
		"[", lb,
		"]", rb,
		",", comma,
	)
	return replacer.Replace(text)
}

// Unescape 还原转义字符。&amp; 必须最后处理，否则会出现二次还原。
func Unescape(text string) string {
	replacer := strings.NewReplacer(
		lb, "[",
		rb, "]",
		comma, ",",
		amp, "&",
	)
	return replacer.Replace(text)
}

// parseCQ 解析 CQ 码的主体，例如 at,qq=123。
func parseCQ(body string) Segment {
	parts := strings.Split(body, ",")
	segment := Segment{Type: Unescape(parts[0]), Data: make(map[string]string, len(parts)-1)}

	for _, part := range parts[1:] {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			// 缺少等号的参数无法解释，按空值保留，避免整条消息失败。
			segment.Data[Unescape(key)] = ""
			continue
		}
		segment.Data[Unescape(key)] = Unescape(value)
	}

	return segment
}
