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
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Takeoff0518/HelloYunhu/internal/app/metrics"
	"github.com/Takeoff0518/HelloYunhu/internal/config"
	"github.com/Takeoff0518/HelloYunhu/internal/diagnostics"
	"github.com/Takeoff0518/HelloYunhu/internal/logging"
	"github.com/Takeoff0518/HelloYunhu/internal/storage"
)

// defaultLogDir 是日志文件的固定目录。
const defaultLogDir = "logs"

// Options 描述一次启动所需的输入。
type Options struct {
	// ConfigPath 是配置文件的路径。
	ConfigPath string
	// Version 与 Commit 写入启动日志，便于确认运行的是哪个构建。
	Version string
	Commit  string
	// LogOutput 是日志的写入目标，为空时写到标准错误。
	LogOutput io.Writer
	// LogDir 是日志目录，为空时使用 ./logs。测试用它指向临时目录。
	LogDir string
}

// EnsureConfig 确保配置文件存在，不存在时生成默认模板。
//
// 返回 true 表示本次生成了配置文件，调用方应当提示用户填写后退出；返回 false
// 表示文件已存在，可以继续加载。这样第一次运行只负责铺好配置，不会带着空令牌
// 去连接平台。
func EnsureConfig(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("检查配置文件 %s: %w", path, err)
	}

	if err := config.WriteTemplate(path); err != nil {
		return false, err
	}
	return true, nil
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

	output, closeOutput, err := openLogOutput(opt.LogDir, cfg.Log, opt.LogOutput)
	if err != nil {
		return err
	}
	defer closeOutput()

	logger, err := newLogger(cfg.Log, output)
	if err != nil {
		return err
	}

	logger.Info("程序启动",
		"version", opt.Version,
		"commit", opt.Commit,
		"config", opt.ConfigPath,
		"instances", len(cfg.Instances),
	)

	// 声明与实现不一致时给一次提示：框架会按声明选择发送方式。
	if cfg.Capabilities.CanSendRecord {
		logger.Warn("can_send_record 声明为真，但平台没有语音发送接口，语音仍按文件发送，客户端收到的会是文件")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	store, err := storage.Open(ctx, cfg.Storage.Path)
	if err != nil {
		return err
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Warn("关闭数据库失败", "err", err)
		}
	}()

	go pruneLoop(ctx, store, cfg.Storage.MessageTTL.Standard(), logger)

	// 运行计数供只读诊断接口使用，所有实例共用。
	counters := metrics.New()
	startedAt := time.Now()

	var (
		wg      sync.WaitGroup
		errOnce sync.Once
		runErr  error
	)

	if cfg.Diagnostics.Enabled {
		server := diagnostics.New(diagnostics.Options{
			Listen:   cfg.Diagnostics.Listen,
			Logger:   logger,
			Snapshot: snapshotFunc(opt.Version, startedAt, len(cfg.Instances), counters),
		})
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := server.Run(ctx); err != nil {
				logger.Warn("诊断服务退出", "err", err)
			}
		}()
	}

	for _, instanceCfg := range cfg.Instances {
		wg.Add(1)
		go func(instanceCfg config.Instance) {
			defer wg.Done()

			if err := runInstance(ctx, instanceCfg, cfg, logger, store, counters); err != nil {
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

// snapshotFunc 构造诊断接口使用的状态查询。
func snapshotFunc(version string, startedAt time.Time, instances int, counters *metrics.Metrics) func() diagnostics.Snapshot {
	return func() diagnostics.Snapshot {
		return diagnostics.Snapshot{
			Version:        version,
			StartedAt:      startedAt,
			UptimeSeconds:  int64(time.Since(startedAt).Seconds()),
			Instances:      instances,
			EventsReceived: counters.EventsReceived(),
			ActionsHandled: counters.ActionsHandled(),
		}
	}
}

// pruneLoop 定期清理过期的消息标识映射。
//
// 映射表的增长速度远快于用户与群，不清理会无限膨胀。启动时立即清理一次，之后
// 按小时执行；保留时长比一小时更短时按保留时长执行。
func pruneLoop(ctx context.Context, store *storage.Store, ttl time.Duration, logger *slog.Logger) {
	interval := time.Hour
	if ttl > 0 && ttl < interval {
		interval = ttl
	}

	prune := func() {
		removed, err := store.Prune(ctx, ttl)
		if err != nil {
			logger.Warn("清理过期的消息标识失败", "err", err)
			return
		}
		if removed > 0 {
			logger.Info("清理过期的消息标识", "removed", removed)
		}
	}

	prune()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			prune()
		}
	}
}

// openLogOutput 创建按天切分的日志输出。
//
// 日志文件始终写入固定目录，文件名是当天日期，跨天自动切换，旧文件在后台压缩并
// 按保留天数清理。终端与文件内容一致，排查问题可以直接取自文件。
func openLogOutput(dir string, cfg config.Log, fallback io.Writer) (io.Writer, func(), error) {
	terminal := fallback
	if terminal == nil {
		terminal = os.Stderr
	}
	if strings.TrimSpace(dir) == "" {
		dir = defaultLogDir
	}

	writer, err := logging.NewRotatingWriter(logging.RotateOptions{
		Dir:      dir,
		KeepDays: cfg.KeepDays,
		Compress: cfg.Compress,
	})
	if err != nil {
		return nil, nil, err
	}

	closeOutput := func() {
		if err := writer.Close(); err != nil {
			fmt.Fprintf(terminal, "关闭日志文件失败: %v\n", err)
		}
	}

	return io.MultiWriter(terminal, writer), closeOutput, nil
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
