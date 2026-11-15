// SPDX-License-Identifier: GPL-3.0-or-later

// Package api 封装云湖开放接口。
//
// 云湖的接口没有签名机制，令牌通过查询参数传递，因此请求地址不能写进日志。
// 接口统一返回 {code, message, data}，code 非 0 表示业务错误。
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

// 单次响应的读取上限。正常响应只有几 KB，这里留出足够余量并防止异常响应占满内存。
const maxResponseBytes = 1 << 20

// defaultTimeout 是单次请求的超时时间。
const defaultTimeout = 30 * time.Second

// Config 是接口客户端的设置。
type Config struct {
	// BaseURL 是开放接口的基础地址，例如 https://chat-go.jwzhd.com/open-apis/v1。
	BaseURL string
	// Token 是机器人令牌。
	Token string
	// HTTPClient 为空时使用带超时的默认客户端。
	HTTPClient *http.Client
}

// Client 是云湖接口客户端。
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New 创建接口客户端。
func New(cfg Config) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("接口地址 %q 无效: %w", cfg.BaseURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("接口地址 %q 的协议必须是 http 或 https", cfg.BaseURL)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("接口地址 %q 缺少主机名", cfg.BaseURL)
	}

	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		return nil, errors.New("机器人令牌不能为空")
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}

	return &Client{baseURL: baseURL, token: token, http: httpClient}, nil
}

// APIError 是云湖返回的业务错误。调用方可以通过错误码区分限流与其它失败。
type APIError struct {
	// Code 是云湖返回的错误码，例如 1007 表示超过调用频率。
	Code int
	// Message 是平台给出的说明，可能为空。
	Message string
	// Op 是出错的接口，用于日志与排查。
	Op string
	// Body 是响应原文的片段，便于排查字段与错误码不一致的情况。
	Body string
}

// Error 实现 error 接口。
func (e *APIError) Error() string {
	var text string
	if e.Message == "" {
		text = fmt.Sprintf("云湖接口 %s 返回错误码 %d", e.Op, e.Code)
	} else {
		text = fmt.Sprintf("云湖接口 %s 返回错误码 %d: %s", e.Op, e.Code, e.Message)
	}

	if e.Body == "" {
		return text
	}
	return text + "（原始响应: " + e.Body + "）"
}

// 错误码常量。
const (
	// CodeRateLimited 表示超过每个接口每秒 10 次的调用上限。
	CodeRateLimited = 1007
)

// BotInfo 查询机器人自身的信息。
//
// 事件中不包含机器人标识，启动时必须先调用这个接口取得 botId，作为 OneBot 的
// self_id 与订阅连接的鉴权参数。
func (c *Client) BotInfo(ctx context.Context) (*model.BotInfo, error) {
	data, err := c.do(ctx, http.MethodGet, "/bot/info", "GET /bot/info", nil, nil)
	if err != nil {
		return nil, err
	}

	var info model.BotInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("解析机器人信息: %w", err)
	}
	if info.BotID == "" {
		return nil, errors.New("机器人信息缺少 botId")
	}

	return &info, nil
}

// SendMessage 发送一条消息。
//
// 云湖一条消息只有一个内容类型，因此 OneBot 的多段消息需要由上层拆分后多次调用。
// 发送失败时不重试，重复发言的代价高于一次失败。
func (c *Client) SendMessage(ctx context.Context, req model.SendMessageRequest) (*model.SendMessageResult, error) {
	if req.RecvID == "" {
		return nil, errors.New("发送消息缺少接收方标识")
	}
	switch req.RecvType {
	case model.RecvTypeGroup, model.RecvTypeUser:
	default:
		return nil, fmt.Errorf("发送消息的接收方类型 %q 不受支持", req.RecvType)
	}
	if req.ContentType == "" {
		return nil, errors.New("发送消息缺少内容类型")
	}

	data, err := c.do(ctx, http.MethodPost, "/bot/send", "POST /bot/send", nil, req)
	if err != nil {
		return nil, err
	}

	result := &model.SendMessageResult{}
	if len(data) == 0 {
		return result, nil
	}
	if err := json.Unmarshal(data, result); err != nil {
		return nil, fmt.Errorf("解析发送结果: %w", err)
	}

	return result, nil
}

// envelope 是接口响应的公共外壳。
//
// 平台文档没有给出完整的响应结构，字段名按 code、message 与 data 解析，并同时
// 接受 msg。这个不确定性记录在调研报告第 11 节，实测后需要回来确认。
type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Msg     string          `json:"msg"`
	Data    json.RawMessage `json:"data"`
}

// message 返回平台给出的说明，两种字段名任取其一。
func (e *envelope) message() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Msg
}

// do 发起一次请求并解析响应。
func (c *Client) do(ctx context.Context, method, path, op string, query url.Values, body any) (json.RawMessage, error) {
	endpoint, err := c.endpoint(path, query)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("编码 %s 的请求体: %w", op, err)
		}
		payload = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
	if err != nil {
		return nil, fmt.Errorf("构造 %s 请求: %w", op, err)
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	return c.execute(request, op)
}

// endpoint 拼接接口地址，并追加令牌与调用方给出的查询参数。
func (c *Client) endpoint(path string, query url.Values) (string, error) {
	parsed, err := url.Parse(c.baseURL + path)
	if err != nil {
		return "", fmt.Errorf("拼接接口地址: %w", err)
	}

	values := parsed.Query()
	values.Set("token", c.token)
	for key, items := range query {
		for _, item := range items {
			values.Add(key, item)
		}
	}
	parsed.RawQuery = values.Encode()

	return parsed.String(), nil
}

// execute 发出请求并解析统一响应。
func (c *Client) execute(request *http.Request, op string) (json.RawMessage, error) {
	request.Header.Set("Accept", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("请求云湖接口 %s: %w", op, err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("读取 %s 的响应: %w", op, err)
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("云湖接口 %s 返回状态码 %d: %s", op, response.StatusCode, excerpt(raw))
	}

	var parsed envelope
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("云湖接口 %s 的响应不是合法 JSON: %w（原始响应: %s）", op, err, excerpt(raw))
	}
	if parsed.Code != 0 {
		return nil, &APIError{Code: parsed.Code, Message: parsed.message(), Op: op, Body: excerpt(raw)}
	}

	return parsed.Data, nil
}

// excerpt 截取响应片段用于错误信息，避免异常响应把日志冲掉。
func excerpt(raw []byte) string {
	const limit = 256

	text := strings.TrimSpace(string(raw))
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}
