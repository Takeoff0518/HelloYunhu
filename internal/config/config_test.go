// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// minimalYAML 是能通过校验的最小配置，用于给各用例补齐必需字段。
const minimalYAML = `
bot:
  token: token-value
onebot:
  - url: ws://127.0.0.1:6199/ws
`

func TestParseAppliesDefaults(t *testing.T) {
	cfg, err := Parse([]byte(minimalYAML))
	if err != nil {
		t.Fatalf("Parse 返回错误: %v", err)
	}

	if cfg.Receive.Mode != ReceiveWebsocket {
		t.Errorf("接收方式默认值为 %q", cfg.Receive.Mode)
	}
	if cfg.Log.Level != "info" || cfg.Log.Format != "text" {
		t.Errorf("日志默认值为 %+v", cfg.Log)
	}
	if cfg.Log.KeepDays != 30 || !cfg.Log.Compress {
		t.Errorf("日志轮转默认值为 %d、%v", cfg.Log.KeepDays, cfg.Log.Compress)
	}
	if got := cfg.Storage.MessageTTL.Standard(); got != 168*time.Hour {
		t.Errorf("消息保留时长默认值为 %v", got)
	}
	if !cfg.Capabilities.CanSendImage || cfg.Capabilities.CanSendRecord {
		t.Errorf("能力声明默认值为 %+v", cfg.Capabilities)
	}
	if len(cfg.OneBot) != 1 {
		t.Fatalf("连接数量为 %d", len(cfg.OneBot))
	}
	if cfg.OneBot[0].Name != "onebot-1" {
		t.Errorf("连接名称默认值为 %q", cfg.OneBot[0].Name)
	}
}

// TestNormalizesConnectionNames 校验未命名的连接按顺序命名，已有名称保持不变。
func TestNormalizesConnectionNames(t *testing.T) {
	const raw = `
bot:
  token: token-value
onebot:
  - url: ws://127.0.0.1:6199/ws
  - name: nonebot
    url: ws://127.0.0.1:8080/onebot/v11/ws
  - url: ws://127.0.0.1:9000/ws
`
	cfg, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse 返回错误: %v", err)
	}

	want := []string{"onebot-1", "nonebot", "onebot-3"}
	for index, name := range want {
		if cfg.OneBot[index].Name != name {
			t.Errorf("第 %d 个连接名称为 %q，期望 %q", index+1, cfg.OneBot[index].Name, name)
		}
	}
}

func TestParseOverridesDefaults(t *testing.T) {
	const raw = `
bot:
  token: token-value
receive:
  mode: both
  webhook:
    listen: 0.0.0.0:8081
    token: 7c1f0a9e5b2d48c3a6f1e0d9b8c7a6f5
    allowed_ips:
      - 203.0.113.0/24
log:
  level: debug
  keep_days: 7
  compress: false
yunhu:
  rate_limit: 5
onebot:
  - url: ws://127.0.0.1:6199/ws
    access_token: secret
`
	cfg, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse 返回错误: %v", err)
	}

	if cfg.Receive.Mode != ReceiveBoth || !cfg.Receive.UsesWebhook() {
		t.Errorf("接收方式为 %q", cfg.Receive.Mode)
	}
	if cfg.Receive.Webhook.Token != "7c1f0a9e5b2d48c3a6f1e0d9b8c7a6f5" {
		t.Errorf("webhook 令牌为 %q", cfg.Receive.Webhook.Token)
	}
	if len(cfg.Receive.Webhook.AllowedIPs) != 1 {
		t.Errorf("来源限制为 %v", cfg.Receive.Webhook.AllowedIPs)
	}
	if cfg.Log.KeepDays != 7 || cfg.Log.Compress {
		t.Errorf("日志轮转设置为 %d、%v", cfg.Log.KeepDays, cfg.Log.Compress)
	}
	if cfg.Yunhu.RateLimit != 5 {
		t.Errorf("限流为 %d", cfg.Yunhu.RateLimit)
	}
	if cfg.OneBot[0].AccessToken != "secret" {
		t.Errorf("连接令牌为 %q", cfg.OneBot[0].AccessToken)
	}
}

func TestValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "缺少机器人令牌",
			raw:  "onebot:\n  - url: ws://127.0.0.1:6199/ws\n",
			want: "bot.token",
		},
		{
			name: "没有框架连接",
			raw:  "bot:\n  token: token-value\nonebot: []\n",
			want: "onebot",
		},
		{
			name: "接收方式非法",
			raw:  "bot:\n  token: t\nreceive:\n  mode: pigeon\nonebot:\n  - url: ws://127.0.0.1:6199/ws\n",
			want: "receive.mode",
		},
		{
			name: "webhook 缺少令牌",
			raw:  "bot:\n  token: t\nreceive:\n  mode: webhook\nonebot:\n  - url: ws://127.0.0.1:6199/ws\n",
			want: "receive.webhook.token",
		},
		{
			name: "webhook 令牌过短",
			raw:  "bot:\n  token: t\nreceive:\n  mode: webhook\n  webhook:\n    token: short\nonebot:\n  - url: ws://127.0.0.1:6199/ws\n",
			want: "至少 24 个字符",
		},
		{
			name: "来源网段非法",
			raw:  "bot:\n  token: t\nreceive:\n  mode: webhook\n  webhook:\n    token: 7c1f0a9e5b2d48c3a6f1e0d9b8c7a6f5\n    allowed_ips:\n      - 不是网段\nonebot:\n  - url: ws://127.0.0.1:6199/ws\n",
			want: "allowed_ips",
		},
		{
			name: "连接名称重复",
			raw:  "bot:\n  token: t\nonebot:\n  - name: a\n    url: ws://127.0.0.1:6199/ws\n  - name: a\n    url: ws://127.0.0.1:6199/ws\n",
			want: "重复",
		},
		{
			name: "框架地址协议错误",
			raw:  "bot:\n  token: t\nonebot:\n  - url: http://127.0.0.1:6199/ws\n",
			want: "ws 或 wss",
		},
		{
			name: "日志保留天数非法",
			raw:  "bot:\n  token: t\nlog:\n  keep_days: 0\nonebot:\n  - url: ws://127.0.0.1:6199/ws\n",
			want: "log.keep_days",
		},
		{
			name: "未知配置项",
			raw:  minimalYAML + "instances:\n  - name: main\n",
			want: "field instances not found",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.raw))
			if err == nil {
				t.Fatal("应返回校验错误")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("错误信息中缺少 %q，实际为: %v", tc.want, err)
			}
		})
	}
}

// TestWebhookTokenOnlyRequiredWhenUsed 校验只用订阅连接时不需要 webhook 令牌。
func TestWebhookTokenOnlyRequiredWhenUsed(t *testing.T) {
	if _, err := Parse([]byte(minimalYAML)); err != nil {
		t.Errorf("默认接收方式不应要求 webhook 令牌: %v", err)
	}
}

// TestExampleConfigIsValid 校验仓库里的示例配置与结构保持同步。
func TestExampleConfigIsValid(t *testing.T) {
	if _, err := Load(filepath.Join("..", "..", "examples", "config.yaml")); err != nil {
		t.Fatalf("示例配置不能通过校验: %v", err)
	}
}
