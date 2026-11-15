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
	defaultStoragePath  = "data/helloyunhu.db"
	defaultMessageTTL   = 168 * time.Hour
	defaultAPIBase      = "https://chat-go.jwzhd.com/open-apis/v1"
	defaultSubscribeURL = "wss://ws.jwzhd.com/subscribe"
)

// Config 是进程的完整配置。
type Config struct {
	// Log 是日志设置。
	Log Log `yaml:"log"`
	// Storage 是持久化设置。
	Storage Storage `yaml:"storage"`
	// Yunhu 是云湖平台的连接地址，所有实例共用。
	Yunhu Yunhu `yaml:"yunhu"`
	// Instances 是机器人实例列表，至少需要一个。
	Instances []Instance `yaml:"instances"`
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
	// File 是日志文件路径，为空时只输出到终端。
	//
	// 设置后同时写入终端与文件，文件以追加方式打开，不做自动轮转。
	File string `yaml:"file"`
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
}

// Instance 描述一个云湖机器人实例。
//
// 一个进程可以同时运行多个实例，实例之间不共享运行状态，因此需要各自的名称
// 与令牌，以及各自的框架连接地址。
type Instance struct {
	// Name 是实例名称，用于日志与诊断，必须唯一。
	Name string `yaml:"name"`
	// Token 是云湖机器人的令牌。
	Token string `yaml:"token"`
	// OneBot 是框架侧的连接设置。
	OneBot OneBot `yaml:"onebot"`
}

// OneBot 是与机器人框架之间的连接设置。
type OneBot struct {
	// URL 是框架监听的反向 WebSocket 地址。
	URL string `yaml:"url"`
	// AccessToken 是框架侧配置的访问令牌，未设置时留空。
	AccessToken string `yaml:"access_token"`
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
		Log: Log{
			Level:  defaultLogLevel,
			Format: defaultLogFormat,
		},
		Storage: Storage{
			Path:       defaultStoragePath,
			MessageTTL: Duration(defaultMessageTTL),
		},
		Yunhu: Yunhu{
			APIBase:      defaultAPIBase,
			SubscribeURL: defaultSubscribeURL,
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

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("配置校验未通过:\n%w", err)
	}

	return cfg, nil
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

	if strings.TrimSpace(c.Storage.Path) == "" {
		problems = append(problems, errors.New("storage.path: 不能为空"))
	}
	if c.Storage.MessageTTL.Standard() <= 0 {
		problems = append(problems, errors.New("storage.message_ttl: 必须大于 0"))
	}

	problems = append(problems, validateHTTPURL("yunhu.api_base", c.Yunhu.APIBase)...)
	problems = append(problems, validateWebSocketURL("yunhu.subscribe_url", c.Yunhu.SubscribeURL)...)

	if len(c.Instances) == 0 {
		problems = append(problems, errors.New("instances: 至少需要配置一个机器人实例"))
	}

	seen := make(map[string]int, len(c.Instances))
	for i, inst := range c.Instances {
		field := fmt.Sprintf("instances[%d]", i)

		switch {
		case strings.TrimSpace(inst.Name) == "":
			problems = append(problems, fmt.Errorf("%s.name: 不能为空", field))
		default:
			if previous, ok := seen[inst.Name]; ok {
				problems = append(problems, fmt.Errorf("%s.name: 与 instances[%d] 重复，实例名称必须唯一", field, previous))
			} else {
				seen[inst.Name] = i
			}
		}

		if strings.TrimSpace(inst.Token) == "" {
			problems = append(problems, fmt.Errorf("%s.token: 不能为空", field))
		}

		problems = append(problems, validateWebSocketURL(field+".onebot.url", inst.OneBot.URL)...)
	}

	return errors.Join(problems...)
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
