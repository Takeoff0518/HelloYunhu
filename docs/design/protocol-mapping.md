# 协议映射说明

本文档记录 HelloYunhu 在两侧协议之间实际使用的映射关系，与代码保持同步。调研依据在 [可行性调研](../research/feasibility-report.md) 中，这里只写结论与尚未确认的部分。

## 1. 事件映射

| 云湖事件 | OneBot 事件 | 状态 |
|---|---|---|
| `message.receive.normal`（群聊） | `message` / `group` / `normal` | 已实现 |
| `message.receive.normal`（私聊，`chatType` 为 `bot`） | `message` / `private` / `friend` | 已实现 |
| `message.receive.instruction` | 与普通消息相同；指令名拼进正文，扩展字段保留 `instruction` | 已实现 |
| `group.join` | `notice` / `group_increase` / `approve`；`operator_id` 为 0 | 已实现 |
| `group.leave` | `notice` / `group_decrease` / `leave`；平台不区分主动退出与被踢 | 已实现 |
| `bot.followed` | `notice` / `friend_add` | 已实现，字段名待真实事件确认 |
| `bot.unfollowed` | `notice` / `yunhu_friend_remove`（非标准） | 已实现，字段名待真实事件确认 |
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
| `send_group_msg` | `POST /bot/send`，`recvType=group` | 已实现，支持文本、@、引用、图片、文件与视频 |
| `send_private_msg` | `POST /bot/send`，`recvType=user` | 已实现 |
| `send_msg` | 按 `message_type` 决定接收方类型 | 已实现 |
| `send_group_forward_msg`、`send_private_forward_msg`、`send_forward_msg` | 展开成文本后走 `POST /bot/send` | 已实现，降级为文本 |
| `get_login_info` | 启动时查询的 `/bot/info` 结果 | 已实现 |
| `get_status` | 本地状态，固定在线 | 已实现 |
| `get_version_info` | 常量，`app_name` 为 HelloYunhu | 已实现 |
| `get_group_info` | `GET /group/info` | 已实现 |
| `get_group_list` | `GET /bot/groups` 分页聚合 | 已实现 |
| `get_group_member_info` | `GET /group/members` 扫描，失败返回空数据 | 已实现，受权限限制 |
| `get_group_member_list` | `GET /group/members` 分页聚合，失败返回空列表 | 已实现，受权限限制 |
| `get_stranger_info` | 云湖无对应接口，返回空昵称 | 已实现 |
| `get_msg` | 从最近消息缓存回放，未命中返回 1404 | 已实现 |
| `.handle_quick_operation` | 直接返回成功 | 已实现 |
| `delete_msg` | `POST /bot/recall` | 已实现，依赖进程内的会话归属 |
| `set_group_ban` | `POST /group/gag-member` | 已实现，需要禁言权限 |
| `set_group_kick` | `POST /group/remove-member` | 已实现，需要移除成员权限 |
| `get_image` | 下载到本地 | 未实现 |
| 其它 | 返回 `retcode=1404` 与说明 | 默认行为 |

`get_group_member_info` 在权限不足时返回空对象而不是错误，这是为了让框架保留 @ 消息段：框架只有在调用成功后才把 @ 段放进消息，而群里 @ 机器人是唤醒的必要条件。

`delete_msg` 只拿到消息标识，云湖还需要会话标识与类型。两者随标识映射一起写进 `message_id` 的 `chat_id` 与 `chat_type` 列，进程重启后仍能撤回旧消息。撤回接口的 `chatType` 取 `user` 或 `group`，事件里的私聊类型是 `bot`，调用前统一换算。

框架之间对标识的序列化习惯不同：AstrBot 用数字，NoneBot2 的引用段把 `id` 序列化为字符串。接口实现同时接受数字与字符串，避免因为一个字段的写法差异让整条调用返回 100。

## 3. 消息段映射

接收方向：

| 云湖内容类型 | OneBot 消息段 | 说明 |
|---|---|---|
| `text` | `at` 段与 `text` 段 | 正文里的「@昵称」剪掉后换成同一位置的 `at` 段，剩下的正文才能被框架当成命令；两边数量对不上时保留原文，@ 段放在正文之前 |
| `image` | `image` 段 | 先下载再以 `base64://` 给出，失败时给出网络地址 |
| `file` | `file` 段 | 带文件名与体积，内容先下载再以 `base64://` 给出 |
| `video` | `video` 段 | 同上，10 MB 级别的视频也能内联 |
| `audio` | `record` 段 | 同上，云湖没有语音发送能力 |
| 其它 | 文本占位 | `[暂不支持的消息类型: x]` |

媒体地址分三类，事件里的字段名与拼接规则不同：

| 内容类型 | 事件字段 | 地址来源 |
|---|---|---|
| `image` | `imageUrl` | 已是完整地址 |
| `file` | `fileUrl` | 相对路径，拼 `https://chat-file.jwznb.com/` |
| `video` | `videoUrl` | 相对路径，拼 `https://chat-video1.jwznb.com/` |
| `audio` | `audioUrl` | 相对路径，拼 `https://chat-audio1.jwznb.com/` |

三类媒体都受 CDN 来源限制，缺少 `Referer: https://myapp.jwznb.com` 时返回 403。框架侧通常不带这个头，因此转换程序先在本进程下载，再以 `base64://` 交给框架；下载失败时退回地址，并在日志里留下记录。

发送方向：

| OneBot 消息段 | 云湖处理 | 说明 |
|---|---|---|
| `text` | 文本消息 | 连续的文本与 @ 合并为一条 |
| `at` | `content.at` 加正文中的 `@昵称` | 只给标识时客户端把提及显示成数字，正文必须写昵称；昵称来自消息事件的缓存，取不到时退回标识 |
| `reply` | `parentId` | 反查映射表，查不到时忽略引用 |
| `image` | 上传后作为图片消息发送 | 支持 base64、网络地址与本地路径 |
| `file` | 上传后作为文件消息发送 | 带文件名与体积 |
| `video` | 上传后作为视频消息发送 | |
| `record` | 上传后按文件消息发送 | 云湖没有语音发送能力，用户点开文件收听 |
| `face`、`json`、`node` 等 | 文本占位 | 保证不丢消息 |

一条 OneBot 消息可能被拆成多条云湖消息：连续的文本与 @ 合并为一条，每个媒体段单独一条。引用的目标只挂在第一条消息上。

发送接口的 `message` 参数同时接受消息段数组与 CQ 码字符串，字符串按 CQ 码解析后再走同一套拆分逻辑。事件方向固定使用消息段数组，框架侧不需要额外配置。

部分框架会把单个消息段直接当作 `message` 传入，这种形态按一段处理。媒体的本地路径可能是 `file:///E:/…` 这种三斜杠写法，读取前会按盘符去掉多余的前导斜杠。

上传前会按内容探测结果补齐文件名后缀，并去掉名字里的目录部分。云湖按文件名后缀判断上传类型：图片接口只接受图片后缀，视频接口只接受 mp4，后缀与接口不匹配时返回 `code -1`。OneBot 侧给出的媒体经常没有可用后缀，`base64://` 形式的内容在读取环节统一叫 `upload.bin`，直接上传会被拒绝。实测数据见第 5 节。

头像不走 OneBot 的协议字段，事件与查询结果里都没有它的位置。客户端通常把整数标识当作 QQ 号去 qlogo 取图，换成非 QQ 平台就会显示成陌生人的头像。Overflow 只在协议端自报 `gensokyo` 时调用非标准的 `get_avatar`，因此兼容模式下程序自报该名字，并提供 `/avatar/<标识>` 的代理，代取时附带来源头。

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
| `GET /bot/groups` | `{"list":[{"groupId":"…","name":"…","avatarUrl":"…"}]}` | 列表包在 `list` 里；群名是 `name` 不是 `groupName`；不返回成员数量 |
| `GET /group/info` | `{"groupId":"…","name":"…","headcount":31,"introduction":"…","avatarUrl":"…"}` | 成员数量是 `headcount`，没有成员上限字段 |
| `GET /group/members`，未授权 | `{"code":1003,"msg":"机器人无获取群成员列表权限，请联系群主或管理员授权"}` | 成员接口默认关闭，需要在机器人后台授权；未授权时返回空列表而不是报错 |
| `POST /image/upload` | `{"imageKey":"FplutsXGo96FnEEdy0pIxjIu7_Ko"}` | 图片上传返回 `imageKey` |
| `POST /file/upload` | `{"fileKey":"FvMpoya0gU76yX9mQY8_l7f6k2qs"}` | 文件上传返回 `fileKey` |
| `POST /video/upload` | `{"videoKey":"Fg61uTCfVc3ldhMtd3Iy-nYwUaAj"}` | 视频上传返回 `videoKey` |
| `POST /image/upload` 的文件名 | 同一张 PNG 用 `upload.png` 上传成功，用 `upload.bin` 或 `a.txt` 上传返回 `{"code":-1,"msg":"文件类型必须为图片"}` | 云湖按文件名后缀判断上传类型，图片内容配错后缀同样被拒 |
| `POST /image/upload` 的内容 | 名字为 `upload.png` 而内容是纯文本时返回 `文件类型必须为图片`；JPEG 内容配 `.png` 名字时上传成功 | 内容也必须是图片，但不要求后缀与内容格式一致 |
| `POST /image/upload` 的表单形态 | 字段名改成 `file`、省略文件名、或直接把 PNG 作为请求体发送，都返回 `{"code":-1,"msg":"图片获取失败"}` | 只接受 multipart 表单，字段名为 `image`，且必须带文件名 |
| `POST /image/upload` 的去重 | 同一张图换不同文件名重复上传，返回的 `imageKey` 完全相同 | `imageKey` 由内容决定，与文件名无关 |
| `POST /video/upload` 的文件名 | 真 MP4 配 `.mp4` 上传成功；同一内容配 `.png` 返回 `文件类型必须为mp4格式视频`；伪 MP4 内容配 `.mp4` 返回 `文件类型必须为图片` | 视频接口同样按后缀判断，平台的错误文案存在错位 |
| `POST /file/upload` 的文件名 | `.bin` 与 `.txt` 都能上传成功 | 文件接口不校验后缀 |
| 发送图文混排 | 一条三段的消息被拆成三条 `POST /bot/send`，依次为文本、图片、文本 | 拆分与顺序符合设计，云湖侧显示为三条消息 |
| 发送私聊 | `recvType` 为 `user`，`recvId` 为用户标识，返回 `messageInfo` | 私聊发送可用 |
| 重启后引用 | 进程重启后仍能把整数标识 `31` 反查为 `parentId=c941fdc2…` | 标识映射落盘有效，引用不因重启失效 |
| @ 的三种正文写法 | 正文写 `@昵称` 加空格时显示为正常提及；只给 `at` 数组不写正文时客户端不显示提及；昵称后只加零宽空格时昵称与后文粘在一起 | 正文必须自己写 `@昵称` 并留一个空格，`content.at` 负责携带标识 |
| 群里 @机器人 发 `/diag` | 事件正文是「@乐正绫TwinkLing 零宽空格 `/diag`」，`content.at` 为 `["11464576"]`；剪掉提及文字后框架收到 `at` 段与 `/diag` 文本，命令正常命中 | 提及文字与 `at` 数组一一对应，顺序一致 |
| 框架侧的提及处理 | NoneBot2 会把消息开头指向自己的 `at` 段落消费掉用于判断「被 @」，之后消息里只剩正文 | 诊断命令看到的 `segments` 为 `text` 属于正常现象，不代表 `at` 段丢失 |
| 群聊文件消息 | `content` 含 `fileName`、`fileUrl`、`etag`、`fileSize`，`fileUrl` 是 `c839….txt` 这样的相对路径 | 需要拼 `https://chat-file.jwznb.com/`，该地址带来源头返回 200，不带返回 403 |
| 群聊视频消息 | `content` 含 `etag`、`videoUrl`、`videoDuration`，`videoUrl` 是 `845f….mp4` | 拼 `https://chat-video1.jwznb.com/`，10.6 MB 的视频带来源头返回 200，不带返回 403，支持 `Range` 分段 |
| 群聊语音消息 | `content` 含 `etag`、`audioUrl`、`audioDuration`，`audioUrl` 是 `1ef1….wav` | 拼 `https://chat-audio1.jwznb.com/`，带来源头返回 200，类型 `audio/x-wav` |
| `POST /bot/recall` | `{"msgId":"…","chatId":"…","chatType":"group"}` 返回 `{"code":1,"msg":"success"}`，机器人自己的消息与他人消息都能撤回 | 撤回接口可用，`chatType` 取 `user` 或 `group` |
| 撤回后的订阅事件 | 撤回成功后订阅通道没有任何事件，日志里既无消息事件也无未知类型事件 | 云湖不推送撤回通知，只能做主动撤回，无法给框架发 `notice.message_recall` |
| `POST /group/gag-member` | `gag` 只接受 600、3600、21600、43200，另有 0 解禁与 -1 永久；60、300、599、601、900、1800、7200、86400 返回 `1002 禁言时长错误` | 档位固定，转换层按不小于请求值的最近档位换算，超过 12 小时按 12 小时 |
| `POST /group/remove-member` | 传 `groupId` 与 `userId` 返回 `{"code":1,"msg":"success"}`，成员被移出群 | 移除成员可用；机器人账号被移出时平台无额外提示 |
| `GET /bot/messages` | 参数 `chat-id` 与 `chat-type=group` 时返回 `{"list":[],"total":0}`，`chat-type=bot` 被拒绝 | 接口存在但当前账号取不到历史，参数语义待确认，暂不使用 |
| 客户端取头像的行为 | Overflow 的 `MemberWrapper.kt`、`FriendWrapper.kt`、`StrangerWrapper.kt` 与 `BotWrapper.kt` 里都是 `if (appName != "gensokyo") null else extGetAvatar(…)`，回退时用 mirai 默认的 `https://q1.qlogo.cn/g?b=qq&nk=<整数>&s=<尺寸>` | 头像由客户端自己拼；只有按名字分派的客户端才会向协议端索取地址 |
| `get_avatar` 的约定 | 参数 `user_id` 必填，取群成员时附带 `group_id`，响应的 `data` 是地址字符串而不是对象 | Gensokyo 的扩展接口，Overflow 用它取群成员、好友、陌生人与机器人自身的头像 |
| 云湖头像地址 | 形如 `https://chat-img.jwznb.com/8448fe….jpg`，带 `Referer: https://myapp.jwznb.com` 才能取到，缺少时返回 403；301 跳到 `chat-img3.jwznb.com` 后是 `.webp` 文件，取回类型为 `image/webp` | 与媒体 CDN 同一条来源规则，客户端直接下载会失败，因此需要由本程序代理；地址后缀与真实格式无关，代理解码后转成 JPEG 或 PNG |
| webp 头像在客户端的表现 | PatPat 插件读取头像后抛 `java.lang.NullPointerException: Cannot invoke "java.awt.image.BufferedImage.getWidth()" because "avatarImage" is null` | Java 的 `ImageIO` 不支持 webp，读到 null；这也是代理必须转码的原因 |
| `POST /bot/send`，图片内容带 `parentId` | 请求成功并返回 `messageInfo.msgId`，撤回同样成功 | `parentId` 与内容类型无关，引用可以挂在图片消息上，不必退化成一条文本 |

代码同时把 `code` 为 0 与 1 视为成功，避免不同接口编码不一致时误判。`data` 结构已确认的接口有 `/bot/info`、`/bot/send`、`/bot/groups`、`/group/info`、`/group/members` 与三个上传接口；成员字段为 `userId`、`nickname`、`groupNickname` 与 `userLevel`。

媒体下载另有实测结论：

| 场景 | 观测 | 结论 |
|---|---|---|
| 下载 `chat-img.jwznb.com` 的图片 | 不带来源头时返回 403 | CDN 校验来源 |
| 附带 `Referer: https://myapp.jwznb.com` 后下载 | 可正常获取 | 社区适配器使用同一来源值，代码对 `*.jwznb.com` 与 `*.jwzhd.com` 自动附带 |
| 文件、视频、语音三类 CDN | 都在 `*.jwznb.com` 下，缺少来源头统一返回 403 | 三条来源规则一致，代码统一附带来源头后下载 |

## 6. 尚未确认的行为

下面几项依赖真实云湖环境继续验证，代码对多种写法都做了兼容，实测后需要回来收敛：

1. `/bot/messages` 的参数语义与返回内容，当前账号取不到数据。
2. 禁言的长期效果：600 秒档位实测生效，解禁（`gag` 为 0）与永久（`-1`）的客户端表现尚未逐一确认。
3. 机器人账号被移出群时平台的具体行为，目前只观察到接口返回成功。
