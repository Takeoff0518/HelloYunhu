# Gensokyo 深拆报告 —— 给 HelloYunhu（云湖 API → OneBot v11）的可复用架构与踩坑清单

> 分析对象：`github.com/Hoshinonyaruko/Gensokyo`（Go，GPL-3.0），把「QQ 官方机器人开放平台（botgo / QQBot OpenAPI v1+v2）」转译成 OneBot v11。
> 代码基线：`main` @ `ce25b67` "Beta530 (#529)"，2026-01-31 21:55:17 +0800（本地克隆 `E:\code\_ref\Gensokyo`，`--depth 50`）。
> 仓库热度（2026-11-14 经 shields.io 取得）：★60、Fork 16、Contributors 8、Open issues 27 / Closed 76、最新 release `v529`、最近提交 2026-01（约 9 个月无提交）、代码体积 1.4 MiB。
> 规模：全部 `.go` 246 个文件 / 73,495 行；剔除 vendored 的 `go-silk`、`proto` 生成码、`webui`、`frontend` 后 239 文件 / 35,453 行。
> 说明：下文所有「文件:行」均指该提交下的真实位置；标 **[未验证]** 的是我没能跑起来实测、只靠静态阅读推断的行为。

---

## 0. 结论先行

1. **Gensokyo 的本质不是"API 翻译器"，而是"ID 虚拟化系统 + 双通道事件总线"**。它 70% 的复杂度来自一件事：官方平台给你的 ID 是 32 字节不透明字符串（`group_openid` / `member_openid` / `msg_id`），而 OneBot v11 生态硬性要求 `int64`。它用 bbolt 建了三张映射表（持久 ID / 消息 ID 缓存 / 会话属性），并允许把映射表远程化（lotus / gRPC）以支持多分片多实例。
2. **协议适配层写得很薄（`callapi` 注册表 226 行），事件转换层写得很厚（`Processor` + `handlers/message_parser.go` 共 ~4000 行）**，说明"坑都在字段语义和富媒体上，不在协议骨架"。
3. **反向 WS 的实现只有 395 行，能跑但不够健壮**：没有 ping/pong、没有 read deadline、没有 action 超时、未知 action 静默丢弃（不回包）。这四点是最值得 HelloYunhu 反着做的部分。
4. **它踩过的最贵的坑是"平台差异"，不是"OneBot 差异"**：官方群的主动消息额度（每群每月 4 条）、被动回复的 `msg_id + msg_seq` 单调递增与 5 分钟窗口、富媒体两段式上传（`RichMediaMessage → FileInfo → MessageToCreate(msg_type=7)`）、审核异步导致"接口报错但消息已发出"。
5. **OneBot 实现差异的坑集中在 4 个开关**：`array`（trss 要段数组）、`twoway_echo`（獭獭/早苗要 echo 回触发词）、`string_ob11`（要不要把 ID 反序列化成字符串）、`native_ob11`（要不要剥离扩展字段）。这四个开关本身就是一份"下游兼容性清单"，HelloYunhu 应该直接内化成默认策略而不是可选项。

---

## 1. 整体架构

### 1.1 目录结构（真实分层）

```
main.go                 启动装配：flag → config → token → api v1/v2 → ws session → 反向ws → gin 路由 → 阻塞等信号（1004 行）
├── config/             config.go 2551 行：yaml 单例 + 150+ 配置项 + 热重载 + 反射 diff
│   └── template/       config_template.go：带注释的默认配置（首次运行落地 config.yml）
├── structs/            Settings 结构体（唯一"配置契约"）
├── callapi/            ActionMessage / ParamsContent（自定义 Unmarshal）+ handler 注册表（226 行）
├── Processor/          上游事件 → OneBot 事件（Processor.go + 8 个 Process*.go）
├── handlers/           下游 action → 上游 API（32 个 handler）+ message_parser.go 1745 行（CQ/段/富媒体）
├── wsclient/           反向 WS 客户端（395 行，唯一的客户端实现）
├── server/             正向 WS 服务端、webhook(ed25519)、/uploadpic 图床、/getid(idmap lotus)、短链
├── httpapi/            正向 HTTP API（844 行，路径白名单式 if 链）
├── idmap/              bbolt 映射库 + lotus HTTP/gRPC 客户端 + proto 服务端实现（1846 行）
├── echo/               echo↔message_id↔msg_type 映射、懒 message_id 池、seq 计数器（602 行）
├── images/             base64 图片压缩、图床 API、OSS 上传（腾讯/百度/阿里）
├── silk/ + go-silk/    AMR/SILK 语音转码（含 1.9 万行生成的 Sdk 表）
├── oss/ url/ acnode/   OSS 适配、短链、敏感词审计
├── mylog/              分级日志 + 文件轮转 + WebUI 日志流
├── botstats/           发信统计（心跳/get_status 的数据源，bbolt）
├── webui/ frontend/    控制台（复用 nonebot-plugin-gocqhttp 的前端）
└── docs/               api介绍.md、markdown 定义、api差异-delete_msg.md
```

**可借鉴的分层判断**：`callapi`（协议）与 `handlers`（业务）分离得很干净——`handlers` 只依赖 `callapi.Client` 接口，不依赖 `wsclient`/`server`，因此同一套 handler 同时服务「反向 WS / 正向 WS / 正向 HTTP / 内部调用」。这是整个项目最值得抄的结构（见 `callapi/callapi.go:190-198` 的两个接口 `Client`、`WebSocketServerClienter` 就是为了打破循环依赖）。

### 1.2 文字版架构图

```
                        ┌─────────────────────── 上游：QQ 官方平台 ───────────────────────┐
                        │  ws://api.sgroup.qq.com/websocket  (botgo session, intent 位)   │
                        │  POST /webhook (ed25519 验签, op=13 回调校验)                   │
                        │  OpenAPI v1 (频道) / v2 (群,C2C): /v2/groups/{gid}/messages ... │
                        └───────────┬───────────────────────────────────▲────────────────┘
                                    │ 事件(dto.WSGroupATMessageData...) │ 发消息/上传/撤回
                                    ▼                                   │
   ┌───────────────────────────────────────────────────────────────────┴──────────────┐
   │ 接入层                                                                           │
   │  main.go: botgo.NewSessionManager().Start / StartSingle / multi.NewShardManager   │
   │  server/webhook.go: gin handler → ValidateSignature → webhookHandler 队列        │
   │  （两条路事件最终都进 Processor，无统一 Envelope，靠 type switch 区分）           │
   └───────────┬──────────────────────────────────────────────────────────────────────┘
               ▼
   ┌──────────────────────────────────────────────────────────────────────────────────┐
   │ 事件转换层  Processor.Process{GroupAT,GuildAT,GuildNormal,ChannelDirect,C2C,...}  │
   │  1) ID 虚拟化  idmap.StoreIDv2(openid) → int64  (bbolt: ids bucket)              │
   │  2) 文本还原   handlers.RevertTransformedText()  <@!openid> → [CQ:at,qq=虚拟号]  │
   │  3) 图片附件   dto.Attachments[] → message 段（url 透传）                        │
   │  4) 消息ID     idmap.StoreCachev2(msg_id) → int（cache bucket）                   │
   │  5) 登记反查   echo.AddMsgID/AddMsgType/AddLazyMessageId（内存 sync.Map）         │
   │  6) 组装 OneBot 事件（post_type/message_type/self_id/sender/...）                 │
   └───────────┬──────────────────────────────────────────────────────────────────────┘
               ▼   BroadcastMessageToAll(map)
   ┌──────────────────────────────────────────────────────────────────────────────────┐
   │ 出口层（多路扇出，无优先级）                                                       │
   │  ├─ 反向 WS 客户端  wsclient.WebSocketClient.SendMessage → writeCh(5000) → conn   │
   │  ├─ 正向 WS 客户端  server.WebSocketServerClient.SendMessage（每人一条 conn）      │
   │  └─ 反向 HTTP POST  Processor.PostMessageToUrls → POST + X-Self-ID 头             │
   └───────────┬──────────────────────────────────────────────────────────────────────┘
               ▼
        下游框架 AstrBot / NoneBot2 / Koishi / trss / 獭獭 ...
               │  {"action":"send_group_msg","params":{...},"echo":123}
               ▼
   ┌──────────────────────────────────────────────────────────────────────────────────┐
   │ API 分发层  callapi.CallAPIFromDict(client, api, apiv2, ActionMessage)           │
   │  handlers map[string]HandlerFunc（init() 注册，无前置中间件、无超时、无并发限制）  │
   └───────────┬──────────────────────────────────────────────────────────────────────┘
               ▼
   ┌──────────────────────────────────────────────────────────────────────────────────┐
   │ Action 处理层 handlers/*.go                                                       │
   │  echo→msg_type 反查类型 → 消息 ID 反查真实 msg_id → CQ 码/段解析 → 上游 API 调用  │
   │  富媒体：base64 → 压缩 → 图床上传拿 URL / RichMedia 上传拿 FileInfo → msg_type=7  │
   │  回包：server struct → client.SendMessage(map) + return jsonString（HTTP 用）     │
   └───────────┬──────────────────────────────────────────────────────────────────────┘
               ▼
        上游 HTTP    apiv2.PostGroupMessage / PostC2CMessage / RichMedia / Retract...
```

**关键数据流事实**：
- 事件出口 **不是单一通道**，`BroadcastMessageToAll` 会同时写反向 WS、正向 WS、反向 HTTP（`Processor/Processor.go:280-379`）。
- action 入口有 **四条**：反向 WS 读循环（`wsclient/ws.go:211`）、正向 WS 读循环（`server/wsserver.go:145`）、正向 HTTP（`httpapi/httpapi.go:21-94`）、内部递归调用（`handlers/send_msg.go:107`）。
- 没有"内部事件模型"这一层——上游 `dto.*` 结构体直接进转换函数，OneBot 结构体直接 `structToMap` 后发出。这是它 32 个 handler 里大量 `switch v := data.(type)` 的根因，也是 HelloYunhu 应该改进的点（见 §7）。

---

## 2. 反向 WebSocket 实现细节（作为 WS 客户端连入框架）

全部逻辑在 **`wsclient/ws.go`（395 行）**，没有第二个文件。

### 2.1 握手头：X-Self-ID / X-Client-Role / Authorization / access_token

`wsclient/ws.go:122-130`（重连时）与 `:302-310`（首次连接时），完全相同：

```go
headers := http.Header{
    "User-Agent":    []string{"CQHttp/4.15.0"},
    "X-Client-Role": []string{"Universal"},
    "X-Self-ID":     []string{fmt.Sprintf("%d", botID)},
}
if token != "" {
    headers["Authorization"] = []string{"Token " + token}
}
```

要点：
- `X-Client-Role` **恒为 `Universal`**，不区分 `/api`、`/event`、`/universal`。也就是说它按"Universal 一端全包"实现，不实现 OneBot v11 里"api 端 / event 端分离两连接"的部署形态。
- `X-Self-ID` = `AppID`（uint64），只有开了 `use_uin` 才换成 QQ 号：`main.go:332-337`、`wsclient/ws.go:284-306`。
- `Authorization` 用 **`Token ` 前缀（大写 T）**，不是 `Bearer `。下游侧两种都要能收（frame 侧 Gensokyo 自己作为服务端时同时接受 `Token `/`Bearer `/裸值，见 `server/wsserver.go:46-62` —— 这是被兼容性逼出来的）。
- token 来源二选一，**URL 查询串优先**：先按 `ws_address` 数组下标去配 `ws_token` 数组，再检查 URL 里有没有 `?access_token=`，有就覆盖（`wsclient/ws.go:285-300`）。这是为了兼容"框架把 token 塞进 URL"的派发习惯。
- **没有** `Sec-WebSocket-Protocol` 子协议协商、没有 `X-Signature`（`grep` 确认 `server/*.go`、`wsclient/*.go` 中不存在 `Subprotocols`）。

### 2.2 连接路径

- **不做路径拼接**：`ws_address` 是用户填的完整 URL（模板默认 `ws://<YOUR_WS_ADDRESS>:<YOUR_WS_PORT>`，`template/config_template.go:7`）。`/ws`、`/api`、`/event`、`/universal` 由用户自己写进配置。
- 后果：它**无法**在没有用户配置的情况下自适应 AstrBot（`/ws`）与 NoneBot2（`/onebot/v11/ws`）的路径差异；这是"配置即兼容"的典型妥协。
- 正向 WS 服务端同样只有一个路径：`r.GET("/"+wspath)`，`wspath == "nil"` 时挂根路径（`main.go:496-507`）。

### 2.3 多机器人 / 多连接

- **单机器人多连接**：`ws_address []string` + `ws_token []string`，`main.go:320-355` 对每个地址起一个 goroutine 建 `NewWebSocketClient`，成功/失败分别投 `wsClientChan`/`errorChan`，用计数器收齐（`:348-355`）。
- **多机器人不支持**：`Processors` 里 `Api`/`Apiv2`/`Settings` 都是单例（`Processor/Processor.go:37-43`），`handlers.AppID`/`BotID` 是包级全局变量（`handlers/message_parser.go:38-39`），`idmap` 的 key 前缀虽然带了 `AppIDString` 但整个进程只有一个 AppID。作者自己在注释里承认："其实不需要用 AppIDString，因为 gensokyo 是单机器人框架。可以试着开发一个，会很棒的"（`Processor/ProcessC2CMessage.go:153-154`）。**多机器人只能多进程 + `lotus` 共享 idmap**。
- 多连接的一个隐患：token 查找是"地址字符串相等 + 取第一个匹配"（`ws.go:288-294`），**同一地址配置两次会拿同一个 token**；重连时同理（`ws.go:109-114`）。

### 2.4 断线重连与心跳

重连（`wsclient/ws.go:102-195`）：
- 触发点只有一处：读循环 `ReadMessage` 出错 → `cancel()` 心跳 → `if !isReconnecting { go client.Reconnect() }`（`:85-99`）。
- 重连循环：`maxRetryAttempts = config.GetReconnecTimes()`（默认 100），每次失败 `sleep 5s`（`:140-157`）；**首次连接**用另一个配额 `launch_reconnect_times`（默认 1，`main.go:331`），也就是"启动时基本只连一次"。
- 重连成功后**重发 lifecycle connect 元事件**（`:161-176`），并重建读写协程。
- 已知缺陷：`defer func(){ client.isReconnecting = false }()` 放在 `Reconnect()` **函数返回时**执行（`:189-191`），此时连接刚建立、读循环刚启动，所以 `isReconnecting` 立刻变回 false；若连接在重连过程中再次断开，会再起一个 `Reconnect`，存在竞态。

心跳（`wsclient/ws.go:241-281`）：
- 间隔 `heart_beat_interval`（秒，默认 5）；载荷里 `"interval": 5000` 是**硬编码毫秒**（`:274`），与实际 ticker 不一致 → 下游若按 `interval` 校准会误判掉线。**这是个真实 bug，HelloYunhu 别抄。**
- 状态字段基本是常量：`packet_received: 34933`、`packet_sent: 8513`（`:263-272`），只有 `message_received/message_sent/last_message_time` 来自 `botstats`。
- 每个心跳周期顺带 `processFailedMessages()` 重发失败队列（`:278`），失败队列来自 writer 协程写失败的累积（`:74-76`）。队列只增不清（除重发成功外无界），是内存泄漏风险点。
- **没有** `SetReadDeadline` / `SetPongHandler` / 主动 `PingMessage`（grep 确认），半开连接只能靠 TCP 或下游踢掉。

### 2.5 echo 与 action 响应关联

**结论：没有 pending 表，echo 只是"原样回填"，同步处理完立刻回包。**

- 入站：`recvMessage` 反序列化成 `callapi.ActionMessage` 后 **`go callapi.CallAPIFromDict(...)`**（`wsclient/ws.go:211-222`）——每条 action 一个 goroutine，**无并发上限、无 per-action 超时、无顺序保证**。
- 出站：handler 自己调 `client.SendMessage(resp)`，响应体里 `Echo: message.Echo` 原值带回（`handlers/delete_msg.go:111-123`、`handlers/get_login_info.go:45-64`）。
- 未知 action：`callapi.CallAPIFromDict` 直接 `mylog.Println("Unsupported action:")` + `return ""`（`callapi/callapi.go:211-216`）→ **不发任何回包**。下游只能等超时（NoneBot2 默认 API 超时约 30s 后抛 `ActionFailed`；AstrBot 侧的具体表现 **[未验证]**，预期是相应调用挂住直到自身超时）。这是本项目最容易让框架"看起来挂了"的行为。
- handler 内部也大量 `return "", nil`（如 `handlers/set_group_ban.go:47,50,62`、`get_group_member_list.go:53,120`），等于"收到但不回包"。
- echo 的类型兼容：`ActionMessage` 自定义 `UnmarshalJSON` 依次尝试 int → string → array → object（`callapi/callapi.go:20-67`），因为不同框架 echo 可能是数字、字符串或对象。回包时不保证类型一致（结构体里是 `interface{}`，`structToMap` 后原样输出）。
- echo 还被用来做**消息类型反查**：`msg_type = echo.GetMsgTypeByKey(echoStr)`（`handlers/send_msg.go:24-27`），即"框架把我上报的事件 echo 回来，我用它判断这条事件属于 group/guild/private"。这是它弥补"action 无法自证目标类型"的办法。开了 `twoway_echo` 后 echo 里塞的是 `AppID_s_毫秒时间戳`（`Processor/ProcessGroupMessage.go:33-34, 225-229`）。

### 2.6 并发与超时

| 环节 | 现状 | 位置 |
|---|---|---|
| 入站并发 | 每 action 一个 goroutine，无上限 | `wsclient/ws.go:97, 221` |
| 出站并发 | 单 writer goroutine + `writeCh`（cap 5000）串行写 | `ws.go:48-51, 66-82, 344` |
| 写阻塞 | `SendMessage` 直接 `client.writeCh <- req`，**通道满即阻塞调用方**（可能是事件广播 goroutine） | `ws.go:48-51` |
| action 超时 | 无 | — |
| HTTP 上游超时 | `botgo.NewOpenAPI(token).WithTimeout(15*time.Second)` | `main.go:172,179` |
| 正向 HTTP API 超时 | 配置项 `http_timeout` 存在但 `httpapi/` 内未使用（grep 无命中） | `structs/structs.go:185` |
| 读超时/存活探测 | 无 read deadline、无 ping | `ws.go` 全文 |
| 事件广播 | `BroadcastMessageToAll` 用 WaitGroup 等全部连接写完才返回；`...FAF` 版直接 fire-and-forget | `Processor/Processor.go:260-277` |

---

## 3. 事件映射经验（QQ 官方字段 → OneBot 字段）

以群 @ 消息为例，`Processor/ProcessGroupMessage.go:23-361` 是完整范本。

### 3.1 映射总表（实测字段读取自代码）

| OneBot v11 字段 | 群里取值 | 私聊(C2C)取值 | 频道取值 | 备注 |
|---|---|---|---|---|
| `post_type` | `"message"` | 同 | 同 | 固定 |
| `message_type` | `"group"` | `"private"` | `"guild"`，或虚拟化成 `"group"` | 频道虚拟群见 §3.4 |
| `sub_type` | `"normal"` | `"friend"` | `"channel"` | `ProcessC2CMessage.go:134`、`ProcessGuildATMessage.go:86` |
| `self_id` | `AppID` 或 `uin`（int64） | 同 | 同 | `ProcessGroupMessage.go:176-181` |
| `user_id` | `idmap.StoreIDv2(author.id)` → int64 | 同 | 同 | 见 §3.2 |
| `group_id` | `idmap.StoreIDv2(group_openid)` → int64 | 无（虚拟化时用 user 的虚拟号） | `idmap.StoreIDv2(channel_id)` | 见 §3.3 |
| `message_id` | `idmap.StoreCachev2(msg_id)` → int | 同 | **直接透传上游 msg_id（字符串）** | 频道分支没转 int，见 `ProcessGuildATMessage.go:70` |
| `time` | `time.Now().Unix()` | 同 | 同 | **没用上游时间戳**，见 §3.5 |
| `sender.user_id` | 同 `user_id` | 同 | 同 | |
| `sender.nickname` | 默认空，配了 `card_nick` 才填常量 | 空 | 空 | `ProcessGroupMessage.go:219-223` |
| `sender.card` | 同上 | 无 | 无 | |
| `sender.role` | master_id 命中→`owner`，否则`member` | 无 | 无 | `:230-247`；**种群里所有人都被标 member，owner 是猜的** |
| `sender.sex/age/area/level` | 固定 `"0"/0/"0"/"0"` | 无 | 无 | 凑字段 |
| `raw_message` | 还原后的 CQ 字符串 | 同 | 同 | |
| `message` | 字符串 或 段数组 | 同 | 同 | 由 `array` 开关决定 |
| `font` / `message_seq` | `0` / `0`（结构体零值） | 同 | 同 | 下游若强校验 `message_seq` 会报错 |
| 扩展字段 | `real_message_type`、`real_user_id`、`real_group_id`、`is_binded_user_id/group_id`、`avatar`、`echo` | 同 | 同 | `native_ob11=true` 时全部不填（`:210-217`） |

### 3.2 「上游 ID 是字符串，OneBot 要求整数」的硬约束处理策略

**核心：MD5 数字前缀 + 位数递增避重 + 双向持久化。**（`idmap/service.go`）

```go
// idmap/service.go:219-250
func GenerateRowID(id string, length int) (int64, error) {
    hasher := md5.New(); hasher.Write([]byte(id))
    hash := hex.EncodeToString(hasher.Sum(nil))
    // 只保留数字
    var digitsBuilder strings.Builder
    for _, r := range hash { if r >= '0' && r <= '9' { digitsBuilder.WriteRune(r) } }
    digits := digitsBuilder.String()
    if len(digits) >= length { rowIDStr = digits[:length] } else { rowIDStr = digits + strings.Repeat("0", length-len(digits)) }
    return strconv.ParseInt(rowIDStr, 10, 64)
}
```

策略要点：
1. **默认 `hash_id: true`**（`template/config_template.go:55`），即 `user_id` 不是 1,2,3 递增，而是 9 位 MD5 数字片段（大约 `1e8~1e9` 量级，看起来像 QQ 号）。
2. **冲突处理**：位数从 9 递增到 18，每位都查 `row-<int>` 反向键是否存在，重复就加一位重算（`idmap/service.go:301-321`）。撞库概率随位数指数下降，但代价是同一 ID 的映射结果**依赖历史状态**（不是纯函数）。
3. **双向表**：正向 `openid → 8字节 big-endian int64`，反向 `row-<int> → openid`，同一 bbolt bucket（`StoreID`/`StoreCache`，`:279-402`）。反查用于发送时还原真实 ID（`RetrieveRowByIDv2`）。
4. **非 hash 模式**（`hash_id: false`）退化为自增行号（`CounterKey = "currentRow"`，`:292-300`）——启动更慢的抗冲突检查省了，但会把"用户数"暴露给下游。
5. **消息 ID 走独立的 `cache` bucket**（`StoreCachev2`，`:590-637`），语义上"可清空、可重建"，与 `ids` bucket（用户/群身份，不能丢）分离。这是很好的一刀：**身份映射必须持久，消息映射可以 TTL**。
6. **高性能模式**：`memory_msgid: true` 时消息 ID 只存内存 `sync.Map` + 每 5 分钟全量清空（`echo/echo.go:339-365`，`main.go:609-611`）。作者注释："单日信息量超过 100 万就需要高性能 SSD 或者开启这个选项"（`config_template.go:154`）——直白承认 bbolt 每次 `db.Update` 是瓶颈。
7. **多实例共享**：`lotus` 开关把同一套 API 暴露成 HTTP `GET /getid?type=N&id=...`（`server/getIDHandler.go:12-80`，type 1~16 编码不同操作）与 gRPC（`idmap/grpc.go` + `proto/idmap.proto`），客户端在 `StoreIDv2`/`StoreCachev2`/`WriteConfigv2`/`RetrieveVirtualValuev2` 里分流（`idmap/service.go:539-587, 933-979, 1305-1354`）。**同一个函数的三种后端（local bbolt / remote HTTP / gRPC）用 if-else 堆在一起，是这套代码里最脏但最实用的设计**。

### 3.3 group_id 如何"伪造"

- 群：`GroupID64, err = idmap.StoreIDv2(data.GroupID)`（`ProcessGroupMessage.go:79`）。`group_openid` 是 32 字符 → 得到 9 位整数群号。
- 频道虚拟成群：`channel_id` 同样走 `StoreIDv2`，再 `idmap.WriteConfigv2(群虚拟号, "type", "guild")` 记住"这个虚拟群其实是频道"（`ProcessGuildATMessage.go:225-252`）。
- 私聊虚拟成群/频道：`GlobalPrivateToChannel` 开关；C2C 私聊固定用字符串 `"group_private"` 作为 idmap 的"伪群号"（`ProcessC2CMessage.go:52`），并在 `WriteConfigv2(fmt.Sprint(userid64), "type", "group_private")` 记账（`:166`）。
- **反查顺序很重要**：发消息时先 `ReadConfigv2(群号, "type")` 判断真实类型，再决定走 `PostGroupMessage` / `PostMessage(channel)` / `PostC2CMessage`（`handlers/send_msg.go:114-163`）。这套"虚拟 ID → 属性表 → 真实类型"的三段式，是它能用一套 OneBot 语义覆盖 5 种上游会话的根因。
- 坑：`ReadConfigv2` 是 bbolt 读，缺失时返回 error → 大量 handler 直接 `return "", nil`（不回包），表现为"框架调用没反应"。

### 3.4 message_type 如何判定

**三种互相独立、会打架的机制叠在一起**：

1. **事件侧主动写**：转换事件时把自己知道的类型写进 `echo`/`idmap config`（`WriteConfigv2(GroupID64,"type","group")`）。
2. **action 侧靠 echo 反查**：`echo.GetMsgTypeByKey(echoStr)`（`handlers/send_msg.go:24-27`）。
3. **兜底靠 ID 长度猜**：`len(groupID) == 32` → 一定是群 openid（`handlers/send_msg.go:56, 81-88`）；再不行就 `StoreIDv2` 后读 `type`。
4. **实在不知道就递归猜三次**：`send_msg` 里 `echo.AddMapping(idInt64, 4)` 然后按 `["group","guild","guild_private"]` 顺序重试，中间 `time.Sleep(send_delay)`（`handlers/send_msg.go:165-182`）。这是全项目最"野生"的设计——**代价是同一条消息可能被发三次**。

### 3.5 时间戳

`Time: time.Now().Unix()`（`ProcessGroupMessage.go:207`）。上游 `data.Timestamp` 有值但没用。后果：事件延迟到达时下游的"消息时间"是错的；引用/撤回的时间窗判断会偏。HelloYunhu 应直接用上游 `timestamp`（秒），并在缺失时回退本地时间。

---

## 4. 消息段 / CQ 码处理

### 4.1 入站（上游 → OneBot message）

**字符串模式（默认）**：`handlers.RevertTransformedText()`（`message_parser.go:1062-1405`）做四件事：
1. QQ 官方 at 语法 `<@!openid>` → `[CQ:at,qq=<虚拟int>]`（正则 `re := regexp.MustCompile(`<@!(\d+)>`)`，`:1110-1138`）。at 机器人自己时：可配 `remove_at` 直接删掉，否则替换成 AppID（`:1117-1123`）。
2. `BotID → AppID` 全局替换（`:1107`），因为官方事件里 at 的是"真实 bot uin"，而下游只知道 AppID。
3. 去前后空格、`/` 前缀（`remove_prefix`）。
4. 白/黑名单前缀过滤（`:1160-1232`），命中黑名单直接 `messageText = ""` → 整个事件被丢弃（`ProcessGroupMessage.go:100-103`）。

**段数组模式（`array: true`）**：`handlers.ConvertToSegmentedMessage()`（`message_parser.go:1425-1553`）：
```go
// 图片段：直接用附件元数据，URL 透传
imageSegment := map[string]interface{}{
    "type": "image",
    "data": map[string]interface{}{
        "file": imageFileMD5 + ".image",   // 从 attachments[].filename 抠掉 {} .png .jpg .gif -
        "subType": "0",
        "url":  attachment.URL,            // ← 关键：不下载、不转 base64、不落盘
    },
}
```
- 顺序：先遍历 attachments 加图片段，再从 `msg.Content` 里抠 `<@!...>` 加 at 段，剩余文本作为一个 text 段。
- **最后强制重排**：`sortMessageSegments()` 把所有段按 `at → text → image` 重排（`:1571-1587`）。**原顺序丢失**，"图 + 文字 + at" 这种真实排版会被打乱。这是为绕开 QQ 官方"文本必须和图片同段发送"的限制，但副作用很大，HelloYunhu 不要抄。

### 4.2 图片：三条路径并存

| 场景 | 做法 | 位置 |
|---|---|---|
| 入站图片 | **URL 透传**，不下载（`attachment.URL` 直接放进 `image.data.url`） | `message_parser.go:1459-1467` |
| 下游发 `[CQ:image,file=http(s)://...]` | 去协议头存成 `url_image`/`url_images`，再按 `RichMediaMessage{FileType:1, URL:...}` 发（上游自己拉） | `:597-604`、`send_group_msg.go:1263-1269` |
| 下游发 `[CQ:image,file=base64://...]` | 解码 → `images.CompressSingleImage` 压缩 → **两选一**：① `uploadpicv2_b64=true` 走 `CreateAndUploadMediaMessage`（富媒体直传，拿 FileInfo，`msg_type=7`）；② 否则上传到**内置图床** `/uploadpic` 换公网 URL（`send_group_msg.go:1215-1269`、`images/upload_api.go:30-60`） | 同上 |

**这一步是 HelloYunhu 最需要提前决策的地方**。Gensokyo 之所以要有内置图床 + 三家 OSS + 反向代理，是因为 QQ 官方群**不接受 base64 直传、也不接受 file://**，只接受 URL 或富媒体 FileInfo；而 base64 是 OneBot 生态最常见的图片传递方式。作者在注释里直说："因为 QQ 群没有 form 方式上传，所以在 gensokyo 内置了图床，需公网，或以 lotus 方式连接位于公网的 gensokyo，要正确地开放对应端口和设置正确的 IP 地址在 config，这对一般用户是有一些难度的"（`send_group_msg.go:1216-1218`）。**如果云湖支持 base64/流式上传（ErisPulse 的 `.Image(file: bytes)` 提示它支持流式上传 [未验证]），这一整层可以省掉。**

### 4.3 富文本（markdown / 按钮）的降级策略

- **发送 markdown**：自定义段 `{"type":"markdown","data":{"data": <对象或base64>}}`，解析后 `json.Marshal` → base64 → 塞进 `foundItems["markdown"]`（`message_parser.go:662-702`），最后由 `parseMDData()` 解成 `dto.Markdown` + `keyboard.MessageKeyboard`，用 `Content: "markdown"` 的 `MessageToCreate` 发（`send_group_msg.go:1270-1289`、`parseMDData` 在 `:1678`）。
- **发送失败即降级**：md 解析失败 → `return nil`（消息静默丢失）。**没有"md 失败退化成纯文本"的兜底**，只是一个 CQ 扩展，需要下游显式使用。
- **接收**：官方事件里没有 md 的入站形态（用户不能发 md），所以入站方向不需要；但**频道/消息审核回执、按钮回调**被额外映射成 notice 事件（`put_interaction`、`OnebotInteractionNotice`）。
- 语音：`silk.IsAMRorSILK` 判断 + `silk.EncoderSilk` 转码 + 上传（`send_group_msg.go:1182-1214`）。**转码是硬需求**，因为官方只收 SILK。云湖如果收 amr/mp3 就可以省掉这层 [未验证]。

### 4.4 CQ 码"编解码库"真相

它**没有独立 CQ 库**。CQ 解析是散落在 `message_parser.go` 里的正则 + 字符串拼接：
- 出站解析：`parseMessageContent()` 只认 `text/image/voice/record/at/avatar/markdown` 六种段（`:583-705`），**没有** `face/share/music/reply/forward/node/xml/json/poke/tts`——尽管 readme 在「已实现 CQ 码」表格里把它们都列了出来（`readme.md:112-144`）。这些 CQ 码进来会被 `default: mylog.Printf("Unhandled segment type")` 静默忽略。
- `[CQ:reply,id=N]` 被正则整个删掉（`:923-925`），变成普通消息。
- `[CQ:at,qq=N]` → `<@!realid>` 反查（`:927-953`）；查不到就原样下发 `<@!N>`，**会发出一条 at 不到人的消息**。

> 结论：Gensokyo 的 CQ 处理是"够用级"实现，作为参考实现可以读，作为库不能复用。HelloYunhu 建议自己写一个严格的 `cqcode` 包（encode/decode + 转义），并明确"不支持的段要么降级为文本描述、要么返回 1404"，而不是静默丢弃。

---

## 5. API 覆盖与降级

### 5.1 实际注册的 action（`grep RegisterHandler` 全量，共 33 个键）

| 分类 | action | 真实后端 | 备注 |
|---|---|---|---|
| 发送 | `send_msg` / `send_group_msg` / `send_to_group` / `send_private_msg` | ✅ 真实 API | 核心路径 |
| 发送 | `send_group_msg_raw` / `send_group_msg_async` / `send_msg_async` / `send_private_msg_async` | ✅ 真实 API | async 变体只是同函数换个注册名 |
| 发送 | `send_guild_channel_msg` / `send_guild_channel_forum` / `send_private_msg_sse` / `send_private_msg_wakeup` | ✅ | 频道/SSE/唤醒 |
| 发送 | `send_group_forward_msg` | ✅ 折叠卡片 | 需 `lazy_message_id` |
| 撤回 | `delete_msg` | ✅（窗口内） | 见 `docs/api差异-delete_msg.md` |
| 查询 | `get_login_info` | ⚠️ 本地伪造 | 昵称取 `custom_bot_name`，ID 取 AppID（`get_login_info.go:36-48`） |
| 查询 | `get_group_info` | ⚠️ **群场景返回硬编码假数据** | 群名"测试群"、`member_count:500`、`max_member_count:1000`（`get_group_info.go:126-143`） |
| 查询 | `get_group_list` | ⚠️ 拿**频道**列表冒充群列表 | `get_group_list.go:77-79` 注释自认 |
| 查询 | `get_group_member_list` / `get_group_member_info` | ⚠️ 读本地缓存，昵称"主人" | `get_group_member_list.go:57-101` |
| 查询 | `get_friend_list` | ⚠️ 本地登记的 C2C 用户 | `ProcessC2CMessage.go:176-180` 写，`get_friend_list.go` 读 |
| 查询 | `get_status` / `get_version_info` / `get_online_clients` | ⚠️ 常量 + botstats | `get_status.go:53-70` |
| 查询 | `get_avatar` / `get_guild_*` / `get_robot_share_link` | ✅/⚠️ 混 | 项目自造扩展 |
| 管理 | `get_group_ban`（←注意名字）/ `get_group_whole_ban` | ❌ 几乎全是 `return "", nil` | `set_group_ban.go:14-63`：群场景"暂未开放该能力"、私聊"暂未适配"、频道才真调 `MemberMute` |
| 交互 | `put_interaction` / `mark_msg_as_read` | ✅ | 按钮回调、已读 |
| 快速操作 | `.handle_quick_operation` / `.handle_quick_operation_async` | ✅ → 转成 `send_group_msg`/`send_private_msg` | `handle_quick_operation.go:14-53` |

**注册名有两个明确的 bug**：
- `set_group_ban` 实际注册成 **`get_group_ban`**（`handlers/set_group_ban.go:15`）→ 下游按 OneBot 标准发 `set_group_ban` 会"Unsupported action"静默无回包；
- `set_group_whole_ban` 注册成 **`get_group_whole_ban`**（`handlers/set_group_whole_ban.go:14`）。

### 5.2 未实现的 action 怎么返回

- **未注册的 action**：`CallAPIFromDict` 打印日志后 `return ""` —— **不回包**（`callapi/callapi.go:211-216`）。
- **已注册但走不通的分支**：同样 `return "", nil` —— 不回包（大量出现）。
- **会回的**：`retcode` 恒为 `0`、`status` 恒为 `ok`，连上游报错也返回成功：
  ```go
  if err != nil {
      response.Message = err.Error()
      //response.RetCode = -1
      //response.RetCode = 0 //官方api审核异步的 审核中默认返回失败,但其实信息发送成功了
      response.Status = "ok"
  }
  ```
  （`handlers/message_parser.go:235-245`，`SendResponse`；`SendResponseSB`/`GetStatusResponse` 等处同款。）
- **没有 1404 / 1403 / 100 等 OneBot 错误码**：整个 `handlers/` 目录里 `retcode` 只有 `0`（grep 结果见 §1 附件）。**它放弃了错误语义**。

### 5.3 哪些必须"假装成功"才能让框架不崩

1. `get_login_info`——几乎所有框架启动时都会调，必须返回 `{user_id, nickname}`；它伪造。
2. `get_status`——部分框架用于健康检查，必须 `online/good = true`。
3. `get_group_info` / `get_group_list`——框架用它渲染"群列表"、做权限判断；群场景没有官方 API，只能伪造（`会员数 500/1000` 这种"合理的假数据"其实是刻意设计，避免下游除以 0 或显示空）。
4. `get_group_member_list`——权限插件（如"只有管理员能执行"）依赖它；返回空数组会导致"全员无权限"，返回带假角色的人员列表反而更好。它选了后者。
5. `send_*`——返回 `retcode: 0` + `message_id`，即使上游报错。因为"审核异步"，返回失败会让框架重试 → 重复发言。**这是最值得学的一条业务判断**。

---

## 6. 工程化经验

### 6.1 配置设计

- 单一 `config.yml`，`version` + `settings` 两段（`config/config.go:25-28`），全量字段在 `structs/structs.go:9-207`：**约 150 个配置项**。
- 首发自动落地带注释模板并替换 `<YOUR_SERVER_DIR>` 为内网 IP（`main.go:80-108`），改完要求回车退出——"配置-重启"两段式。
- 热重载：`fsnotify` 监听 + 反射 diff，命中 `restartRequiredFields`（`config/config.go:38-48`，含 ws 地址/token/appid/端口/日志级别等）就**自动重启进程**。这个"重启白名单"本身就是"哪些配置改了必须重启"的文档，很值得抄。
- 代价：150 个 key、2551 行的手写 getter（`GetXxx()` 每个 5 行），**新增一个配置要改 4 处**（structs / template / config.go getter / 使用点）。HelloYunhu 建议：配置结构体 + 直接字段访问，或 zconf/viper 绑定，别手写 getter。

### 6.2 多实例

- 横向：`shard_count`/`shard_id`，多个进程各连一个 shard（`main.go:304-317`）。
- 数据面：`lotus` 模式把 idmap 与图床集中到"主节点"，其余节点通过 HTTP/gRPC 访问（`idmap/service.go:539-587`、`main.go:415-450`）。
- 代价：**所有 handler 的每次 ID 转换都可能变成一次跨机 HTTP 请求**（`StoreIDv2`/`StoreCachev2` 无本地缓存），延迟从微秒级变毫秒级；`main.go:423` 的 `grpc.WithInsecure()` 也没有鉴权。**[未验证]** 高并发下这是不是主要瓶颈；从代码看 `StoreIDv2` 在单条群消息里至少被调用 2~4 次（群号、用户号、消息 ID、sender role），是可预期的。

### 6.3 日志与排错

- `mylog`（437 行）：DEBUG/INFO/WARN/ERROR 四级 + 文件轮转（`log_suffix_per_mins`）+ 推流到 WebUI（`mylog/mylog.go:28-90`）；`botgo.SetLogger` 把上游 SDK 日志也接进来（`main.go:130-133`）。
- 错误落盘：`save_error` 打开后把失败的请求体/错误原文写文件（`send_group_msg.go:2056-2061`）。
- 排错友好的两个设计：① 把收到的 action 参数截断打印（`wsclient.TruncateMessage`，`ws.go:225-238`）；② 每个事件 `PrintStructWithFieldNames()` 反射打印全字段（`Processor/Processor.go:191-213`）。**代价是性能**——这也是它需要 `disable_error_chan`（性能模式）跳过 at/图片解析的原因（`ProcessGroupMessage.go:96-128`）。
- 运维命令：`-tidy` 重建配置、`-clean_ids`/`-del_ids`/`-del_cache` 清表、`-compaction` 压实 bbolt（`main.go:57-66, 222-245`）。**bbolt 需要手动 compaction** 是它的运维债（`idmap/service.go:167-213`）。

### 6.4 性能

- 官方口径："10 MB 内存占用，端口错开可多开"（`readme.md:252-254`）。
- 代码里的性能开关：`disable_error_chan`（跳过文本解析）、`memory_msgid`（消息 ID 走内存）、`lotus_grpc`（比 HTTP 快）、`send_delay`（递归重试间隔）、`forward_msg_limit`。
- 明显的性能陷阱：① 每 action 一个 goroutine + 每条消息多次 bbolt 写事务；② `BroadcastMessageToAll` 等全部连接写完；③ `structToMap` 用 `json.Marshal`+`Unmarshal` 做结构体转 map（`Processor/Processor.go:216-221`），每个事件至少一次全量序列化往返；④ `PrintStructWithFieldNames` 反射打印。

### 6.5 常见问题与维护状态

- 热度与活性：★60 / issue 27 open : 76 closed（关闭率 ~74%，说明作者响应过但也在快速关单）、最后提交 2026-01、release `v529`。**活跃度低（近 9 个月无提交）**，对 HelloYunhu 而言"参考其设计、不要依赖其维护"。
- 作者的 ISSUE 政策（`readme.md:240-250`）明确"不使用模板 / 询问已知问题 / 重复提问 直接关闭"，且"开发者没有义务回复"。这解释了为什么 issue 关闭率高但可检索的知识少——**很多坑只能读代码发现**（本报告的价值所在）。
- 从代码里能"考古"出的高频问题（对应 issue 大概率都是这些）：
  1. 连不上反向 WS：路径/token/`X-Self-ID` 三件套（见 §2.1）。
  2. `at` 不正常：`develop_access_token_dir` 模式下拿不到真实 bot uin，作者自己注释"请从日志手动获取 bot 的真实 id 并设置，不然 at 会不正常"（`main.go:209`）。
  3. 群消息只能发几条就没额度：`msg_id` 取不到 → 走主动消息（每月 4 条），见 §6.6。
  4. 撤回失败：`message_id` 反查不到（`cache` bucket 被清或服务重启），`docs/api差异-delete_msg.md` 专门讲这个。
  5. 图片裂开：图床不可达（`uploadpic` 需公网）。
  6. 中文/emoji 乱码：`go-silk` 与 Windows 代码页问题。
  7. 启动卡住：`launch_reconnect_times` 默认 1，反向 WS 同步连接会阻塞 WebUI 启动（`config_template.go:11` 注释原话）。

### 6.6 它踩过的坑：平台差异 vs OneBot 实现差异

**A. 平台差异（换成云湖后大概率仍然存在，但形态不同）**

| 坑 | 证据 | 云湖是否同样存在 |
|---|---|---|
| 主动消息额度：群每月 4 条；被动回复需 5 分钟内且 `msg_seq` 单调递增 | `handlers/send_group_msg.go:245-255`、`SendStackMessages`（`:2068-2101`，错误码 `22009` 重回栈） | **[未验证]** 需查云湖限流；但"被动窗口 + seq 单调"是 IM 平台通病，建议默认实现 |
| 富媒体两段式上传（RichMedia → FileInfo → msg_type=7） | `images/upload_api.go:30-60`、`send_group_msg.go:1215-1254` | **[未验证]** 云湖支持流式/一次性上传的可能性更大 |
| 审核异步：接口报错但消息已发出 | `handlers/message_parser.go:239` 注释 | 通用 |
| ID 不透明且变长（32 字节 openid / 短 msg_id 混用） | 全项目 `len(id)==32` 判断 | 云湖 ID 也是字符串，**必然存在** |
| 无群成员列表 API → 只能伪造/缓存 | `get_group_member_list.go:57-101` | 云湖有成员/标签 API（ErisPulse 文档），**这项能真做** |
| `at` 是文本语法 `<@!id>` 而非结构化段 | `ProcessGroupMessage.go:99` | 云湖有结构化 at 段 [未验证]，可能不用正则 |
| 语音必须 SILK | `silk/`、`send_group_msg.go:1182-1188` | 可能不需要 [未验证] |
| 撤回窗口 | `docs/api差异-delete_msg.md` | 通用 |

**B. OneBot 实现差异（换平台后 100% 仍然存在，是转译层的"永恒税"）**

| 坑 | Gensokyo 的对策 | 证据 |
|---|---|---|
| `group_id`/`user_id` 有的是 int 有的是 string（Koishi/trss/NoneBot 各不同） | `ParamsContent` 自定义 `UnmarshalJSON`，统一转 string | `callapi/callapi.go:108-180` |
| `echo` 可能是 int / string / array / object | 四级 fallback 反序列化 | `callapi/callapi.go:20-67` |
| `message` 可能是 CQ 字符串 / 段数组 / 单段 map（trss / Koishi / 标准三种形态） | `parseMessageContent` 三个 `case` 分支 | `message_parser.go:554-910` |
| 部分框架不接受扩展字段（多出的 `real_*` 会报错） | `native_ob11: true` 全剥离 | `ProcessGroupMessage.go:210-217` |
| 部分框架要求 `message_id` 是 int，部分要 string | `string_ob11` 开关 | `structs/structs.go:140` |
| 獭獭/早苗需要"双向 echo"才能对上触发词 | `twoway_echo` + echo 里塞 `AppID_s_ts` | `ProcessGroupMessage.go:225-229`、`SendResponse:182-225`（自动撤回也靠它） |
| trss 要段数组 | `array: true` | `structs/structs.go:137` |
| 框架对 `status`/`retcode`/`data` 形状敏感 | 每个 handler 各写一个 `XxxResponse` struct（12 个不同形状） | `handlers/*.go` |
| 框架会调 `get_group_info`/`get_login_info` 做权限与渲染 | 伪造 + 常量 | §5.3 |

---

## 7. 可复用性评估

### 7.1 若 HelloYunhu 用 Go：可直接借鉴的包/思路

| 目标 | 抄什么 | 不改什么 / 改什么 |
|---|---|---|
| WS 客户端骨架 | `wsclient/ws.go` 的 `writeCh` 单写协程（gorilla 不允许并发写）、lifecycle 元事件、重连循环 | **必须补**：read deadline + ping/pong、action 超时、`isReconnecting` 用 `defer` 在**连接生命周期**而非函数返回时复位 |
| 协议分发 | `callapi` 的 `map[string]HandlerFunc` + `init()` 注册 + `Client` 接口（handler 不感知传输层） | **必须补**：未注册 action 回 `retcode:1404` 而不是静默；加 per-action context 超时；加并发信号量 |
| ID 映射 | `idmap` 的「正向 openid→int64 / 反向 row-N→openid」双表 + MD5 数字前缀 + 位数递增避重 + `ids`/`cache` 双 bucket 分离 | 建议换成 **SQLite（WAL）+ 内存 LRU** 或 bbolt + 批量写；bbolt 手动 compaction 是运维负担；lotus 三后端 if-else 建议抽成 `IDStore` 接口 |
| 会话属性表 | `WriteConfigv2(虚拟ID, "type"/"guild_id", ...)` 的 KV 思路 | 建议正式化成 `session_type(chat_id) enum{group,private,guild,c2c}` 表，而不是散装 KV |
| 消息 ID 反查 | `echo` 的 `appid_key → msg_id` 多粒度登记（按 s、按群、按群+人、按 echo）+ 懒 ID 池（4 分钟窗口、usageCount 轮转） | 保留多粒度登记；懒 ID 池建议加 TTL 清理 + 上限 |
| 媒体 | `images/` 的压缩 + 上传抽象；`oss/` 三家实现 | 若云湖能直传就删掉图床层；保留"压缩"因为上游普遍有大小限制 |
| CQ/段 | 不要抄（正则散落、顺序被重排、只认 6 种段） | 自己写 `cqcode` + `segment` 包，严格 en/decode，未知段降级 |
| 配置 | 模板自动落地 + 反射 diff + 「重启白名单」 | **不要**抄 150 key / 2551 行 getter；用结构体直读 + 少量 hot-reload 项 |
| 日志 | 分级 + 文件轮转 + WebUI 推流；错误请求体落盘 | 直接抄思路 |
| 运维 | `-tidy` / `-clean_ids` / `-compaction` 等子命令 | 直接抄 |

### 7.2 若 HelloYunhu 用 Python：哪些设计仍然适用

**完全适用（语言无关的设计）**：
1. 「事件转换 / action 分发 / 传输」三层分离 + 依赖倒置接口（Python 用 `Protocol` / ABC）。
2. 双表 ID 映射 + 双 bucket 语义（身份持久 / 消息可 TTL）——Python 可用 SQLite + `sqlite3` 单连接 + 内存 dict LRU。
3. 会话属性表决定"虚拟 ID 的真实类型"。
4. 多粒度消息 ID 登记 + 懒 ID 池（被动回复窗口）。
5. 「未实现 action 必须回包」——用 `retcode: 1404` 明确拒绝。
6. 「发送类 API 即使上游报错也回 `retcode: 0`」的业务判断（避免框架重试造成重复消息）。
7. 配置热重载 + 重启白名单。
8. 启动时启发式自检（例如：连上后立刻 `get_login_info` 回填自身 ID）。

**不适用 / 需替换**：
- goroutine + channel 的 writer 串行化 → Python 用 `asyncio.Queue` + 单 task 消费（**同样必须串行写**，否则 `websockets` 库会报并发写）。
- bbolt → SQLite（`WAL`）或 `lmdb`；注意 Python 的 GIL 对高吞吐的影响，重 IO 用 async。
- gRPC lotus → 不必做，单进程 + SQLite 足够；确需多实例再上 Redis 共享映射。
- 反射打印结构体 → `dataclass` + `asdict`。
- 手写 150 个 getter → `pydantic-settings`。

**Python 侧额外的坑（Gensokyo 没遇到但 HelloYunhu 会遇到）**：
- `websockets` / `aiohttp` 的自动 ping 与 OneBot 框架的 pong 语义不一致，需显式配置 `ping_interval`。
- 整数化 ID 要注意 Python 的 `int` 无上限，但下游（NoneBot）会把 `user_id` 当 `int` 用，超过 2^63 也不会溢出，反而比 Go 安全；但 **JSON 序列化时不要变成 float**。
- `asyncio` 下 bbolt 类同步 IO 会阻塞事件循环，必须 `run_in_executor` 或换 async 驱动。

---

## 8. 给 HelloYunhu 的 10 条设计建议

1. **先定 ID 策略，再写任何代码。** 云湖的 `userId/groupId/msgId` 都是字符串，OneBot 要 int。直接照抄 Gensokyo 的「MD5 数字前缀 + 9~18 位避重 + 双向表」，但把表放 **SQLite(WAL)** 而不是 bbolt，并把"身份映射（永久）"与"消息映射（TTL 24h）"分成两张表；提供一个 `--rebuild-msgmap` 子命令。**不要**用自增 ID（会暴露用户规模，且多实例冲突）。

2. **未实现的 action 必须回包，且必须是 `retcode: 1404`。** Gensokyo 用静默 `return ""` 让框架等超时，这是它最大的体验灾难（`callapi/callapi.go:211-216`）。HelloYunhu 应在分发层统一兜底：
   ```json
   {"status":"failed","retcode":1404,"data":null,"message":"action not supported: get_forward_msg","echo":<原样>}
   ```
   并维护一张 `ACTION_SUPPORT` 表（支持 / 降级 / 拒绝三态）写进 README。

3. **发送类 action 永远返回成功，但要显式记录真实结果。** 平台审核异步、限流异步，报错 ≠ 没发出。返回 `retcode: 0` + 一个**可反查的** `message_id`（即使上游没给，也要生成并在本地登记），真实错误写结构化日志 + 可选 webhook 上报。避免框架重试造成重复消息（Gensokyo 的 `message_parser.go:239` 就是这个判断）。

4. **被动回复窗口 + `seq` 单调递增要作为一等公民。** 为每个 `chat_id` 维护 `last_inbound_msg_id` 与 `seq` 计数器（Redis/SQLite 均可），发送时若在窗口内则带 `reply_to` + `seq++`；窗口外走主动消息并**检查配额**，超限时不要发送而是返回 `retcode: 0` + 记 warning（否则框架会疯）。这是 Gensokyo 用 `echo` + `AddMappingSeq` + `SendStackMessages(22009)` 拼出来的（`send_group_msg.go:2068-2101`），但它用了 4 个包级全局变量；HelloYunhu 用一个 `PassiveWindowManager` 类收口。

5. **反向 WS 客户端必须补齐 Gensokyo 缺的四件套**：① `SetReadDeadline` + 应用层 ping/pong（15~30s）；② 每个 action 的 `asyncio.wait_for` / `context.WithTimeout`（建议 30s，可配）；③ 未注册 action 的 1404 回包；④ 写队列满时的背压策略（丢弃 + 计数 + 告警，而不是阻塞事件广播）。这四点在 Gensokyo 里全缺（`wsclient/ws.go` grep 无 deadline/ping）。

6. **握手头按"双保险"实现，并把路径做成可组合模板。** 发 `X-Self-ID`、`X-Client-Role: Universal`、`Authorization: Token <t>`（同时接受下游回连时的 `Bearer`/裸值，参考 `server/wsserver.go:46-62`）；`ws_url` 支持 `{path}` 占位（`/ws`、`/onebot/v11/ws`、`/api`+`/event` 分离三种预设），而不是让用户手写完整 URL。

7. **不要重排消息段。** Gensokyo 的 `sortMessageSegments()` 把段强制排成 `at→text→image`（`message_parser.go:1571-1587`），丢了原始顺序。HelloYunhu 应保留上游顺序；如果云湖限制"文本和图片不能同条"，再在**发送侧**按段切分成多条消息，而不是在**接收侧**篡改语义。

8. **消息段编解码要有独立包 + 白名单 + 降级表。** 支持 `text/at/image/face/reply/forward`（云湖能对应上的），其余统一降级为 `[不支持的消息类型: xxx]` 文本或直接 `retcode:1404`。**不要**像 Gensokyo 一样让 `[CQ:face]`、`[CQ:share]` 等静默消失（`message_parser.go:703-705`），也不要像它一样在 `readme` 的「已实现 CQ 码」表里列出大量未实现的 CQ 码（`readme.md:112-144`）——文档与实现不一致比缺功能更伤。

9. **配置：30 个 key 以内，全部带默认值与校验。** Gensokyo 150 个 key / 2551 行 getter 是反面教材。HelloYunhu 用 `pydantic-settings`（或 Go `koanf`），分 3 段：`upstream`（云湖 token/webhook/ws）、`onebot`（ws 地址、token、上报格式、自实现开关）、`behavior`（ID 策略、媒体策略、被动窗口、日志级别）。只对 5 个以内的项做热重载，其余明确"需重启"并在日志里打印 `restart required: <field>`。

10. **加一个"自检 + 诊断"入口，而不是靠用户读日志。** Gensokyo 的排错靠 `PrintStructWithFieldNames` 刷屏（`Processor/Processor.go:191-213`）+ 用户翻日志。HelloYunhu 应提供：
    - 启动自检：连上游 → 拉自身信息 → 连下游 WS → 发一条 `lifecycle` + 一条 `heartbeat` → 打印 ✅/❌ 清单；
    - `GET /debug/state`：连接数、最后收发时间、ID 映射条数、被动窗口余量、最近 10 条失败 action（含 retcode 与原因）；
    - `GET /debug/idmap?real=<id>`：直接看映射关系，省掉"为什么群号变了"的排查；
    - `--selftest` 子命令：模拟一条群消息走完整链路（不发到上游）。

---

## 附录 A：关键代码位置速查

| 主题 | 文件:行 |
|---|---|
| 反向 WS 握手头 / token 解析 | `wsclient/ws.go:122-130`, `:285-310`, `:377-395` |
| 反向 WS 重连 | `wsclient/ws.go:102-195` |
| 反向 WS 心跳（含 interval 硬编码 bug） | `wsclient/ws.go:241-281` |
| 反向 WS 单写协程 / 写队列 | `wsclient/ws.go:48-82`, `:344` |
| action 入站分发（go routine） | `wsclient/ws.go:211-222` |
| action 注册表 / 未知 action 静默 | `callapi/callapi.go:200-226` |
| 复杂参数类型兼容（group_id/echo） | `callapi/callapi.go:20-180` |
| 群事件转换全流程 | `Processor/ProcessGroupMessage.go:23-361` |
| C2C 私聊转换 | `Processor/ProcessC2CMessage.go:22-180` |
| 频道→群虚拟化 | `Processor/ProcessGuildATMessage.go:60-95`, `:225-265` |
| 多路广播 / 反向 HTTP POST | `Processor/Processor.go:243-379`, `:392-455` |
| MD5 数字 ID 生成 | `idmap/service.go:219-250` |
| 正向/反向映射读写 | `idmap/service.go:279-330`, `:342-402`, `:404-439` |
| lotus 三后端分流 | `idmap/service.go:539-587`, `:933-979`, `:1305-1413` |
| 会话属性 KV | `idmap/service.go:933-1001` |
| echo↔msg_type↔msg_id | `echo/echo.go:134-257` |
| 懒 message_id 池（4 分钟/usageCount） | `echo/messageidmap.go:36-196` |
| 段数组生成 + 强制重排 | `handlers/message_parser.go:1425-1587` |
| CQ/段解析（出站） | `handlers/message_parser.go:549-910` |
| at 双向转换 | `handlers/message_parser.go:916-998`, `:1109-1138` |
| 发送回执 / retcode 恒 0 | `handlers/message_parser.go:87-264` |
| send_msg 递归猜类型 | `handlers/send_msg.go:20-185` |
| 群发消息 + 懒 ID + 被动转主动 | `handlers/send_group_msg.go:35-330`, `:2068-2101` |
| base64 图片上传两路径 | `handlers/send_group_msg.go:1215-1269` |
| 撤回 | `handlers/delete_msg.go:20-133` |
| 伪造群信息 | `handlers/get_group_info.go:82-167` |
| 伪造群成员列表 | `handlers/get_group_member_list.go:48-120` |
| 禁言（几乎空实现 + 注册名错误） | `handlers/set_group_ban.go:14-63` |
| 正向 WS 服务端（token/生命周期/无心跳） | `server/wsserver.go:44-169` |
| webhook ed25519 验签 | `server/webhook.go:59-120` |
| 正向 HTTP API（路径白名单式 if 链） | `httpapi/httpapi.go:21-94` |
| 配置加载 / 热重载 / 重启白名单 | `config/config.go:25-101` |
| 全量配置项定义 | `structs/structs.go:9-207` |
| 默认配置与注释 | `template/config_template.go:7-235` |
| 启动装配（含反向 WS 启动阻塞） | `main.go:57-108`, `:288-393`, `:482-507` |

## 附录 B：GitHub 链接

- 仓库：<https://github.com/Hoshinonyaruko/Gensokyo>
- 本报告基线提交：`ce25b675ab7cf2861773eb5cef87ad6f0f931e0b`
- readme（含夸大声明）：<https://github.com/Hoshinonyaruko/Gensokyo/blob/main/readme.md>
- 反向 WS 客户端：<https://github.com/Hoshinonyaruko/Gensokyo/blob/main/wsclient/ws.go>
- 事件转换：<https://github.com/Hoshinonyaruko/Gensokyo/blob/main/Processor/ProcessGroupMessage.go>
- ID 映射：<https://github.com/Hoshinonyaruko/Gensokyo/blob/main/idmap/service.go>
- CQ/段解析：<https://github.com/Hoshinonyaruko/Gensokyo/blob/main/handlers/message_parser.go>

## 附录 C：未验证清单

以下结论来自静态阅读，未跑实测，标注为 **未验证**：

1. 云湖的限流/主动消息额度与被动窗口语义（参考 ErisPulse 文档只提到发送类型，未给额度）——**未验证**。
2. 云湖是否支持 base64 / 流式上传（ErisPulse 有 `.Image(file: bytes, stream=True)`，暗示支持，但未见到官方文档原文）——**未验证**。
3. 云湖事件 `at` 是结构化段还是文本语法——**未验证**。
4. 云湖 ID 长度/形态（是否也是 32 字节类 openid）——**未验证**，但"字符串 ID"这点有 ErisPulse 文档佐证。
5. Gensokyo 在 `lotus` 远程 idmap 模式下的实际吞吐与延迟——**未验证**（未压测）。
6. `native_ob11` / `string_ob11` / `array` / `twoway_echo` 四个开关与具体框架版本的对应矩阵——**未验证**（按 readme 与代码注释归纳，未逐框架实测）。
7. open issues 的具体条目内容——未能取得（GitHub API 速率限制 403，issue 页面为 JS 渲染未取到正文），故 §6.5 的"高频问题"是从代码与注释反推的**未验证**推断。
