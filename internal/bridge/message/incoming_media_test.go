// SPDX-License-Identifier: GPL-3.0-or-later

package message

import (
	"testing"

	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
)

const (
	cdnURL    = "https://chat-img.jwznb.com/7ce5a9d210af25b78d2fdc783b56c38f.jpg"
	base64URL = "base64:///9j/4AAQSkZJRgABAQ"
)

// mediaURL 取出各类型媒体段里的网络地址字段。
func mediaURL(t *testing.T, segment obmodel.OutSegment) string {
	t.Helper()

	switch data := segment.Data.(type) {
	case obmodel.ImageData:
		return data.URL
	case obmodel.FileData:
		return data.URL
	case obmodel.VideoData:
		return data.URL
	case obmodel.RecordData:
		return data.URL
	default:
		t.Fatalf("未知的媒体段类型 %T", segment.Data)
		return ""
	}
}

func TestIncomingMediaDropsURLWhenInlined(t *testing.T) {
	// 内容已经内联成 base64 时，地址只会让框架去下载一个 403 的链接。
	segments := []obmodel.OutSegment{
		IncomingImage(base64URL, cdnURL, "a.jpg"),
		IncomingFile(base64URL, cdnURL, "a.txt"),
		IncomingVideo(base64URL, cdnURL),
		IncomingAudio(base64URL, cdnURL),
	}

	for _, segment := range segments {
		if got := mediaURL(t, segment); got != "" {
			t.Errorf("%s 段在内容内联时不应带地址，当前为 %q", segment.Type, got)
		}
	}
}

func TestIncomingMediaKeepsURLWhenNotInlined(t *testing.T) {
	// 下载失败、只能给出地址的降级路径必须保留地址，否则框架什么都拿不到。
	segments := []obmodel.OutSegment{
		IncomingImage(cdnURL, cdnURL, "a.jpg"),
		IncomingFile(cdnURL, cdnURL, "a.txt"),
		IncomingVideo(cdnURL, cdnURL),
		IncomingAudio(cdnURL, cdnURL),
	}

	for _, segment := range segments {
		if got := mediaURL(t, segment); got != cdnURL {
			t.Errorf("%s 段应保留地址，当前为 %q", segment.Type, got)
		}
	}
}

func TestInlineURL(t *testing.T) {
	if got := inlineURL(base64URL, cdnURL); got != "" {
		t.Errorf("内联时返回 %q，期望空", got)
	}
	if got := inlineURL(cdnURL, cdnURL); got != cdnURL {
		t.Errorf("未内联时返回 %q，期望保留原地址", got)
	}
}
