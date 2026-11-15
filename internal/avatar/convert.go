// SPDX-License-Identifier: GPL-3.0-or-later

package avatar

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/webp"
)

// jpegQuality 是把 webp 转成 JPEG 时使用的质量。
//
// 云湖头像的实际内容是 webp，而 mirai 侧的插件普遍用 Java 的 ImageIO 读图，
// 它不认识 webp，读到的是 null。1080×1080 的头像转 PNG 会接近 1 MB，转 JPEG
// 只有 80 KB 左右，因此不透明时用 JPEG，带透明通道时退回 PNG。
const jpegQuality = 85

// convertWebP 把 webp 转成客户端能解码的格式。
func convertWebP(data []byte) (imagePayload, error) {
	img, err := webp.Decode(bytes.NewReader(data))
	if err != nil {
		return imagePayload{}, err
	}

	var buffer bytes.Buffer
	if hasAlpha(img) {
		if err := png.Encode(&buffer, img); err != nil {
			return imagePayload{}, err
		}
		return imagePayload{data: buffer.Bytes(), contentType: "image/png"}, nil
	}

	if err := jpeg.Encode(&buffer, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return imagePayload{}, err
	}
	return imagePayload{data: buffer.Bytes(), contentType: "image/jpeg"}, nil
}

// hasAlpha 判断图片是否带有非全不透明的像素。
func hasAlpha(img image.Image) bool {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if _, _, _, alpha := img.At(x, y).RGBA(); alpha != 0xffff {
				return true
			}
		}
	}
	return false
}
