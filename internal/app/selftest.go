// SPDX-License-Identifier: GPL-3.0-or-later

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Takeoff0518/HelloYunhu/internal/bridge/identity"
	"github.com/Takeoff0518/HelloYunhu/internal/config"
	onebotclient "github.com/Takeoff0518/HelloYunhu/internal/onebot/client"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/api"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/subscribe"
)

// 自检结果的状态。
const (
	// StatusPassed 表示检查通过。
	StatusPassed = "通过"
	// StatusFailed 表示检查失败。
	StatusFailed = "失败"
	// StatusSkipped 表示检查因前置条件不足被跳过。
	StatusSkipped = "跳过"
)

// probeTimeout 是单项连接检查的超时时间。
const probeTimeout = 10 * time.Second

// CheckResult 是自检中的一项结果。
type CheckResult struct {
	// Name 是检查项名称。
	Name string
	// Status 取 StatusPassed、StatusFailed 或 StatusSkipped。
	Status string
	// Detail 是补充说明，失败时是错误原因。
	Detail string
}

// Selftest 检查配置与各实例的连接，返回逐项结果。
//
// 配置不合法时只返回配置检查一项，因为后续检查都需要配置。连接检查会真实发起
// 请求：查询机器人信息、建立一次订阅连接、向框架握手各一次。
func Selftest(ctx context.Context, opt Options) []CheckResult {
	cfg, err := config.Load(opt.ConfigPath)
	if err != nil {
		return []CheckResult{{Name: "配置检查", Status: StatusFailed, Detail: err.Error()}}
	}

	results := []CheckResult{{
		Name:   "配置检查",
		Status: StatusPassed,
		Detail: fmt.Sprintf("%s，共 %d 个实例", opt.ConfigPath, len(cfg.Instances)),
	}}

	for _, instance := range cfg.Instances {
		results = append(results, selftestInstance(ctx, instance, cfg)...)
	}

	return results
}

// selftestInstance 检查一个实例的三条连接。
func selftestInstance(ctx context.Context, instance config.Instance, cfg *config.Config) []CheckResult {
	prefix := "实例 " + instance.Name + " · "

	apiClient, err := api.New(api.Config{BaseURL: cfg.Yunhu.APIBase, Token: instance.Token})
	if err != nil {
		return []CheckResult{{Name: prefix + "接口客户端", Status: StatusFailed, Detail: err.Error()}}
	}

	var results []CheckResult

	info, infoErr := probeBotInfo(ctx, apiClient)
	if infoErr != nil {
		results = append(results, CheckResult{Name: prefix + "机器人信息", Status: StatusFailed, Detail: infoErr.Error()})
	} else {
		results = append(results, CheckResult{
			Name:   prefix + "机器人信息",
			Status: StatusPassed,
			Detail: fmt.Sprintf("botId=%s 昵称=%s", info.BotID, info.Nickname),
		})
	}

	if err := probeSubscribe(ctx, instance, cfg); err != nil {
		results = append(results, CheckResult{Name: prefix + "订阅连接", Status: StatusFailed, Detail: err.Error()})
	} else {
		results = append(results, CheckResult{Name: prefix + "订阅连接", Status: StatusPassed, Detail: "握手成功"})
	}

	// 框架握手需要机器人标识，取不到时无法构造 X-Self-ID。
	if infoErr != nil {
		results = append(results, CheckResult{
			Name:   prefix + "框架握手",
			Status: StatusSkipped,
			Detail: "需要先取得机器人信息",
		})
		return results
	}

	if err := probeFramework(ctx, instance, info.BotID); err != nil {
		results = append(results, CheckResult{Name: prefix + "框架握手", Status: StatusFailed, Detail: err.Error()})
	} else {
		results = append(results, CheckResult{Name: prefix + "框架握手", Status: StatusPassed, Detail: "握手成功"})
	}

	return results
}

// probeBotInfo 查询机器人信息。
func probeBotInfo(ctx context.Context, client *api.Client) (*model.BotInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	return client.BotInfo(ctx)
}

// probeSubscribe 建立一次订阅连接后立即关闭。
func probeSubscribe(ctx context.Context, instance config.Instance, cfg *config.Config) error {
	subscriber, err := subscribe.New(subscribe.Options{
		Config: subscribe.Config{URL: cfg.Yunhu.SubscribeURL, Token: instance.Token},
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	return subscriber.Probe(ctx)
}

// probeFramework 向框架发起一次握手后立即关闭。
func probeFramework(ctx context.Context, instance config.Instance, botID string) error {
	selfID, _ := identity.ParseID(botID)

	client, err := onebotclient.New(onebotclient.Options{
		Config: onebotclient.Config{
			URL:         instance.OneBot.URL,
			AccessToken: instance.OneBot.AccessToken,
			SelfID:      selfID,
		},
		Handler: func(context.Context, string, json.RawMessage) (any, error) { return nil, nil },
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	return client.Probe(ctx)
}
