// SPDX-License-Identifier: GPL-3.0-or-later

// Package message 负责 OneBot 消息段与云湖消息之间的转换。
//
// 云湖一条消息只有一个内容类型，OneBot 的一条消息可以包含多个消息段，因此两个
// 方向的转换都不是一一对应：接收方向需要把内容拆成消息段，发送方向需要拆分后
// 多次发送。首版只处理文本与 @，媒体在 M2 接入，其余类型降级为占位文本，保证
// 用户至少能看到提示。
package message

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/Takeoff0518/HelloYunhu/internal/bridge/identity"
	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

// 降级用的占位文本。
const (
	PlaceholderImage = "[图片]"
	PlaceholderFile  = "[文件]"
	PlaceholderVideo = "[视频]"
	PlaceholderAudio = "[语音]"
	PlaceholderFace  = "[表情]"
	PlaceholderReply = "[引用]"
)

// IncomingText 把云湖文本消息转换为 OneBot 消息段。
//
// 云湖的 @ 只记录用户标识，正文里是被 @ 的人的昵称，两者没有直接对应关系，
// 因此 @ 段统一放在正文之前。M2 取得群成员信息后可以还原 @ 在正文中的位置。
func IncomingText(content *model.TextContent) obmodel.OutMessage {
	segments := make(obmodel.OutMessage, 0, len(content.At)+1)

	for _, raw := range content.At {
		qq, _ := identity.ParseID(raw)
		segments = append(segments, obmodel.At(qq))
	}

	segments = append(segments, obmodel.Text(content.Text))
	return segments
}

// IncomingPlaceholder 为暂不支持的内容类型生成占位文本。
func IncomingPlaceholder(contentType string) obmodel.OutMessage {
	var text string

	switch contentType {
	case model.ContentTypeImage:
		text = PlaceholderImage
	case model.ContentTypeFile:
		text = PlaceholderFile
	case model.ContentTypeVideo:
		text = PlaceholderVideo
	case model.ContentTypeAudio:
		text = PlaceholderAudio
	default:
		text = "[暂不支持的消息类型: " + contentType + "]"
	}

	return obmodel.OutMessage{obmodel.Text(text)}
}

// OutgoingMessage 是由 OneBot 消息段整理出的待发送内容。
type OutgoingMessage struct {
	// Text 是消息正文，@ 与占位文本都写在正文里。
	Text string
	// At 是被 @ 的用户标识列表，作为云湖的 content.at 传递。
	At []string
	// ReplyMessageID 是引用回复的目标消息，0 表示没有引用。
	ReplyMessageID int64
}

// FromOneBot 把 OneBot 消息段整理成一条云湖文本消息。
//
// 首版只处理文本、@ 与引用，其余类型写成占位文本；任何一个消息段解析失败都返回
// 错误，由调用方如实回给框架，而不是发出一条残缺的消息。
func FromOneBot(segments []obmodel.InSegment) (OutgoingMessage, error) {
	var result OutgoingMessage
	var builder strings.Builder

	for index, segment := range segments {
		switch segment.Type {
		case "text":
			var data obmodel.TextData
			if err := json.Unmarshal(segment.Data, &data); err != nil {
				return OutgoingMessage{}, fmt.Errorf("第 %d 个文本段解析失败: %w", index, err)
			}
			builder.WriteString(data.Text)

		case "at":
			qq, all, err := parseAtSegment(segment.Data)
			if err != nil {
				return OutgoingMessage{}, fmt.Errorf("第 %d 个 @ 段解析失败: %w", index, err)
			}
			if all {
				builder.WriteString("@全体成员")
				continue
			}
			result.At = append(result.At, strconv.FormatInt(qq, 10))
			// 云湖要求正文里出现 @昵称，没有成员信息时先用标识代替。
			builder.WriteString("@" + strconv.FormatInt(qq, 10))
			builder.WriteString(" ")

		case "reply":
			var data obmodel.ReplyData
			if err := json.Unmarshal(segment.Data, &data); err != nil {
				return OutgoingMessage{}, fmt.Errorf("第 %d 个引用段解析失败: %w", index, err)
			}
			if result.ReplyMessageID == 0 {
				result.ReplyMessageID = data.ID
			}

		case "image":
			builder.WriteString(PlaceholderImage)
		case "file":
			builder.WriteString(PlaceholderFile)
		case "video":
			builder.WriteString(PlaceholderVideo)
		case "record":
			builder.WriteString(PlaceholderAudio)
		case "face", "mface", "dice", "rps":
			builder.WriteString(PlaceholderFace)
		case "json", "xml", "markdown", "node", "forward", "poke":
			builder.WriteString("[暂不支持的消息段: " + segment.Type + "]")
		default:
			builder.WriteString("[未知消息段: " + segment.Type + "]")
		}
	}

	result.Text = builder.String()
	if result.Text == "" && len(result.At) == 0 {
		return OutgoingMessage{}, fmt.Errorf("消息内容为空")
	}

	return result, nil
}

// parseAtSegment 解析 @ 段的用户标识。
func parseAtSegment(raw json.RawMessage) (int64, bool, error) {
	var data struct {
		QQ json.RawMessage `json:"qq"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return 0, false, err
	}

	var text string
	if err := json.Unmarshal(data.QQ, &text); err == nil {
		if text == "all" {
			return 0, true, nil
		}
		qq, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return 0, false, fmt.Errorf("@ 的用户标识 %q 不是整数", text)
		}
		return qq, false, nil
	}

	var qq int64
	if err := json.Unmarshal(data.QQ, &qq); err != nil {
		return 0, false, fmt.Errorf("@ 段缺少用户标识: %w", err)
	}
	return qq, false, nil
}
