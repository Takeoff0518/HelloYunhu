// SPDX-License-Identifier: GPL-3.0-or-later

// Package app 负责装配配置、日志与各子系统，并管理进程的生命周期。
//
// 这一层是唯一了解全部组件的包。协议转换、连接维护与存储实现都不了解彼此之外
// 的部分，由这里接在一起。一个进程可以运行多个机器人实例，实例之间不共享运行
// 状态，任何一个实例失败都会让整个进程退出。
package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

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

// Run 加载配置、初始化日志并运行各实例，直到上下文结束。
//
// 配置有任何一项不合法时直接返回错误，进程由调用方负责退出，不进入运行状态。
// 实例运行中出错时取消其它实例并返回该错误，由调用方打印与退出。
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

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg      sync.WaitGroup
		errOnce sync.Once
		runErr  error
	)

	for _, instanceCfg := range cfg.Instances {
		wg.Add(1)
		go func(instanceCfg config.Instance) {
			defer wg.Done()

			if err := runInstance(ctx, instanceCfg, cfg, logger); err != nil {
				errOnce.Do(func() {
					runErr = fmt.Errorf("实例 %s: %w", instanceCfg.Name, err)
				})
				cancel()
			}
		}(instanceCfg)
	}

	<-ctx.Done()
	wg.Wait()

	if runErr != nil {
		return runErr
	}

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
