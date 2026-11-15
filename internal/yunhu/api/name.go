// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"net/http"
	"path"
	"strings"
)

// defaultUploadName 是取不到文件名时使用的名字。后缀由内容决定，这里不能只看
// 名字，否则图片会被云湖当成普通文件拒绝。
const defaultUploadName = "upload.bin"

// mediaExtensions 把内容探测结果映射到云湖要求的文件名后缀。
//
// 云湖按文件名后缀判断上传类型：图片接口只接受图片后缀，视频接口只接受 mp4，
// 后缀与接口不匹配时返回 code -1 与“文件类型必须为图片”。内容也要是真的媒体，
// 两边都符合才收。
var mediaExtensions = map[string]string{
	"image/png":    ".png",
	"image/jpeg":   ".jpg",
	"image/gif":    ".gif",
	"image/webp":   ".webp",
	"image/bmp":    ".bmp",
	"image/x-icon": ".ico",
	"video/mp4":    ".mp4",
}

// fileName 返回上传时使用的文件名。
//
// OneBot 侧给出的媒体经常没有可用后缀：base64 内容在读取环节统一叫 upload.bin，
// 部分框架的图片标识是随机字符串，网络地址也可能没有后缀。这些名字会让云湖判错
// 类型，因此上传前按内容补齐后缀，并去掉名字里的目录部分，只把文件名发给平台。
func fileName(name string, data []byte) string {
	// 本地路径的形式可能是 E:\work\a.png，两种分隔符都要处理。
	if index := strings.LastIndexAny(name, `/\`); index >= 0 {
		name = name[index+1:]
	}
	if name == "" {
		name = defaultUploadName
	}

	ext := mediaExtensions[http.DetectContentType(data)]
	if ext == "" {
		return name
	}
	if strings.EqualFold(path.Ext(name), ext) {
		return name
	}

	return strings.TrimSuffix(name, path.Ext(name)) + ext
}
