// SPDX-License-Identifier: GPL-3.0-or-later

package model

import "encoding/json"

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

// FileContent 解析文件内容。
func (m *Message) FileContent() (*FileContent, error) {
	if m.ContentType != ContentTypeFile {
		return nil, ErrUnsupportedContent
	}

	var content FileContent
	if err := json.Unmarshal(m.Content, &content); err != nil {
		return nil, err
	}
	return &content, nil
}

// VideoContent 是 contentType 为 video 时的消息内容。
type VideoContent struct {
	VideoURL      string `json:"videoUrl"`
	VideoDuration int    `json:"videoDuration"`
}

// VideoContent 解析视频内容。
func (m *Message) VideoContent() (*VideoContent, error) {
	if m.ContentType != ContentTypeVideo {
		return nil, ErrUnsupportedContent
	}

	var content VideoContent
	if err := json.Unmarshal(m.Content, &content); err != nil {
		return nil, err
	}
	return &content, nil
}

// AudioContent 是 contentType 为 audio 时的消息内容。
type AudioContent struct {
	AudioURL      string `json:"audioUrl"`
	AudioDuration int    `json:"audioDuration"`
}

// AudioContent 解析语音内容。
func (m *Message) AudioContent() (*AudioContent, error) {
	if m.ContentType != ContentTypeAudio {
		return nil, ErrUnsupportedContent
	}

	var content AudioContent
	if err := json.Unmarshal(m.Content, &content); err != nil {
		return nil, err
	}
	return &content, nil
}
