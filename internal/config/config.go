// SPDX-License-Identifier: GPL-3.0-or-later

// Package config 定义配置结构、默认值、加载与校验规则。
//
// 配置在进程启动时一次性读取并完整校验。任何一项不合法都属于致命错误，
// 由调用方打印具体原因后退出，不做静默降级。
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Takeoff0518/HelloYunhu/internal/logging"
)

// 默认值。加载配置时先取默认值，再用文件中的内容覆盖。
const (
	defaultLogLevel     = "info"
	defaultLogFormat    = "text"
	defaultLogKeepDays  = 30
	defaultStoragePath  = "data/helloyunhu.db"
	defaultMessageTTL   = 168 * time.Hour
	defaultAPIBase      = "https://chat-go.jwzhd.com/open-apis/v1"
	defaultSubscribeURL = "wss://ws.jwzhd.com/subscribe"
	defaultRateLimit    = 10
	defaultDiagnoseAddr = "127.0.0.1:9090"
	defaultWebhookAddr  = "127.0.0.1:8081"
)

// 事件接收方式。
const (
	// ReceiveWebsocket 只用订阅连接接收事件。
	ReceiveWebsocket = "websocket"
	// ReceiveWebhook 只用 HTTP Webhook 接收事件。
	ReceiveWebhook = "webhook"
	// ReceiveBoth 两种方式同时使用，按事件标识去重。
	ReceiveBoth = "both"
)

// Webhook 令牌的最小长度。云湖的 Webhook 没有签名，URL 里的令牌就是唯一凭据。
const minWebhookTokenLen = 24

// Config 是进程的完整配置。一个进程服务一个云湖机器人。
type Config struct {
	// Bot 是云湖机器人本身。
	Bot Bot `yaml:"bot"`
	// Receive 决定事件从哪里进来。
	Receive Receive `yaml:"receive"`
	// OneBot 是我们对外提供的 OneBot 连接，至少一个。
	OneBot []OneBot `yaml:"onebot"`
	// Log 是日志设置。
	Log Log `yaml:"log"`
	// Storage 是持久化设置。
	Storage Storage `yaml:"storage"`
	// Yunhu 是云湖平台的连接地址。
	Yunhu Yunhu `yaml:"yunhu"`
	// Capabilities 是向框架声明的能力。
	Capabilities Capabilities `yaml:"capabilities"`
	// Diagnostics 是只读诊断接口的设置。
	Diagnostics Diagnostics `yaml:"diagnostics"`
}

// Bot 是云湖机器人本身。
type Bot struct {
	// Token 是云湖机器人令牌。
	Token string `yaml:"token"`
}

// Receive 决定事件从哪里进来。
type Receive struct {
	// Mode 取 websocket、webhook 或 both。
	Mode string `yaml:"mode"`
	// Webhook 只在 Mode 含 webhook 时生效。
	Webhook Webhook `yaml:"webhook"`
}

// UsesWebsocket 表示是否使用订阅连接接收事件。
func (r Receive) UsesWebsocket() bool {
	return r.Mode == "" || r.Mode == ReceiveWebsocket || r.Mode == ReceiveBoth
}

// UsesWebhook 表示是否使用 HTTP Webhook 接收事件。
func (r Receive) UsesWebhook() bool {
	return r.Mode == ReceiveWebhook || r.Mode == ReceiveBoth
}

// Webhook 是 HTTP Webhook 服务的设置。
type Webhook struct {
	// Listen 是监听地址，例如 0.0.0.0:8081。
	Listen string `yaml:"listen"`
	// Token 是路径里的令牌，云湖推送事件时用它校验来源。
	Token string `yaml:"token"`
	// AllowedIPs 是允许的来源网段，留空表示不限制。
	AllowedIPs []string `yaml:"allowed_ips"`
}

// OneBot 是一条到机器人框架的反向 WebSocket 连接。
type OneBot struct {
	// Name 用于日志与诊断，未填时按顺序生成 onebot-1、onebot-2。
	Name string `yaml:"name"`
	// URL 是框架监听的反向 WebSocket 地址。
	URL string `yaml:"url"`
	// AccessToken 是框架侧配置的访问令牌，未设置时留空。
	AccessToken string `yaml:"access_token"`
}

// Log 是日志设置。
type Log struct {
	// Level 是日志级别，可选 debug、info、warn、error。
	Level string `yaml:"level"`
	// Format 是输出格式，可选 text 与 json。
	Format string `yaml:"format"`
	// RawEvents 为真时在 debug 级别输出订阅收到的原始事件。
	//
	// 只用于排查协议问题：原始事件包含消息正文与昵称，不属于常规日志内容。
	RawEvents bool `yaml:"raw_events"`
	// KeepDays 是日志保留天数，-1 表示永不删除，默认为 30。
	KeepDays int `yaml:"keep_days"`
	// Compress 为真时把前一天的日志压缩成 gzip，默认为真。
	Compress bool `yaml:"compress"`
}

// Storage 是持久化设置。
type Storage struct {
	// Path 是 SQLite 文件路径。
	Path string `yaml:"path"`
	// MessageTTL 是消息标识映射的保留时长。
	MessageTTL Duration `yaml:"message_ttl"`
}

// Yunhu 是云湖平台的服务地址。
type Yunhu struct {
	// APIBase 是开放接口的基础地址。
	APIBase string `yaml:"api_base"`
	// SubscribeURL 是事件订阅端点，令牌以查询参数的形式追加在后面。
	SubscribeURL string `yaml:"subscribe_url"`
	// RateLimit 是每秒最多发起的接口调用次数，0 表示不限制。
	//
	// 云湖的上限是每秒十次，默认留一点余量。发送消息不做重试，这里只做匀速。
	RateLimit int `yaml:"rate_limit"`
}

// Capabilities 是向框架声明的能力，对应 OneBot 的 can_send_image 与 can_send_record。
//
// 这是给框架看的声明，不是开关。声明与实际能力不一致时，框架会按错误的前提选择
// 发送方式，问题要到客户端才暴露。
type Capabilities struct {
	// CanSendImage 表示能否发送图片，默认为真。
	CanSendImage bool `yaml:"can_send_image"`
	// CanSendRecord 表示能否发送语音，默认为假。
	//
	// 云湖没有语音发送接口，语音只能按文件上传，客户端收到的是文件而不是语音气泡。
	// 改成真只会让框架发出 record 段，实际效果仍然是文件。
	CanSendRecord bool `yaml:"can_send_record"`
}

// UnmarshalYAML 按字段是否出现决定是否覆盖默认值。
//
// 布尔值的零值与「没有配置」无法区分，因此先用指针接住，未出现的字段保留默认值。
func (c *Capabilities) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		CanSendImage  *bool `yaml:"can_send_image"`
		CanSendRecord *bool `yaml:"can_send_record"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	if raw.CanSendImage != nil {
		c.CanSendImage = *raw.CanSendImage
	}
	if raw.CanSendRecord != nil {
		c.CanSendRecord = *raw.CanSendRecord
	}
	return nil
}

// Diagnostics 是只读诊断接口的设置。
//
// 首个版本不提供管理界面，只暴露健康检查与运行状态两个只读端点。
type Diagnostics struct {
	// Enabled 为真时在 Listen 上启动诊断服务。
	Enabled bool `yaml:"enabled"`
	// Listen 是监听地址，例如 127.0.0.1:9090。
	Listen string `yaml:"listen"`
}

// Duration 是支持 "168h"、"30m" 这类写法的时间间隔，序列化时输出同样的文字形式。
type Duration time.Duration

// UnmarshalYAML 把配置中的字符串转换为时间间隔。
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var text string
	if err := value.Decode(&text); err != nil {
		return fmt.Errorf("需要写成带单位的字符串，例如 168h: %w", err)
	}
	parsed, err := time.ParseDuration(text)
	if err != nil {
		return fmt.Errorf("无法解析时间间隔 %q: %w", text, err)
	}
	*d = Duration(parsed)
	return nil
}

// MarshalYAML 以 time.Duration 的文字形式输出。
func (d Duration) MarshalYAML() (any, error) {
	return time.Duration(d).String(), nil
}

// Standard 返回标准库表示的时间间隔。
func (d Duration) Standard() time.Duration {
	return time.Duration(d)
}

// Default 返回带默认值的配置。
func Default() *Config {
	return &Config{
		Receive: Receive{
			Mode: ReceiveWebsocket,
			Webhook: Webhook{
				Listen: defaultWebhookAddr,
			},
		},
		Log: Log{
			Level:    defaultLogLevel,
			Format:   defaultLogFormat,
			KeepDays: defaultLogKeepDays,
			Compress: true,
		},
		Storage: Storage{
			Path:       defaultStoragePath,
			MessageTTL: Duration(defaultMessageTTL),
		},
		Yunhu: Yunhu{
			APIBase:      defaultAPIBase,
			SubscribeURL: defaultSubscribeURL,
			RateLimit:    defaultRateLimit,
		},
		Capabilities: Capabilities{
			CanSendImage: true,
		},
		Diagnostics: Diagnostics{
			Listen: defaultDiagnoseAddr,
		},
	}
}

// Load 读取配置文件并校验。返回的错误包含文件路径，便于定位问题。
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件 %s: %w", path, err)
	}

	cfg, err := Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("解析配置文件 %s: %w", path, err)
	}

	return cfg, nil
}

// Parse 从 YAML 内容解析配置并校验，用于避免测试依赖真实文件。
func Parse(raw []byte) (*Config, error) {
	cfg := Default()

	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	// 拒绝未知字段。配置项拼写错误时直接报错，而不是被默默忽略。
	decoder.KnownFields(true)

	if err := decoder.Decode(cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("配置内容为空")
		}
		return nil, fmt.Errorf("YAML 格式错误: %w", err)
	}

	cfg.normalize()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("配置校验未通过:\n%w", err)
	}

	return cfg, nil
}

// normalize 补齐可推导的默认值：接收方式与连接名称。
func (c *Config) normalize() {
	if strings.TrimSpace(c.Receive.Mode) == "" {
		c.Receive.Mode = ReceiveWebsocket
	}
	if strings.TrimSpace(c.Receive.Webhook.Listen) == "" && c.Receive.UsesWebhook() {
		c.Receive.Webhook.Listen = defaultWebhookAddr
	}

	for index := range c.OneBot {
		if strings.TrimSpace(c.OneBot[index].Name) == "" {
			c.OneBot[index].Name = fmt.Sprintf("onebot-%d", index+1)
		}
	}
}

// Validate 校验配置，并把发现的问题一次性全部返回。
func (c *Config) Validate() error {
	var problems []error

	if _, err := logging.ParseLevel(c.Log.Level); err != nil {
		problems = append(problems, fmt.Errorf("log.level: %w", err))
	}
	if _, err := logging.ParseFormat(c.Log.Format); err != nil {
		problems = append(problems, fmt.Errorf("log.format: %w", err))
	}
	if c.Log.KeepDays == 0 || c.Log.KeepDays < -1 {
		problems = append(problems, errors.New("log.keep_days: 必须是 -1 或大于 0，-1 表示永不删除"))
	}

	if strings.TrimSpace(c.Storage.Path) == "" {
		problems = append(problems, errors.New("storage.path: 不能为空"))
	}
	if c.Storage.MessageTTL.Standard() <= 0 {
		problems = append(problems, errors.New("storage.message_ttl: 必须大于 0"))
	}

	if strings.TrimSpace(c.Bot.Token) == "" {
		problems = append(problems, errors.New("bot.token: 不能为空"))
	}

	problems = append(problems, c.validateReceive()...)

	if c.Yunhu.RateLimit < 0 {
		problems = append(problems, errors.New("yunhu.rate_limit: 不能为负数"))
	}
	if c.Diagnostics.Enabled && strings.TrimSpace(c.Diagnostics.Listen) == "" {
		problems = append(problems, errors.New("diagnostics.listen: 启用诊断接口时不能为空"))
	}

	problems = append(problems, validateHTTPURL("yunhu.api_base", c.Yunhu.APIBase)...)
	problems = append(problems, validateWebSocketURL("yunhu.subscribe_url", c.Yunhu.SubscribeURL)...)

	problems = append(problems, c.validateOneBot()...)

	return errors.Join(problems...)
}

// validateReceive 校验事件接收方式的设置。
func (c *Config) validateReceive() []error {
	var problems []error

	switch c.Receive.Mode {
	case ReceiveWebsocket, ReceiveWebhook, ReceiveBoth:
	default:
		problems = append(problems, fmt.Errorf(
			"receive.mode: %q 不受支持，可选 websocket、webhook 或 both", c.Receive.Mode))
	}

	if !c.Receive.UsesWebhook() {
		return problems
	}

	webhook := c.Receive.Webhook
	if strings.TrimSpace(webhook.Token) == "" {
		problems = append(problems, errors.New(
			"receive.webhook.token: 使用 webhook 时不能为空，云湖的推送没有签名，路径令牌是唯一的凭据"))
	} else if len(webhook.Token) < minWebhookTokenLen {
		problems = append(problems, fmt.Errorf(
			"receive.webhook.token: 至少 %d 个字符", minWebhookTokenLen))
	}
	if strings.TrimSpace(webhook.Listen) == "" {
		problems = append(problems, errors.New("receive.webhook.listen: 使用 webhook 时不能为空"))
	}

	for _, entry := range webhook.AllowedIPs {
		if _, _, err := net.ParseCIDR(strings.TrimSpace(entry)); err != nil {
			problems = append(problems, fmt.Errorf(
				"receive.webhook.allowed_ips: %q 不是合法的网段，例如 203.0.113.0/24", entry))
		}
	}

	return problems
}

// validateOneBot 校验框架连接的设置。
func (c *Config) validateOneBot() []error {
	var problems []error

	if len(c.OneBot) == 0 {
		problems = append(problems, errors.New("onebot: 至少需要配置一个框架连接"))
		return problems
	}

	seen := make(map[string]int, len(c.OneBot))
	for index, conn := range c.OneBot {
		field := fmt.Sprintf("onebot[%d]", index)

		if previous, ok := seen[conn.Name]; ok {
			problems = append(problems, fmt.Errorf(
				"%s.name: 与 onebot[%d] 重复，连接名称必须唯一", field, previous))
		} else {
			seen[conn.Name] = index
		}

		problems = append(problems, validateWebSocketURL(field+".url", conn.URL)...)
	}

	return problems
}

// validateHTTPURL 校验 HTTP 类地址。
func validateHTTPURL(field, raw string) []error {
	if strings.TrimSpace(raw) == "" {
		return []error{fmt.Errorf("%s: 不能为空", field)}
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return []error{fmt.Errorf("%s: 无法解析地址 %q: %w", field, raw, err)}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return []error{fmt.Errorf("%s: 协议必须是 http 或 https，当前为 %q", field, parsed.Scheme)}
	}
	if parsed.Host == "" {
		return []error{fmt.Errorf("%s: 缺少主机名", field)}
	}
	return nil
}

// validateWebSocketURL 校验 WebSocket 地址。
func validateWebSocketURL(field, raw string) []error {
	if strings.TrimSpace(raw) == "" {
		return []error{fmt.Errorf("%s: 不能为空", field)}
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return []error{fmt.Errorf("%s: 无法解析地址 %q: %w", field, raw, err)}
	}
	if parsed.Scheme != "ws" && parsed.Scheme != "wss" {
		return []error{fmt.Errorf("%s: 协议必须是 ws 或 wss，当前为 %q", field, parsed.Scheme)}
	}
	if parsed.Host == "" {
		return []error{fmt.Errorf("%s: 缺少主机名", field)}
	}
	return nil
}
