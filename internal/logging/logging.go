// SPDX-License-Identifier: GPL-3.0-or-later

// Package logging 初始化结构化日志。
//
// 日志使用标准库的 log/slog，字段名统一使用小写下划线形式。程序不在日志中
// 输出消息正文、令牌与其他隐私数据。
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Format 表示日志的输出格式。
type Format string

// 支持两种输出格式：面向终端阅读的文本，以及便于采集的 JSON。
const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

// Options 描述日志的输出目标、级别与格式。
type Options struct {
	// Output 是日志写入目标，为空时写到标准错误。
	Output io.Writer
	// Level 是日志级别，默认由 ParseLevel 给出。
	Level slog.Level
	// Format 是日志格式，默认是文本格式。
	Format Format
}

// ParseLevel 把配置中的级别名称转换为 slog.Level，空字符串按 info 处理。
func ParseLevel(name string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("级别 %q 不受支持，可选 debug、info、warn 或 error", name)
	}
}

// ParseFormat 把配置中的格式名称转换为 Format，空字符串按 text 处理。
func ParseFormat(name string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", string(FormatText):
		return FormatText, nil
	case string(FormatJSON):
		return FormatJSON, nil
	default:
		return "", fmt.Errorf("格式 %q 不受支持，可选 text 或 json", name)
	}
}

// New 按给定选项创建日志实例。
func New(opt Options) (*slog.Logger, error) {
	output := opt.Output
	if output == nil {
		output = os.Stderr
	}

	handlerOpts := &slog.HandlerOptions{Level: opt.Level}

	switch opt.Format {
	case "", FormatText:
		return slog.New(slog.NewTextHandler(output, handlerOpts)), nil
	case FormatJSON:
		return slog.New(slog.NewJSONHandler(output, handlerOpts)), nil
	default:
		return nil, fmt.Errorf("日志格式 %q 不受支持", opt.Format)
	}
}

// Discard 返回一个丢弃全部输出的日志实例，供测试与关闭日志的路径使用。
func Discard() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
