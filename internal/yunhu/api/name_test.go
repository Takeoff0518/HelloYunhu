// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"testing"
)

// testPNG 返回一张 1×1 的 PNG，用于判断上传文件名是否补齐了后缀。
func testPNG(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("编码 PNG 失败: %v", err)
	}
	return buffer.Bytes()
}

// testJPEG 返回一张 1×1 的 JPEG。
func testJPEG(t *testing.T) []byte {
	t.Helper()

	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 1, 1)), nil); err != nil {
		t.Fatalf("编码 JPEG 失败: %v", err)
	}
	return buffer.Bytes()
}

// testMP4 返回一段能让内容探测识别为 video/mp4 的文件头。
func testMP4() []byte {
	return []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom")
}

func TestFileName(t *testing.T) {
	pngData := testPNG(t)
	jpegData := testJPEG(t)
	textData := []byte("这不是图片")

	cases := []struct {
		name string
		in   string
		data []byte
		want string
	}{
		{"base64 内容补上图片后缀", "upload.bin", pngData, "upload.png"},
		{"后缀已经正确时保持不变", "a.PNG", pngData, "a.PNG"},
		{"后缀与内容不符时按内容改写", "a.bin", jpegData, "a.jpg"},
		{"jpeg 写法统一成 jpg", "a.jpeg", jpegData, "a.jpg"},
		{"视频内容补上 mp4 后缀", "upload.bin", testMP4(), "upload.mp4"},
		{"本地路径只保留文件名", `E:\work\a.png`, pngData, "a.png"},
		{"类 Unix 路径只保留文件名", "/tmp/a.png", pngData, "a.png"},
		{"名字为空时给出默认名", "", pngData, "upload.png"},
		{"探测不出媒体类型时保持原样", "upload.bin", textData, "upload.bin"},
		{"探测不出媒体类型且没有名字", "", textData, "upload.bin"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := fileName(c.in, c.data); got != c.want {
				t.Errorf("fileName(%q) 返回 %q，期望 %q", c.in, got, c.want)
			}
		})
	}
}

func TestUploadImageFixesExtension(t *testing.T) {
	pngData := testPNG(t)

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("解析上传表单失败: %v", err)
		}
		file, header, err := r.FormFile("image")
		if err != nil {
			t.Fatalf("读取上传文件失败: %v", err)
		}
		defer func() {
			_ = file.Close()
		}()

		if header.Filename != "upload.png" {
			t.Errorf("文件名为 %q，期望 upload.png", header.Filename)
		}
		if content, err := io.ReadAll(file); err != nil || !bytes.Equal(content, pngData) {
			t.Errorf("上传内容与原图不一致: %v", err)
		}

		_, _ = io.WriteString(w, `{"code":1,"data":{"imageKey":"key-1"}}`)
	})

	if _, err := client.UploadImage(context.Background(), "upload.bin", pngData); err != nil {
		t.Fatalf("UploadImage 返回错误: %v", err)
	}
}

func TestUploadVideoFixesExtension(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("解析上传表单失败: %v", err)
		}
		_, header, err := r.FormFile("video")
		if err != nil {
			t.Fatalf("读取上传文件失败: %v", err)
		}

		if header.Filename != "upload.mp4" {
			t.Errorf("文件名为 %q，期望 upload.mp4", header.Filename)
		}

		_, _ = io.WriteString(w, `{"code":1,"data":{"videoKey":"key-2"}}`)
	})

	if _, err := client.UploadVideo(context.Background(), "upload.bin", testMP4()); err != nil {
		t.Fatalf("UploadVideo 返回错误: %v", err)
	}
}
