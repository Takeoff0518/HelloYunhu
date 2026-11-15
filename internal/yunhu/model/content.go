// SPDX-License-Identifier: GPL-3.0-or-later

package model

import (
	"encoding/json"
	"strings"
)

// 媒体文件的基础地址。
//
// 图片内容给的是完整地址，文件、视频与语音只给一个以扩展名结尾的路径，例如
// c839e60da61443b482164ce5d22d6eaf.txt、845f229e78d252f55380a1aaffbd61ef.mp4 与
// 1ef16d49336db863bdb61e5f52d0fea4.wav，需要拼到对应的 CDN 上才能下载。三类地址
// 实测都返回 200，且都要求来源头，缺少时返回 403。
const (
	mediaFileBase  = "https://chat-file.jwznb.com/"
	mediaVideoBase = "https://chat-video1.jwznb.com/"
	mediaAudioBase = "https://chat-audio1.jwznb.com/"
)

// resolveMediaURL 把相对路径按给定基础地址拼成完整地址，已是完整地址时原样返回。
func resolveMediaURL(raw, base string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	return base + strings.TrimLeft(raw, "/")
}

// ImageContent 是 contentType 为 image 时的消息内容。
type ImageContent struct {
	// ImageURL 是图片地址。
	ImageURL string `json:"imageUrl"`
	// ImageName 是文件名。
	ImageName string `json:"imageName"`
	// ImageKey 是平台内部的图片标识。
	ImageKey string `json:"imageKey"`
	// Width 与 Height 是图片尺寸，像素。
	Width  int `json:"imageWidth"`
	Height int `json:"imageHeight"`
}

// ImageContent 解析图片内容。内容类型不是 image 时返回 ErrUnsupportedContent。
func (m *Message) ImageContent() (*ImageContent, error) {
	if m.ContentType != ContentTypeImage {
		return nil, ErrUnsupportedContent
	}

	var content ImageContent
	if err := json.Unmarshal(m.Content, &content); err != nil {
		return nil, err
	}
	if content.ImageURL == "" {
		return nil, errImageURLMissing
	}

	return &content, nil
}

// errImageURLMissing 表示图片内容缺少地址。
var errImageURLMissing = &missingFieldError{field: "imageUrl"}

// missingFieldError 表示响应缺少必需字段。
type missingFieldError struct {
	field string
}

// Error 实现 error 接口。
func (e *missingFieldError) Error() string {
	return "缺少字段 " + e.field
}

// FileContent 是 contentType 为 file 时的消息内容。
type FileContent struct {
	FileName string `json:"fileName"`
	FileURL  string `json:"fileUrl"`
	FileKey  string `json:"fileKey"`
	FileSize int64  `json:"fileSize"`
}

// FileContent 解析文件内容。地址为相对路径时会拼成完整地址。
func (m *Message) FileContent() (*FileContent, error) {
	if m.ContentType != ContentTypeFile {
		return nil, ErrUnsupportedContent
	}

	var content FileContent
	if err := json.Unmarshal(m.Content, &content); err != nil {
		return nil, err
	}
	content.FileURL = resolveMediaURL(content.FileURL, mediaFileBase)

	return &content, nil
}

// VideoContent 是 contentType 为 video 时的消息内容。
type VideoContent struct {
	VideoURL      string `json:"videoUrl"`
	VideoDuration int    `json:"videoDuration"`
}

// VideoContent 解析视频内容。地址为相对路径时会拼成完整地址。
func (m *Message) VideoContent() (*VideoContent, error) {
	if m.ContentType != ContentTypeVideo {
		return nil, ErrUnsupportedContent
	}

	var content VideoContent
	if err := json.Unmarshal(m.Content, &content); err != nil {
		return nil, err
	}
	content.VideoURL = resolveMediaURL(content.VideoURL, mediaVideoBase)

	return &content, nil
}

// AudioContent 是 contentType 为 audio 时的消息内容。
type AudioContent struct {
	AudioURL      string `json:"audioUrl"`
	AudioDuration int    `json:"audioDuration"`
}

// AudioContent 解析语音内容。地址为相对路径时会拼成完整地址。
func (m *Message) AudioContent() (*AudioContent, error) {
	if m.ContentType != ContentTypeAudio {
		return nil, ErrUnsupportedContent
	}

	var content AudioContent
	if err := json.Unmarshal(m.Content, &content); err != nil {
		return nil, err
	}
	content.AudioURL = resolveMediaURL(content.AudioURL, mediaAudioBase)

	return &content, nil
}
