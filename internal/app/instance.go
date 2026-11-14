// SPDX-License-Identifier: GPL-3.0-or-later

package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Takeoff0518/HelloYunhu/internal/bridge/event"
	"github.com/Takeoff0518/HelloYunhu/internal/bridge/identity"
	"github.com/Takeoff0518/HelloYunhu/internal/bridge/media"
	"github.com/Takeoff0518/HelloYunhu/internal/config"
	"github.com/Takeoff0518/HelloYunhu/internal/onebot/action"
	onebotclient "github.com/Takeoff0518/HelloYunhu/internal/onebot/client"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/api"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/subscribe"
)

// 事件通道容量。容量满时订阅连接的读循环会等待，背压传导到网络层。
const eventBufferSize = 64

// runInstance 运行一个机器人实例，直到上下文结束或出错。
//
// messages 由上层提供，多个实例共用同一份持久化映射表。
func runInstance(ctx context.Context, instanceCfg config.Instance, cfg *config.Config, logger *slog.Logger, messages identity.MessageStore) error {
	log := logger.With("instance", instanceCfg.Name)

	apiClient, err := api.New(api.Config{BaseURL: cfg.Yunhu.APIBase, Token: instanceCfg.Token})
	if err != nil {
		return fmt.Errorf("创建接口客户端: %w", err)
	}

	// 事件里没有机器人自身的标识，必须先查询。
	info, err := apiClient.BotInfo(ctx)
	if err != nil {
		return fmt.Errorf("获取机器人信息: %w", err)
	}

	selfID, exact := identity.ParseID(info.BotID)
	if !exact {
		log.Warn("机器人标识无法解析为整数，已退化为哈希值", "bot_id", info.BotID)
	}

	// 媒体读取无状态，两个方向共用同一个实例。
	mediaLoader := &media.Loader{}

	dispatcher := &action.Dispatcher{
		SelfID:   selfID,
		Nickname: info.Nickname,
		API:      apiClient,
		Messages: messages,
		Media:    mediaLoader,
		Logger:   log,
	}

	var onebot *onebotclient.Client
	onebot, err = onebotclient.New(onebotclient.Options{
		Config: onebotclient.Config{
			URL:         instanceCfg.OneBot.URL,
			AccessToken: instanceCfg.OneBot.AccessToken,
			SelfID:      selfID,
		},
		Handler: dispatcher.Handle,
		Logger:  log,
		OnConnect: func(ctx context.Context) error {
			// 连接建立后立即发送生命周期事件，框架据此打印「已连接」。
			return onebot.SendEvent(event.Lifecycle(selfID, time.Now()))
		},
	})
	if err != nil {
		return fmt.Errorf("创建框架客户端: %w", err)
	}

	subscriber, err := subscribe.New(subscribe.Options{
		Config: subscribe.Config{URL: cfg.Yunhu.SubscribeURL, Token: instanceCfg.Token},
		Logger: log,
	})
	if err != nil {
		return fmt.Errorf("创建订阅客户端: %w", err)
	}

	converter := &event.Converter{
		SelfID:   selfID,
		BotID:    info.BotID,
		Messages: messages,
		Media:    mediaLoader,
		Logger:   log,
	}

	events := make(chan model.Envelope, eventBufferSize)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg   sync.WaitGroup
		once sync.Once
		errs error
	)
	record := func(err error) {
		once.Do(func() {
			errs = err
		})
		cancel()
	}

	run := func(name string, fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(); err != nil {
				log.Error("连接异常退出", "component", name, "err", err)
				record(fmt.Errorf("%s 异常退出: %w", name, err))
			}
		}()
	}

	run("订阅连接", func() error { return subscriber.Run(ctx, events) })
	run("框架连接", func() error { return onebot.Run(ctx) })
	run("事件转发", func() error { return forward(ctx, events, converter, onebot, log) })

	<-ctx.Done()
	cancel()
	wg.Wait()

	return errs
}

// forward 把云湖事件转换后写入框架连接，直到上下文结束。
func forward(ctx context.Context, events <-chan model.Envelope, converter *event.Converter, sender *onebotclient.Client, logger *slog.Logger) error {
	for {
		select {
		case <-ctx.Done():
			return nil

		case envelope := <-events:
			converted, ok, err := converter.Convert(ctx, envelope)
			if err != nil {
				logger.Warn("转换事件失败，已跳过", "event_type", envelope.Header.EventType, "err", err)
				continue
			}
			if !ok {
				continue
			}

			if err := sender.SendEvent(converted); err != nil {
				if errors.Is(err, onebotclient.ErrNotConnected) {
					logger.Warn("框架连接尚未建立，事件已丢弃", "event_type", envelope.Header.EventType)
					continue
				}
				logger.Warn("写入框架连接失败，事件已丢弃", "event_type", envelope.Header.EventType, "err", err)
			}
		}
	}
}
