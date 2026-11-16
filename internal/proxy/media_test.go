// SPDX-License-Identifier: GPL-3.0-or-later

package proxy

import (
	"bytes"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Takeoff0518/HelloYunhu/internal/proxypath"
)

// encode 把地址编码成媒体路径。
func encode(raw string) string {
	return proxypath.Media + base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// newUpstream 启动一个模拟 CDN，返回服务与收到的请求。
func newUpstream(t *testing.T, body []byte, status int) (*httptest.Server, *[]*http.Request) {
	t.Helper()

	requests := make([]*http.Request, 0, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Clone(r.Context()))
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Accept-Ranges", "bytes")
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Length", "4")
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)

	return server, &requests
}

// newMedia 构造只允许访问 127.0.0.1 的转发器。
func newMedia() *Media {
	return &Media{StreamThreshold: 1024, Max: 1 << 20, Concurrency: 2, Hosts: []string{"127.0.0.1"}}
}

func TestDecodeTarget(t *testing.T) {
	raw := "https://chat-img.jwznb.com/a.jpg"
	if target, ok := decodeTarget(encode(raw)); !ok || target.String() != raw {
		t.Fatalf("解析结果为 %v、%v", target, ok)
	}

	invalid := []string{
		"/media/",
		"/media/not-base64!!",
		proxypath.Media + base64.RawURLEncoding.EncodeToString([]byte("file:///etc/passwd")),
		proxypath.Media + base64.RawURLEncoding.EncodeToString([]byte("不是地址")),
	}
	// 域名限制在 hostAllowed 里判断，不在解码这一步，因此这里只验证协议与格式。
	for _, path := range invalid {
		if _, ok := decodeTarget(path); ok {
			t.Errorf("decodeTarget(%q) 应当失败", path)
		}
	}
}

func TestMediaRejectsForeignHost(t *testing.T) {
	recorder := httptest.NewRecorder()
	newMedia().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, encode("https://example.com/a.png"), nil))

	if recorder.Code != http.StatusNotFound {
		t.Errorf("状态码为 %d，期望 404", recorder.Code)
	}
}

func TestMediaServesSmallFile(t *testing.T) {
	upstream, requests := newUpstream(t, []byte("data"), http.StatusOK)

	recorder := httptest.NewRecorder()
	newMedia().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, encode(upstream.URL+"/a.png"), nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码为 %d", recorder.Code)
	}
	if !bytes.Equal(recorder.Body.Bytes(), []byte("data")) {
		t.Errorf("响应体为 %q", recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type 为 %q", got)
	}
	if len(*requests) != 1 || (*requests)[0].Header.Get("Range") != "" {
		t.Errorf("上游请求为 %+v", *requests)
	}
}

func TestMediaForwardsRange(t *testing.T) {
	upstream, requests := newUpstream(t, []byte("data"), http.StatusOK)

	request := httptest.NewRequest(http.MethodGet, encode(upstream.URL+"/a.png"), nil)
	request.Header.Set("Range", "bytes=0-1")

	newMedia().ServeHTTP(httptest.NewRecorder(), request)

	if len(*requests) != 1 {
		t.Fatalf("上游请求数为 %d", len(*requests))
	}
	if got := (*requests)[0].Header.Get("Range"); got != "bytes=0-1" {
		t.Errorf("上游收到的 Range 为 %q", got)
	}
}

func TestMediaHead(t *testing.T) {
	upstream, _ := newUpstream(t, []byte("data"), http.StatusOK)

	recorder := httptest.NewRecorder()
	newMedia().ServeHTTP(recorder, httptest.NewRequest(http.MethodHead, encode(upstream.URL+"/a.png"), nil))

	if recorder.Code != http.StatusOK {
		t.Errorf("状态码为 %d", recorder.Code)
	}
	if recorder.Body.Len() != 0 {
		t.Error("HEAD 请求不应返回内容")
	}
}

func TestMediaUpstreamError(t *testing.T) {
	upstream, _ := newUpstream(t, nil, http.StatusForbidden)

	recorder := httptest.NewRecorder()
	newMedia().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, encode(upstream.URL+"/a.png"), nil))

	if recorder.Code != http.StatusBadGateway {
		t.Errorf("状态码为 %d，期望 502", recorder.Code)
	}
}

func TestMediaBusy(t *testing.T) {
	upstream, _ := newUpstream(t, []byte("data"), http.StatusOK)
	handler := newMedia()
	handler.Concurrency = 1

	// 占住唯一的名额，模拟并发已满。
	if !handler.acquire() {
		t.Fatal("应当取得名额")
	}
	defer handler.release()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, encode(upstream.URL+"/a.png"), nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Errorf("状态码为 %d，期望 503", recorder.Code)
	}
}

func TestMediaMethodNotAllowed(t *testing.T) {
	recorder := httptest.NewRecorder()
	newMedia().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/media/x", nil))

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("状态码为 %d，期望 405", recorder.Code)
	}
}

func TestRunReportsBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("占用端口失败: %v", err)
	}
	defer func() {
		_ = listener.Close()
	}()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})

	// 端口已被占用时应当把错误交回上层，而不是安静地不提供服务。
	if err := Run(t.Context(), Options{Addr: listener.Addr().String(), Media: handler}); err == nil {
		t.Fatal("端口被占用时应返回错误")
	}
}
