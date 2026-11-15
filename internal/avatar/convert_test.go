// SPDX-License-Identifier: GPL-3.0-or-later

package avatar

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"testing"
)

// tinyWebP 是一张 8×8 的红色 webp，由 ffmpeg 生成，用于验证转码。
const tinyWebP = "UklGRjwAAABXRUJQVlA4IDAAAADQAQCdASoIAAgAAgA0JaACdLoB+AADsAD+8Oj3/yC5YXXI1/8gP+QH/ID/+PIAAAA="

func tinyWebPBytes(t *testing.T) []byte {
	t.Helper()

	data, err := base64.StdEncoding.DecodeString(tinyWebP)
	if err != nil {
		t.Fatalf("解码 webp 样本失败: %v", err)
	}
	return data
}

func TestConvertWebPOpaque(t *testing.T) {
	result, err := convertWebP(tinyWebPBytes(t))
	if err != nil {
		t.Fatalf("转码失败: %v", err)
	}

	if result.contentType != "image/jpeg" {
		t.Errorf("内容类型为 %q，不透明的图片应转成 JPEG", result.contentType)
	}
	if _, err := jpeg.Decode(bytes.NewReader(result.data)); err != nil {
		t.Errorf("结果不是合法 JPEG: %v", err)
	}
}

func TestConvertWebPRejectsGarbage(t *testing.T) {
	if _, err := convertWebP([]byte("不是 webp")); err == nil {
		t.Fatal("无法解码的内容应返回错误")
	}
}

func TestHasAlpha(t *testing.T) {
	opaque := image.NewRGBA(image.Rect(0, 0, 1, 1))
	opaque.Set(0, 0, color.RGBA{R: 255, A: 255})
	if hasAlpha(opaque) {
		t.Error("全不透明的图片不应判定为带透明通道")
	}

	translucent := image.NewRGBA(image.Rect(0, 0, 1, 1))
	translucent.Set(0, 0, color.RGBA{R: 255, A: 128})
	if !hasAlpha(translucent) {
		t.Error("半透明像素应判定为带透明通道")
	}
}

func TestWebPProxyConvertsToJPEG(t *testing.T) {
	upstream, _ := newUpstream(t, tinyWebPBytes(t), "image/webp", http.StatusOK)

	store := NewStore()
	store.Put(10001, upstream.URL+"/a.jpg")

	recorder := httptest.NewRecorder()
	newProxy(store).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/avatar/10001", nil))

	if got := recorder.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("Content-Type 为 %q，期望转成 JPEG", got)
	}
	if _, err := jpeg.Decode(bytes.NewReader(recorder.Body.Bytes())); err != nil {
		t.Errorf("返回内容无法按 JPEG 解码: %v", err)
	}
}
