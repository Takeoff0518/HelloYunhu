// SPDX-License-Identifier: GPL-3.0-or-later

package avatar

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PathPrefix 是头像代理的固定路径前缀，后面直接跟整数标识。
const PathPrefix = "/avatar/"

// 默认值。
const (
	defaultTTL      = 6 * time.Hour
	defaultMaxBytes = 1 << 20
	fetchTimeout    = 10 * time.Second
	maxCacheEntries = 512
)

// yunhuReferer 是云湖媒体 CDN 要求的来源地址。
const yunhuReferer = "https://myapp.jwznb.com"

// yunhuHosts 是需要附带来源头的域名后缀。
var yunhuHosts = []string{".jwznb.com", ".jwzhd.com"}

// Proxy 把云湖头像转给客户端。
//
// 客户端只拿到本程序给出的地址，实际取图由这里完成，来源头也在这一步附加。
type Proxy struct {
	// Store 提供标识到头像地址的对应关系。
	Store *Store
	// Client 用于取图，为空时使用带超时的默认客户端。
	Client *http.Client
	// Logger 为空时丢弃日志。
	Logger *slog.Logger
	// Hosts 是允许取图的域名后缀，为空时使用云湖 CDN 的地址。
	//
	// 限制来源是为了不把代理变成任意地址的抓取器：标识来自客户端，地址来自云湖，
	// 两者都不能指向别处。
	Hosts []string
	// TTL 是图片缓存的保留时长，0 表示使用默认值。
	TTL time.Duration
	// MaxBytes 是单张头像的体积上限，0 表示使用默认值。
	MaxBytes int64

	mu    sync.Mutex
	cache map[int64]cachedImage
}

// cachedImage 是一张已经取回并缓存的头像。
type cachedImage struct {
	data        []byte
	contentType string
	expires     time.Time
}

// imagePayload 是要写回客户端的图片内容。
type imagePayload struct {
	data        []byte
	contentType string
}

// ServeHTTP 处理客户端对头像的请求。
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "只支持 GET", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := parsePath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	result := p.image(r.Context(), userID)

	w.Header().Set("Content-Type", result.contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(result.data)))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(result.data)
}

// image 返回一张头像，取不到时给出占位图。
func (p *Proxy) image(ctx context.Context, userID int64) imagePayload {
	if cached, ok := p.cached(userID); ok {
		return cached
	}

	source, known := p.Store.Get(userID)
	if !known {
		return imagePayload{data: placeholderBytes(), contentType: "image/png"}
	}

	result, err := p.fetch(ctx, source)
	if err != nil {
		p.logger().Debug("取头像失败，返回占位图", "user_id", userID, "err", err)
		return imagePayload{data: placeholderBytes(), contentType: "image/png"}
	}

	p.remember(userID, result)
	return result
}

// cached 返回未过期的缓存。
func (p *Proxy) cached(userID int64) (imagePayload, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry, ok := p.cache[userID]
	if !ok || time.Now().After(entry.expires) {
		return imagePayload{}, false
	}
	return imagePayload{data: entry.data, contentType: entry.contentType}, true
}

// remember 记录一张取回的头像。
func (p *Proxy) remember(userID int64, result imagePayload) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cache == nil {
		p.cache = make(map[int64]cachedImage)
	}
	if len(p.cache) >= maxCacheEntries {
		for key := range p.cache {
			delete(p.cache, key)
			break
		}
	}
	p.cache[userID] = cachedImage{
		data:        result.data,
		contentType: result.contentType,
		expires:     time.Now().Add(p.ttl()),
	}
}

// fetch 取回一张头像。
func (p *Proxy) fetch(ctx context.Context, source string) (imagePayload, error) {
	parsed, err := url.Parse(source)
	if err != nil {
		return imagePayload{}, err
	}

	host := parsed.Hostname()
	if !p.hostAllowed(host) {
		return imagePayload{}, errors.New("头像地址不在允许的域名内: " + host)
	}

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return imagePayload{}, err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0")
	if isYunhuHost(host) {
		request.Header.Set("Referer", yunhuReferer)
	}

	response, err := p.client().Do(request)
	if err != nil {
		return imagePayload{}, err
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode != http.StatusOK {
		return imagePayload{}, errors.New("头像地址返回状态码 " + strconv.Itoa(response.StatusCode))
	}

	contentType := response.Header.Get("Content-Type")
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return imagePayload{}, errors.New("头像地址返回的内容不是图片: " + contentType)
	}

	limit := p.maxBytes()
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return imagePayload{}, err
	}
	if int64(len(data)) > limit {
		return imagePayload{}, errors.New("头像体积超过上限")
	}

	mediaType := strings.TrimSpace(strings.Split(contentType, ";")[0])
	return imagePayload{data: data, contentType: mediaType}, nil
}

// hostAllowed 判断是否允许从这个域名取图。
func (p *Proxy) hostAllowed(host string) bool {
	hosts := p.Hosts
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

// client 返回取图用的 HTTP 客户端。
func (p *Proxy) client() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	return &http.Client{Timeout: fetchTimeout}
}

// ttl 返回生效的缓存时长。
func (p *Proxy) ttl() time.Duration {
	if p.TTL > 0 {
		return p.TTL
	}
	return defaultTTL
}

// maxBytes 返回生效的体积上限。
func (p *Proxy) maxBytes() int64 {
	if p.MaxBytes > 0 {
		return p.MaxBytes
	}
	return defaultMaxBytes
}

// logger 返回可用的日志实例。
func (p *Proxy) logger() *slog.Logger {
	if p.Logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return p.Logger
}

// parsePath 从请求路径里取出整数标识。
//
// 允许在标识后面带一个图片后缀，部分客户端会自行补上扩展名。
func parsePath(path string) (int64, bool) {
	if !strings.HasPrefix(path, PathPrefix) {
		return 0, false
	}

	raw := strings.TrimPrefix(path, PathPrefix)
	if strings.IndexByte(raw, '/') >= 0 {
		return 0, false
	}
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
		raw = strings.TrimSuffix(raw, ext)
	}

	userID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || userID <= 0 {
		return 0, false
	}
	return userID, true
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

// Run 启动头像代理直到上下文结束。
func (p *Proxy) Run(ctx context.Context, listen string) error {
	server := &http.Server{
		Addr:              listen,
		Handler:           p,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			p.logger().Error("头像代理退出", "listen", listen, "err", err)
		}
	}()

	p.logger().Info("头像代理已启动", "listen", listen, "path", PathPrefix+"<标识>")

	select {
	case <-ctx.Done():
	case <-done:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}

	<-done
	return nil
}
