// SPDX-License-Identifier: GPL-3.0-or-later

// Package client 维护到机器人框架的反向 WebSocket 连接。
//
// 连接由本程序主动发起，框架监听端口。一条 Universal 连接同时承担收发：本程序
// 写入事件，框架写入接口调用；接口调用的响应必须原样带回框架给出的 echo。
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Takeoff0518/HelloYunhu/internal/logging"
	"github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
)

// maxFrameBytes 是单帧读取上限。
//
// 框架会把图片与语音整段内联成 base64，单帧可能有几十 MB，常见的默认上限会让
// 连接以 1009 断开，这里放宽到 64 MB。
const maxFrameBytes = 64 << 20

// 重连间隔。初次失败后按倍数增长，避免框架未启动时反复冲击。
const (
	minReconnectDelay = time.Second
	maxReconnectDelay = 30 * time.Second
)

// writeTimeout 是单次写入的超时时间。
const writeTimeout = 10 * time.Second

// handshakeTimeout 是握手超时时间。
const handshakeTimeout = 10 * time.Second

// ErrNotConnected 表示到框架的连接尚未建立。
var ErrNotConnected = errors.New("到框架的连接尚未建立")

// Config 是连接设置。
type Config struct {
	// URL 是框架监听的地址，例如 ws://127.0.0.1:6199/ws。
	URL string
	// AccessToken 是框架侧配置的访问令牌，没有设置时留空。
	AccessToken string
	// SelfID 是机器人标识，握手时放在 X-Self-ID 头里。
	SelfID int64
	// Role 是连接角色，留空时使用 Universal。
	Role string
}

// Handler 处理框架发来的接口调用。返回的 data 会写进响应的 data 字段。
type Handler func(ctx context.Context, action string, params json.RawMessage) (any, error)

// Options 是客户端的依赖。
type Options struct {
	Config  Config
	Handler Handler
	Logger  *slog.Logger
	// OnConnect 在握手成功后调用，用于发送生命周期事件。
	OnConnect func(ctx context.Context) error
}

// Client 是到框架的反向 WebSocket 客户端。
type Client struct {
	cfg       Config
	handler   Handler
	onConnect func(ctx context.Context) error
	logger    *slog.Logger
	dialer    *websocket.Dialer

	// 重连间隔，测试里可以改小。
	minDelay time.Duration
	maxDelay time.Duration

	mu   sync.Mutex
	conn *websocket.Conn
}

// New 创建客户端。
func New(opt Options) (*Client, error) {
	cfg := opt.Config

	parsed, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("框架地址 %q 无效: %w", cfg.URL, err)
	}
	if parsed.Scheme != "ws" && parsed.Scheme != "wss" {
		return nil, fmt.Errorf("框架地址 %q 的协议必须是 ws 或 wss", cfg.URL)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("框架地址 %q 缺少主机名", cfg.URL)
	}
	if cfg.SelfID <= 0 {
		return nil, errors.New("机器人标识必须是正整数")
	}
	if opt.Handler == nil {
		return nil, errors.New("缺少接口处理器")
	}
	if cfg.Role == "" {
		cfg.Role = "Universal"
	}

	logger := opt.Logger
	if logger == nil {
		logger = logging.Discard()
	}

	return &Client{
		cfg:       cfg,
		handler:   opt.Handler,
		onConnect: opt.OnConnect,
		logger:    logger,
		dialer: &websocket.Dialer{
			Proxy:            http.ProxyFromEnvironment,
			HandshakeTimeout: handshakeTimeout,
		},
		minDelay: minReconnectDelay,
		maxDelay: maxReconnectDelay,
	}, nil
}

// Run 持续维护连接，直到上下文结束。连接断开后按指数退避重连。
func (c *Client) Run(ctx context.Context) error {
	delay := c.minDelay

	for {
		if ctx.Err() != nil {
			return nil //nolint:nilerr // 上下文取消按正常退出处理
		}

		established, err := c.connect(ctx)
		if ctx.Err() != nil {
			// 上下文取消会强制关闭连接，读循环必然带错误返回，这里属于正常退出。
			return nil //nolint:nilerr // 取消不是故障
		}

		c.logger.Warn("框架连接中断，准备重连", "err", err, "retry_in", delay.String())

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

// SendEvent 把一条事件写进连接。
func (c *Client) SendEvent(event any) error {
	return c.writeJSON(event)
}

// Connected 报告当前是否已建立连接。
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.conn != nil
}

// Probe 建立一次连接后立即关闭，用于自检。
//
// 只验证握手与头部是否正确，不发送事件，因此不会让框架登记成可用连接。
func (c *Client) Probe(ctx context.Context) error {
	conn, response, err := c.dialer.DialContext(ctx, c.cfg.URL, c.header())
	if err != nil {
		if response != nil {
			_ = response.Body.Close()
			return fmt.Errorf("握手失败，状态码 %d: %w", response.StatusCode, err)
		}
		return fmt.Errorf("连接框架: %w", err)
	}

	if response != nil {
		_ = response.Body.Close()
	}
	_ = conn.Close()

	return nil
}

// header 返回握手头。
func (c *Client) header() http.Header {
	header := http.Header{}
	header.Set("X-Self-ID", strconv.FormatInt(c.cfg.SelfID, 10))
	header.Set("X-Client-Role", c.cfg.Role)
	if c.cfg.AccessToken != "" {
		header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	}
	return header
}

// connect 建立一次连接并读取消息，连接断开时返回。
//
// 返回的布尔值表示这次连接是否成功建立过，用于决定重连间隔是否重置。
func (c *Client) connect(ctx context.Context) (bool, error) {
	conn, response, err := c.dialer.DialContext(ctx, c.cfg.URL, c.header())
	if err != nil {
		if response != nil {
			_ = response.Body.Close()
			return false, fmt.Errorf("握手失败，状态码 %d: %w", response.StatusCode, err)
		}
		return false, fmt.Errorf("连接框架: %w", err)
	}
	if response != nil {
		// gorilla 在握手成功后已经把响应体读完，这里显式关闭以便 linter 与读者确认。
		_ = response.Body.Close()
	}
	defer func() {
		_ = conn.Close()
	}()

	conn.SetReadLimit(maxFrameBytes)
	c.setConn(conn)
	defer c.setConn(nil)

	// 上下文结束时主动关闭连接，否则读循环会一直阻塞到对端断开。
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-stop:
		}
	}()

	c.logger.Info("已连接机器人框架", "url", c.cfg.URL, "self_id", c.cfg.SelfID)

	if c.onConnect != nil {
		if err := c.onConnect(ctx); err != nil {
			c.logger.Warn("发送连接事件失败", "err", err)
		}
	}

	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return true, fmt.Errorf("读取框架消息: %w", err)
		}
		c.handleFrame(ctx, payload)
	}
}

// handleFrame 处理框架写入的一帧，按接口调用解析。
func (c *Client) handleFrame(ctx context.Context, payload []byte) {
	var request model.Request
	if err := json.Unmarshal(payload, &request); err != nil {
		c.logger.Warn("框架写入的帧无法解析", "err", err)
		return
	}
	if strings.TrimSpace(request.Action) == "" {
		c.logger.Warn("框架写入的帧缺少 action，已忽略")
		return
	}

	// 接口调用可能涉及网络请求，单独用协程处理，避免阻塞读循环。
	go c.dispatch(ctx, request)
}

// dispatch 执行一次接口调用并写回响应。
func (c *Client) dispatch(ctx context.Context, request model.Request) {
	c.logger.Debug("收到接口调用", "action", request.Action)

	data, err := c.handler(ctx, request.Action, request.Params)
	if err != nil {
		var actionErr *model.ActionError
		if errors.As(err, &actionErr) {
			c.logger.Info("接口调用未成功", "action", request.Action, "retcode", actionErr.Retcode, "wording", actionErr.Wording)
			c.respond(model.Failure(actionErr.Retcode, actionErr.Wording, request.Echo))
			return
		}

		c.logger.Error("接口调用失败", "action", request.Action, "err", err)
		c.respond(model.Failure(model.RetcodeServerError, err.Error(), request.Echo))
		return
	}

	c.respond(model.Success(data, request.Echo))
}

// respond 写回接口响应。
func (c *Client) respond(response model.Response) {
	if err := c.writeJSON(response); err != nil {
		c.logger.Warn("回写接口响应失败", "err", err)
	}
}

// writeJSON 序列化并写入一帧。
func (c *Client) writeJSON(value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("编码消息: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return ErrNotConnected
	}

	if err := c.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return fmt.Errorf("设置写入超时: %w", err)
	}
	if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		return fmt.Errorf("写入框架连接: %w", err)
	}

	return nil
}

// setConn 记录当前连接，传 nil 表示连接已断开。
func (c *Client) setConn(conn *websocket.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn = conn
}
