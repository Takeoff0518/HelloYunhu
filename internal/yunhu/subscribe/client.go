// SPDX-License-Identifier: GPL-3.0-or-later

// Package subscribe 维护与云湖事件订阅端点的 WebSocket 连接。
//
// 订阅端点是未公开接口，地址由配置提供，令牌以查询参数的形式追加。连接断开后
// 自动重连，事件逐条交给上层处理；这里不解释事件内容，只保证传输与重连。
package subscribe

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Takeoff0518/HelloYunhu/internal/logging"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

// maxFrameBytes 是单帧读取上限。订阅事件只有几 KB，这里留出足够余量。
const maxFrameBytes = 4 << 20

// 重连间隔，按倍数增长。
const (
	minReconnectDelay = time.Second
	maxReconnectDelay = 30 * time.Second
)

// handshakeTimeout 是握手超时时间。
const handshakeTimeout = 10 * time.Second

// Config 是订阅连接设置。
type Config struct {
	// URL 是不含令牌的订阅地址，例如 wss://ws.jwzhd.com/subscribe。
	URL string
	// Token 是机器人令牌。
	Token string
}

// Options 是客户端的依赖。
type Options struct {
	Config Config
	Logger *slog.Logger
	// RawEvents 为真时在 debug 级别输出收到的原始事件。
	RawEvents bool
}

// Client 是订阅连接客户端。
type Client struct {
	url       *url.URL
	token     string
	logger    *slog.Logger
	dialer    *websocket.Dialer
	rawEvents bool

	// 重连间隔，测试里可以改小。
	minDelay time.Duration
	maxDelay time.Duration
}

// New 创建订阅客户端。
func New(opt Options) (*Client, error) {
	parsed, err := url.Parse(opt.Config.URL)
	if err != nil {
		return nil, fmt.Errorf("订阅地址 %q 无效: %w", opt.Config.URL, err)
	}
	if parsed.Scheme != "ws" && parsed.Scheme != "wss" {
		return nil, fmt.Errorf("订阅地址 %q 的协议必须是 ws 或 wss", opt.Config.URL)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("订阅地址 %q 缺少主机名", opt.Config.URL)
	}
	if opt.Config.Token == "" {
		return nil, fmt.Errorf("机器人令牌不能为空")
	}

	logger := opt.Logger
	if logger == nil {
		logger = logging.Discard()
	}

	return &Client{
		url:       parsed,
		token:     opt.Config.Token,
		logger:    logger,
		rawEvents: opt.RawEvents,
		dialer: &websocket.Dialer{
			Proxy:            http.ProxyFromEnvironment,
			HandshakeTimeout: handshakeTimeout,
		},
		minDelay: minReconnectDelay,
		maxDelay: maxReconnectDelay,
	}, nil
}

// Probe 建立一次订阅连接后立即关闭，用于自检。
//
// 只验证握手与鉴权，不等待事件，因此不会在自检阶段长时间占用连接。
func (c *Client) Probe(ctx context.Context) error {
	conn, response, err := c.dialer.DialContext(ctx, c.endpoint(), nil)
	if err != nil {
		if response != nil {
			_ = response.Body.Close()
			return fmt.Errorf("订阅握手失败，状态码 %d: %w", response.StatusCode, err)
		}
		return fmt.Errorf("连接订阅端点: %w", err)
	}

	if response != nil {
		_ = response.Body.Close()
	}
	_ = conn.Close()

	return nil
}

// endpoint 返回带令牌的订阅地址。
func (c *Client) endpoint() string {
	endpoint := *c.url
	query := endpoint.Query()
	query.Set("token", c.token)
	endpoint.RawQuery = query.Encode()

	return endpoint.String()
}

// Run 持续接收事件并写入 out，直到上下文结束。
//
// out 的容量由调用方决定。写入是阻塞的，这样订阅连接产生的背压会传导到读循环，
// 而不是在内存里无限堆积事件。
func (c *Client) Run(ctx context.Context, out chan<- model.Envelope) error {
	delay := c.minDelay

	for {
		if ctx.Err() != nil {
			return nil //nolint:nilerr // 上下文取消按正常退出处理
		}

		established, err := c.listen(ctx, out)
		if ctx.Err() != nil {
			// 上下文取消会强制关闭连接，读循环必然带错误返回，这里属于正常退出。
			return nil //nolint:nilerr // 取消不是故障
		}

		c.logger.Warn("订阅连接中断，准备重连", "err", err, "retry_in", delay.String())

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}

		if established {
			delay = c.minDelay
		} else {
			delay *= 2
			if delay > c.maxDelay {
				delay = c.maxDelay
			}
		}
	}
}

// listen 建立一次连接并把收到的事件写入 out。
//
// 返回的布尔值表示连接是否成功建立过，用于决定重连间隔是否重置。
func (c *Client) listen(ctx context.Context, out chan<- model.Envelope) (bool, error) {
	conn, response, err := c.dialer.DialContext(ctx, c.endpoint(), nil)
	if err != nil {
		if response != nil {
			_ = response.Body.Close()
			return false, fmt.Errorf("订阅握手失败，状态码 %d: %w", response.StatusCode, err)
		}
		return false, fmt.Errorf("连接订阅端点: %w", err)
	}
	if response != nil {
		// gorilla 在握手成功后已经把响应体读完，这里显式关闭以便 linter 与读者确认。
		_ = response.Body.Close()
	}
	defer func() {
		_ = conn.Close()
	}()

	conn.SetReadLimit(maxFrameBytes)

	// 上下文结束时主动关闭连接，否则读循环会阻塞到对端断开。
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-stop:
		}
	}()

	c.logger.Info("已连接云湖事件订阅")

	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return true, fmt.Errorf("读取订阅消息: %w", err)
		}

		if c.rawEvents {
			// 原始事件包含消息正文与昵称，只在显式开启时输出，且限制长度。
			c.logger.Debug("收到原始订阅事件", "payload", truncate(payload, maxRawEventBytes))
		}

		var envelope model.Envelope
		if err := json.Unmarshal(payload, &envelope); err != nil {
			c.logger.Warn("订阅消息无法解析，已忽略", "err", err)
			continue
		}

		select {
		case out <- envelope:
		case <-ctx.Done():
			return true, nil
		}
	}
}

// maxRawEventBytes 是原始事件日志的截断长度。
const maxRawEventBytes = 4096

// truncate 把内容截断到给定长度，超出时在末尾标注。
func truncate(payload []byte, limit int) string {
	if len(payload) <= limit {
		return string(payload)
	}
	return string(payload[:limit]) + "...(已截断)"
}
