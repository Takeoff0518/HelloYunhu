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
# 完整配置项说明见 docs/guides/configuration.md 与 examples/config.yaml 的注释。
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

# 两个方向的媒体体积上限。
media:
  # 云湖的多媒体超过该体积时不再内联为 base64，消息段里只给出地址。
  inline_limit: 20MiB
  # 框架给出的多媒体超过该体积时，放弃这次发送。
  read_limit: 20MiB

# 对外提供云湖媒体与头像的 HTTP 服务，默认关闭。
proxy:
  enabled: false
  listen: 127.0.0.1:8082
  # 客户端访问代理时使用的地址，留空按 listen 推导。
  base_url: ""
  stream_threshold: 8MiB
  media_max: 1GiB
  concurrency: 4

# 兼容模式，默认关闭。开启后自报 gensokyo 并实现 get_avatar，
# 供 Overflow 这类 mirai 客户端显示云湖头像，需要 proxy.enabled 同时为真。
compat:
  gensokyo:
    enabled: false

# 群标签。云湖用群标签表达 QQ 的专属头衔。
tags:
  # 设置头衔时标签不存在就自动创建。
  auto_create: true
  color: "#03A9F4"
  # 持有标签管理权限时按群主上报角色。
  report_owner: true
  special_title:
    # replace 表示设置后只剩这一个标签，append 表示只追加。
    mode: replace
    # 头衔置空时解绑成员的全部标签。
    clear_on_empty: true

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
