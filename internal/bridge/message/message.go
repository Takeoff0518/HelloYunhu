// SPDX-License-Identifier: GPL-3.0-or-later

// Package message 负责 OneBot 消息段与云湖消息之间的转换。
//
// 云湖一条消息只有一个内容类型，OneBot 的一条消息可以包含多个消息段，因此两个
// 方向的转换都不是一一对应：接收方向需要把内容拆成消息段，发送方向需要拆分后
// 多次发送。首版处理文本、@、引用与图片，其余类型降级为占位文本，保证用户至少
// 能看到提示。
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
// 因此 @ 段统一放在正文之前。取得群成员信息后可以还原 @ 在正文中的位置。
func IncomingText(content *model.TextContent) obmodel.OutMessage {
	segments := make(obmodel.OutMessage, 0, len(content.At)+1)

	for _, raw := range content.At {
		qq, _ := identity.ParseID(raw)
		segments = append(segments, obmodel.At(qq))
	}

	segments = append(segments, obmodel.Text(content.Text))
	return segments
}

// IncomingImage 构造图片消息段。
//
// AstrBot 在发送图片时使用 base64，接收时同样接受 base64 与网络地址。调用方
// 优先给出 base64，下载失败时退化为网络地址。
func IncomingImage(file, url, name string) obmodel.OutSegment {
	return obmodel.OutSegment{
		Type: "image",
		Data: obmodel.ImageData{File: file, URL: url, Name: name},
	}
}

// IncomingFile 构造文件消息段。
func IncomingFile(file, url, name string) obmodel.OutSegment {
	return obmodel.OutSegment{
		Type: "file",
		Data: obmodel.FileData{File: file, URL: url, Name: name},
	}
}

// IncomingVideo 构造视频消息段。
func IncomingVideo(file, url string) obmodel.OutSegment {
	return obmodel.OutSegment{
		Type: "video",
		Data: obmodel.VideoData{File: file, URL: url},
	}
}

// IncomingAudio 构造语音消息段。
func IncomingAudio(file, url string) obmodel.OutSegment {
	return obmodel.OutSegment{
		Type: "record",
		Data: obmodel.RecordData{File: file, URL: url},
	}
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

// MediaPart 描述一条待上传的媒体消息。
type MediaPart struct {
	// Kind 是消息段类型，目前只有 image。
	Kind string
	// Source 是 OneBot 消息段给出的 file 值，可能是 base64、网络地址或本地路径。
	Source string
	// Name 是建议的文件名，为空时由读取环节决定。
	Name string
}

// Part 是拆分后的一段待发送内容，对应一条云湖消息。
type Part struct {
	// Text 是消息正文，@ 与占位文本都写在正文里。
	Text string
	// At 是被 @ 的用户标识列表，作为云湖的 content.at 传递。
	At []string
	// ReplyMessageID 是引用回复的目标消息，0 表示没有引用。
	ReplyMessageID int64
	// Media 非空时表示这一段是媒体消息，Text 为空。
	Media *MediaPart
}

// Split 把 OneBot 消息段拆成若干待发送部分。
//
// 连续的文本、@ 与引用合并成一条文本消息；图片单独成为一条媒体消息；其余类型
// 按占位文本并入文本消息。任何一个消息段解析失败都返回错误，由调用方如实回给
// 框架，而不是发出残缺的消息。
func Split(segments []obmodel.InSegment) ([]Part, error) {
	var (
		parts   []Part
		current Part
		builder strings.Builder
	)

	flush := func() {
		if builder.Len() == 0 && len(current.At) == 0 {
			return
		}
		current.Text = builder.String()
		parts = append(parts, current)
		current = Part{}
		builder.Reset()
	}

	for index, segment := range segments {
		switch segment.Type {
		case "text":
			var data obmodel.TextData
			if err := json.Unmarshal(segment.Data, &data); err != nil {
				return nil, fmt.Errorf("第 %d 个文本段解析失败: %w", index, err)
			}
			builder.WriteString(data.Text)

		case "at":
			qq, all, err := parseAtSegment(segment.Data)
			if err != nil {
				return nil, fmt.Errorf("第 %d 个 @ 段解析失败: %w", index, err)
			}
			if all {
				builder.WriteString("@全体成员")
				continue
			}
			current.At = append(current.At, strconv.FormatInt(qq, 10))
			// 云湖要求正文里出现 @昵称，没有成员信息时先用标识代替。
			builder.WriteString("@" + strconv.FormatInt(qq, 10))
			builder.WriteString(" ")

		case "reply":
			var data obmodel.ReplyData
			if err := json.Unmarshal(segment.Data, &data); err != nil {
				return nil, fmt.Errorf("第 %d 个引用段解析失败: %w", index, err)
			}
			// 引用的目标只挂在当前这条消息上，避免一次发送多段时每段都带引用。
			if current.ReplyMessageID == 0 {
				current.ReplyMessageID = data.ID
			}

		case "image", "file", "video", "record":
			source, name, err := parseMediaSegment(segment, index)
			if err != nil {
				return nil, err
			}
			flush()
			parts = append(parts, Part{Media: &MediaPart{Kind: segment.Type, Source: source, Name: name}})

		case "face", "mface", "dice", "rps":
			builder.WriteString(PlaceholderFace)
		case "json", "xml", "markdown", "node", "forward", "poke":
			builder.WriteString("[暂不支持的消息段: " + segment.Type + "]")
		default:
			builder.WriteString("[未知消息段: " + segment.Type + "]")
		}
	}

	flush()

	if len(parts) == 0 {
		return nil, fmt.Errorf("消息内容为空")
	}

	return parts, nil
}

// parseMediaSegment 解析媒体段的来源与文件名。
func parseMediaSegment(segment obmodel.InSegment, index int) (string, string, error) {
	var data struct {
		File string `json:"file"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(segment.Data, &data); err != nil {
		return "", "", fmt.Errorf("第 %d 个 %s 段解析失败: %w", index, segment.Type, err)
	}
	if data.File == "" {
		return "", "", fmt.Errorf("第 %d 个 %s 段缺少 file", index, segment.Type)
	}

	return data.File, data.Name, nil
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
