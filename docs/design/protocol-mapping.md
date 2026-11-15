# 协议映射说明

本文档记录 HelloYunhu 在两侧协议之间实际使用的映射关系，与代码保持同步。调研依据在 [可行性调研](../research/feasibility-report.md) 中，这里只写结论与尚未确认的部分。

## 1. 事件映射

| 云湖事件 | OneBot 事件 | 状态 |
|---|---|---|
| `message.receive.normal`（群聊） | `message` / `group` / `normal` | 已实现 |
| `message.receive.normal`（私聊，`chatType` 为 `bot`） | `message` / `private` / `friend` | 已实现 |
| `message.receive.instruction` | 与普通消息相同；指令名拼进正文，扩展字段保留 `instruction` | 已实现 |
| `group.join` | `notice` / `group_increase` | 未实现，记录日志后跳过 |
| `group.leave` | `notice` / `group_decrease` | 未实现，记录日志后跳过 |
| `bot.followed` | `notice` / `friend_add` | 未实现，记录日志后跳过 |
| `bot.unfollowed` | 无标准类型，使用自定义 `notice_type` | 未实现 |
| 连接建立 | `meta_event` / `lifecycle` / `connect` | 已实现，握手后立即发送 |
| 心跳 | `meta_event` / `heartbeat` | 未实现 |

指令消息的指令名放在 `instructionName`，正文只有参数。OneBot 没有原生命令，框架按正文匹配命令，因此转换时把指令名与正文拼成一条消息：指令名 `/quote`、正文 `test` 会拼成 `/quote test`。原始指令名同时保留在扩展字段 `yunhu.instruction` 中，需要精确判断的插件可以直接读它。

消息事件中的字段对应关系：

| OneBot 字段 | 取值 |
|---|---|
| `self_id` | `/bot/info` 返回的 `botId`，启动时查询并缓存 |
| `message_id` | 映射表中的整数，接收时分配 |
| `user_id` | `sender.senderId` 直接解析 |
| `group_id` | 群聊取 `chat.chatId`，私聊不填 |
| `time` | 事件头部的 `eventTime` 除以 1000 |
| `message` | 由消息内容转换而来，始终是数组 |
| `raw_message` | 文本消息为正文，其它类型为占位文本 |
| `sender` | `senderId`、`senderNickname`，角色固定为 `member` |
| `yunhu` | 扩展字段，保留 `msg_id`、`chat_id`、`chat_type`、`event_id` |

机器人自己发出的消息按 `senderId` 与机器人标识比较后丢弃，防止自问自答。

## 2. 接口映射

| OneBot 接口 | 云湖实现 | 状态 |
|---|---|---|
| `send_group_msg` | `POST /bot/send`，`recvType=group` | 已实现，支持文本、@、引用与图片 |
| `send_private_msg` | `POST /bot/send`，`recvType=user` | 已实现 |
| `send_msg` | 按 `message_type` 决定接收方类型 | 已实现 |
| `get_login_info` | 启动时查询的 `/bot/info` 结果 | 已实现 |
| `get_status` | 本地状态，固定在线 | 已实现 |
| `get_version_info` | 常量，`app_name` 为 HelloYunhu | 已实现 |
| `get_group_info` | `GET /group/info` | 已实现 |
| `get_group_list` | `GET /bot/groups` 分页聚合 | 已实现 |
| `get_group_member_info` | `GET /group/members` 扫描，失败返回空数据 | 已实现 |
| `get_group_member_list` | `GET /group/members` 分页聚合 | 已实现 |
| `get_stranger_info` | 云湖无对应接口，返回空昵称 | 已实现 |
| `get_msg` | 从最近消息缓存回放，未命中返回 1404 | 已实现 |
| `.handle_quick_operation` | 直接返回成功 | 已实现 |
| `delete_msg` | `POST /bot/recall` | 未实现，M3 |
| `set_group_ban` | `POST /group/gag-member` | 未实现，M3 |
| `set_group_kick` | `POST /group/remove-member` | 未实现，M3 |
| `get_image` | 下载到本地 | 未实现 |
| 其它 | 返回 `retcode=1404` 与说明 | 默认行为 |

`get_group_member_info` 在权限不足时返回空对象而不是错误，这是为了让框架保留 @ 消息段：框架只有在调用成功后才把 @ 段放进消息，而群里 @ 机器人是唤醒的必要条件。

## 3. 消息段映射

接收方向：

| 云湖内容类型 | OneBot 消息段 | 说明 |
|---|---|---|
| `text` | `at` 段加 `text` 段 | @ 段统一放在正文之前，正文保留原文 |
| `image` | `image` 段 | 先下载再以 `base64://` 给出，失败时给出网络地址 |
| `file` | `file` 段 | 带地址与文件名，不下载正文 |
| `video` | `video` 段 | 带地址 |
| `audio` | `record` 段 | 带地址，云湖没有语音发送能力 |
| 其它 | 文本占位 | `[暂不支持的消息类型: x]` |

发送方向：

| OneBot 消息段 | 云湖处理 | 说明 |
|---|---|---|
| `text` | 文本消息 | 连续的文本与 @ 合并为一条 |
| `at` | `content.at` 加正文中的 @标识 | 缺少成员昵称时用标识代替 |
| `reply` | `parentId` | 反查映射表，查不到时忽略引用 |
| `image` | 上传后作为图片消息发送 | 支持 base64、网络地址与本地路径 |
| `file` | 上传后作为文件消息发送 | 带文件名与体积 |
| `video` | 上传后作为视频消息发送 | |
| `record` | 上传后按文件消息发送 | 云湖没有语音发送能力，用户点开文件收听 |
| `face`、`json`、`node` 等 | 文本占位 | 保证不丢消息 |

一条 OneBot 消息可能被拆成多条云湖消息：连续的文本与 @ 合并为一条，每个媒体段单独一条。引用的目标只挂在第一条消息上。

发送接口的 `message` 参数同时接受消息段数组与 CQ 码字符串，字符串按 CQ 码解析后再走同一套拆分逻辑。事件方向固定使用消息段数组，框架侧不需要额外配置。

## 4. 标识映射

| 云湖标识 | OneBot 标识 | 处理方式 |
|---|---|---|
| `botId` | `self_id` | 十进制直接解析，失败退化为稳定哈希 |
| `senderId`、`userId` | `user_id` | 同上 |
| 群 `chatId` | `group_id` | 同上 |
| `msgId`（32 位十六进制） | `message_id` | SQLite 映射表，自增整数 |

映射表在重启后保持不变，旧消息的引用与撤回不会因为重启失效。记录按保留时长清理，默认七天。

## 5. 已确认的观测

以下行为在真实服务端上观察过：

| 场景 | 观测 | 结论 |
|---|---|---|
| `GET /bot/info`，非法令牌 | `{"code":1003,"msg":"非法token"}` | 响应外壳使用 `code` 与 `msg`，没有 `message` 字段；错误码是四位数字 |
| `GET /bot/info`，有效令牌 | `{"code":1,"msg":"success","data":{…}}` | **成功时 `code` 为 1**，不是 0；`data` 含 `botId`、`nickname`、`avatarUrl`、`introduction`、`isBanned`、`isStop`、`banEndTime`、`banReason` |
| 订阅端点，非法令牌 | HTTP 401 | 端点存在且校验令牌，令牌以查询参数传递 |
| 订阅端点，有效令牌 | 握手成功 | 订阅连接可用，地址与鉴权方式符合预期 |
| `message.receive.normal` 事件 | `senderUserLevel` 为 `"owner"` 这类字符串，`senderType` 为 `"user"`；`instructionId` 与 `commandId` 位于 `message` 内且为数字 | 角色是字符串而不是等级数字，指令标识在消息体内 |
| 群聊文本消息 | `chat.chatType` 为 `group`，`msgId` 为 32 位十六进制，`contentType` 为 `text`，`content` 只含 `text` | 与调研结论一致，`at` 数组只在需要时出现 |
| 群聊图片消息 | `content` 含 `imageUrl`、`imageName`、`etag`、`imageWidth`、`imageHeight` | 图片内容字段可用，`imageKey` 只在上传后的场景出现 |
| `message.receive.instruction` 事件 | `instructionName` 与 `commandName` 均为 `/quote`，正文 `text` 只有参数 `test` | 指令名不在正文里，需拼回正文才能让框架匹配命令 |
| `POST /bot/send` 成功 | `{"messageInfo":{"msgId":"…","recvId":"…","recvType":"…"}}` | 消息标识在 `messageInfo.msgId`，不在顶层 |

代码同时把 `code` 为 0 与 1 视为成功，避免不同接口编码不一致时误判。成功响应的 `data` 结构只对 `/bot/info` 与 `/bot/send` 确认过，其余接口仍按常见写法解析。

媒体下载另有两条实测结论：

| 场景 | 观测 | 结论 |
|---|---|---|
| 下载 `chat-img.jwznb.com` 的图片 | 不带来源头时返回 403 | CDN 校验来源 |
| 附带 `Referer: https://myapp.jwznb.com` 后下载 | 可正常获取 | 社区适配器使用同一来源值，代码对 `*.jwznb.com` 与 `*.jwzhd.com` 自动附带 |

## 6. 尚未确认的行为

下面几项依赖真实云湖环境验证，代码对多种写法都做了兼容，实测后需要回来收敛：

1. 除 `/bot/info` 与 `/bot/send` 之外各接口成功响应的 `data` 结构与字段名。
2. 群列表与群成员列表的包装字段，当前接受直接数组与 `list`、`items`、`members`、`groups`、`data`。
3. 群成员列表的字段名与角色取值，消息事件里的角色已确认为 `owner` 这类字符串。
4. 发送消息时 `parentId` 是否被平台采纳为引用关系，请求已带上该字段，效果待验证。
5. 文件与视频发送所需的字段组合，当前按 `fileKey` 加文件名与体积、`videoKey` 组织。
6. 撤回、编辑与禁言接口的请求参数与返回结构。
