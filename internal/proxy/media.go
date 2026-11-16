// SPDX-License-Identifier: GPL-3.0-or-later

package proxy

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Takeoff0518/HelloYunhu/internal/proxypath"
)

// 默认值。
const (
	// fetchTimeout 是等待上游响应头的时间。
	fetchTimeout = 10 * time.Second
	// defaultStreamThreshold 是未设置分档阈值时的取值。
	defaultStreamThreshold = 8 << 20
	// defaultMaxBytes 是未设置体积上限时的取值。
	defaultMaxBytes = 1 << 30
)

// yunhuReferer 是云湖媒体 CDN 要求的来源地址。
const yunhuReferer = "https://myapp.jwznb.com"

// yunhuHosts 是需要附带来源头的域名后缀。
var yunhuHosts = []string{".jwznb.com", ".jwzhd.com"}

// Media 把云湖的媒体转发给客户端。
//
// 媒体路径里带着原始地址的 base64url 编码，因此不需要维护地址表，重启后旧地址
// 仍然可用。允许访问的域名固定为云湖的 CDN，避免代理被用来抓取任意地址。
type Media struct {
	// StreamThreshold 是整块读取与边取边发的分界体积，0 表示使用默认值。
	StreamThreshold int64
	// Max 是单次转发允许的体积上限，0 表示使用默认值。
	Max int64
	// Concurrency 是同时向上游发起的请求数，0 表示只允许 1 个。
	Concurrency int
	// Hosts 是允许取图的域名后缀，为空时使用云湖 CDN 的地址。
	Hosts []string
	// Client 用于取图，为空时使用只限制响应头时间的默认客户端。
	Client *http.Client
	// Logger 为空时丢弃日志。
	Logger *slog.Logger

	once sync.Once
	sem  chan struct{}
}

// ServeHTTP 处理客户端对媒体的请求。
func (m *Media) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "只支持 GET", http.StatusMethodNotAllowed)
		return
	}

	target, ok := decodeTarget(r.URL.Path)
	if !ok || !m.hostAllowed(target.Hostname()) {
		http.NotFound(w, r)
		return
	}

	if !m.acquire() {
		w.Header().Set("Retry-After", "5")
		http.Error(w, "取图请求过多，请稍后重试", http.StatusServiceUnavailable)
		return
	}
	defer m.release()

	response, err := m.fetch(r, target)
	if err != nil {
		m.logger().Debug("转发媒体失败", "host", target.Hostname(), "err", err)
		http.Error(w, "取媒体失败", http.StatusBadGateway)
		return
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if limit := m.max(); limit > 0 && response.ContentLength > limit {
		http.Error(w, "媒体体积超过上限", http.StatusBadGateway)
		return
	}

	copyHeaders(w.Header(), response.Header)
	if response.ContentLength >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(response.ContentLength, 10))
	}
	w.WriteHeader(response.StatusCode)

	if r.Method == http.MethodHead {
		return
	}

	// 小文件先取完再返回，响应要么完整要么中断；大文件边取边发，避免整块占用内存。
	if response.ContentLength >= 0 && response.ContentLength <= m.threshold() {
		m.buffered(w, response.Body)
		return
	}
	m.streamed(w, response.Body)
}

// buffered 把不超过阈值的媒体整块读完再返回。
func (m *Media) buffered(w http.ResponseWriter, body io.Reader) {
	data, err := io.ReadAll(io.LimitReader(body, m.threshold()+1))
	if err != nil {
		// 响应头已经发出，只能断开连接，让客户端按长度不符判为失败。
		m.logger().Warn("读取媒体失败", "err", err)
		return
	}
	if _, err := w.Write(data); err != nil {
		m.logger().Debug("写入媒体失败", "err", err)
	}
}

// streamed 把超过阈值的媒体边取边发。
func (m *Media) streamed(w http.ResponseWriter, body io.Reader) {
	reader := body
	if limit := m.max(); limit > 0 {
		reader = io.LimitReader(body, limit)
	}

	buffer := make([]byte, 32<<10)
	if _, err := io.CopyBuffer(w, reader, buffer); err != nil {
		m.logger().Debug("转发媒体中断", "err", err)
	}
}

// fetch 向上游发起请求。
func (m *Media) fetch(r *http.Request, target *url.URL) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(r.Context(), fetchTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, r.Method, target.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0")
	if isYunhuHost(target.Hostname()) {
		request.Header.Set("Referer", yunhuReferer)
	}
	// 视频拖动需要分段，客户端给出的范围原样转给上游。
	if value := r.Header.Get("Range"); value != "" {
		request.Header.Set("Range", value)
	}

	response, err := m.client().Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		_ = response.Body.Close()
		return nil, errors.New("上游返回状态码 " + strconv.Itoa(response.StatusCode))
	}

	return response, nil
}

// acquire 取得一个取图名额。名额已满时立即返回 false，不排队等待。
func (m *Media) acquire() bool {
	select {
	case m.semaphore() <- struct{}{}:
		return true
	default:
		return false
	}
}

// release 释放取图名额。
func (m *Media) release() {
	select {
	case <-m.semaphore():
	default:
	}
}

// semaphore 返回取图名额通道，首次使用时创建。
func (m *Media) semaphore() chan struct{} {
	m.once.Do(func() {
		size := m.Concurrency
		if size <= 0 {
			size = 1
		}
		m.sem = make(chan struct{}, size)
	})
	return m.sem
}

// client 返回取图用的 HTTP 客户端。
//
// 只限制等待响应头的时间：媒体可能在流式转发，给整个请求设超时会截断大文件。
func (m *Media) client() *http.Client {
	if m.Client != nil {
		return m.Client
	}
	return &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: fetchTimeout}}
}

// threshold 返回生效的分档阈值。
func (m *Media) threshold() int64 {
	if m.StreamThreshold > 0 {
		return m.StreamThreshold
	}
	return defaultStreamThreshold
}

// max 返回生效的体积上限。
func (m *Media) max() int64 {
	if m.Max > 0 {
		return m.Max
	}
	return defaultMaxBytes
}

// logger 返回可用的日志实例。
func (m *Media) logger() *slog.Logger {
	if m.Logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return m.Logger
}

// decodeTarget 从请求路径里取出原始地址。
func decodeTarget(path string) (*url.URL, bool) {
	if !strings.HasPrefix(path, proxypath.Media) {
		return nil, false
	}

	encoded := strings.TrimPrefix(path, proxypath.Media)
	if encoded == "" || strings.ContainsAny(encoded, "/?#") {
		return nil, false
	}

	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, false
	}

	target, err := url.Parse(string(raw))
	if err != nil || target.Host == "" {
		return nil, false
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, false
	}

	return target, true
}

// hostAllowed 判断是否允许从这个域名取图。
func (m *Media) hostAllowed(host string) bool {
	hosts := m.Hosts
	if len(hosts) == 0 {
		hosts = yunhuHosts
	}

	for _, suffix := range hosts {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

// isYunhuHost 判断地址是否属于云湖的 CDN。
func isYunhuHost(host string) bool {
	for _, suffix := range yunhuHosts {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

// copyHeaders 复制客户端需要的响应头。
func copyHeaders(target, source http.Header) {
	for _, name := range []string{"Content-Type", "Content-Range", "Accept-Ranges", "Last-Modified", "Etag"} {
		if value := source.Get(name); value != "" {
			target.Set(name, value)
		}
	}
}
