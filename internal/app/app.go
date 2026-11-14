// SPDX-License-Identifier: GPL-3.0-or-later

// Package app 负责装配配置、日志与各子系统，并管理进程的生命周期。
//
// 这一层是唯一了解全部组件的包。协议转换、连接维护与存储实现都不了解彼此之外
// 的部分，由这里接在一起。
package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/Takeoff0518/HelloYunhu/internal/config"
	"github.com/Takeoff0518/HelloYunhu/internal/logging"
)

// Options 描述一次启动所需的输入。
type Options struct {
	// ConfigPath 是配置文件的路径。
	ConfigPath string
	// Version 与 Commit 写入启动日志，便于确认运行的是哪个构建。
	Version string
	Commit  string
	// LogOutput 是日志的写入目标，为空时写到标准错误。
	LogOutput io.Writer
}

// Run 加载配置、初始化日志，并运行到上下文结束。
//
// 配置有任何一项不合法时直接返回错误，进程由调用方负责退出，不进入运行状态。
func Run(ctx context.Context, opt Options) error {
	cfg, err := config.Load(opt.ConfigPath)
	if err != nil {
		return err
	}

	logger, err := newLogger(cfg.Log, opt.LogOutput)
	if err != nil {
		return err
	}

	logger.Info("程序启动",
		"version", opt.Version,
		"commit", opt.Commit,
		"config", opt.ConfigPath,
		"instances", len(cfg.Instances),
	)

	for _, instance := range cfg.Instances {
		logger.Info("装载机器人实例",
			"instance", instance.Name,
			"onebot_url", instance.OneBot.URL,
		)
	}

	// 连接逻辑在 M1 接入，此处先保留运行与退出的完整路径。
	logger.Warn("当前构建尚未实现连接逻辑，进程不会收发任何消息")

	<-ctx.Done()

	logger.Info("进程结束")
	return nil
}

// newLogger 按配置创建日志实例。
func newLogger(cfg config.Log, output io.Writer) (*slog.Logger, error) {
	level, err := logging.ParseLevel(cfg.Level)
	if err != nil {
		return nil, fmt.Errorf("日志级别: %w", err)
	}
	format, err := logging.ParseFormat(cfg.Format)
	if err != nil {
		return nil, fmt.Errorf("日志格式: %w", err)
	}

	return logging.New(logging.Options{Output: output, Level: level, Format: format})
}
