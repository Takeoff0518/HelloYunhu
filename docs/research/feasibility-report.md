# HelloYunhu 技术摸底报告

> 项目定位：把「云湖（Yunhu / yhchat.com）机器人开放平台」与「OneBot v11 协议」互转的**薄协议桥**，
> 让原本跑在 QQ（NapCat / Lagrange / go-cqhttp）上的机器人框架（AstrBot、NoneBot2、Koishi、CQ 插件）几乎零改动地接入云湖。
> 首选连接形态：**反向 WebSocket**（框架做服务端，HelloYunhu 做客户端）。
>
> 调研时间：2026-11-14　｜　调研方式：官方文档原文 + 源码级验证（本地 clone AstrBot / aiocqhttp / Gensokyo / Koishi 适配器 / 官方 SDK）+ aiocqhttp 黑盒实测 + 社区适配器源码比对
>
> 证据等级标注：**【核实】**＝有一手链接、源码或接口数据；**【实测待办】**＝文档未覆盖、必须自己跑一遍确认；**【推测】**＝间接推断。

---

## 0. 结论先行（TL;DR）

### 0.1 可行性：高，且是市场空白

- **【核实】截至 2026-11-14，全网没有任何「云湖 ↔ OneBot v11」的协议转译实现。**
  GitHub 以 `yunhu onebot` / `yhchat onebot` / `yunhu cqhttp` / `yunhu napcat` / `云湖 onebot` 检索，唯一命中是一个 1 star、无 license 的 OneBot **12** 事件转换脚本（无网络层、无 action API、无反向 WS）。
  社区现有项目全部是「**平台适配器/插件**」（Koishi、NoneBot、ErisPulse、AstrBot 插件），而不是「**OneBot 协议端**」。
- 这意味着：既有 OneBot 生态的插件与工作流**无法**直接复用；而 HelloYunhu 的定位恰好是补这个缺口。
- 另一个背景：**OneBot v11 的实现端正在萎缩**——Lagrange V2 已官方宣布「不再支持 OneBot 11」，NapCat 仍是主力但风控掉线问题长期存在。也就是说，一个新的、稳定的、可对接非 QQ 平台的 OneBot 协议端，本身就是增量价值，而不只是「云湖补丁」。

### 0.2 五条关键技术判断

1. **云湖确实有 WebSocket 事件推送**（`wss://ws.jwzhd.com/subscribe?token=...`），且**不需要公网 IP**；
   但**官方 v2 文档只写 HTTP POST Webhook**，该端点是社区（Koishi / AstrBot 插件）在用的未文档化通道。
   → 架构必须 **WS 优先 + Webhook 兜底**，端点可配置，并做健康检查。这是**全项目最高风险项**。
2. **云湖事件里没有「机器人自己是谁」**（无 self_id 字段）；私聊时 `chat.chatId` 恰好等于机器人 ID。
   → 启动时 `GET /open-apis/v1/bot/info` 取 `botId`，作为 OneBot 的 `self_id`。**【核实】**
3. **云湖的 ID 全是字符串，消息 ID 是 32 位十六进制**，而 OneBot v11 / aiocqhttp 大量路径要求**整数**（例如 AstrBot 处理引用时直接 `int(m["data"]["id"])`）。
   → **必须自建 ID 映射层**（`string ↔ int64` + 持久化），这是第二个硬骨头。**【核实】**（AstrBot 源码）
4. **云湖没有「一条消息混排图文」的概念**：一条消息只有一个 `contentType`（text / image / markdown / file / video / html…），图片必须**先上传换 imageKey** 才能发，不能直接传 URL。
   → OneBot 的 `[文本][图片][@]` 消息数组必须**拆成多条云湖消息**，或降级为 markdown 内嵌链接。**【核实】**
5. **AstrBot 的 aiocqhttp 适配器完全够用，但有几个「硬要求」**：事件 `message` 必须是**数组格式**（字符串会直接报错并丢弃）、连上后必须发 `meta_event.lifecycle.connect` 才会打印「已连接」、API 响应必须带 `status` 字段且 `echo` 原样回传。**【核实】**（源码）

### 0.3 语言选型：推荐 Go（Python 为可接受备选，Java 不建议）

- **Go**：单二进制分发（Windows 双击即用，对 AstrBot 小白用户最友好）、并发模型天然适配「多机器人 × 多连接」、官方有 Go SDK、参考项目 Gensokyo 就是 Go、内存占用低（与 Python 的 AstrBot 同机部署时很重要）。
- **Python**：迭代最快、与 AstrBot/aiocqhttp 同生态（可直接读源码对齐语义）、官方 Python SDK 可用；代价是分发（PyInstaller/uv/Docker）和长期维护成本。
- **Java**：DaenMax 的第三方 SDK 确实最成熟，但 JVM 部署对目标用户过重、启动慢、社区不匹配；**排除**。

> 建议：**Go 做正式实现**；若你个人 Python 生产力明显更高，选 Python 也不会错——协议复杂度不高，真正的成本在「踩坑验证」，不在语言。

### 0.4 工作量估算（单人）

| 阶段 | 内容 | 估时 |
|---|---|---|
| M0 PoC | 云湖 WS 订阅 → 转 OneBot 事件 → 连上 AstrBot → 文本双向 | 3–5 天 |
| M1 可用 | 图片/at/引用/撤回 + `get_login_info`/`get_group_info`/`get_group_member_list`/`get_msg` + ID 映射 + 限流 | 1–1.5 周 |
| M2 好用 | 私聊/文件/视频/语音/markdown/按钮/转发降级 + 多实例 + 配置 + 日志 + Docker | 1.5–2 周 |
| M3 产品化 | Webhook 双模、内置隧道、Web 控制台、扩展 action、测试与文档 | 2–3 周 |

**MVP ≈ 2 周，可用版本 ≈ 1 个月，产品化 ≈ 2 个月。**

---

## 1. 目标、边界与成功标准

### 1.1 目标

1. 把一个云湖机器人「伪装」成一个 OneBot v11 协议端（等价于 NapCat 的角色）。
2. 支持 OneBot v11 的**反向 WebSocket**（框架为服务端）：`ws://host:6199/ws`，带头 `X-Self-ID`、`X-Client-Role`、可选 `Authorization`。
3. 行为向 **aiocqhttp** 靠拢（AstrBot 的默认 OneBot 适配器就是 aiocqhttp），其次兼容 NoneBot2 的 OneBot v11 适配器与 Koishi 的 onebot 插件。
4. 单实例可托管多个云湖机器人（多个 token），多实例共存不串消息。

### 1.2 首版非目标

- 正向 WebSocket / HTTP POST 上报（可后续加，成本不高但优先级低）。
- OneBot 12 协议。
- 云湖「用户账号」协议（`wss://chat-ws-go.jwzhd.com/ws`，protobuf、需登录，属非官方逆向，**不碰**）。
- 完整实现 OneBot 全部 action（约 60 个），首版覆盖框架实际会用到的 20 个左右 + 明确返回「不支持」。

### 1.3 成功标准（可验收）

- AstrBot 的日志出现 `aiocqhttp(OneBot v11) 适配器已连接。`
- 群里 @机器人 发「你好」→ LLM 回复 → 云湖群收到文本；发图片 → 云湖收到图片。
- 引用一条消息再 @机器人 → AstrBot 能拿到被引用的内容（`reply` 段正确）。
- 长消息/多段消息（文本+图片+at）不崩、不乱序、不重复。
- 断网 30 秒后自动重连，且不重复回复（幂等）。

---

## 2. 云湖开放平台：事实清单

### 2.1 接入模型

- 控制台创建机器人 → 得到 **Token**（长期有效，泄露可找官方重置）。
- 所有服务端 API 走 `https://chat-go.jwzhd.com/open-apis/v1/...`，**token 放在 URL query**（`?token=xxx`），无签名、无 AppSecret。**【核实】**
- 事件推送两条路：
  - **HTTP Webhook**（官方文档口径）：控制台填「订阅 URL」，平台 POST JSON 过来。
  - **WebSocket**（未文档化，社区在用）：`wss://ws.jwzhd.com/subscribe?token=<token>`，收到的是同构的 `{version, header, event}` JSON。**【核实：Koishi 适配器源码 + 文档】**
- 订阅是**按事件类型**在控制台勾选的：普通消息、指令消息、关注/取关、入群/退群、机器人设置；按钮/A2UI/快捷菜单事件**无需勾选，始终推送**。**【核实】**

### 2.2 服务端 API 清单（首版相关）

| 能力 | 方法与路径 | 关键参数 | 备注 |
|---|---|---|---|
| 发送消息 | `POST /bot/send` | `recvId`、`recvType(user\|group)`、`contentType`、`content`、`parentId?` | 10 次/秒 |
| 批量发送 | `POST /bot/batch_send` | `recvIds[]`、`recvType`… | 同上 |
| 编辑消息 | `POST /bot/edit` | `msgId`、`recvId`、`recvType`、`contentType`、`content` | 仅 text/image/file/markdown/html |
| 撤回消息 | `POST /bot/recall` | `msgId`、`chatId`、`chatType(user\|group)` | 注意 chatType 取值与事件不一致 |
| 消息列表 | `GET /bot/messages` | `chat-id`、`chat-type`、`message-id?`、`before?`、`after?` | 可做 `get_msg`/历史兜底 |
| 图片上传 | `POST /image/upload` | multipart `image`，≤10MB | 返回 `imageKey` |
| 视频上传 | `POST /video/upload` | multipart | ≤20MB |
| 文件上传 | `POST /file/upload` | multipart `file`，≤20MB | 返回 `fileKey` |
| 机器人信息 | `GET /bot/info` | — | 含 `botId`、昵称、封禁状态 |
| 机器人所在群 | `GET /bot/groups` | `page`、`size(≤100)` | 分页 |
| 群信息 | `GET /group/info` | `groupId` | 含 `headcount`、`permissions` |
| 群成员列表 | `GET /group/members` | `groupId`、`page`、`size` | **需群主/管理员授权**，默认关闭 |
| 禁言 | `POST /group/gag-member` | `userId`、`groupId`、`gag`(秒，-1 永久) | 需 `allowGagMember=1` |
| 移除成员 | `POST /group/remove-member` | `userId`、`groupId` | 需 `allowRemoveMember=1` |
| 群消息类型限制 | `POST /group/msg-type-limit` | `groupId`、`type`(逗号分隔) | 需 `allowEditGroupInfo=1` |
| 群标签 CRUD | `/group/tag*` | — | OneBot 无对应，做扩展 |
| 看板 | `/bot/board*` | — | OneBot 无对应，做扩展 |
| 流式消息 | `POST /bot/stream` | chunked/SSE | AstrBot 的 aiocqhttp 适配器不支持流式，暂不用 |

**缺口（OneBot 有、云湖没有）**：好友列表（文档标注「待上线」）、陌生人信息、群公告、群文件、精华消息、戳一戳、消息已读、群名片修改、设管理员、全体禁言、群名修改。

### 2.3 事件模型

统一外层：

```json
{ "version": "1.0",
  "header": { "eventId": "…", "eventTime": 1761936492340, "eventType": "message.receive.normal" },
  "event": { … } }
```

| eventType | 是否需订阅 | 关键字段 |
|---|---|---|
| `message.receive.normal` | 是 | `sender{senderId,senderNickname,senderUserLevel,senderAvatarUrl}`、`chat{chatId,chatType(bot\|group)}`、`message{msgId,parentId,sendTime,chatId,chatType,contentType,content}` |
| `message.receive.instruction` | 是 | 同上，另带 `instructionId/instructionName/commandId/commandName` |
| `bot.followed` / `bot.unfollowed` | 是 | `userId`、`nickname`、`avatarUrl`、`time` |
| `group.join` / `group.leave` | 是 | `chatId`(群)、`userId`、`nickname` |
| `button.report.inline` | 否 | `msgId`、`recvId`、`recvType`、`userId`、`value` |
| `a2ui.button.report` | 否 | `msgId`、`userId`、`actionName`、`formContext`、`interactionJson` |
| `bot.shortcut.menu` | 否 | `menuId`、`chatId`、`senderId`、`sendTime` |
| `bot.setting` | 是 | `groupId`、`groupName`、`settingJson` |

`contentType` 决定 `content` 结构：`text{text,buttons[][],at[]}`、`image{imageUrl,imageName,imageKey,imageWidth,imageHeight}`、`file{fileName,fileUrl,fileKey,fileSize}`、`video{videoUrl,videoDuration}`、`audio{audioUrl,audioDuration}`、`markdown{text}`、`html{...}`，另有 `post/expression/form/live_video/live_audio`。

### 2.4 平台限制与坑（必读）

| 项 | 事实 | 影响 |
|---|---|---|
| 频控 | **每个接口 10 次/秒**，超限 `code=1007` | 必须做令牌桶 + 队列 + 退避 |
| 图片/文件 | 图片 ≤10MB，文件/视频 ≤20MB | 超过要降级提示 |
| 无音频发送 | `contentType` 无 `audio` | 语音只能转文件（或 Koishi 那样转视频） |
| @ 机制 | `at` 只传 userId 数组，**text 里必须自己写 `@昵称`** | 转 OneBot `at` 段要反查昵称，或从 text 反解 |
| 私聊语义 | 事件里私聊 `chatType="bot"`、`chatId`=机器人 ID | 发私聊 `recvType="user"`、`recvId`= **senderId** |
| 撤回语义 | `/bot/recall` 的 `chatType` 取 `user\|group`，与事件的 `bot\|group` **不一致** | 写死映射，别直接透传 |
| 无 self_id | 事件不含机器人自身 ID | 启动 `GET /bot/info` |
| ID 形态 | 全字符串；`msgId` 是 32 位 hex | 必须映射成 int64 给 OneBot |
| Webhook 无验签 | 无签名、无 nonce、无 timestamp；**多个 bot 配同一路径会互相收消息** | 自建 path secret、IP 白名单、去重 |
| 重试策略 | **官方文档完全未提** | 按 at-least-once 设计，`eventId` 幂等 |
| 公网要求 | Webhook 需公网可达（明文 http 也接受）；**WS 方式不需要** | WS 优先 |
| 群成员权限 | 群成员列表默认**关闭**，需群主/管理员开 | `get_group_member_list` 可能 403 → 降级 |
| 媒体 CDN | 云湖 CDN 为 `*.jwznb.com` / `*.jwzhd.com`；社区反馈直连可能受 Referer 限制 | 图片需下载转 base64 或本地代理（**实测待办**） |

### 2.5 安全现状

- Token 走 query，容易被网关/日志记录；建议：日志脱敏、反向代理隐藏 query。
- Webhook 无鉴权：任何人知道 URL 就能伪造事件。缓解：URL 里带随机 secret、IP 白名单（云湖出口 IP 未知，**实测待办**）、校验 `header.eventId` 与 `msgId` 的合理性。
- 机器人封禁状态：`/bot/info` 返回 `isBanned/banEndTime/banReason`，建议启动自检并提示用户。

---

## 3. OneBot v11 侧要求（以 aiocqhttp 为准）

### 3.1 反向 WebSocket 握手

```http
GET /ws HTTP/1.1
Host: 127.0.0.1:6199
Connection: Upgrade
Upgrade: websocket
X-Self-ID: 123456789          # 云湖 botId
X-Client-Role: Universal      # Universal / API / Event
Authorization: Bearer <token> # 仅当框架配置了 token
```

aiocqhttp v1.4.4 源码 + 黑盒实测结论：**【核实】**

| 项 | 事实 |
|---|---|
| 端点 | `/ws`、`/ws/`、`/ws/api`、`/ws/event` 全部等价（`strict_slashes=False`，同一个 handler）；`/` 只接受 POST（HTTP 上报） |
| 路径是否参与分发 | **不参与**。写 `/onebot/v11/ws` 会直接 404 握手失败 |
| 角色来源 | `X-Client-Role` 头（`event`/`api`/`universal`，大小写不敏感）；**缺头 → HTTP 400**；值非法 → 握手挂起不返回 |
| `X-Self-ID` | api/universal 必需（作为 `_wsr_api_clients` 的 key），缺失 → HTTP 400 |
| 鉴权 | 只认 `Authorization: Bearer <t>` / `Token <t>`；**`?access_token=` 完全无效（恒 401）**；token 配空串则完全不校验 |
| token 错 | 403（不是 401，别误判成「没带 token」） |
| 同一 self_id 重复连接 | 字典覆盖、后来者赢，旧连接变僵尸 → **不要用同一 self_id 并发开多条 universal** |
| Universal 语义 | 同一条连接上：带 `post_type` 的是事件，不带的是 API 返回 |

> 首版建议：连 `/ws` + `X-Client-Role: Universal` + `X-Self-ID: <botId>`，一条连接搞定。

### 3.2 事件 JSON 的硬性字段

aiocqhttp 的 `Event.from_payload` 只做两件事：取 `post_type` 和 `{post_type}_type`；缺任一就返回 `None` 并**静默丢弃**。**【核实】**

- 消息事件必填：`post_type=message`、`message_type`、`time`、`self_id`、`message_id`、`user_id`、`message`、`raw_message`、`font`、`sender{}`（群聊还要 `group_id`、`sub_type`）。
- 通知事件必填：`post_type=notice`、`notice_type`、`time`、`self_id`。
- 元事件必填：`post_type=meta_event`、`meta_event_type`；生命周期事件还必须有 `sub_type=connect`，事件名才是 `meta_event.lifecycle.connect`，AstrBot 才会打印「已连接」。

### 3.3 API 调用与响应

aiocqhttp 发出的请求：`{"action": "...", "params": {...}, "echo": {"seq": 123}}`。**【核实，含实测】**

```json
{"action":"send_group_msg",
 "params":{"group_id":1,"message":[{"type":"text","data":{"text":"x"}}],"self_id":10001},
 "echo":{"seq":1}}
```

注意：**`self_id` 不会被剔除**，它既用于路由又原样出现在 `params` 里 —— 我们的 action 处理必须容忍这个多余参数。

我们的响应必须是：

```json
{"status": "ok", "retcode": 0, "data": {...}, "echo": {"seq": 123}}
```

实测矩阵（aiocqhttp 1.4.4 + AstrBot master）：

| 我们回什么 | 调用方看到 |
|---|---|
| `status:"ok"` + 原样回 echo | ✅ 返回 `data` |
| `status:"failed"` | ❌ 抛 `ActionFailed`（`e.retcode` 可取） |
| **缺 `status`** | ❌ `KeyError: 'status'`，直接冒到业务代码（不是 ActionFailed，抓不住） |
| `status:"ok"` 但 `retcode:100` | ✅ **视为成功**（retcode 完全不被检查） |
| echo 拍平成 int / 省略 / 字符串 | ❌ 每次调用卡满 **180 秒**（AstrBot 的 `api_timeout_sec`）后 `NetworkError` |

结论：

- **`status` 必填，`echo` 必须原样回传对象**，`retcode` 只是给人和日志看的（建议仍规范填 0/1/100/1404）。
- `data` 缺失会被当成 `None`（不是空对象），AstrBot 里 `if at_info:` 之类的判断会走 else 分支。
- 若事件处理器返回了非 None 字典，aiocqhttp 会回调 `.handle_quick_operation_async`；**必须实现并返回 ok**，否则日志报错。

### 3.4 AstrBot（aiocqhttp 适配器）的具体行为

源码 `astrbot/core/platform/sources/aiocqhttp/*`：**【核实】**

| 行为 | 要求 |
|---|---|
| 连接方式 | AstrBot 起 WS **服务端**（默认 `0.0.0.0:6199`），URL `ws://host:6199/ws` |
| 事件 message | **必须是数组**；字符串会 `logger.critical`、往会话回一条错误提示、并抛异常丢弃 |
| 事件 sender | **必须存在**（`assert event.sender is not None`）且 `sender["user_id"]` 必填；缺失 → 异常被吞、消息静默消失 |
| 事件路由 | 只注册了 `message.group`、`message.private`、`notice`、`request`；`message_type=discuss/guild` 无人消费 |
| 发送 | 调 `send_group_msg(group_id=int, message=array)` / `send_private_msg(user_id=int, message=array)`，可能带 `self_id` 路由参数；ID 非纯数字时退化为 `send_msg(event=...)` |
| 图片/语音 | **一律转成 `base64://...`** 再放进 `image`/`record` 段（单帧可能几十 MB，注意 WS 帧上限） |
| 文件 | `file` 段的 `file` 字段是 `file://` URI 或 file_id；文件会单独逐条发送 |
| 引用 | 收到 `reply` 段时调 `get_msg(message_id=int)`，并期望返回**完整消息事件**（含 `message_type`、`sender`、`message`），否则降级为普通 Reply 段 |
| @ | 收到 `at` 段时调 `get_group_member_info(group_id, user_id)` 取 `card`，再 `get_stranger_info` 取 `nick`/`nickname` |
| 群信息 | `get_group_info` 要 `group_name`、`member_count`；`get_group_member_list` 要 `[{user_id, nickname/card, role}]` |
| 文件下载 | 收到 `file` 段可能调 `get_group_file_url(file_id, group_id)` / `get_private_file_url(file_id, user_id)` |
| 转发 | `send_group_forward_msg` / `send_private_forward_msg` / `get_forward_msg` |
| 特殊段 | `poke` 被当作**消息段**塞进 `send_group_msg`（`{"type":"poke","data":{...}}`）；`json` 段的 `data` 是 **dict** 不是字符串 |
| 流式 | 该适配器 `support_streaming_message=False` |
| 启动 API | **不调用 `get_login_info`**（全仓库无此调用）→ 我们不必强依赖，但 NoneBot/Koishi 与插件会调，仍要实现 |

### 3.5 五条最容易翻车的硬约束（按后果排序）

1. **`get_group_member_info` 失败会让「@机器人」彻底唤不醒。**
   AstrBot 只有在 `get_group_member_info` **成功**后才把 `at` 段 append 进消息；抛 `ActionFailed` 时整段丢弃（源码里 `At` 在 try 内）。而唤醒判定是 `At.qq == self_id`。
   **保命写法**：即使没有权限，也返回 `{"status":"ok","retcode":0,"data":{}}` —— `data` 为空会走 else 分支补一个空昵称的 `At`，@ 仍生效。
2. **`message_id` 必须能 `int()`。**
   AstrBot 解析引用时直接 `int(m["data"]["id"])` 再调 `get_msg`；非数字会被 `except` 吞掉，退化成空 `Reply`，引用唤醒失效。
3. **事件里丢了 `message_type`/`post_type` 就是静默消失。**
   aiocqhttp 的 `Event.from_payload` 只查这两个键，缺了返回 `None`，**无日志、无异常**。这是最耗时的一类 bug。
4. **API 回包缺 `status` 或 echo 没原样回 → 表现为「发一条消息要三分钟」。**
   前者 `KeyError`；后者每次等满 180 秒。二者都不容易被误认为是协议问题。
5. **base64 大帧会被 WS 读上限直接掐断。**
   AstrBot 会把图片/语音整段 base64 内联，单帧可达数十 MB；而常见 WS 库的默认读上限很小（Go `coder/websocket` 默认 32KB、Python `websockets` 默认 1MiB），超限会以 1009 关闭连接（AstrBot 上游已有同类事故 #10926）。→ 无论选什么语言，**读上限必须显式放大（≥64MB）或关闭**。

### 3.6 其它框架的差异（兼容性目标）

- **NoneBot2（OneBot v11 适配器）**：连接后会调 `get_login_info`、`get_status`、`get_version_info`；对 `message_id` 类型较宽容（字符串也能存），但 `delete_msg` / `get_msg` 需要能回查。
- **Koishi（onebot 插件）**：`.getLoginInfo`、`.getGroupList`、`.getGroupMemberList`，对 `retcode` 敏感。
- **CQ 码**：老插件走字符串格式 `[CQ:at,qq=123] 你好`。云湖没有 CQ 概念，**必须双向编解码**（含 `&#91;`/`&#93;`/`&amp;`/`&#44;` 转义），且不能对已经是数组的消息做二次解析。

---

## 4. 参考项目 Gensokyo：能抄什么、别抄什么

Gensokyo（Go，QQ 官方机器人 API → OneBot v11）没有云湖相关代码，但它把「协议桥」这件事的**骨架和坑**都演了一遍。**注意：GPL-3.0，只能读设计，不能拷代码。**

### 4.1 规模分布说明了一切（Gensokyo main @ 2026-01，约 3.5 万行 Go，剔除 vendored）

| 模块 | 行数 | 说明 |
|---|---|---|
| `handlers/message_parser.go` | 1745 | CQ 码/段解析，坑最多 |
| `config/config.go` | 2551 | 150+ 配置项手写 getter，**反面教材** |
| `idmap/service.go` | 1846 | 字符串 ID ↔ int64 双向表，**最该抄** |
| `wsclient/ws.go` | 395 | 反向 WS 客户端，骨架很薄 |
| `callapi/callapi.go` | 226 | action 注册表，骨架很薄 |

**结论：协议骨架（WS + action 路由）只占 5%，95% 的工作量在「字段语义、ID、富媒体、降级」。** 这直接决定了 HelloYunhu 的排期：不要低估「消息段转换 + 边界条件」，那是主要成本。

### 4.2 值得直接借鉴的五个设计

1. **`idmap` 双表分离**：`ids`（身份，必须持久）与 `cache`（消息 ID，可清空重建）分两个 bucket；生成算法是「MD5 → 只留数字 → 取前 N 位（9 位起，冲突加到 18 位）」，让假 ID 看起来像 QQ 号。云湖只需要把「消息 ID 表」做成 TTL 24h 的 SQLite 表即可。
2. **`wsclient` 单写协程 + 写队列**：WebSocket 并发写会 panic，必须一条 goroutine 串行写；断线期间消息进队列。
3. **`callapi` 用 `map[string]HandlerFunc` + `init()` 注册**，handler 只依赖 `callapi.Client` 接口 —— 同一套 handler 可以同时服务「反向 WS / 正向 WS / HTTP」多条入口。HelloYunhu 首版只有反向 WS，但这个接口抽象值得保留。
4. **配置模板 + 「改哪些必须重启」白名单**（Gensokyo 用反射 diff）—— 对小白用户非常友好，避免热加载把连接搞挂。
5. **自检命令**（`-tidy`、`-clean_ids`、`-del_cache`）：运维排错靠这个，不靠用户读日志。

### 4.3 必须避开的五个坑（Gensokyo 的真实教训）

1. **未注册的 action 静默不回包**（`callapi.go` 里 `return ""`）→ 下游永远等超时，是「最大体验灾难」。
   **HelloYunhu：一律回 `retcode=1404` + 明确 wording。**
2. **没有 per-action 超时、没有 ReadDeadline、没有 ping/pong** → 半死连接无法发现。
   **HelloYunhu：读超时 + 应用层心跳 + 写队列背压（丢弃计数并告警，不阻塞调用方）。**
3. **发送类 API 一律返回 `retcode:0`**（QQ 审核异步，"报错但其实发出去了"）——这是平台的业务判断，**不能照搬到云湖**：云湖是同步接口，应当如实返回错误，但对「框架可能重试导致重复发言」要保持警惕（发送不自动重试）。
4. **强制重排消息段**（`at→text→image`）→ 语义丢失。**绝不要在接收侧重排**，只能在上游限制时于**发送侧**拆分。
5. **README 虚标能力**（把 `get_image`/`get_msg`/`get_forward_msg` 标成已实现，实际没有）→ 用户信任崩塌。**能力支持表必须与代码同步。**

### 4.4 给 HelloYunhu 的设计结论（浓缩版）

- 先定 ID 策略再写代码：SQLite(WAL) + 双表 + `--rebuild-msgmap`。
- 未实现 action 回 1404，并维护一份「action 支持三态表」写进 README。
- 发送返回真实的 `message_id`（我们自己的 int64），同时把云湖真实 `msgId` 记进扩展字段。
- 配置 **≤30 项**，分三段（upstream / onebot / behavior），全部有默认值与校验；只对 ≤5 项做热重载。
- 内置 `/debug/state`（连接数、最后收发时间、映射条数、最近 10 条失败 action）——排错成本直接砍半。

---

## 5. 总体架构

### 5.1 数据流

```
┌──────────────┐   ①事件(WS 优先/Webhook 兜底)   ┌───────────────────────────┐
│   云湖平台    │ ───────────────────────────────▶ │  HelloYunhu               │
│  (bot token) │ ◀─────────────────────────────── │                           │
└──────────────┘   ④调用开放 API 发消息/群管      │  ① Ingest  (WS/Webhook)   │
                                                 │  ② Normalize (内部消息模型)│
┌──────────────┐   ②OneBot 事件(JSON, 数组消息)   │  ③ Dedup   (eventId/msgId)│
│  AstrBot /   │ ◀─────────────────────────────── │  ④ Map     (事件/段/ID)    │
│  NoneBot2 /  │ ───────────────────────────────▶ │  ⑤ Dispatch(action 路由)  │
│  Koishi      │   ③OneBot API 调用(action/echo)  │  ⑥ Ratelimit(10/s/接口)   │
│ (WS 服务端)   │                                  │  ⑦ Outbound(HTTP→云湖)    │
└──────────────┘                                  └───────────────────────────┘
```

### 5.2 模块划分（建议）

| 模块 | 职责 | 关键点 |
|---|---|---|
| `yunhu/subscribe` | WS 客户端（`wss://ws.jwzhd.com/subscribe?token=`） | 心跳 ping 30s、5s 重连、断线补拉 |
| `yunhu/webhook` | HTTP 服务端（`/yunhu/{instance}/{secret}`） | 兜底模式、快速 200、body 限流 |
| `yunhu/api` | 开放 API 客户端 | token 注入、超时、重试、错误码→OneBot retcode |
| `onebot/server` | 反向 WS 客户端（连框架） | `X-Self-ID`/`X-Client-Role`/`Authorization`、写队列、重连 |
| `onebot/actions` | action 注册表 | 20+ 个 action，未实现返回 1404 |
| `onebot/events` | 事件构造器 | 严格字段、数组格式、`self_id` 注入 |
| `bridge/cqcode` | CQ 码编解码 | 转义、数组↔字符串 |
| `bridge/segments` | 消息段 ↔ 云湖 content | 拆分/合并/降级策略 |
| `bridge/idmap` | ID 映射 | `msgId(hex) ↔ int64`、`chatId ↔ group_id`、持久化 |
| `bridge/dedup` | 幂等去重 | LRU + TTL，`eventId` 与 `msgId` |
| `bridge/ratelimit` | 令牌桶 | 按接口维度 10/s，1007 指数退避 |
| `config` | 配置 | YAML + 环境变量 + 热加载 |
| `webui` | 内置控制台 | 可选，M3 |

### 5.3 配置草案

```yaml
onebot:
  # 连到 AstrBot 的 aiocqhttp 服务端
  url: ws://127.0.0.1:6199/ws
  role: Universal          # Universal / API / Event
  access_token: ""         # 与 AstrBot 配置一致
  reconnect_interval: 3s
  heartbeat_interval: 30s

instances:
  - name: main
    token: "云湖机器人 Token"
    self_id: auto          # 留空则启动时 GET /bot/info 自动获取
    subscribe:
      mode: websocket      # websocket | webhook
      ws_url: wss://ws.jwzhd.com/subscribe
      webhook_listen: 0.0.0.0:8080
      webhook_path: /yunhu/main/<random-secret>
    behavior:
      drop_self_messages: true      # 过滤机器人自己发的消息，防自激
      media_mode: base64            # base64 | proxy | passthrough
      reply_quote: true             # 是否把 parentId 转成 reply 段
      unsupported_action: 1404      # 未实现 action 的 retcode
```

### 5.4 多实例模型

- 每个云湖 bot token = 一个 `instance` = 一条到 AstrBot 的 WS 连接，`X-Self-ID` 用该 bot 的 `botId`。
- aiocqhttp 的 `_wsr_api_clients` 是 `self_id → ws` 的字典，**天然支持多账号共用一个适配器端口**；AstrBot 调用时用 `self_id` 路由（或事件上下文路由）。
- 云湖侧：WS 模式下每个实例独立一条 WS，互不影响；Webhook 模式下必须**每个实例不同 path**（否则会串消息，官方适配器文档明确警告）。

---

## 6. 映射矩阵

### 6.1 事件映射（云湖 → OneBot）

| 云湖 eventType | OneBot 事件 | 映射要点 | 置信度 |
|---|---|---|---|
| `message.receive.normal`（group） | `message` / `group` / `normal` | `group_id=chat.chatId`、`user_id=sender.senderId`、`message_id=映射(msgId)`、`sender{user_id,nickname,card:"",role:owner/admin/member}` | 高 |
| `message.receive.normal`（bot） | `message` / `private` / `friend` | `user_id=sender.senderId`（**不是 chatId**） | 高 |
| `message.receive.instruction` | `message`（同上） + `yunhu` 扩展字段 | 附加 `yunhu_command{id,name}`；`raw_message` 保持原文 | 中 |
| `bot.followed` | `notice` / `friend_add` | `user_id`、`time` | 高 |
| `bot.unfollowed` | `notice` / `yunhu_friend_remove`（非标） | OneBot 无标准类型，用自定义 | 中 |
| `group.join` | `notice` / `group_increase` / `approve` | `group_id`、`user_id`、`operator_id=0` | 高 |
| `group.leave` | `notice` / `group_decrease` / `leave` | 无法区分主动退/被踢（事件无操作者） | 中 |
| `button.report.inline` | `notice` / `yunhu_button_report` | 附 `msg_id`、`value`、`recv_id` | 低（自定义） |
| `a2ui.button.report` | `notice` / `yunhu_a2ui_report` | 附 `action_name`、`form_context` | 低 |
| `bot.shortcut.menu` | `notice` / `yunhu_shortcut_menu` | 附 `menu_id`、`chat_id` | 低 |
| `bot.setting` | `notice` / `yunhu_bot_setting` | 附 `setting_json` | 低 |
| （连接建立） | `meta_event` / `lifecycle` / `connect` | 必须发，否则 AstrBot 不打印已连接 | 高 |
| （定时） | `meta_event` / `heartbeat` | 30s 一次，`status{online,good}`、`interval` | 高 |

> 「低」不代表做不了，而是 OneBot 没有标准语义，只能自定义 `notice_type`；对 AstrBot 这类框架，自定义 notice 会被归为 `OTHER_MESSAGE`（AstrBot 的行为），插件要自己判断 raw payload。

### 6.2 API 映射（OneBot → 云湖）

| OneBot action | 云湖实现 | 备注 |
|---|---|---|
| `send_msg` | `/bot/send`（按 `message_type` 决定 recvType） | 兜底入口 |
| `send_private_msg` | `/bot/send` `recvType=user` | `user_id` → senderId |
| `send_group_msg` | `/bot/send` `recvType=group` | `group_id` → chatId |
| `send_group_forward_msg` | 降级：合并为文本摘要 | 云湖无合并转发 |
| `send_private_forward_msg` | 同上 | |
| `delete_msg` | `/bot/recall` | `chatType` 需 `user\|group` |
| `get_msg` | 本地缓存回放；miss 时 `/bot/messages` 兜底 | 必须返回**完整事件** |
| `get_login_info` | `/bot/info`（缓存） | 返回 `user_id/nickname` |
| `get_status` | 本地状态 | `{online:true, good:true}` |
| `get_version_info` | 常量 | `{app_name:"HelloYunhu", protocol_version:"v11"}` |
| `get_group_info` | `/group/info` | `member_count=headcount`，`max_member_count` 伪造 |
| `get_group_list` | `/bot/groups` 分页聚合 | 需遍历 page 直到取满 |
| `get_group_member_info` | `/group/members` 扫描或事件缓存 | 无权限时返回合成数据 |
| `get_group_member_list` | `/group/members` 分页聚合 | 权限默认关闭 → 可能空数组 |
| `get_friend_list` | 无 API → `[]` | 文档标注「待上线」 |
| `get_stranger_info` | 缓存/合成 | 无 API |
| `set_group_ban` | `/group/gag-member` | `duration` 秒；0=解禁 |
| `set_group_kick` | `/group/remove-member` | `reject_add_request` 无对应 |
| `get_image` | 下载到本地，返回 `file://` 路径 | 需处理 CDN Referer |
| `can_send_image` / `can_send_record` | 常量 true | |
| `.handle_quick_operation_async` | 直接返回 ok | aiocqhttp 专用 |
| 扩展：`yunhu_edit_msg` | `/bot/edit` | 云湖独有能力 |
| 扩展：`yunhu_set_board` | `/bot/board` | |
| 未实现 | `retcode=1404` + `wording` | 别静默成功 |

### 6.3 消息段映射

**云湖 → OneBot（收）**

| 云湖 contentType | OneBot 段 | 说明 |
|---|---|---|
| `text` | `text` + `at`（若有 `content.at`） | `at` 需从 text 中定位 `@昵称` 并切分 |
| `image` | `image{file:url, url}` | 建议下载转本地/base64，规避 Referer |
| `file` | `file{name, url}` | AstrBot 会再调 `get_group_file_url` → 未实现会报错（可返回 url） |
| `video` | `video{file:url}` | |
| `audio` | `record{file:url}` | |
| `markdown` | `text`（降级）或自定义 `markdown` 段 | AstrBot 会把 markdown 段转 Plain |
| `html` | `text`（去标签） | |
| `post/expression/form/...` | `text` 占位 `[暂不支持: type]` | 保证不丢消息 |

**OneBot → 云湖（发）**

| OneBot 段 | 云湖处理 | 策略 |
|---|---|---|
| `text` | `contentType=text` | 多段文本可合并 |
| `image`（url/base64/file） | 下载 → `/image/upload` → `imageKey` | **必须上传**，不能直传 URL |
| `at` | 收集 userId → `content.at`，并在 text 前置 `@昵称` | 昵称从 `get_group_member_info` 缓存取 |
| `reply` | `parentId=映射回(msgId)` | 若无法映射则忽略 |
| `record` | `/file/upload` → `contentType=file` | 云湖无 audio 发送 |
| `video` | `/video/upload` | |
| `file` | `/file/upload` | |
| `face`/`mface`/`dice`/`rps` | 文本占位 `[表情]` | |
| `json`/`xml` | 文本占位 | |
| `markdown` | `contentType=markdown`（原生） | 我们的优势能力 |
| `node`/`forward` | 合并为文本 | |

**混排拆分规则**（核心算法）：

```
输入 segments（按序）
1. 先把连续的 text/at/reply 聚成一个 text 消息（at 收集到 content.at，reply 取 parentId）
2. 遇到 image/file/video/record：先 flush 当前 text 消息，再单独发媒体消息
3. 每个媒体消息同样可以带 parentId（仅第一条带，避免刷屏引用）
4. 返回 message_id：取第一条云湖返回的 msgId
```

失败降级：上传失败 → 把 `[图片: 文件名]` 追加到文本消息，保证用户至少看到提示（NextBridge 也是这么做的）。

### 6.4 ID 映射（重点）

| 云湖 | OneBot | 策略 |
|---|---|---|
| `botId`（数字字符串） | `self_id`（int64） | 直接 parse；失败则 hash 后取正数 |
| `senderId` / `userId` | `user_id` | 直接 parse（云湖是纯数字） |
| `chatId`（群） | `group_id` | 直接 parse |
| `msgId`（32 hex） | `message_id`（int64） | **自增计数器** + 双向表；表项含 `msgId/chatId/chatType/direction/time` |
| 反向：`message_id` → 撤回/引用 | `msgId` | 查表；miss 时返回 1404 |

要点：

- int64 用**单调递增计数器**（从 1 开始），不要用 hash——hash 有碰撞风险且不可读。
- 表要**落盘**（sqlite/bbolt），否则重启后 AstrBot 缓存的 `message_id` 全部失效（引用/撤回会失败，但不会崩）。
- 保留 `yunhu_msg_id` 在事件的扩展字段里，方便插件越过映射直接用。
- 清理策略：LRU（保留最近 N 万条）/ TTL（如 24h）。

### 6.5 能力缺口与降级策略

| 缺口 | 降级 | 用户感知 |
|---|---|---|
| 无好友列表 | `get_friend_list` → `[]` | 依赖好友列表的插件不可用 |
| 无群成员权限 | 返回空列表 / 合成 `{user_id, nickname:"", role:"member"}` | @ 昵称可能显示为 ID |
| 无合并转发 | 拼成一条长文本 | 长文可能被截断 |
| 无语音发送 | 转文件 | 用户要点开 |
| 无图文混排 | 拆多条 | 顺序正确即可 |
| 无群公告/群文件 | 返回 1404 | 明确报错 |
| 无戳一戳 | 降级为文本 `[戳一戳]` | |
| 无全体禁言 | 1404 | |

---

## 7. 关键技术难点与方案

1. **CQ 码 vs 数组的双向兼容**
   AstrBot 发的是数组，老插件发的是字符串。入口统一：`if string → parse CQ → array`；出口按连接/配置决定（默认数组，`message_post_format=string` 时转 CQ）。
   CQ 转义是经典坑：`&` → `&amp;`、`[` → `&#91;`、`]` → `&#93;`、`,` → `&#44;`，**解码只做一次**。

2. **图片必须上传**
   AstrBot 一律 `base64://`，老插件可能给 URL / `file://` / 本地路径。统一走 `resolve → bytes → /image/upload → imageKey`。加 LRU 缓存（同图重复发送不重复上传），并限制大小 10MB。

3. **引用消息**
   收：`parentId` 非空 → 构造 `reply` 段 + `id=映射(父消息)`；如果父消息不在缓存，仍给 `id`，AstrBot 会调 `get_msg`，我们返回合成事件（内容未知就返回 `[引用消息]` 占位）。
   发：`reply` 段 → `parentId`。

4. **@ 的双向转换（含唤醒陷阱）**
   收：`content.at=[uid...]` + text 内含 `@昵称`。按 at 顺序在 text 里匹配 `@xxx`，切出 `at` 段，保留顺序（OneBot 的 at 是有位置的）。
   发：把 `at` 段的 qq 收集成数组；同时把 text 中的 `@xxx` 补全成 `@昵称`（云湖要求 text 里也要有），昵称来自成员缓存，取不到就用 ID。
   **关键**：AstrBot 在 `get_group_member_info` 失败时会丢掉整个 `at` 段 → 群里 @ 机器人唤不醒。所以这个 action 永远要回 `status:ok`（`data` 可为空对象）。

5. **message_id 必须数字**
   AstrBot 的 reply 处理直接 `int()`。所以 message_id 一律发我们自己的 int64，并在事件里额外给 `yunhu_msg_id`。

6. **私聊 chatType 不一致**
   事件 `bot`、发送 `user`、撤回 `user`。写一个 `normalizeChatType()`，全项目只此一处。

7. **自激过滤**
   `senderId == botId` 的消息直接丢弃（云湖是否推送机器人自己的消息**待实测**，但必须先防）。

8. **限流与退避**
   按「接口 + token」维度做令牌桶（10/s），队列满时拒绝并返回 1200（发送失败）。收到 1007 时指数退避重试（最多 3 次，只对幂等操作：上传/查询；**发送消息不自动重试**，避免重复刷屏）。

9. **幂等去重**
   `eventId` LRU（10 分钟窗口）；`msgId` 二次去重。WS 断线重连 + Webhook 双开时尤其重要。

10. **优雅降级**
    所有「云湖没有的能力」统一返回 `retcode=1404` + 明确 `wording`，并在日志里 warning 一次（去重），让用户知道为什么插件不工作。**绝不能静默成功**——那会让用户以为是 LLM 的问题。

11. **大帧与内存**
    AstrBot 把图片/语音整段 base64 内联，单帧可达数十 MB。WS 层帧上限必须放大（≥64MB），并做**流式落盘**而不是全量驻留内存；并发上限制（例如同时最多 4 个大帧）防打爆。

12. **特殊段兼容**
    `poke` 会被 AstrBot 当作**消息段**塞进 `send_group_msg`（OneBot v11 标准没有它）→ 要么支持（映射成文本/忽略），要么整条发送会失败，建议「忽略该段但成功发送其余段」。`json` 段的 `data` 是 dict 不是字符串，注意序列化差异。

---

## 8. 语言选型

### 8.1 对比矩阵（权重按本项目实际情况）

| 维度 | 权重 | Go | Python | Java |
|---|---|---|---|---|
| 分发/部署（目标用户是小白的 Windows/Docker） | 25% | ★★★★★ 单 exe，交叉编译 | ★★☆ PyInstaller 体积大、易误报 | ★★ JRE，启动慢 |
| 并发/多实例（N bot × 2 条连接 + HTTP 服务） | 15% | ★★★★★ goroutine | ★★★ asyncio 够用 | ★★★★ 线程池 |
| 迭代速度（协议细节要反复试） | 20% | ★★★ 类型安全但啰嗦 | ★★★★★ 改完即跑 | ★★ 编译+依赖重 |
| 生态对齐（AstrBot/aiocqhttp 同语言） | 10% | ★★ 只能读 | ★★★★★ 可直接 import 对照 | ★ |
| 官方/参考 SDK | 10% | ★★★★ 官方 Go SDK + Gensokyo | ★★★★ 官方 Py SDK | ★★★ DaenMax |
| 长期维护/贡献门槛 | 10% | ★★★★ 二进制无环境问题 | ★★★ 版本/依赖漂移 | ★★ |
| 资源占用（与 AstrBot 同机） | 10% | ★★★★★ ~20MB | ★★★ ~80MB | ★★ ~300MB |

### 8.2 结论

**首选 Go。** 理由：这是「基础设施型」组件，用户期望的是「下载 → 填 token → 双击运行」，Go 的单二进制直接命中；多机器人并发模型天然；Gensokyo 提供了可抄的分层；官方 Go SDK 可当字段参考。

**何时应该选 Python**：如果你更熟 Python、想 3 天内出可演示 PoC、或者计划让 HelloYunhu 未来「内嵌」进 AstrBot 作为插件（而不是独立进程）。Python 方案的所有依赖（aiohttp / websockets / fastapi）都成熟，性能对聊天机器人完全够。

**Java 不建议**：除了「你本人是 Java 工程师」这一条，没有别的理由。

### 8.3 无论选哪个语言，都要先做的三件事

1. 写一个**协议探针脚本**（20 行）：连 WS、打印原始事件、调 send 打印响应。所有「待实测」项都用它确认。
2. 用 **AstrBot + aiocqhttp 真实跑通**，不要只看文档。
3. 把 **CQ 码编解码**与 **ID 映射**当独立模块写单测。

---

## 9. 里程碑与排期

| 里程碑 | 交付物 | 验收 |
|---|---|---|
| **M0 · PoC（3–5 天）** | WS 订阅 → OneBot 事件 → 连 AstrBot → 文本双向 | 群里 @bot 说话能收到 LLM 回复 |
| **M1 · 可用（+1.5 周）** | 图片/at/引用/撤回、`get_login_info`/`get_group_info`/`get_group_member_list`/`get_msg`、ID 映射落盘、限流 | 常见 AstrBot 插件（对话、绘图、搜索）可用 |
| **M2 · 好用（+2 周）** | 私聊、文件/视频/语音、markdown、按钮事件、转发降级、多实例、YAML 配置、日志、Docker | 两个云湖 bot 同时跑不串消息 |
| **M3 · 产品化（+3 周）** | Webhook 双模、内置隧道（cloudflared）、Web 控制台、扩展 action、文档与示例 | 小白用户 5 分钟接入 |

---

## 10. 风险登记册

| # | 风险 | 概率 | 影响 | 缓解 |
|---|---|---|---|---|
| 1 | `wss://ws.jwzhd.com/subscribe` 是未文档化端点，可能变更/关闭 | 中 | 高（核心链路） | WS/Webhook 双模、端点可配置、启动自检、文档写明 |
| 2 | Webhook 无验签，事件可伪造 | 高 | 中 | path secret、IP 白名单（待实测出口 IP）、幂等 |
| 3 | 云湖 API 演进快（2026 新增群管/标签/A2UI/流式），字段变动 | 高 | 中 | 解析全 defensive、未知 contentType 降级、版本自检 |
| 4 | 机器人被封禁/限流 | 中 | 高 | `/bot/info` 自检、1007 退避、日志告警 |
| 5 | 平台体量小（官方称 10 万活跃 vs AstrBot 4.7 万 star） | 高 | 中 | 定位为「AstrBot/NoneBot 的云湖接入方案」，绑教程与一键部署 |
| 6 | AstrBot 自身升级导致 aiocqhttp 行为变化 | 中 | 中 | 版本兼容测试、CI 跑 AstrBot 冒烟 |
| 7 | 媒体 CDN 防盗链导致图片显示失败 | 中 | 中 | 下载转 base64、本地代理 |
| 8 | 消息 ID 映射丢失导致引用/撤回失效 | 中 | 低 | 落盘 + 兜底 1404 |
| 9 | Token 泄露（query 参数） | 中 | 高 | 日志脱敏、文档警告 |
| 10 | 法律/合规（非官方协议桥） | 低 | 中 | 不逆向用户账号协议、不绕过平台限制、README 声明 |
| 11 | OneBot v11 生态整体萎缩（Lagrange V2 弃 OB11、新框架转向 OB12/Milky） | 中 | 中 | 同时保留「OneBot 12 / 自定义 JSON 事件」的演进空间，协议层与业务层解耦 |

---

## 11. 待实测清单

### 11.1 已经验证、不用再测的（本次调研已完成）

- aiocqhttp 的握手头、路径、401/403/400 行为、token 只认 Authorization。
- 事件必填字段、缺字段静默丢弃的规则。
- API 回包 `status` 必填、`echo` 必须原样回对象、`retcode` 不被检查。
- AstrBot 不调用任何启动 API、不强制心跳与 lifecycle。
- `message` 必须是数组、`sender` 必须存在、`message_id` 必须可 `int()`。
- `get_group_member_info` 失败会丢 `at` 段（影响唤醒）。
- Gensokyo 的架构、ID 映射算法、以及「未实现 action 静默不回包」等教训。

### 11.2 仍必须实测的（云湖侧，14 项）

1. `wss://ws.jwzhd.com/subscribe?token=` 是否稳定、握手是否要额外参数、断线后是否重放事件。
2. `/bot/send` 响应里 `msgId` 到底在 `data.msgId` / `data.messageId` / `data.messageInfo.msgId`（社区三种都兼容，说明确实不一致）。
3. 机器人自己发的消息是否会作为 `message.receive.normal` 推回来（决定自激过滤是否必须）。
4. 私聊发送 `recvType` 到底接受 `user` 还是 `bot`（事件给的是 `bot`）。
5. `content.at` 与 text 中 `@昵称` 的对应规则（顺序、多 @、@全体）。
6. 云湖 CDN 图片直连是否需要 Referer；AstrBot 下载是否 403。
7. Webhook 失败是否重试、次数与间隔、`eventId` 是否重复。
8. 群成员列表未授权时的错误码，以及能否从普通消息事件里「顺便」缓存昵称。
9. `parentId` 引用是否要求父消息在最近 N 条内。
10. 撤回时限（是否有 2 分钟限制）。
11. markdown/html 消息在客户端的渲染范围（是否支持图片、表格）。
12. 频控是「每接口 10/s」还是「每 token 10/s」。
13. 同一 token 能否同时开 WS 与 Webhook（去重是否必要）。
14. `send` 是否支持一次请求多段（不支持则确认拆分策略）。

---

## 12. 参考与证据

**云湖官方**
- 开发须知：https://www.yhchat.com/document-v2/
- 服务端 API 列表（含频控）：https://www.yhchat.com/document-v2/getting-started/server-api-list
- 事件订阅详述（事件全量结构）：https://www.yhchat.com/document-v2/event-subscription/event-subscription
- 发送消息 / 批量 / 编辑 / 撤回 / 消息列表：`/message-management/*`
- 媒体上传：`/message-management/media-upload/{image,video,file}-upload`
- 群管理：`/group-management/{group-info,group-members,gag-user,remove-group-member,group-message-type-control}`
- 机器人信息：`/bot-management/{bot-info,bot-groups}`
- 错误码：https://www.yhchat.com/document-v2/getting-started/server-error-codes
- 官方 SDK：`/official-sdks/{python-sdk,golang-sdk,java-sdk}`

**OneBot / aiocqhttp / AstrBot**
- OneBot 11 标准：https://github.com/botuniverse/onebot-11　（在线版 https://11.onebot.dev）
- 反向 WebSocket：https://11.onebot.dev/communication/ws-reverse
- aiocqhttp：https://github.com/nonebot/aiocqhttp（关键文件 `aiocqhttp/__init__.py`、`api_impl.py`、`event.py`）
- AstrBot aiocqhttp 适配器：`astrbot/core/platform/sources/aiocqhttp/`
- AstrBot 文档：https://docs.astrbot.app

**参考实现 / 生态**
- Gensokyo（Go，QQ 官方 API→OneBot）：https://github.com/Hoshinonyaruko/Gensokyo
- Koishi 云湖适配器（平台事实标准文档）：https://fly1919.github.io/adapter-yunhupro/
- NoneBot 云湖适配器：https://github.com/molanp/nonebot-adapter-yunhu
- AstrBot 云湖插件：https://github.com/Soffd/astrbot_plugin_yunhuAdapter
- ErisPulse 云湖适配器：https://pypi.org/project/ErisPulse-YunhuAdapter/
- NextBridge 云湖驱动（媒体代理/拆分策略参考）：https://docs.siiway.org/zh-CN/nextbridge/v0.3/drivers/yunhu

> 生态全景（各项目活跃度、许可证、月下载量）见附录：[生态调研](ecosystem-report.md)；
> Gensokyo 源码级拆解（含 40+ 条「主题 → 文件:行」速查）见附录：[参考项目分析](gensokyo-notes.md)。

---

## 13. 下一步：第一周做什么

1. **Day 1：协议探针**（不写正式代码）
   - 20 行脚本：连 `wss://ws.jwzhd.com/subscribe?token=` → 打印原始事件；调 `/bot/send` → 打印完整响应（确认 msgId 位置）；调 `/bot/info` 拿 botId。
   - 把 §11.2 的 14 项里能测的先测掉一半。
2. **Day 2：最小闭环**
   - 起 AstrBot（Docker 一条命令），配置 aiocqhttp，端口 6199，token 记下。
   - 写 HelloYunhu 骨架：WS 订阅 → 转一条文本事件 → 连 AstrBot → 发 lifecycle → 收 `send_msg` → 调云湖 `/bot/send`。
   - 验收：群里 @bot 说「你好」，云湖收到回复。
3. **Day 3：ID 与消息段**
   - 落 `idmap`（SQLite）+ `cqcode` + `segments` 三个包，写单测。
4. **Day 4：AstrBot 硬约束对齐**
   - `get_group_member_info` 恒回 ok、`get_msg` 回完整事件、`message` 数组、`sender` 必填、`status` 必填、`echo` 回传。
5. **Day 5：图片与 at**
   - `base64://` → 上传换 imageKey；at ↔ `content.at` + `@昵称` 文本。
6. **周末：写 README + Dockerfile + 一份「5 分钟接入」教程**，发到 AstrBot 社区/V2EX/云湖社区收集反馈。

> 判断是否继续投入的**决策点**：如果 Day 5 结束时「群里 @bot 对话 + 发图 + 引用」三条链路都通，就值得按 M1–M3 排期；如果 WS 端点不稳定或云湖发送接口有硬限制（例如必须公网、必须审核），则降级为「Webhook + 内网穿透」方案再评估。

---

## 附录 A：一分钟上手（给未来的 README 用）

```bash
# 1. AstrBot 里创建 OneBot v11 机器人，反向 WS 端口 6199，token 记下来
# 2. 云湖控制台创建机器人，拿到 token，勾选"普通消息事件""群成员事件"
# 3. 启动 HelloYunhu
helloyunhu -config config.yaml

# 4. 云湖里私聊机器人发 "ping"，收到 "pong" 即成功
```

## 附录 B：术语

| 术语 | 含义 |
|---|---|
| 协议端 / 实现端 | 实现 OneBot 协议、连到框架的程序（NapCat、Lagrange、HelloYunhu） |
| 反向 WS | 协议端主动连框架（框架是服务端） |
| CQ 码 | OneBot 的字符串消息格式 `[CQ:type,key=value]` |
| 消息段 | OneBot 的数组消息格式 `{type,data}` |
| self_id | 机器人自身账号 ID |
| retcode | OneBot API 返回码（0 成功，1 异步，100 参数错，1404 未实现） |
