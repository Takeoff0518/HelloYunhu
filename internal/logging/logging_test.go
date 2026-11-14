// SPDX-License-Identifier: GPL-3.0-or-later

package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    slog.Level
		wantErr string
	}{
		{name: "空字符串按 info", input: "", want: slog.LevelInfo},
		{name: "标准级别", input: "debug", want: slog.LevelDebug},
		{name: "大写写法", input: "WARN", want: slog.LevelWarn},
		{name: "首尾空白", input: "  error  ", want: slog.LevelError},
		{name: "未知级别", input: "verbose", wantErr: "不受支持"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLevel(tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseLevel(%q) 的错误为 %v，期望包含 %q", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseLevel(%q) 返回错误: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("ParseLevel(%q) = %v，期望 %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Format
		wantErr string
	}{
		{name: "空字符串按 text", input: "", want: FormatText},
		{name: "文本", input: "text", want: FormatText},
		{name: "JSON 大写", input: "JSON", want: FormatJSON},
		{name: "未知格式", input: "xml", wantErr: "不受支持"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFormat(tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseFormat(%q) 的错误为 %v，期望包含 %q", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFormat(%q) 返回错误: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("ParseFormat(%q) = %v，期望 %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewTextOutput(t *testing.T) {
	var buf bytes.Buffer
	logger, err := New(Options{Output: &buf, Level: slog.LevelInfo})
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}

	logger.Info("启动完成", "instance_id", "main")

	output := buf.String()
	if !strings.Contains(output, "level=INFO") {
		t.Fatalf("文本输出缺少级别字段: %q", output)
	}
	if !strings.Contains(output, "msg=启动完成") {
		t.Fatalf("文本输出缺少消息内容: %q", output)
	}
	if !strings.Contains(output, "instance_id=main") {
		t.Fatalf("文本输出缺少自定义字段: %q", output)
	}
}

func TestNewJSONOutput(t *testing.T) {
	var buf bytes.Buffer
	logger, err := New(Options{Output: &buf, Level: slog.LevelInfo, Format: FormatJSON})
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}

	logger.Info("启动完成", "instance_id", "main")

	output := buf.String()
	for _, want := range []string{`"level":"INFO"`, `"msg":"启动完成"`, `"instance_id":"main"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("JSON 输出缺少 %s: %q", want, output)
		}
	}
}

func TestNewFiltersLowerLevels(t *testing.T) {
	var buf bytes.Buffer
	logger, err := New(Options{Output: &buf, Level: slog.LevelWarn})
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}

	logger.Info("不应出现")

	if buf.Len() != 0 {
		t.Fatalf("低于配置级别的日志不应输出: %q", buf.String())
	}
}

func TestNewUnknownFormat(t *testing.T) {
	if _, err := New(Options{Output: &bytes.Buffer{}, Format: Format("xml")}); err == nil {
		t.Fatal("未知格式应返回错误")
	}
}
