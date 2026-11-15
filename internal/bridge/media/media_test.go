// SPDX-License-Identifier: GPL-3.0-or-later

package media

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadBase64(t *testing.T) {
	loader := Loader{}

	name, data, err := loader.Load(context.Background(), "base64://"+base64.StdEncoding.EncodeToString([]byte("hello")))
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("内容为 %q", data)
	}
	if !strings.HasSuffix(name, ".bin") {
		t.Errorf("文件名为 %q", name)
	}
}

func TestLoadBase64WithoutPadding(t *testing.T) {
	loader := Loader{}

	_, data, err := loader.Load(context.Background(), "base64://"+base64.RawStdEncoding.EncodeToString([]byte("hi")))
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if string(data) != "hi" {
		t.Errorf("内容为 %q", data)
	}
}

func TestLoadBrokenBase64(t *testing.T) {
	if _, _, err := (Loader{}).Load(context.Background(), "base64://不是 base64"); err == nil {
		t.Fatal("非法 base64 应返回错误")
	}
}

func TestLoadLocalFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.png")
	if err := os.WriteFile(path, []byte("png-bytes"), 0o600); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}

	loader := Loader{}

	if _, data, err := loader.Load(context.Background(), path); err != nil || string(data) != "png-bytes" {
		t.Fatalf("读取路径失败: %v, %q", err, data)
	}
	if _, data, err := loader.Load(context.Background(), "file://"+path); err != nil || string(data) != "png-bytes" {
		t.Fatalf("读取 file:// 失败: %v, %q", err, data)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, _, err := (Loader{}).Load(context.Background(), filepath.Join(t.TempDir(), "absent.png")); err == nil {
		t.Fatal("文件不存在应返回错误")
	}
}

func TestLoadURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/a.png" {
			t.Errorf("请求路径为 %q", r.URL.Path)
		}
		_, _ = io.WriteString(w, "image-bytes")
	}))
	defer server.Close()

	name, data, err := (Loader{}).Load(context.Background(), server.URL+"/a.png")
	if err != nil {
		t.Fatalf("下载失败: %v", err)
	}
	if string(data) != "image-bytes" {
		t.Errorf("内容为 %q", data)
	}
	if name != "a.png" {
		t.Errorf("文件名为 %q", name)
	}
}

func TestLoadURLTooLarge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 128)))
	}))
	defer server.Close()

	loader := Loader{MaxBytes: 32}
	_, _, err := loader.Load(context.Background(), server.URL+"/big.png")
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("错误为 %v，期望 ErrTooLarge", err)
	}
}

func TestLoadURLBadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	if _, _, err := (Loader{}).Load(context.Background(), server.URL+"/denied.png"); err == nil {
		t.Fatal("非 200 响应应返回错误")
	}
}

func TestRefererFor(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "云湖图片域名", source: "https://chat-img.jwznb.com/a.jpg", want: yunhuReferer},
		{name: "云湖头像域名", source: "https://chat-img3.jwznb.com/a.webp", want: yunhuReferer},
		{name: "云湖主域名", source: "https://ws.jwzhd.com/a.png", want: yunhuReferer},
		{name: "其它站点", source: "https://example.com/a.png", want: ""},
		{name: "本地路径", source: "a.png", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := refererFor(tt.source); got != tt.want {
				t.Errorf("来源为 %q，期望 %q", got, tt.want)
			}
		})
	}
}

func TestLoadEmptySource(t *testing.T) {
	if _, _, err := (Loader{}).Load(context.Background(), "  "); err == nil {
		t.Fatal("空来源应返回错误")
	}
}

func TestLocalPath(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "框架给出的三斜杠写法", source: "/E:/work/image.png", want: "E:/work/image.png"},
		{name: "小写盘符", source: "/c:/temp/a.txt", want: "c:/temp/a.txt"},
		{name: "没有盘符时保持不变", source: "/var/tmp/a.txt", want: "/var/tmp/a.txt"},
		{name: "本身就是本机路径", source: "E:/work/image.png", want: "E:/work/image.png"},
		{name: "短路径不变", source: "/a", want: "/a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := localPath(tt.source); got != tt.want {
				t.Errorf("结果为 %q，期望 %q", got, tt.want)
			}
		})
	}
}

func TestLoadFileURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("file url"), 0o600); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}

	// 框架常用 file:/// 加绝对路径的形式，斜杠统一转成正斜杠。
	source := "file:///" + strings.TrimPrefix(strings.ReplaceAll(path, "\\", "/"), "/")
	if _, data, err := (Loader{}).Load(context.Background(), source); err != nil {
		t.Fatalf("读取失败: %v", err)
	} else if string(data) != "file url" {
		t.Errorf("内容为 %q", data)
	}
}
