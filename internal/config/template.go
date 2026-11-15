// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"errors"
	"fmt"
	"os"
)

// ErrExists 表示配置文件已经存在，调用方应当直接加载而不是覆盖。
var ErrExists = errors.New("配置文件已存在")

// Template 是首次运行时生成的配置内容。
//
// 令牌与框架地址留空，加载时会因为校验不通过而给出明确提示，避免拿占位文本去
// 请求平台。文件权限固定为 0600，因为里面会写入机器人令牌。
const Template = `# HelloYunhu 配置文件。
#
# 本文件由程序在首次运行时生成。填写机器人令牌与框架地址后，再次运行程序即可。
# 完整配置项说明见 examples/config.yaml 的注释，发布后并入手册。
# 文件包含机器人令牌，不要提交到版本库。

# 云湖机器人本身。一个进程服务一个机器人，多个机器人请开多个进程。
bot:
  token: ""

# 事件从哪里进来。
receive:
  # websocket 用订阅连接，webhook 用 HTTP 推送，both 同时使用。
  mode: websocket
  # 只有 mode 含 webhook 时才需要这一段。
  webhook:
    listen: 127.0.0.1:8081
    # 写在推送地址的路径里，至少 24 个字符。云湖的推送没有签名，它就是唯一的凭据。
    token: ""
    allowed_ips: []

# 我们对外提供的 OneBot 连接，至少一个。每个连接对应一个框架。
onebot:
  - name: astrbot
    url: ws://127.0.0.1:6199/ws
    access_token: ""

log:
  level: info
  format: text
  raw_events: false
  keep_days: 30
  compress: true

storage:
  path: data/helloyunhu.db
  message_ttl: 168h

yunhu:
  api_base: https://chat-go.jwzhd.com/open-apis/v1
  subscribe_url: wss://ws.jwzhd.com/subscribe
  rate_limit: 10

capabilities:
  can_send_image: true
  can_send_record: false

diagnostics:
  enabled: false
  listen: 127.0.0.1:9090
`

// WriteTemplate 把默认配置写到 path。
//
// 文件已存在时返回 ErrExists，不覆盖用户已有的配置。写入用独占方式创建，避免
// 两个进程同时首次运行时互相覆盖。
func WriteTemplate(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrExists
		}
		return fmt.Errorf("创建配置文件 %s: %w", path, err)
	}

	if _, err := file.WriteString(Template); err != nil {
		_ = file.Close()
		return fmt.Errorf("写入配置文件 %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("关闭配置文件 %s: %w", path, err)
	}

	return nil
}
