// SPDX-License-Identifier: GPL-3.0-or-later

package model

import "testing"

func TestResolveMediaURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		base string
		want string
	}{
		{
			name: "文件只给相对路径",
			raw:  "c839e60da61443b482164ce5d22d6eaf.txt",
			base: mediaFileBase,
			want: "https://chat-file.jwznb.com/c839e60da61443b482164ce5d22d6eaf.txt",
		},
		{
			name: "视频只给相对路径",
			raw:  "845f229e78d252f55380a1aaffbd61ef.mp4",
			base: mediaVideoBase,
			want: "https://chat-video1.jwznb.com/845f229e78d252f55380a1aaffbd61ef.mp4",
		},
		{
			name: "语音只给相对路径",
			raw:  "1ef16d49336db863bdb61e5f52d0fea4.wav",
			base: mediaAudioBase,
			want: "https://chat-audio1.jwznb.com/1ef16d49336db863bdb61e5f52d0fea4.wav",
		},
		{name: "前导斜杠", raw: "/a/b.mp4", base: mediaVideoBase, want: "https://chat-video1.jwznb.com/a/b.mp4"},
		{name: "已是完整地址", raw: "https://cdn.example.com/a.png", base: mediaFileBase, want: "https://cdn.example.com/a.png"},
		{name: "空地址", raw: "", base: mediaFileBase, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveMediaURL(tt.raw, tt.base); got != tt.want {
				t.Errorf("结果为 %q，期望 %q", got, tt.want)
			}
		})
	}
}

func TestVideoContentResolvesURL(t *testing.T) {
	message := &Message{
		ContentType: ContentTypeVideo,
		Content:     []byte(`{"etag":"lqGDvJdeesUQhg5EnmPZbHTuziO0","videoUrl":"845f229e78d252f55380a1aaffbd61ef.mp4","videoDuration":10}`),
	}

	content, err := message.VideoContent()
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if content.VideoDuration != 10 {
		t.Errorf("视频时长为 %d", content.VideoDuration)
	}
	if content.VideoURL != "https://chat-video1.jwznb.com/845f229e78d252f55380a1aaffbd61ef.mp4" {
		t.Errorf("视频地址为 %q", content.VideoURL)
	}
}

func TestAudioContentResolvesURL(t *testing.T) {
	// 结构取自群里的真实语音事件。
	message := &Message{
		ContentType: ContentTypeAudio,
		Content:     []byte(`{"etag":"Fl5mxUHChiXYYb14kwRcTphq0fOr","audioUrl":"1ef16d49336db863bdb61e5f52d0fea4.wav","audioDuration":1}`),
	}

	content, err := message.AudioContent()
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if content.AudioDuration != 1 {
		t.Errorf("语音时长为 %d", content.AudioDuration)
	}
	if content.AudioURL != "https://chat-audio1.jwznb.com/1ef16d49336db863bdb61e5f52d0fea4.wav" {
		t.Errorf("语音地址为 %q", content.AudioURL)
	}
}

func TestFileContentResolvesURL(t *testing.T) {
	// 结构取自群里的真实文件事件。
	message := &Message{
		ContentType: ContentTypeFile,
		Content:     []byte(`{"fileName":"hello-yunhu(3).txt","fileUrl":"c839e60da61443b482164ce5d22d6eaf.txt","etag":"FvMpoya0gU76yX9mQY8_l7f6k2qs","fileSize":34}`),
	}

	content, err := message.FileContent()
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if content.FileName != "hello-yunhu(3).txt" || content.FileSize != 34 {
		t.Errorf("文件内容为 %+v", content)
	}
	if content.FileURL != "https://chat-file.jwznb.com/c839e60da61443b482164ce5d22d6eaf.txt" {
		t.Errorf("文件地址为 %q", content.FileURL)
	}
}
