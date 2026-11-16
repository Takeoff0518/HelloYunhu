// SPDX-License-Identifier: GPL-3.0-or-later

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Takeoff0518/HelloYunhu/internal/app/metrics"
	"github.com/Takeoff0518/HelloYunhu/internal/avatar"
	"github.com/Takeoff0518/HelloYunhu/internal/bridge/cache"
	"github.com/Takeoff0518/HelloYunhu/internal/bridge/dedup"
	"github.com/Takeoff0518/HelloYunhu/internal/bridge/event"
	"github.com/Takeoff0518/HelloYunhu/internal/bridge/identity"
	"github.com/Takeoff0518/HelloYunhu/internal/bridge/media"
	"github.com/Takeoff0518/HelloYunhu/internal/config"
	"github.com/Takeoff0518/HelloYunhu/internal/onebot/action"
	onebotclient "github.com/Takeoff0518/HelloYunhu/internal/onebot/client"
	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
	"github.com/Takeoff0518/HelloYunhu/internal/proxy"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/api"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/subscribe"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/webhook"
)

// 事件通道容量。容量满时订阅连接的读循环会等待，背压传导到网络层。
const eventBufferSize = 64

// connection 是一条到框架的连接及其名称。
type connection struct {
	name   string
	client *onebotclient.Client
}

// botStatus 记录运行期状态，供诊断接口读取。
//
// 机器人标识与昵称在启动时查询，连接列表在建立客户端时写入；诊断服务可能在
// 另一条协程里读取，因此用读写锁保护。
type botStatus struct {
	mu          sync.RWMutex
	botID       string
	nickname    string
	connections []connection
}

// setBot 记录机器人标识与昵称。
func (s *botStatus) setBot(id, nickname string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.botID = id
	s.nickname = nickname
}

// setConnections 记录连接列表。
func (s *botStatus) setConnections(connections []connection) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.connections = connections
}

// snapshot 返回机器人标识、昵称与每个连接是否在线。
func (s *botStatus) snapshot() (string, string, []connectionState) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	states := make([]connectionState, 0, len(s.connections))
	for _, conn := range s.connections {
		states = append(states, connectionState{Name: conn.name, Online: conn.client.Connected()})
	}

	return s.botID, s.nickname, states
}

// connectionState 是一个连接的在线状态。
type connectionState struct {
	Name   string
	Online bool
}

// runBot 运行一个云湖机器人，直到上下文结束或出错。
//
// 事件转换一次后广播给所有框架连接；任一条连接断开只影响它自己，重连由连接
// 客户端负责，不会让进程退出。订阅连接或机器人信息查询失败属于致命错误。
func runBot(ctx context.Context, cfg *config.Config, logger *slog.Logger, messages identity.MessageStore, counters *metrics.Metrics, status *botStatus) error {
	log := logger

	apiClient, err := api.New(api.Config{
		BaseURL:       cfg.Yunhu.APIBase,
		Token:         cfg.Bot.Token,
		RatePerSecond: cfg.Yunhu.RateLimit,
		Logger:        log,
	})
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
	log.Info("已连接云湖机器人", "bot_id", info.BotID, "nickname", info.Nickname)
	status.setBot(info.BotID, info.Nickname)

	// 两个方向各用一个读取器，上限分别由配置决定：接收方向超限时改为给出地址，
	// 发送方向超限时放弃这次发送。
	receiveMedia := &media.Loader{MaxBytes: cfg.Media.InlineLimit.Bytes()}
	sendMedia := &media.Loader{MaxBytes: cfg.Media.ReadLimit.Bytes()}
	// 最近消息缓存用于 get_msg 回放，容量按默认值。
	recent := cache.NewMessages(0)
	// 昵称缓存供发送 @ 使用，来源是收到的消息事件。
	nicknames := cache.NewNicknames(0)

	// 头像地址表只在代理与兼容模式同时开启时建立：没有地址可给时不必收集。
	var avatarStore *avatar.Store
	avatarBase := cfg.AvatarBaseURL()
	if avatarBase != "" {
		avatarStore = avatar.NewStore()
	}

	dispatcher := &action.Dispatcher{
		SelfID:        selfID,
		Nickname:      info.Nickname,
		API:           apiClient,
		Messages:      messages,
		Media:         sendMedia,
		Recent:        recent,
		Nicknames:     nicknames,
		CanSendImage:  cfg.Capabilities.CanSendImage,
		CanSendRecord: cfg.Capabilities.CanSendRecord,
		AppName:       cfg.AppName(),
		AvatarBaseURL: avatarBase,
		Avatars:       avatarStore,
		Tags: action.TagOptions{
			AutoCreate:   cfg.Tags.AutoCreate,
			Color:        cfg.Tags.Color,
			Replace:      cfg.Tags.SpecialTitle.Mode == config.TagModeReplace,
			ClearOnEmpty: cfg.Tags.SpecialTitle.ClearOnEmpty,
			ReportOwner:  cfg.Tags.ReportOwner,
		},
		Logger: log,
	}

	connections := make([]connection, 0, len(cfg.OneBot))
	for _, connCfg := range cfg.OneBot {
		connLog := log.With("onebot", connCfg.Name)

		var client *onebotclient.Client
		client, err = onebotclient.New(onebotclient.Options{
			Config: onebotclient.Config{
				URL:         connCfg.URL,
				AccessToken: connCfg.AccessToken,
				SelfID:      selfID,
			},
			Handler: func(ctx context.Context, name string, params json.RawMessage) (any, error) {
				counters.ActionHandled()
				return dispatcher.Handle(ctx, name, params)
			},
			Logger: connLog,
			OnConnect: func(ctx context.Context) error {
				// 连接建立后立即发送生命周期事件，框架据此打印「已连接」。
				return client.SendEvent(event.Lifecycle(selfID, time.Now()))
			},
		})
		if err != nil {
			return fmt.Errorf("创建框架连接 %s: %w", connCfg.Name, err)
		}

		connections = append(connections, connection{name: connCfg.Name, client: client})
	}
	status.setConnections(connections)

	converter := &event.Converter{
		SelfID:       selfID,
		BotID:        info.BotID,
		Messages:     messages,
		Media:        receiveMedia,
		Nicknames:    nicknames,
		Avatars:      avatarStore,
		MediaBaseURL: cfg.ProxyBaseURL(),
		Logger:       log,
	}

	// 机器人自己的头像也记下，客户端会为它单独要一次。
	if avatarStore != nil {
		avatarStore.Put(selfID, info.AvatarURL)
	}

	var subscriber *subscribe.Client
	if cfg.Receive.UsesWebsocket() {
		subscriber, err = subscribe.New(subscribe.Options{
			Config:    subscribe.Config{URL: cfg.Yunhu.SubscribeURL, Token: cfg.Bot.Token},
			Logger:    log,
			RawEvents: cfg.Log.RawEvents,
		})
		if err != nil {
			return fmt.Errorf("创建订阅客户端: %w", err)
		}
	}

	events := make(chan model.Envelope, eventBufferSize)

	// 两种接收方式同时使用时按事件标识去重，只启用其中一种时不需要。
	var filter *dedup.Filter
	if cfg.Receive.Mode == config.ReceiveBoth {
		filter = dedup.New(dedup.Options{})
	}

	duplicated := func(envelope model.Envelope) bool {
		if filter == nil || !filter.Seen(envelope.Header.EventID) {
			return false
		}
		log.Debug("重复事件已过滤", "event_id", envelope.Header.EventID)
		return true
	}

	// Webhook 侧不能阻塞：请求挂住会让平台重试，直接返回忙碌更稳妥。
	webhookDeliver := func(_ context.Context, envelope model.Envelope) error {
		if duplicated(envelope) {
			return nil
		}
		select {
		case events <- envelope:
			return nil
		default:
			return webhook.ErrBusy
		}
	}

	// 订阅侧保持背压：读循环等待，不主动丢事件。
	subscriptionDeliver := func(ctx context.Context, envelope model.Envelope) {
		if duplicated(envelope) {
			return
		}
		select {
		case events <- envelope:
		case <-ctx.Done():
		}
	}

	var webhookServer *webhook.Server
	if cfg.Receive.UsesWebhook() {
		webhookServer, err = webhook.New(webhook.Options{
			Token:      cfg.Receive.Webhook.Token,
			AllowedIPs: cfg.Receive.Webhook.AllowedIPs,
			Logger:     log,
			Deliver:    webhookDeliver,
		})
		if err != nil {
			return fmt.Errorf("创建 Webhook 服务: %w", err)
		}
	}

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
				log.Error("组件异常退出", "component", name, "err", err)
				record(fmt.Errorf("%s 异常退出: %w", name, err))
			}
		}()
	}

	if subscriber != nil {
		incoming := make(chan model.Envelope, eventBufferSize)
		run("订阅连接", func() error { return subscriber.Run(ctx, incoming) })
		run("订阅事件投递", func() error {
			for {
				select {
				case <-ctx.Done():
					return nil
				case envelope := <-incoming:
					subscriptionDeliver(ctx, envelope)
				}
			}
		})
	}
	if webhookServer != nil {
		run("Webhook 接收", func() error {
			return webhookServer.Run(ctx, cfg.Receive.Webhook.Listen)
		})
	}
	if cfg.Proxy.Enabled {
		options := proxy.Options{
			Addr: cfg.Proxy.Listen,
			Media: &proxy.Media{
				StreamThreshold: cfg.Proxy.StreamThreshold.Bytes(),
				Max:             cfg.Proxy.MediaMax.Bytes(),
				Concurrency:     cfg.Proxy.Concurrency,
				Logger:          log,
			},
			Logger: log,
		}
		// 头像路径只在兼容模式开启时提供：没有 get_avatar，客户端不会来取。
		if avatarStore != nil {
			options.Avatar = &avatar.Proxy{Store: avatarStore, Logger: log}
		}

		run("代理服务", func() error {
			return proxy.Run(ctx, options)
		})
	}
	for _, conn := range connections {
		conn := conn
		run("框架连接 "+conn.name, func() error { return conn.client.Run(ctx) })
	}
	run("事件转发", func() error {
		return forward(ctx, events, converter, connections, recent, counters, log)
	})

	<-ctx.Done()
	cancel()
	wg.Wait()

	return errs
}

// forward 把云湖事件转换后广播给所有框架连接，直到上下文结束。
//
// 转换出的消息事件同时放入最近消息缓存，供 get_msg 回放。某条连接未建立或写
// 入失败只记录日志，不影响其它连接。
func forward(ctx context.Context, events <-chan model.Envelope, converter *event.Converter, connections []connection, recent *cache.Messages, counters *metrics.Metrics, logger *slog.Logger) error {
	for {
		select {
		case <-ctx.Done():
			return nil

		case envelope := <-events:
			counters.EventReceived()
			converted, ok, err := converter.Convert(ctx, envelope)
			if err != nil {
				logger.Warn("转换事件失败，已跳过", "event_type", envelope.Header.EventType, "err", err)
				continue
			}
			if !ok {
				continue
			}

			if messageEvent, isMessage := converted.(*obmodel.MessageEvent); isMessage {
				recent.Put(messageEvent.MessageID, messageEvent)
			}

			for _, conn := range connections {
				if err := conn.client.SendEvent(converted); err != nil {
					if errors.Is(err, onebotclient.ErrNotConnected) {
						logger.Warn("框架连接尚未建立，事件已丢弃",
							"onebot", conn.name, "event_type", envelope.Header.EventType)
						continue
					}
					logger.Warn("写入框架连接失败，事件已丢弃",
						"onebot", conn.name, "event_type", envelope.Header.EventType, "err", err)
				}
			}
		}
	}
}
