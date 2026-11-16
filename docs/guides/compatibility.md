# 功能支持情况

本文档记录当前版本在[云湖](https://www.yhchat.com/)机器人开放平台与 [OneBot v11](https://github.com/botuniverse/onebot-11) 之间的能力对照，逐项标明状态、限制与原因。协议细节见 [协议映射说明](../design/protocol-mapping.md)，测试方法见 [测试指南](testing.md)。

## 1. 状态符号

| 符号 | 含义 |
|---|---|
| `✓` | 已实现 |
| `部分` | 已实现，但带条件限制，限制写在备注里 |
| `计划中` | 未实现，在开发计划范围内 |
| `待验证` | 平台行为尚未确认，代码按降级方式处理 |
| `✗` | 不做，原因写在备注或第 5 节 |

“接收”指云湖到框架，“发送”指框架到云湖。带 `✗` 的条目不是缺漏，是平台没有对应能力，或者实现它没有意义。

## 1.1 事件接收方式

一个进程服务一个云湖机器人，事件有两条入口，`receive.mode` 三选一：

| 方式 | 配置 | 是否↔公网 | 凭据 | 状态 |
|---|---|---|---|---|
| 订阅连接 | `mode: websocket`（默认） | 不需要 | 机器人令牌拼在 URL 查询参数里 | `✓` |
| Webhook | `mode: webhook` | 需要云湖能访问到监听端口 | 路径令牌，必填且至少 24 个字符 | `✓` |
| 两者同时 | `mode: both` | Webhook 的要求同样成立 | 同上，按 `eventId` 去重 | `✓` |

Webhook 的推送地址是 `http://<listen>/yunhu/<token>`，直接把这段填进云湖控制台的订阅 URL。云湖的推送没有签名，知道地址就能伪造事件，所以令牌必须认真设置，`allowed_ips` 能再加一道锁。

去重表在内存里，10 分钟过期，进程重启后清空；重启窗口内平台重试的事件可能重复转发一次。

## 2. 消息类型

| 消息类型 | 云湖内容类型 | OneBot 消息段 | 接收 | 发送 | 备注 |
|---|---|---|---|---|---|
| 纯文本 | `text` | `text` | `✓` | `✓` | 接收时按 `at` 数组把正文里的提及剪成 `at` 段 |
| @ 提及 | `text` 的 `at` 数组 | `at` | `✓` | `部分` | 发送时正文必须写昵称，昵称缓存里没有这个人时显示标识 |
| @全体成员 | `text` 的 `at` 数组 | `at`，`qq` 为 `all` | `待验证` | `部分` | 发送时降级为文字 `@全体成员`，平台没有全员提及参数 |
| 引用回复 | `text` 的 `parentId` | `reply` | `✓` | `✓` | 接收方向按 `parentId` 生成 `reply` 段，被引用消息不在缓存时框架的 `get_msg` 会返回 1404；发送方向引用只挂在拆分后的第一条消息上 |
| 合并转发 | 无 | `forward`、`node` | `✗` | `部分` | 发送时展开成文本，超过 2000 字符分段；平台没有转发类型，接收方向不会出现 |
| 图片 | `image` | `image` | `✓` | `✓` | 收发两侧都转成 base64 |
| 文件 | `file` | `file` | `✓` | `✓` | 保留文件名与体积 |
| 视频 | `video` | `video` | `✓` | `✓` | 已验证 10.6 MB 的视频 |
| 语音 | `audio` | `record` | `✓` | `部分` | 发送时按文件上传，客户端显示为文件 |
| 表情 | `待验证` | `face` | `待验证` | `✗` | 发送时降级为 `[表情]` |
| Markdown | `markdown` | 无 | `✗` | `✗` | 接收时降级为 `[暂不支持的消息类型: markdown]` |
| HTML | `html` | 无 | `✗` | `✗` | 平台有这个内容类型，用法尚未确认 |
| 其它消息段 | 未确认 | `json`、`xml`、`poke`、`mface`、`dice`、`rps` | `待验证` | `✗` | 发送时降级为占位文本 |

一条云湖消息只有一个内容类型，一条 OneBot 消息可以包含多个消息段。发送一条「文本 + 图片 + 文本」的消息时，程序按顺序拆成三条云湖消息分别发出。

## 3. 事件

| 云湖事件 | OneBot 事件 | 状态 | 备注 |
|---|---|---|---|
| `message.receive.normal`，`chatType` 为 `group` | `message` → `group` → `normal` | `✓` | 机器人自己的消息按 `senderId` 过滤 |
| `message.receive.normal`，`chatType` 为 `bot` | `message` → `private` → `friend` | `✓` | 私聊的 `chatId` 是机器人标识，用户标识取 `senderId` |
| `message.receive.instruction` | 与普通消息相同 | `✓` | 指令名拼回正文，原始指令名放在扩展字段 `yunhu.instruction` |
| 连接建立 | `meta_event` → `lifecycle` → `connect` | `✓` | 握手成功后立即发送 |
| 心跳 | `meta_event` → `heartbeat` | `计划中` | AstrBot 不依赖心跳 |
| `group.join` | `notice` → `group_increase` | `✓` | `sub_type` 固定 `approve`，平台没有操作者字段，`operator_id` 为 0 |
| `group.leave` | `notice` → `group_decrease` | `✓` | 平台不区分主动退出与被踢，`sub_type` 固定 `leave` |
| `bot.followed` | `notice` → `friend_add` | `✓` | 事件里的字段名尚未在真实环境抓包确认，解析同时接受 `userId`、`user_id`、`senderId` |
| `bot.unfollowed` | 自定义 `notice_type` | `✓` | 取 `yunhu_friend_remove`，OneBot 没有标准类型 |
| 撤回 | `notice` → `message_recall` | `✗` | 平台不推送撤回事件 |
| 其它事件 | 无 | `✗` | 在 debug 级别记录事件类型后丢弃 |

## 4. 接口

| OneBot 接口 | 云湖实现 | 状态 | 备注 |
|---|---|---|---|
| `send_group_msg` | `POST /bot/send`，`recvType=group` | `✓` | 多段消息拆成多条云湖消息，按顺序发送 |
| `send_private_msg` | `POST /bot/send`，`recvType=user` | `✓` |  |
| `send_msg` | 按 `message_type` 选择接收方 | `✓` |  |
| `send_group_forward_msg` | 展开为文本后发送 | `部分` | 节点树逐行展开，长内容按 2000 字符分段 |
| `send_private_forward_msg` | 展开为文本后发送 | `部分` | 同上 |
| `send_forward_msg` | 展开为文本后发送 | `部分` | 按参数里的 `group_id`、`user_id` 推断接收方 |
| `get_login_info` | 启动时 `GET /bot/info` 的结果 | `✓` | 机器人标识与昵称在启动时缓存 |
| `get_status` | 本地状态 | `✓` | 固定返回在线 |
| `get_version_info` | 常量 | `✓` | `app_name` 默认为 `HelloYunhu`，开启兼容模式后为 `gensokyo` |
| `get_avatar` | 头像代理 | `部分` | 非标准接口，只在 `compat.gensokyo.enabled` 为真时提供，参数 `user_id` 与可选的 `group_id`，返回头像地址 |
| `get_group_info` | `GET /group/info` | `✓` |  |
| `get_group_list` | `GET /bot/groups` | `✓` | 分页聚合 |
| `get_group_member_info` | `GET /group/members` | `部分` | 需要机器人开启成员列表权限。失败时返回空对象，避免框架丢弃 @ 消息段 |
| `get_group_member_list` | `GET /group/members` | `部分` | 同样需要成员列表权限。失败时返回空列表 |
| `get_stranger_info` | 无对应接口 | `部分` | 返回空昵称，框架可以继续 |
| `get_friend_list` | 无对应接口 | `部分` | 官方 API 列表里的「获取机器人用户列表」标注为待上线，当前返回空数组 |
| `get_msg` | 进程内的最近消息缓存 | `部分` | 只覆盖接收方向，容量 512 条，重启后丢失 |
| `delete_msg` | `POST /bot/recall` | `✓` | 依赖落库的会话归属，重启后仍可撤回；超出映射保留期后失效 |
| `set_group_ban` | `POST /group/gag-member` | `部分` | 只接受 600、3600、21600、43200 秒，0 解禁，-1 永久 |
| `set_group_kick` | `POST /group/remove-member` | `部分` | `reject_add_request` 没有对应参数，记日志后忽略 |
| `.handle_quick_operation` | 直接返回成功 | `✓` | 框架处理快速操作后的回调 |
| `can_send_image` | 配置项 `capabilities.can_send_image` | `✓` | 默认回答真，可在配置里改成假 |
| `can_send_record` | 配置项 `capabilities.can_send_record` | `✓` | 默认回答假。改成真只影响框架的判断，语音仍按文件发送 |
| `get_image` | 无 | `✗` | 收发两侧的图片段都已内联 base64，框架没有二次取图的场景 |
| `set_group_card`、`set_group_name`、`set_group_admin`、`set_group_whole_ban` | 无 | `✗` | 平台没有开放这些能力 |
| 群公告、精华消息、戳一戳 | 无 | `✗` | 平台没有开放这些能力 |
| `get_group_msg_history` 等历史消息查询 | 无 | `✗` | 平台没有按标识查询历史消息的接口 |
| 上表未列出的接口 | 无 | `✗` | 返回 1404 与说明，不返回成功 |

接口名可以带 `_async` 与 `_rate_limited` 后缀，这是 OneBot 规定的调用方式，不是新接口。程序会剥掉后缀，用同一个实现同步执行，响应的 `status` 仍然是 `ok`。标准允许同步返回，调用方只关心有没有失败，因此不额外做队列。

## 5. 平台限制与替代做法

**撤回没有事件通知。** 平台不推送撤回事件，框架收不到 `notice.message_recall`。需要撤回时由框架调用 `delete_msg`，参数是程序分配的消息整数标识。

**禁言只有四个档位。** 平台只接受 600、3600、21600 与 43200 秒，另外 0 表示解禁，-1 表示永久。其它时长向上取到最近的档位，超过 43200 秒时按 43200 秒处理，因此「禁言一分钟」实际是十分钟。

**@ 的显示依赖昵称。** 平台的正文里必须出现 `@昵称` 才会渲染成提及，只给 `at` 数组时客户端显示数字标识。昵称来自收到过的消息事件，进程刚启动、还没见过这个人时，第一条 @ 会显示标识。

**语音只能按文件发送。** 平台没有语音发送接口，发送 `record` 段时按文件上传，客户端表现为一个文件，需要点开才能收听。

**合并转发只能降级为文本。** 平台没有合并转发类型，也没有对应的接口。发送时把节点树逐行展开，嵌套内容缩进一层，每条节点写成「昵称: 正文」，节点里的图片、文件、语音用占位文字表示，原始结构无法还原。展开结果超过 2000 字符时按节点边界分段，多段时每段开头带「[合并转发 i/n]」。只带标识、没有内容的转发展开成一行「[空的合并转发]」。嵌套超过 8 层或节点超过 500 个时截断，并注明省略。

**Markdown 与 HTML 只做降级。** 平台有这两种内容类型，但收发两向都还没有可用的协议细节，接收时给出占位文本，发送时同样降级为文本，不会伪造内容。

**历史消息查不到。** 平台没有按标识查询历史消息的接口。`get_msg` 只能回放本进程收到过的消息，容量 512 条，重启后清空。

**发送消息不自动重试。** 机器人重复发言的代价高于一次发送失败，失败通过接口返回值与日志体现。

**消息标识需要映射。** 云湖的消息标识是 32 位十六进制字符串，OneBot 侧要求整数，两者依靠一张落库的映射表。映射记录默认保留 168 小时，过期或映射丢失后，引用与撤回会失败并返回 1404 或忽略引用。

**媒体统一转 base64。** 云湖的媒体地址带来源校验，框架直接下载会返回 403，因此由程序下载后以 `base64://` 交给框架。内容内联时消息段里不再带 `url` 字段，避免框架顺手去下载那个打不开的地址；只有下载失败、只能给出地址时才保留。单次读取上限 20 MB，一条十几兆的视频会让单条 WebSocket 消息明显变大。

**头像取决于客户端，默认显示的是 QQ 头像。** OneBot v11 的 `sender` 没有头像字段，程序也不提供头像。框架与插件普遍把整数标识当作 QQ 号去 qlogo 取图，接在云湖上得到的是陌生人的头像。Overflow 这类客户端只在协议端自报 `gensokyo` 时才走非标准的 `get_avatar`，开启 `compat.gensokyo` 之后程序会自报该名字并提供头像代理，客户端显示的是云湖里的真实头像。云湖头像的实际内容是 webp，Java 的 `ImageIO` 读不了，代理会在返回前转成 JPEG，带透明通道的转成 PNG。AstrBot 与 NoneBot2 不调用这个接口，开启后它们的行为不变。

**事件固定以消息段数组发出。** CQ 码只支持发送入口，框架发来的 `message` 是字符串时按 CQ 码解析。事件不会以 CQ 码字符串形式发出。

## 6. 计划内

- 心跳事件。
- 三平台发布包与校验和。Docker 镜像推迟到发布之后。
- 云湖上线「获取机器人用户列表」后，把 `get_friend_list` 从空数组换成真实数据。
- `get_msg` 覆盖发送方向的消息。
- 并发 10 条消息不乱序与多连接隔离的验收。
- 订阅连接的断线重连实测。
