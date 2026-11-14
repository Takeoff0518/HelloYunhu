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

func TestLoadEmptySource(t *testing.T) {
	if _, _, err := (Loader{}).Load(context.Background(), "  "); err == nil {
		t.Fatal("空来源应返回错误")
	}
}
