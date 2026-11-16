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
	"strconv"
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
	defaultProxyAddr    = "127.0.0.1:8082"
	defaultInlineLimit  = 20 << 20
	defaultReadLimit    = 20 << 20
	defaultStreamLimit  = 8 << 20
	defaultMediaMax     = 1 << 30
	defaultConcurrency  = 4
)

// 体积与并发的取值范围。
var (
	// maxInlineLimit 是接收方向内联上限的上限，再大就会让单条事件占用过多内存。
	maxInlineLimit = Size(64 << 20)
	// maxStreamThreshold 是分档阈值的上限，超过就没有分档的意义。
	maxStreamThreshold = Size(256 << 20)
)

// maxProxyConcurrency 是同时向上游发起的取图请求数上限。
const maxProxyConcurrency = 32

// 向框架自报的程序名。兼容模式使用另一个项目的名字，见 Compat。
const (
	appNameDefault = "HelloYunhu"
	appNameCompat  = "gensokyo"
)

// 群标签的默认取值，以及 set_group_special_title 的两种语义。
const (
	// defaultTagColor 是自动创建标签时使用的颜色。
	defaultTagColor = "#03A9F4"
	// TagModeReplace 表示设置头衔时解绑该用户的其余标签，保证只剩一个。
	TagModeReplace = "replace"
	// TagModeAppend 表示设置头衔时只追加，不动已有标签。
	TagModeAppend = "append"
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
	// Media 是两个方向的媒体体积上限。
	Media Media `yaml:"media"`
	// Proxy 是对外提供云湖媒体与头像的 HTTP 服务。
	Proxy Proxy `yaml:"proxy"`
	// Compat 是给非标准客户端准备的兼容设置。
	Compat Compat `yaml:"compat"`
	// Tags 是群标签相关的设置。
	Tags Tags `yaml:"tags"`
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

// Compat 是与非标准客户端配合的设置。
//
// 这些开关会让程序以别的项目身份出现，只在不这么做时客户端取不到数据的情况下才
// 使用。默认全部关闭，关闭时程序的行为与没有这些能力时完全一致。
type Compat struct {
	// Gensokyo 是兼容 Gensokyo 扩展的设置。
	Gensokyo Gensokyo `yaml:"gensokyo"`
}

// Gensokyo 是自报 Gensokyo 身份并提供头像扩展的设置。
//
// Overflow 这类客户端只在协议端自报 gensokyo 时才调用非标准的 get_avatar 取头像，
// 其余情况一律拿整数标识当 QQ 号去 qlogo 取，接在非 QQ 平台上会显示成陌生人的
// 头像。开启之后程序自报这个名字，并由代理服务提供云湖的头像地址。
//
// 头像地址由 Proxy 段提供，因此开启这一项要求 proxy.enabled 也为真。
type Gensokyo struct {
	// Enabled 是兼容模式的唯一开关。
	Enabled bool `yaml:"enabled"`
}

// Tags 是群标签相关的设置。
//
// 云湖把群标签当作群级资源：每个群有一组标签定义，成员与标签的关系单独存储。
// 这些设置只影响标准接口 set_group_special_title 的行为，扩展接口不受约束。
type Tags struct {
	// AutoCreate 表示设置头衔时标签不存在就自动创建。
	//
	// 打开之后插件设置任意头衔都能生效，不需要群主事先建好同名标签；代价是标签
	// 列表里会出现插件用过的一次性名字。关闭时按标签不存在报错，并在说明里列出
	// 当前已有的标签。
	AutoCreate bool `yaml:"auto_create"`
	// Color 是自动创建标签时使用的颜色，格式为 #RRGGBB。
	Color string `yaml:"color"`
	// SpecialTitle 是标准接口 set_group_special_title 的行为。
	SpecialTitle SpecialTitle `yaml:"special_title"`
}

// SpecialTitle 是标准接口 set_group_special_title 的行为。
//
// OneBot 的这个接口只给一个字符串，而云湖允许一个成员挂多个标签，因此需要说明
// 用哪一种语义。
type SpecialTitle struct {
	// Mode 取 replace 或 append。replace 表示设置后只剩这一个标签，与 QQ 的
	// 单值头衔一致；append 表示只追加，不动成员已有的标签。
	Mode string `yaml:"mode"`
	// ClearOnEmpty 表示头衔置空时解绑该成员的剩余全部标签。
	//
	// 为假时置空返回错误而不是静默成功：调用方以为清掉了、实际没清，比报错更
	// 难排查。
	ClearOnEmpty bool `yaml:"clear_on_empty"`
}

// Media 是两个方向的媒体体积上限。
//
// 这一组只限制单次能处理多大的媒体，与代理服务无关。
type Media struct {
	// InlineLimit 是接收方向内联为 base64 的体积上限。
	//
	// 云湖的多媒体超过这个体积时不再内联为 base64，消息段里只给出地址。
	InlineLimit Size `yaml:"inline_limit"`
	// ReadLimit 是发送方向读取框架给出媒体的体积上限。
	//
	// 框架给出的多媒体超过这个体积时，放弃这次发送。
	ReadLimit Size `yaml:"read_limit"`
}

// Proxy 是对外提供云湖媒体与头像的 HTTP 服务。
//
// 客户端拿到的地址由本程序代取，取图时附带云湖 CDN 要求的来源头。关闭时消息段里
// 不再给出可下载的地址，头像接口按未实现处理。
type Proxy struct {
	// Enabled 为真时启动代理服务。
	Enabled bool `yaml:"enabled"`
	// Listen 是监听地址。
	Listen string `yaml:"listen"`
	// BaseURL 是客户端访问代理时使用的地址，只含协议、主机与端口。
	//
	// 留空时按 Listen 推导。框架与本程序不在同一台机器时，必须填写框架能访问到的
	// 地址，否则客户端会把 127.0.0.1 解析到它自己。
	BaseURL string `yaml:"base_url"`
	// StreamThreshold 是整块读取与边取边发的分界体积。
	//
	// 不超过该体积时先取完再返回，响应要么完整要么报错；超过时边取边发，避免大
	// 文件整块占用内存。
	StreamThreshold Size `yaml:"stream_threshold"`
	// MediaMax 是单次转发允许的体积上限，0 表示不限。
	MediaMax Size `yaml:"media_max"`
	// Concurrency 是同时向上游发起的取图请求数。
	Concurrency int `yaml:"concurrency"`
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

// Size 是带单位的体积，支持 B、KiB、MiB、GiB 与 KB、MB、GB 两类写法。
//
// 带 i 的单位按 1024 进位，不带 i 的按 1000 进位。配置里统一写成 20MiB、1GiB
// 这类形式，避免直接写字节数时数不清零。
type Size int64

// sizeUnits 是支持的体积单位，长后缀排在前面，避免 20MiB 被当成 20B。
var sizeUnits = []struct {
	suffix string
	factor int64
}{
	{suffix: "GIB", factor: 1 << 30},
	{suffix: "MIB", factor: 1 << 20},
	{suffix: "KIB", factor: 1 << 10},
	{suffix: "GB", factor: 1000 * 1000 * 1000},
	{suffix: "MB", factor: 1000 * 1000},
	{suffix: "KB", factor: 1000},
	{suffix: "B", factor: 1},
}

// ParseSize 解析带单位的体积。
func ParseSize(text string) (int64, error) {
	raw := strings.TrimSpace(text)
	if raw == "" {
		return 0, errors.New("体积不能为空")
	}

	upper := strings.ToUpper(raw)
	for _, unit := range sizeUnits {
		if !strings.HasSuffix(upper, unit.suffix) {
			continue
		}

		number := strings.TrimSpace(raw[:len(raw)-len(unit.suffix)])
		value, err := strconv.ParseInt(number, 10, 64)
		if err != nil || value < 0 {
			return 0, fmt.Errorf("无法解析体积 %q，数字部分应为非负整数", raw)
		}
		return value * unit.factor, nil
	}

	return 0, fmt.Errorf("体积 %q 缺少单位，可写 B、KiB、MiB、GiB、KB、MB 或 GB", raw)
}

// UnmarshalYAML 把配置中的字符串转换为体积。
func (s *Size) UnmarshalYAML(value *yaml.Node) error {
	var text string
	if err := value.Decode(&text); err != nil {
		return fmt.Errorf("需要写成带单位的字符串，例如 8MiB: %w", err)
	}
	parsed, err := ParseSize(text)
	if err != nil {
		return err
	}
	*s = Size(parsed)
	return nil
}

// MarshalYAML 以可读的形式输出，能整除时用 MiB 或 GiB，否则给出字节数。
func (s Size) MarshalYAML() (any, error) {
	switch {
	case s > 0 && s%(1<<30) == 0:
		return fmt.Sprintf("%dGiB", s/(1<<30)), nil
	case s > 0 && s%(1<<20) == 0:
		return fmt.Sprintf("%dMiB", s/(1<<20)), nil
	case s > 0 && s%(1<<10) == 0:
		return fmt.Sprintf("%dKiB", s/(1<<10)), nil
	default:
		return fmt.Sprintf("%dB", int64(s)), nil
	}
}

// Bytes 返回字节数。
func (s Size) Bytes() int64 {
	return int64(s)
}

// String 返回可读的体积。
func (s Size) String() string {
	value, _ := s.MarshalYAML()
	return value.(string)
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
		Media: Media{
			InlineLimit: Size(defaultInlineLimit),
			ReadLimit:   Size(defaultReadLimit),
		},
		Proxy: Proxy{
			Listen:          defaultProxyAddr,
			StreamThreshold: Size(defaultStreamLimit),
			MediaMax:        Size(defaultMediaMax),
			Concurrency:     defaultConcurrency,
		},
		Tags: Tags{
			AutoCreate: true,
			Color:      defaultTagColor,
			SpecialTitle: SpecialTitle{
				Mode:         TagModeReplace,
				ClearOnEmpty: true,
			},
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

	if c.Proxy.Enabled && strings.TrimSpace(c.Proxy.BaseURL) == "" {
		c.Proxy.BaseURL = deriveBaseURL(c.Proxy.Listen)
	}

	// 颜色比较时不分大小写，存成大写便于日志与配置互相对照。
	if color := strings.TrimSpace(c.Tags.Color); color != "" {
		c.Tags.Color = strings.ToUpper(color)
	}
}

// AppName 返回向框架自报的程序名。
//
// 兼容模式开启时自报 gensokyo，供只认这个名字的客户端启用头像扩展。这个名字是
// 兼容用的，不代表本项目的身份。
func (c *Config) AppName() string {
	if c.Compat.Gensokyo.Enabled {
		return appNameCompat
	}
	return appNameDefault
}

// ProxyBaseURL 返回代理服务的对外地址，服务未启用时为空。
func (c *Config) ProxyBaseURL() string {
	if !c.Proxy.Enabled {
		return ""
	}
	return strings.TrimRight(c.Proxy.BaseURL, "/")
}

// AvatarBaseURL 返回头像地址的对外前缀，代理未启用或兼容模式关闭时为空。
func (c *Config) AvatarBaseURL() string {
	if !c.Compat.Gensokyo.Enabled {
		return ""
	}
	return c.ProxyBaseURL()
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
	problems = append(problems, c.validateMedia()...)
	problems = append(problems, c.validateProxy()...)
	problems = append(problems, c.validateCompat()...)
	problems = append(problems, c.validateTags()...)

	return errors.Join(problems...)
}

// validateMedia 校验两个方向的媒体体积上限。
func (c *Config) validateMedia() []error {
	var problems []error

	if c.Media.InlineLimit <= 0 {
		problems = append(problems, errors.New("media.inline_limit: 必须大于 0"))
	} else if c.Media.InlineLimit > maxInlineLimit {
		problems = append(problems, fmt.Errorf(
			"media.inline_limit: 不超过 %s，再大就会让单条事件占用过多内存", maxInlineLimit))
	}
	if c.Media.ReadLimit <= 0 {
		problems = append(problems, errors.New("media.read_limit: 必须大于 0"))
	}

	return problems
}

// validateProxy 校验代理服务的设置。
//
// 开关为假时忽略取值，但写法本身仍然校验，避免开关打开后才暴露拼写问题。
func (c *Config) validateProxy() []error {
	var problems []error

	listen := strings.TrimSpace(c.Proxy.Listen)
	if listen != "" {
		if _, _, err := net.SplitHostPort(listen); err != nil {
			problems = append(problems, fmt.Errorf(
				"proxy.listen: %q 不是合法的监听地址，例如 127.0.0.1:8082", c.Proxy.Listen))
		}
	} else if c.Proxy.Enabled {
		problems = append(problems, errors.New("proxy.listen: 启用代理服务时不能为空"))
	}

	baseURL := strings.TrimSpace(c.Proxy.BaseURL)
	if baseURL != "" {
		problems = append(problems, validateBaseURL("proxy.base_url", baseURL)...)
	} else if c.Proxy.Enabled {
		problems = append(problems, errors.New(
			"proxy.base_url: 启用代理服务时不能为空，且无法从监听地址推导"))
	}

	if c.Proxy.StreamThreshold <= 0 {
		problems = append(problems, errors.New("proxy.stream_threshold: 必须大于 0"))
	} else if c.Proxy.StreamThreshold > maxStreamThreshold {
		problems = append(problems, fmt.Errorf(
			"proxy.stream_threshold: 不超过 %s，超过就没有分档的意义", maxStreamThreshold))
	}
	if c.Proxy.MediaMax < 0 {
		problems = append(problems, errors.New("proxy.media_max: 不能为负数，0 表示不限"))
	} else if c.Proxy.MediaMax > 0 && c.Proxy.MediaMax < c.Proxy.StreamThreshold {
		problems = append(problems, errors.New("proxy.media_max: 不能小于 proxy.stream_threshold，0 表示不限"))
	}
	if c.Proxy.Concurrency < 1 || c.Proxy.Concurrency > maxProxyConcurrency {
		problems = append(problems, fmt.Errorf("proxy.concurrency: 取 1 到 %d", maxProxyConcurrency))
	}

	return problems
}

// validateCompat 校验兼容模式的设置。
func (c *Config) validateCompat() []error {
	if !c.Compat.Gensokyo.Enabled {
		return nil
	}
	if !c.Proxy.Enabled {
		return []error{errors.New(
			"compat.gensokyo.enabled: 需要 proxy.enabled 同时为真，否则客户端拿到的头像地址无法访问")}
	}
	return nil
}

// validateTags 校验群标签的设置。
//
// 颜色与语义在自动创建标签、设置头衔时才会用到，但写法本身总是校验，避免打开
// 自动创建之后才发现拼写错误。
func (c *Config) validateTags() []error {
	var problems []error

	if !isHexColor(c.Tags.Color) {
		problems = append(problems, fmt.Errorf(
			"tags.color: %q 需要写成 #RRGGBB，例如 %s", c.Tags.Color, defaultTagColor))
	}

	switch c.Tags.SpecialTitle.Mode {
	case TagModeReplace, TagModeAppend:
	default:
		problems = append(problems, fmt.Errorf(
			"tags.special_title.mode: %q 不受支持，可选 replace 或 append", c.Tags.SpecialTitle.Mode))
	}

	return problems
}

// isHexColor 判断是否为 #RRGGBB 形式。
func isHexColor(color string) bool {
	if len(color) != 7 || color[0] != '#' {
		return false
	}
	for _, char := range color[1:] {
		switch {
		case char >= '0' && char <= '9', char >= 'A' && char <= 'F', char >= 'a' && char <= 'f':
		default:
			return false
		}
	}
	return true
}

// deriveBaseURL 按监听地址推导客户端访问头像时使用的前缀。
//
// 监听在全部网卡上时取回环地址：能启动代理的机器上通常也能访问 127.0.0.1，而
// 0.0.0.0 对客户端不是一个可用的目标。
func deriveBaseURL(listen string) string {
	host, port, err := net.SplitHostPort(strings.TrimSpace(listen))
	if err != nil {
		return ""
	}
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

// validateBaseURL 校验头像代理的对外前缀。
//
// 前缀只允许协议、主机与端口，路径由程序固定拼成 /avatar/<标识>，因此带路径的
// 写法会被拒绝，而不是被默默拼接成一个奇怪的地址。
func validateBaseURL(field, raw string) []error {
	problems := validateHTTPURL(field, raw)
	if len(problems) > 0 {
		return problems
	}

	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return []error{fmt.Errorf("%s: 无法解析地址 %q: %w", field, raw, err)}
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return []error{fmt.Errorf(
			"%s: 只填写协议、主机与端口，不要带路径，头像地址由程序拼成 %s/avatar/<标识>", field, strings.TrimRight(raw, "/"))}
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return []error{fmt.Errorf("%s: 不能带查询串或片段", field)}
	}
	return nil
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
