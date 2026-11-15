// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"strings"
	"testing"
)

// compatYAML 在基础配置上追加一段兼容模式设置。
func compatYAML(body string) string {
	return minimalYAML + "compat:\n  gensokyo:\n" + body
}

func TestCompatDefaults(t *testing.T) {
	cfg, err := Parse([]byte(minimalYAML))
	if err != nil {
		t.Fatalf("Parse 返回错误: %v", err)
	}

	compat := cfg.Compat.Gensokyo
	if compat.Enabled {
		t.Error("兼容模式默认应关闭")
	}
	if compat.Listen != defaultAvatarAddr {
		t.Errorf("监听地址默认值为 %q", compat.Listen)
	}
	if compat.BaseURL != "" {
		t.Errorf("关闭时不应推导对外地址，当前为 %q", compat.BaseURL)
	}
	if got := cfg.AppName(); got != "HelloYunhu" {
		t.Errorf("程序名为 %q", got)
	}
	if got := cfg.AvatarBaseURL(); got != "" {
		t.Errorf("关闭时头像前缀应为空，当前为 %q", got)
	}
}

func TestCompatEnabledDerivesBaseURL(t *testing.T) {
	cfg, err := Parse([]byte(compatYAML("    enabled: true\n")))
	if err != nil {
		t.Fatalf("Parse 返回错误: %v", err)
	}

	if !cfg.Compat.Gensokyo.Enabled {
		t.Fatal("兼容模式应为开启")
	}
	if got := cfg.AppName(); got != "gensokyo" {
		t.Errorf("程序名为 %q", got)
	}
	if got := cfg.Compat.Gensokyo.BaseURL; got != "http://127.0.0.1:8082" {
		t.Errorf("推导的对外地址为 %q", got)
	}
	if got := cfg.AvatarBaseURL(); got != "http://127.0.0.1:8082" {
		t.Errorf("头像前缀为 %q", got)
	}
}

func TestCompatDerivesFromListen(t *testing.T) {
	cases := []struct {
		listen string
		want   string
	}{
		{listen: "0.0.0.0:9000", want: "http://127.0.0.1:9000"},
		{listen: "192.168.1.5:9000", want: "http://192.168.1.5:9000"},
		{listen: "[::]:9001", want: "http://127.0.0.1:9001"},
	}

	for _, c := range cases {
		t.Run(c.listen, func(t *testing.T) {
			cfg, err := Parse([]byte(compatYAML("    enabled: true\n    listen: \"" + c.listen + "\"\n")))
			if err != nil {
				t.Fatalf("Parse 返回错误: %v", err)
			}
			if got := cfg.Compat.Gensokyo.BaseURL; got != c.want {
				t.Errorf("推导的对外地址为 %q，期望 %q", got, c.want)
			}
		})
	}
}

func TestCompatKeepsExplicitBaseURL(t *testing.T) {
	cfg, err := Parse([]byte(compatYAML("    enabled: true\n    base_url: http://example.com:8082/\n")))
	if err != nil {
		t.Fatalf("Parse 返回错误: %v", err)
	}

	if got := cfg.AvatarBaseURL(); got != "http://example.com:8082" {
		t.Errorf("头像前缀为 %q，末尾斜杠应被去掉", got)
	}
}

func TestCompatValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "监听地址缺少端口",
			raw:  compatYAML("    enabled: true\n    listen: 127.0.0.1\n"),
			want: "compat.gensokyo.listen",
		},
		{
			name: "对外地址带路径",
			raw:  compatYAML("    enabled: true\n    base_url: http://example.com/avatar\n"),
			want: "不要带路径",
		},
		{
			name: "对外地址带查询串",
			raw:  compatYAML("    enabled: true\n    base_url: http://example.com/?a=1\n"),
			want: "查询串",
		},
		{
			name: "关闭时写法非法仍然报错",
			raw:  compatYAML("    base_url: ftp://example.com\n"),
			want: "compat.gensokyo.base_url",
		},
		{
			name: "字段名拼错",
			raw:  compatYAML("    enable: true\n"),
			want: "field enable not found",
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

func TestDeriveBaseURLRejectsInvalid(t *testing.T) {
	if got := deriveBaseURL("127.0.0.1"); got != "" {
		t.Errorf("缺少端口时应返回空，当前为 %q", got)
	}
}
