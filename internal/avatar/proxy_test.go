// SPDX-License-Identifier: GPL-3.0-or-later

package avatar

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
)

// testPNG 返回一张 2×2 的 PNG，用于判断代理是否原样转发。
func testPNG(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("编码 PNG 失败: %v", err)
	}
	return buffer.Bytes()
}

// newUpstream 启动一个提供头像的模拟 CDN，并返回它收到的请求次数。
func newUpstream(t *testing.T, data []byte, contentType string, status int) (*httptest.Server, *int32) {
	t.Helper()

	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", contentType)
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(server.Close)

	return server, &hits
}

// newProxy 构造一个只允许访问 127.0.0.1 的代理，测试用地址都指向本地模拟服务。
func newProxy(store *Store) *Proxy {
	return &Proxy{Store: store, Hosts: []string{"127.0.0.1"}}
}

func TestProxyServesAvatar(t *testing.T) {
	pngData := testPNG(t)
	upstream, hits := newUpstream(t, pngData, "image/png", http.StatusOK)

	store := NewStore()
	store.Put(10001, upstream.URL+"/a.png")
	proxy := newProxy(store)

	recorder := httptest.NewRecorder()
	proxy.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/avatar/10001", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码为 %d", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type 为 %q", got)
	}
	if !bytes.Equal(recorder.Body.Bytes(), pngData) {
		t.Error("返回的内容与上游不一致")
	}

	// 第二次请求走缓存，不应再访问上游。
	proxy.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/avatar/10001", nil))
	if got := atomic.LoadInt32(hits); got != 1 {
		t.Errorf("上游收到 %d 次请求，期望 1 次", got)
	}
}

func TestProxyFallsBackToPlaceholder(t *testing.T) {
	pngData := testPNG(t)

	cases := []struct {
		name        string
		status      int
		contentType string
		body        []byte
	}{
		{name: "上游返回 403", status: http.StatusForbidden, contentType: "text/html", body: pngData},
		{name: "上游返回网页", status: http.StatusOK, contentType: "text/html", body: []byte("<html></html>")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			upstream, _ := newUpstream(t, c.body, c.contentType, c.status)
			store := NewStore()
			store.Put(10001, upstream.URL+"/a.png")

			recorder := httptest.NewRecorder()
			newProxy(store).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/avatar/10001", nil))

			if recorder.Code != http.StatusOK {
				t.Fatalf("状态码为 %d，期望回退成功", recorder.Code)
			}
			if got := recorder.Header().Get("Content-Type"); got != "image/png" {
				t.Errorf("Content-Type 为 %q", got)
			}
			if !bytes.Equal(recorder.Body.Bytes(), placeholderBytes()) {
				t.Error("应返回占位图")
			}
		})
	}
}

func TestProxyUnknownUser(t *testing.T) {
	recorder := httptest.NewRecorder()
	newProxy(NewStore()).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/avatar/10001", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码为 %d", recorder.Code)
	}
	if !bytes.Equal(recorder.Body.Bytes(), placeholderBytes()) {
		t.Error("未知用户应返回占位图")
	}
}

func TestProxyRejectsForeignHost(t *testing.T) {
	store := NewStore()
	store.Put(10001, "http://example.com/a.png")

	recorder := httptest.NewRecorder()
	newProxy(store).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/avatar/10001", nil))

	if !bytes.Equal(recorder.Body.Bytes(), placeholderBytes()) {
		t.Error("不在允许域名内的地址不应被取回")
	}
}

func TestProxyRejectsOversize(t *testing.T) {
	pngData := testPNG(t)
	upstream, _ := newUpstream(t, pngData, "image/png", http.StatusOK)

	store := NewStore()
	store.Put(10001, upstream.URL+"/a.png")

	proxy := newProxy(store)
	proxy.MaxBytes = 8

	recorder := httptest.NewRecorder()
	proxy.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/avatar/10001", nil))

	if !bytes.Equal(recorder.Body.Bytes(), placeholderBytes()) {
		t.Error("超过体积上限时应返回占位图")
	}
}

func TestProxyMethodNotAllowed(t *testing.T) {
	recorder := httptest.NewRecorder()
	newProxy(NewStore()).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/avatar/10001", nil))

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("状态码为 %d，期望 405", recorder.Code)
	}
}

func TestParsePath(t *testing.T) {
	valid := map[string]int64{
		"/avatar/10001":     10001,
		"/avatar/10001.jpg": 10001,
		"/avatar/10001.png": 10001,
	}
	for path, want := range valid {
		got, ok := parsePath(path)
		if !ok || got != want {
			t.Errorf("parsePath(%q) 返回 %d、%v，期望 %d", path, got, ok, want)
		}
	}

	invalid := []string{"/avatar/", "/avatar/0", "/avatar/-1", "/avatar/abc", "/avatar/10001/2", "/other/10001"}
	for _, path := range invalid {
		if _, ok := parsePath(path); ok {
			t.Errorf("parsePath(%q) 应当失败", path)
		}
	}
}

func TestIsYunhuHost(t *testing.T) {
	if !isYunhuHost("chat-img.jwznb.com") {
		t.Error("云湖图片 CDN 应当被识别")
	}
	if isYunhuHost("example.com") {
		t.Error("外部域名不应被识别为云湖 CDN")
	}
}

func TestProxyHead(t *testing.T) {
	pngData := testPNG(t)
	upstream, _ := newUpstream(t, pngData, "image/png", http.StatusOK)

	store := NewStore()
	store.Put(10001, upstream.URL+"/a.png")

	recorder := httptest.NewRecorder()
	newProxy(store).ServeHTTP(recorder, httptest.NewRequest(http.MethodHead, "/avatar/10001", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码为 %d", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Length"); got != strconv.Itoa(len(pngData)) {
		t.Errorf("Content-Length 为 %q", got)
	}
	if recorder.Body.Len() != 0 {
		t.Error("HEAD 请求不应返回内容")
	}
}
