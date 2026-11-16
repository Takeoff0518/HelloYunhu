// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"strings"
	"testing"
)

// withProxy 在基础配置上追加一段代理设置。
func withProxy(body string) string {
	return minimalYAML + "proxy:\n" + body
}

func TestMediaDefaults(t *testing.T) {
	cfg, err := Parse([]byte(minimalYAML))
	if err != nil {
		t.Fatalf("Parse 返回错误: %v", err)
	}

	if got := cfg.Media.InlineLimit.Bytes(); got != 20<<20 {
		t.Errorf("接收方向内联上限为 %d", got)
	}
	if got := cfg.Media.ReadLimit.Bytes(); got != 20<<20 {
		t.Errorf("发送方向读取上限为 %d", got)
	}
}

func TestMediaOverrides(t *testing.T) {
	raw := minimalYAML + `
media:
  inline_limit: 8MiB
  read_limit: 32MiB
`
	cfg, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse 返回错误: %v", err)
	}

	if got := cfg.Media.InlineLimit.Bytes(); got != 8<<20 {
		t.Errorf("接收方向内联上限为 %d", got)
	}
	if got := cfg.Media.ReadLimit.Bytes(); got != 32<<20 {
		t.Errorf("发送方向读取上限为 %d", got)
	}
}

func TestProxyDefaults(t *testing.T) {
	cfg, err := Parse([]byte(minimalYAML))
	if err != nil {
		t.Fatalf("Parse 返回错误: %v", err)
	}

	if cfg.Proxy.Enabled {
		t.Error("代理默认应关闭")
	}
	if cfg.Proxy.Listen != defaultProxyAddr {
		t.Errorf("监听地址默认为 %q", cfg.Proxy.Listen)
	}
	if got := cfg.Proxy.StreamThreshold.Bytes(); got != 8<<20 {
		t.Errorf("分档阈值为 %d", got)
	}
	if got := cfg.Proxy.MediaMax.Bytes(); got != 1<<30 {
		t.Errorf("单次上限为 %d", got)
	}
	if cfg.Proxy.Concurrency != 4 {
		t.Errorf("并发数为 %d", cfg.Proxy.Concurrency)
	}
	if got := cfg.ProxyBaseURL(); got != "" {
		t.Errorf("关闭时对外地址应为空，当前为 %q", got)
	}
}

func TestProxyEnabledDerivesBaseURL(t *testing.T) {
	cfg, err := Parse([]byte(withProxy("  enabled: true\n")))
	if err != nil {
		t.Fatalf("Parse 返回错误: %v", err)
	}

	if got := cfg.ProxyBaseURL(); got != "http://127.0.0.1:8082" {
		t.Errorf("对外地址为 %q", got)
	}
}

func TestProxyKeepsExplicitBaseURL(t *testing.T) {
	cfg, err := Parse([]byte(withProxy("  enabled: true\n  base_url: http://example.com:8082/\n")))
	if err != nil {
		t.Fatalf("Parse 返回错误: %v", err)
	}

	if got := cfg.ProxyBaseURL(); got != "http://example.com:8082" {
		t.Errorf("对外地址为 %q，末尾斜杠应被去掉", got)
	}
}

func TestCompatRequiresProxy(t *testing.T) {
	raw := minimalYAML + "compat:\n  gensokyo:\n    enabled: true\n"

	_, err := Parse([]byte(raw))
	if err == nil {
		t.Fatal("兼容模式没有代理时应当报错")
	}
	if !strings.Contains(err.Error(), "compat.gensokyo.enabled") {
		t.Errorf("错误为 %v", err)
	}
}

func TestCompatWithProxy(t *testing.T) {
	raw := minimalYAML + "proxy:\n  enabled: true\ncompat:\n  gensokyo:\n    enabled: true\n"

	cfg, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse 返回错误: %v", err)
	}
	if got := cfg.AppName(); got != "gensokyo" {
		t.Errorf("程序名为 %q", got)
	}
	if got := cfg.AvatarBaseURL(); got != "http://127.0.0.1:8082" {
		t.Errorf("头像前缀为 %q", got)
	}
}

func TestProxyValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "监听地址缺少端口",
			raw:  withProxy("  enabled: true\n  listen: 127.0.0.1\n"),
			want: "proxy.listen",
		},
		{
			name: "对外地址带路径",
			raw:  withProxy("  enabled: true\n  base_url: http://example.com/media\n"),
			want: "不要带路径",
		},
		{
			name: "对外地址带查询串",
			raw:  withProxy("  enabled: true\n  base_url: http://example.com/?a=1\n"),
			want: "查询串",
		},
		{
			name: "关闭时写法非法仍然报错",
			raw:  withProxy("  base_url: ftp://example.com\n"),
			want: "proxy.base_url",
		},
		{
			name: "分档阈值为零",
			raw:  withProxy("  stream_threshold: 0MiB\n"),
			want: "proxy.stream_threshold",
		},
		{
			name: "分档阈值过大",
			raw:  withProxy("  stream_threshold: 512MiB\n"),
			want: "proxy.stream_threshold",
		},
		{
			name: "单次上限小于分档阈值",
			raw:  withProxy("  stream_threshold: 8MiB\n  media_max: 4MiB\n"),
			want: "proxy.media_max",
		},
		{
			name: "并发数为零",
			raw:  withProxy("  concurrency: 0\n"),
			want: "proxy.concurrency",
		},
		{
			name: "字段名拼错",
			raw:  withProxy("  enable: true\n"),
			want: "field enable not found",
		},
		{
			name: "内联上限过大",
			raw:  minimalYAML + "media:\n  inline_limit: 128MiB\n",
			want: "media.inline_limit",
		},
		{
			name: "读取上限为零",
			raw:  minimalYAML + "media:\n  read_limit: 0MiB\n",
			want: "media.read_limit",
		},
		{
			name: "体积缺少单位",
			raw:  minimalYAML + "media:\n  inline_limit: \"20\"\n",
			want: "缺少单位",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse([]byte(c.raw))
			if err == nil {
				t.Fatal("应当返回错误")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("错误为 %v，期望包含 %q", err, c.want)
			}
		})
	}
}

func TestParseSize(t *testing.T) {
	cases := []struct {
		text string
		want int64
	}{
		{text: "20MiB", want: 20 << 20},
		{text: "1GiB", want: 1 << 30},
		{text: "8mib", want: 8 << 20},
		{text: "512KiB", want: 512 << 10},
		{text: "20MB", want: 20 * 1000 * 1000},
		{text: "1024B", want: 1024},
	}

	for _, c := range cases {
		got, err := ParseSize(c.text)
		if err != nil {
			t.Errorf("ParseSize(%q) 返回错误: %v", c.text, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseSize(%q) 返回 %d，期望 %d", c.text, got, c.want)
		}
	}

	if _, err := ParseSize("8"); err == nil {
		t.Error("缺少单位时应当报错")
	}
}

func TestDeriveBaseURLRejectsInvalid(t *testing.T) {
	if got := deriveBaseURL("127.0.0.1"); got != "" {
		t.Errorf("缺少端口时应返回空，当前为 %q", got)
	}
}
