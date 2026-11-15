// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// minimalYAML 是能通过校验的最小配置，用于给各用例补齐必需字段。
const minimalYAML = `
instances:
  - name: main
    token: token-value
    onebot:
      url: ws://127.0.0.1:6199/ws
`

func TestParseAppliesDefaults(t *testing.T) {
	cfg, err := Parse([]byte(minimalYAML))
	if err != nil {
		t.Fatalf("Parse 返回错误: %v", err)
	}

	if cfg.Log.Level != "info" || cfg.Log.Format != "text" {
		t.Errorf("日志默认值为 %+v，期望 info 与 text", cfg.Log)
	}
	if cfg.Storage.Path != "data/helloyunhu.db" {
		t.Errorf("存储路径默认值为 %q", cfg.Storage.Path)
	}
	if got := cfg.Storage.MessageTTL.Standard(); got != 168*time.Hour {
		t.Errorf("消息保留时长默认值为 %v，期望 168h", got)
	}
	if cfg.Yunhu.APIBase != "https://chat-go.jwzhd.com/open-apis/v1" {
		t.Errorf("接口地址默认值为 %q", cfg.Yunhu.APIBase)
	}
	if cfg.Yunhu.SubscribeURL != "wss://ws.jwzhd.com/subscribe" {
		t.Errorf("订阅地址默认值为 %q", cfg.Yunhu.SubscribeURL)
	}
	if len(cfg.Instances) != 1 {
		t.Fatalf("实例数量为 %d，期望 1", len(cfg.Instances))
	}
	if cfg.Instances[0].OneBot.AccessToken != "" {
		t.Errorf("访问令牌默认值应为空，实际为 %q", cfg.Instances[0].OneBot.AccessToken)
	}
	if !cfg.Capabilities.CanSendImage {
		t.Error("can_send_image 默认应为真")
	}
	if cfg.Capabilities.CanSendRecord {
		t.Error("can_send_record 默认应为假，平台只能按文件发送语音")
	}
	if cfg.Log.KeepDays != 30 {
		t.Errorf("日志保留天数默认值为 %d，期望 30", cfg.Log.KeepDays)
	}
	if !cfg.Log.Compress {
		t.Error("日志压缩默认应为真")
	}
}

// TestLogRotationOverrides 校验日志保留天数与压缩开关可以覆盖。
func TestLogRotationOverrides(t *testing.T) {
	const raw = minimalYAML + `
log:
  keep_days: 7
  compress: false
`
	cfg, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse 返回错误: %v", err)
	}
	if cfg.Log.KeepDays != 7 {
		t.Errorf("保留天数为 %d，期望 7", cfg.Log.KeepDays)
	}
	if cfg.Log.Compress {
		t.Error("compress 应被覆盖为假")
	}
}

// TestLogKeepDaysInfinite 校验 -1 表示永不删除。
func TestLogKeepDaysInfinite(t *testing.T) {
	cfg, err := Parse([]byte(minimalYAML + "log:\n  keep_days: -1\n"))
	if err != nil {
		t.Fatalf("Parse 返回错误: %v", err)
	}
	if cfg.Log.KeepDays != -1 {
		t.Errorf("保留天数为 %d，期望 -1", cfg.Log.KeepDays)
	}
}

// TestLogKeepDaysInvalid 校验 0 与小于 -1 的取值被拒绝。
func TestLogKeepDaysInvalid(t *testing.T) {
	for _, value := range []string{"0", "-2"} {
		raw := minimalYAML + "log:\n  keep_days: " + value + "\n"
		if _, err := Parse([]byte(raw)); err == nil {
			t.Errorf("keep_days 为 %s 时应报错", value)
		} else if !strings.Contains(err.Error(), "log.keep_days") {
			t.Errorf("错误信息应指出 log.keep_days，实际为: %v", err)
		}
	}
}

func TestCapabilityOverrides(t *testing.T) {
	const raw = minimalYAML + `
capabilities:
  can_send_image: false
  can_send_record: true
`
	cfg, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse 返回错误: %v", err)
	}
	if cfg.Capabilities.CanSendImage {
		t.Error("can_send_image 应被覆盖为假")
	}
	if !cfg.Capabilities.CanSendRecord {
		t.Error("can_send_record 应被覆盖为真")
	}
}

// TestCapabilityPartialOverride 校验只写一项时，另一项保留默认值。
//
// 布尔值的零值与「没有配置」无法区分，这里专门覆盖指针解码的那条路径。
func TestCapabilityPartialOverride(t *testing.T) {
	const raw = minimalYAML + `
capabilities:
  can_send_record: true
`
	cfg, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse 返回错误: %v", err)
	}
	if !cfg.Capabilities.CanSendImage {
		t.Error("未配置的 can_send_image 应保留默认的真")
	}
}

func TestParseOverridesDefaults(t *testing.T) {
	const raw = `
log:
  level: debug
  format: json
storage:
  path: /var/lib/helloyunhu/data.db
  message_ttl: 24h
yunhu:
  api_base: https://example.com/open-apis/v1
  subscribe_url: wss://example.com/subscribe
instances:
  - name: first
    token: token-one
    onebot:
      url: ws://127.0.0.1:6199/ws
      access_token: secret
  - name: second
    token: token-two
    onebot:
      url: ws://127.0.0.1:6200/ws
`

	cfg, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse 返回错误: %v", err)
	}

	if cfg.Log.Level != "debug" || cfg.Log.Format != "json" {
		t.Errorf("日志配置为 %+v", cfg.Log)
	}
	if cfg.Storage.Path != "/var/lib/helloyunhu/data.db" {
		t.Errorf("存储路径为 %q", cfg.Storage.Path)
	}
	if got := cfg.Storage.MessageTTL.Standard(); got != 24*time.Hour {
		t.Errorf("消息保留时长为 %v", got)
	}
	if cfg.Yunhu.APIBase != "https://example.com/open-apis/v1" {
		t.Errorf("接口地址为 %q", cfg.Yunhu.APIBase)
	}
	if cfg.Yunhu.SubscribeURL != "wss://example.com/subscribe" {
		t.Errorf("订阅地址为 %q", cfg.Yunhu.SubscribeURL)
	}
	if len(cfg.Instances) != 2 {
		t.Fatalf("实例数量为 %d，期望 2", len(cfg.Instances))
	}
	if cfg.Instances[1].Name != "second" || cfg.Instances[1].OneBot.URL != "ws://127.0.0.1:6200/ws" {
		t.Errorf("第二个实例为 %+v", cfg.Instances[1])
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "空配置",
			raw:  "",
			want: []string{"配置内容为空"},
		},
		{
			name: "未知字段",
			raw:  minimalYAML + "\nunknown_field: 1\n",
			want: []string{"YAML 格式错误", "unknown_field"},
		},
		{
			name: "日志级别不合法",
			raw:  "log:\n  level: verbose\n" + minimalYAML,
			want: []string{"log.level", "不受支持"},
		},
		{
			name: "日志格式不合法",
			raw:  "log:\n  format: xml\n" + minimalYAML,
			want: []string{"log.format", "不受支持"},
		},
		{
			name: "存储路径为空",
			raw:  "storage:\n  path: \"\"\n" + minimalYAML,
			want: []string{"storage.path"},
		},
		{
			name: "消息保留时长不合法",
			raw:  "storage:\n  message_ttl: 0h\n" + minimalYAML,
			want: []string{"storage.message_ttl"},
		},
		{
			name: "消息保留时长格式错误",
			raw:  "storage:\n  message_ttl: 7 days\n" + minimalYAML,
			want: []string{"无法解析时间间隔"},
		},
		{
			name: "接口地址协议错误",
			raw:  "yunhu:\n  api_base: ftp://example.com\n" + minimalYAML,
			want: []string{"yunhu.api_base", "http 或 https"},
		},
		{
			name: "订阅地址协议错误",
			raw:  "yunhu:\n  subscribe_url: https://example.com/subscribe\n" + minimalYAML,
			want: []string{"yunhu.subscribe_url", "ws 或 wss"},
		},
		{
			name: "没有实例",
			raw:  "log:\n  level: debug\n",
			want: []string{"instances", "至少需要配置一个机器人实例"},
		},
		{
			name: "实例名称为空",
			raw: `
instances:
  - name: ""
    token: token-value
    onebot:
      url: ws://127.0.0.1:6199/ws
`,
			want: []string{"instances[0].name"},
		},
		{
			name: "实例名称重复",
			raw: `
instances:
  - name: main
    token: token-one
    onebot:
      url: ws://127.0.0.1:6199/ws
  - name: main
    token: token-two
    onebot:
      url: ws://127.0.0.1:6200/ws
`,
			want: []string{"instances[1].name", "重复"},
		},
		{
			name: "实例令牌为空",
			raw: `
instances:
  - name: main
    token: ""
    onebot:
      url: ws://127.0.0.1:6199/ws
`,
			want: []string{"instances[0].token"},
		},
		{
			name: "框架地址协议错误",
			raw: `
instances:
  - name: main
    token: token-value
    onebot:
      url: http://127.0.0.1:6199/ws
`,
			want: []string{"instances[0].onebot.url", "ws 或 wss"},
		},
		{
			name: "框架地址为空",
			raw: `
instances:
  - name: main
    token: token-value
    onebot:
      url: ""
`,
			want: []string{"instances[0].onebot.url", "不能为空"},
		},
		{
			name: "多个问题一起返回",
			raw:  "log:\n  level: verbose\n  format: xml\n",
			want: []string{"log.level", "log.format", "instances"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.raw))
			if err == nil {
				t.Fatal("期望返回错误，实际通过校验")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("错误信息中缺少 %q，实际为:\n%v", want, err)
				}
			}
		})
	}
}

func TestParseRawEvents(t *testing.T) {
	raw := "log:\n  raw_events: true\n" + minimalYAML

	cfg, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse 返回错误: %v", err)
	}
	if !cfg.Log.RawEvents {
		t.Error("raw_events 应为 true")
	}

	defaults, err := Parse([]byte(minimalYAML))
	if err != nil {
		t.Fatalf("Parse 返回错误: %v", err)
	}
	if defaults.Log.RawEvents {
		t.Error("raw_events 默认应为 false")
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(minimalYAML), 0o600); err != nil {
		t.Fatalf("写入临时配置失败: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 返回错误: %v", err)
	}
	if len(cfg.Instances) != 1 || cfg.Instances[0].Name != "main" {
		t.Errorf("加载后的实例为 %+v", cfg.Instances)
	}
}

func TestLoadMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.yaml")

	_, err := Load(path)
	if err == nil {
		t.Fatal("文件不存在时应返回错误")
	}
	if !strings.Contains(err.Error(), "读取配置文件") || !strings.Contains(err.Error(), path) {
		t.Errorf("错误信息应说明读取失败与文件路径，实际为: %v", err)
	}
}

func TestLoadExample(t *testing.T) {
	// 示例配置必须能通过校验，否则用户复制后无法直接使用。
	if _, err := Load(filepath.Join("..", "..", "examples", "config.yaml")); err != nil {
		t.Fatalf("示例配置未通过校验: %v", err)
	}
}

func TestDurationMarshal(t *testing.T) {
	got, err := Duration(90 * time.Minute).MarshalYAML()
	if err != nil {
		t.Fatalf("MarshalYAML 返回错误: %v", err)
	}
	if got != "1h30m0s" {
		t.Errorf("序列化结果为 %v，期望 1h30m0s", got)
	}
}
