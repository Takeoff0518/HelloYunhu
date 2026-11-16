# 扩展字段与非标准行为

本文件写给基于 OneBot v11 编写插件的人，说明本程序在标准之外增加了哪些字段、改变了哪些行为。标准字段的含义不再重复，以 OneBot v11 规范为准。

判断顺序是先用扩展字段，取不到再退回标准字段。扩展字段统一使用下划线命名，与 OneBot 的命名风格一致；标准客户端会忽略不认识的字段，加上它们不影响已有插件。

## 1. 消息事件上的 `yunhu` 对象

每条消息事件都带一个 `yunhu` 对象，保留云湖的原始信息。

| 字段 | 类型 | 说明 |
|---|---|---|
| `msg_id` | 字符串 | 云湖的消息标识，32 位十六进制 |
| `chat_id` | 字符串 | 会话标识。群聊为群标识，私聊为机器人标识 |
| `chat_type` | 字符串 | `group` 或 `bot` |
| `event_id` | 字符串 | 事件标识，平台没有给出时省略 |
| `instruction` | 字符串 | 指令消息的指令名，例如 `/quote`，普通消息省略该字段 |

```json
{
  "post_type": "message",
  "message_type": "group",
  "self_id": 11464576,
  "user_id": 2272345,
  "message_id": 152,
  "yunhu": {
    "msg_id": "7d33ec10a1b24c0e9f8d5a6b3c2d1e0f",
    "chat_id": "695367192",
    "chat_type": "group",
    "event_id": "9c1f…",
    "instruction": "/quote"
  }
}
```

作用是让插件直接拿到云湖的字符串标识。OneBot 侧的消息标识是映射出来的整数，调用云湖自身的接口、或者排查映射问题时，需要原始值。

指令消息的指令名已经拼进正文，形如 `/quote 参数`，插件按正文匹配命令即可。`instruction` 只服务于需要精确判断指令名的场景。

## 2. 通知事件的扩展

通知事件的 `yunhu` 对象保留原始标识与昵称。

| 字段 | 类型 | 说明 |
|---|---|---|
| `event_id` | 字符串 | 事件标识 |
| `user_id` | 字符串 | 云湖的用户标识 |
| `group_id` | 字符串 | 云湖的群标识，好友事件没有该字段 |
| `nickname` | 字符串 | 触发事件的用户昵称 |

平台没有操作者字段，所有通知事件的 `operator_id` 都是 0。

| 事件 | `notice_type` | `sub_type` | 说明 |
|---|---|---|---|
| 关注机器人 | `friend_add` | 无 | 标准类型 |
| 取关机器人 | `yunhu_friend_remove` | 无 | OneBot 没有取关的标准类型，这里自定义 |
| 入群 | `group_increase` | `approve` | 平台不区分主动加入与审批通过 |
| 退群 | `group_decrease` | `leave` | 平台不区分主动退出与被踢，统一按主动退出上报 |

## 3. `get_group_info` 的 `yunhu_permissions`

`get_group_info` 的响应多一个 `yunhu_permissions` 对象，表示机器人在这个群获得的管理权限。

```json
{
  "group_id": 695367192,
  "group_name": "瓦纳海姆大学(筹)",
  "member_count": 31,
  "max_member_count": 31,
  "yunhu_permissions": {
    "allow_edit_group_info": true,
    "allow_gag_member": true,
    "allow_remove_member": false,
    "allow_group_tag_manage": false,
    "allow_get_group_members": false
  }
}
```

| 字段 | 对应能力 |
|---|---|
| `allow_edit_group_info` | 修改群名称、群简介这类群资料 |
| `allow_gag_member` | 禁言成员，`set_group_ban` 需要它 |
| `allow_remove_member` | 移除成员，`set_group_kick` 需要它 |
| `allow_group_tag_manage` | 管理群标签 |
| `allow_get_group_members` | 读取群成员列表，`get_group_member_list` 需要它，默认未授权 |

这五项是机器人在该群的能力，不是操作人的身份。云湖按项授权，每一项独立开关，权限由群主或管理员在群后台逐项授予。

平台返回的是数字 `0` 与 `1`，这里换成布尔值。取名与取值都遵循 OneBot 的命名习惯。

## 4. 机器人自身的角色

OneBot 的 `role` 只有 `owner`、`admin`、`member` 三个取值，云湖的权限是五个开关，两者无法一一对应。本程序的换算规则如下。

| 场景 | 取值来源 |
|---|---|
| 消息事件的 `sender.role` | 操作人的成员等级 `senderUserLevel`，原样映射 |
| `get_group_member_info` 查机器人自己 | 由 `yunhu_permissions` 推导 |
| `get_group_member_list` 里机器人那一条 | 同上 |
| `get_group_member_info` 查其他人 | 平台给出的成员等级 |

`sender.role` 是操作人的身份，与机器人无关。云湖允许群主把管理权限授予机器人，而发命令的人可能只是普通成员。按操作人角色校验的插件会拒绝这类命令，这与 QQ 上的习惯一致：普通成员不能命令机器人管理群。

推导规则只有一条：**四个写入权限里任意一项为真，机器人报 `admin`；四项都为假时报 `member`。**参与判定的是 `allow_edit_group_info`、`allow_gag_member`、`allow_remove_member`、`allow_group_tag_manage`，只读的 `allow_get_group_members` 不算管理能力。

机器人永远不会报 `owner`。云湖的群主始终是真人，机器人不可能是群主，报这个值等于替它声明一个不存在的身份。

推导每次都向平台查询群信息，没有缓存。群主调整权限的时机常常就是插件报告权限不足之后，缓存会让刚授予的权限在一段时间内继续被拒绝。查询失败时按 `member` 上报并记录一条警告，不猜测 `admin`。

`get_group_member_info` 查询机器人自己时不调用成员列表接口，也不依赖 `allow_get_group_members`，因此未授权成员列表的群同样能回答。`get_group_member_list` 在平台列表里没有机器人时补一条，插件遍历列表查找机器人时才有结果。

给插件作者的建议：需要精确能力时读 `yunhu_permissions`，不要只凭 `role` 判断。权限不足时不要静默退出，本程序的写操作失败会带回明确的说明，例如「机器人在群 695367192 没有禁言权限，请在云湖群后台为机器人授予 allowGagMember」。

## 5. `get_avatar` 与程序名

`get_avatar` 不是 OneBot v11 的接口，来自 Gensokyo 的扩展约定。

| 参数 | 类型 | 说明 |
|---|---|---|
| `user_id` | 整数 | 要取头像的用户标识，必填 |
| `group_id` | 整数 | 可以传，本程序不使用 |

返回值是字符串，不是对象，内容是可以直接下载的头像地址：

```json
{"status": "ok", "retcode": 0, "data": "http://127.0.0.1:8082/avatar/2272345"}
```

只有在 `proxy.enabled` 与 `compat.gensokyo.enabled` 同时为真时提供，其余情况返回 1404。该地址返回的图片已经转码成 JPEG，带透明通道的转成 PNG，客户端不需要支持 webp。没有见过该用户的头像时返回一张灰色占位图，不返回错误。

`get_version_info` 的 `app_name` 默认是 `HelloYunhu`，开启兼容模式后是 `gensokyo`，`protocol_version` 固定为 `v11`。Overflow 只在协议端自报 `gensokyo` 时才调用 `get_avatar`，这是兼容模式存在的唯一原因。

## 6. 媒体地址

接收方向的媒体段，`file` 是内联内容，`url` 是可下载地址，两者的取值取决于代理是否开启。

| 情况 | `file` | `url` |
|---|---|---|
| 代理开启，下载成功 | `base64://…` | `http://主机:端口/media/<token>` |
| 代理开启，下载失败 | 与 `url` 相同 | `http://主机:端口/media/<token>` |
| 代理关闭，下载成功 | `base64://…` | 省略 |
| 代理关闭，下载失败 | 原始地址 | 原始地址 |

`<token>` 是原始地址的 base64url 编码，不带签名，进程重启后旧地址仍然可用。代理只转发云湖 CDN 上的地址，其它地址返回 404。请求支持 `Range` 与 `HEAD`，超过 `proxy.stream_threshold` 的媒体边取边发。

超过 `media.inline_limit` 的媒体不内联，`file` 与 `url` 都是代理地址。

头像地址形如 `http://主机:端口/avatar/<用户标识>`，与 `get_avatar` 返回的值一致。两段路径共用同一个监听端口，配置见[配置说明](configuration.md)。

## 7. 行为差异

下面这些不是新增字段，而是调用方需要知道的行为差异。

| 接口或场景 | 行为 |
|---|---|
| `_async`、`_rate_limited` 后缀 | 剥掉后缀后同步执行，响应 `status` 仍为 `ok`，没有异步队列 |
| `.handle_quick_operation` | 直接返回成功，用于 aiocqhttp 处理快速操作后的回调 |
| 一次 `send_group_msg` | 可能拆成多条云湖消息：文本与 @ 合成一条，每张图片、每个文件各自一条。返回值里的 `message_id` 是第一条 |
| 发送失败 | 不自动重试，重复发言的代价高于一次失败 |
| `set_group_ban` | 平台只接受 600、3600、21600、43200 秒，0 解禁，-1 永久。其它时长为正时向上取到最近的档位，超过 43200 按 43200 处理 |
| `set_group_kick` | `reject_add_request` 没有对应参数，记录日志后忽略 |
| `get_msg` | 只覆盖本进程收到的消息，容量 512 条，重启后返回 1404 |
| `get_group_member_info` | 查询其他人且没有成员列表权限时返回空对象，不返回错误 |
| `get_group_member_list` | 没有成员列表权限时只返回机器人自己一条 |
| `get_stranger_info` | 平台没有对应接口，`nickname` 为空字符串 |
| `get_friend_list` | 平台没有机器人用户列表接口，返回空数组 |
| `get_status` | 固定返回 `online` 与 `good` 为真 |
| 合并转发 | 平台没有转发类型，发送时展开成文本，接收方向不会出现转发事件 |
| 机器人自己发出的消息 | 不作为事件推送，也不会出现 `message_sent`，避免自问自答 |
| 心跳 | 只发 `lifecycle` 元事件，不发 `heartbeat` |
| CQ 码 | 只有发送入口接受字符串形式，事件固定以消息段数组发出 |
| `@全体成员` | 云湖没有全体提及能力，`qq` 写成 `all` 时正文里输出「@全体成员」文字，不产生真实提及 |
| Markdown、HTML | 收发两向都没有可用协议细节，接收给占位文本，发送降级为文本 |

## 8. 返回码

| `retcode` | `status` | 含义 |
|---|---|---|
| 0 | `ok` | 成功 |
| 100 | `failed` | 参数缺失或无法解析 |
| 1404 | `failed` | 接口未实现，或目标数据不存在 |
| 1500 | `failed` | 上游接口失败，或机器人没有该项权限 |

失败响应都带 `wording`，内容是给人看的中文说明。权限不足时说明里会写出缺少的权限名，便于在云湖群后台对应到具体开关。
