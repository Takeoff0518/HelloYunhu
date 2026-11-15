// SPDX-License-Identifier: GPL-3.0-or-later

// Package webhook 接收云湖通过 HTTP 推送的事件。
//
// 云湖的 Webhook 没有签名，URL 路径里的令牌是唯一的凭据，因此这里只接受路径
// 完全匹配的 POST 请求，可选地限制来源网段，并且不在日志里输出请求体。
package webhook

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

// PathPrefix 是 Webhook 的固定路径前缀。
const PathPrefix = "/yunhu/"

// DefaultMaxBodyBytes 是请求体上限。
const DefaultMaxBodyBytes = 1 << 20

// ErrBusy 表示事件通道已满，调用方应让平台稍后重试。
var ErrBusy = errors.New("事件通道已满")

// Options 是 Webhook 服务的设置。
type Options struct {
	// Token 是路径里的令牌。
	Token string
	// AllowedIPs 是允许的来源网段，为空表示不限制。
	AllowedIPs []string
	// Logger 为空时丢弃日志。
	Logger *slog.Logger
	// Deliver 把解析后的事件交给上层，返回错误时按 503 回复。
	Deliver func(ctx context.Context, envelope model.Envelope) error
	// MaxBodyBytes 是请求体上限，0 表示使用默认值。
	MaxBodyBytes int64
}

// Server 是 Webhook 接收服务。
type Server struct {
	token      []byte
	allowed    []*net.IPNet
	logger     *slog.Logger
	deliver    func(ctx context.Context, envelope model.Envelope) error
	maxBody    int64
	listenAddr string
}

// New 创建 Webhook 服务。
func New(opt Options) (*Server, error) {
	if strings.TrimSpace(opt.Token) == "" {
		return nil, errors.New("webhook 令牌不能为空")
	}
	if opt.Deliver == nil {
		return nil, errors.New("缺少事件投递函数")
	}

	allowed := make([]*net.IPNet, 0, len(opt.AllowedIPs))
	for _, entry := range opt.AllowedIPs {
		_, network, err := net.ParseCIDR(strings.TrimSpace(entry))
		if err != nil {
			return nil, fmt.Errorf("来源网段 %q 无效: %w", entry, err)
		}
		allowed = append(allowed, network)
	}

	logger := opt.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	maxBody := opt.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = DefaultMaxBodyBytes
	}

	return &Server{
		token:   []byte(opt.Token),
		allowed: allowed,
		logger:  logger,
		deliver: opt.Deliver,
		maxBody: maxBody,
	}, nil
}

// Handler 返回接收事件的处理器。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(PathPrefix, s.handle)

	return mux
}

// handle 校验请求并投递事件。
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "只接受 POST", http.StatusMethodNotAllowed)
		return
	}

	// 路径必须精确等于前缀加令牌，错误的路径与错误的令牌同样返回 404，
	// 避免通过状态码判断令牌是否正确。
	want := PathPrefix + string(s.token)
	if subtle.ConstantTimeCompare([]byte(r.URL.Path), []byte(want)) != 1 {
		http.NotFound(w, r)
		return
	}

	if !s.ipAllowed(r.RemoteAddr) {
		s.logger.Warn("Webhook 来源不在允许的网段内", "remote_addr", r.RemoteAddr)
		http.Error(w, "来源不受允许", http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.maxBody))
	if err != nil {
		http.Error(w, "请求体过大", http.StatusRequestEntityTooLarge)
		return
	}

	var envelope model.Envelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		http.Error(w, "请求体不是合法的事件", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(envelope.Header.EventType) == "" {
		http.Error(w, "事件缺少 eventType", http.StatusBadRequest)
		return
	}

	if err := s.deliver(r.Context(), envelope); err != nil {
		if errors.Is(err, ErrBusy) {
			http.Error(w, "暂时无法处理", http.StatusServiceUnavailable)
			return
		}
		s.logger.Warn("Webhook 事件投递失败", "event_type", envelope.Header.EventType, "err", err)
		http.Error(w, "无法处理事件", http.StatusServiceUnavailable)
		return
	}

	// 平台对响应格式没有明确要求，返回空体与 200 表示已接收。
	w.WriteHeader(http.StatusOK)
}

// ipAllowed 判断来源是否在允许的网段内。没有配置网段时一律放行。
func (s *Server) ipAllowed(remoteAddr string) bool {
	if len(s.allowed) == 0 {
		return true
	}

	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	for _, network := range s.allowed {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// Run 启动服务直到上下文结束。
func (s *Server) Run(ctx context.Context, listen string) error {
	s.listenAddr = listen

	server := &http.Server{
		Addr:              listen,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("Webhook 服务退出", "listen", listen, "err", err)
		}
	}()

	s.logger.Info("Webhook 接收已启动", "listen", listen, "path", PathPrefix+"<token>")

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
