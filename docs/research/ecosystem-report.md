# 云湖（Yunhu / yhchat.com）机器人生态调研报告
### —— 面向「云湖 ↔ OneBot v11 转译层」的竞品与基建情报

> 调研时间：**2026-11-14**（Asia/Shanghai）
> 方法：官方文档原文抓取（yhchat.com/document-v2）+ GitHub REST API（star/推送时间/README 原文）+ npm registry API + PyPI JSON/pypistats + Gitee/Codeberg/Maven Central。
> 标注约定：**【已核实】** = 有一手链接可点开的原文/接口数据；**【推测】** = 基于间接证据的推断，落地前需实测。
> 数据快照说明：star 数、下载量为 2026-11-14 单点采样，会漂移。

---

## 0. 结论先行（TL;DR）

1. **官方 SDK 只有 Python 和 Go 两个**（Java 明写「开发中」，官方文档让用户去用第三方 Daen 的 Java SDK）。两个官方 SDK 功能面都很窄：只有「事件订阅 + 发消息 + 批量发消息」，**没有任何签名/验签、没有媒体上传、没有群管理**。PyPI 上官方包 `yunhu` 月下载量仅 **49 次**，基本没人用。
2. **社区生态的中心不在官方 SDK，而在四个框架适配器**：Koishi（`adapter-yunhupro`，482 下载/月）、AstrBot（`astrbot_plugin_yunhuAdapter`）、NoneBot2（`nonebot-adapter-yunhu`，222 下载/月）、ErisPulse（`ErisPulse-YunhuAdapter`，740 下载/月）。这四家已经把「云湖事件 → 各自框架事件」跑通了，**但没有一家对外提供 OneBot v11 接口**。
3. **「云湖 ↔ OneBot v11 转译层」目前是空白市场。** 全网只找到 3 个「接近但不等于」的东西：
   - `wsu2059q/YunhuEventToOnebot12`：只做**事件 JSON 格式转换**（云湖 → OneBot**12**），不含任何网络通信，1 star，无 license；
   - `siiway/NextBridge`：**消息桥**（云湖 driver + QQ driver，QQ 侧支持 NapCat/Lagrange/OneBot v11），做的是「按规则转发消息」，**不是 OneBot 协议服务端**；
   - `ErisPulse-YunhuAdapter`：内部把云湖事件映射成 OneBot**12** 语义，是框架私有实现，不暴露 OneBot 端口。
4. **平台侧最关键的一条事实**：除官方文档写的 HTTP Webhook 之外，云湖还存在一个**未文档化但在生产中被广泛使用的 WebSocket 事件网关 `wss://ws.jwzhd.com/subscribe`**（Koishi 适配器默认走它、AstrBot 插件保留它）。它「无需公网 IP」，这对我们做 OneBot 转译层是决定性的架构利好——**应当 WebSocket 优先、Webhook 兜底**。（该端点**未出现在官方 v2 文档中**，属社区事实标准，稳定性无承诺。）
5. **回调没有验签机制**。云湖 Webhook 仅靠「URL 保密」做安全，社区文档原话：「云湖的 Webhook 推送没有身份验证机制」。这对 OneBot 转译层是**必须自己补的安全缺口**（事件伪造 = 任意消息注入）。

---

## 1. 官方 SDK（文档「官方 SDK」章节）

文档原文入口：<https://www.yhchat.com/document-v2/official-sdks/python-sdk>、<https://www.yhchat.com/document-v2/official-sdks/golang-sdk>、<https://www.yhchat.com/document-v2/official-sdks/java-sdk>

| 语言 | 仓库 | Star | 最近提交 | 维护状态 | 能力覆盖 | 备注 |
|---|---|---|---|---|---|---|
| Python | [yhchat/bot-python-sdk](https://github.com/yhchat/bot-python-sdk) | 16 | 2026-05-18 | **活跃（低频）** | 事件订阅、发消息、批量发消息；**无验签、无媒体上传、无群管理** | MIT；PyPI 包名 `yunhu`，最新 **0.0.12**；月下载 **49** 次 |
| Go | [yhchat/bot-go-sdk](https://github.com/yhchat/bot-go-sdk) | 10 | 2026-06-23 | **活跃（低频）** | 同上（事件订阅 + 发送 + 批量）；含 `basic` / `custom_router` 两个示例 | 文档写 MIT，但 GitHub API 未识别到 LICENSE 文件（**待核实**）；示例默认端口 8805，回调路径 `/sub` |
| Java | —（无官方仓库） | — | — | **未发布** | 文档明写「官方版本开发中，推荐使用第三方 Java SDK」 | 官方把用户导流到 DaenMax 的项目 |

**官方 SDK 的提交内容【已核实】**（GitHub commits API）：
- Python：`2026-05-18 增加机器人设置订阅、增加A2UI消息按钮汇报事件`；`2026-05-10 0.0.12`；`2026-05-10 增加流式消息接口`；上一条是 `2025-02-28 v0.0.11`。
- Go：`2026-06-23 add a2ui.button.report event`；`2025-06-11 add bot.setting event`；`2025-05-22 Add the function of sending streaming messages`。

> **判断【已核实】**：官方 SDK 是「跟得上平台新增事件、但功能面极窄」的维护形态——近两年主要动作是**补事件类型**，没有补工程能力（重试、限流、验签、媒体、群管）。它们适合当**协议参考实现**，不适合当我们转译层的底座（缺少我们要用的大量 API）。
> 另一个证据：官方 Java SDK 页面直接把用户推给社区项目，说明官方对「多语言 SDK」投入有限。

---

## 2. 非官方 SDK / 框架 / 适配器

### 2.1 总表

| # | 项目 | 语言 | 定位 | Star | 最近活动 | License | 月下载 | 结论 |
|---|---|---|---|---|---|---|---|---|
| 1 | [DaenMax/yhchat-sdk-core](https://github.com/daenmax/yhchat-sdk-core)（主库在 [Gitee](https://gitee.com/daenmax/yhchat-sdk-core)） | Java | SDK（Spring Boot Starter 风格） | 3（GitHub 镜像） | **2026-08-17 v1.4.0** | MIT | Maven Central 仅 1.2.0 | **生态最成熟的 SDK**，官方点名推荐 |
| 2 | [DaenMax/yhchat-sdk-demo](https://github.com/daenmax/yhchat-sdk-demo) | Java | 示例工程 | 5 | 2026-08-17 | — | — | 配套 Demo（SpringBoot3 + JDK21） |
| 3 | [ErisPulse/ErisPulse-YunhuAdapter](https://github.com/ErisPulse/ErisPulse-YunhuAdapter) | Python | 框架适配器（ErisPulse 生态） | 2 | 2026-11-10 | MIT | **740** | PyPI 版本已到 **4.6.3**，WS/Webhook 双模、多 Bot |
| 4 | [ErisPulse/ErisPulse](https://github.com/ErisPulse/ErisPulse) | Python | 多平台机器人框架 | **58** | 2026-11-14 | MIT | — | 15+ 平台，云湖是其一；OneBot12 语义 |
| 5 | [FLY1919/adapter-yunhupro](https://github.com/FLY1919/adapter-yunhupro) | TypeScript | Koishi 适配器 | 9 | **2026-10-09 v2.3.3** | Apache-2.0 | **482** | **工程质量最高**，110 个版本，文档站完整 |
| 6 | [WindyPear-Team/koishi-plugin-adapter-yunhu](https://github.com/WindyPear-Team/koishi-plugin-adapter-yunhu) | TypeScript | Koishi 适配器（原版） | 少量 | npm 1.2.0 / **2025-05-31** | — | 78 | **基本停更**，已被 #5 取代 |
| 7 | [molanp/nonebot-adapter-yunhu](https://github.com/molanp/nonebot-adapter-yunhu) | Python | NoneBot2 适配器 | 4 | 2026-01-24 | MIT | 222 | 维护中，功能中等，仅 Webhook |
| 8 | [Soffd/astrbot_plugin_yunhuAdapter](https://github.com/Soffd/astrbot_plugin_yunhuAdapter) | Python | AstrBot 插件 | 4 | **2026-10-17** | MIT | — | **功能覆盖最全**（含群管/看板/文件库/21 个 Agent Tool） |
| 9 | siiway NextBridge `yunhu` driver（[文档](https://docs.siiway.org/zh-CN/nextbridge/v0.3/drivers/yunhu)，[仓库](https://github.com/siiway/NextBridge)） | Python（Gitea 统计 96.2%） | **消息桥**（非 SDK、非 OneBot） | 50（NextBridge） | 2026-11-07 | GPL-3.0 | — | 云湖 ⇄ QQ/Discord/TG… 按规则转发 |
| 10 | [TASA-Ed/nanoyunhu](https://github.com/TASA-Ed/nanoyunhu) | TypeScript | 「无头云湖」（用户账号协议端） | 8 | 2026-10-21 | **AGPL-3.0** | npm `@tasaed/nanoyunhu` | 客户端协议，不是 bot 开放 API |
| 11 | [wsu2059q/YunhuEventToOnebot12](https://github.com/wsu2059q/YunhuEventToOnebot12) | Python | 事件格式转换（→ OneBot12） | 1 | 2026-02-23 | **无** | PyPI `yunhu2onebot12` | **无网络层**，纯 JSON 映射 |
| 12 | [jibukeshi/yunhu_bot_php](https://github.com/jibukeshi/yunhu_bot_php) | PHP | SDK | 11 | **2024-10-19** | — | — | **停更**（2024） |
| 13 | [S-i-l-v-e-t/YHChatPythonSDK](https://github.com/S-i-l-v-e-t/YHChatPythonSDK) | Python | SDK | 9 | **2024-11-29** | — | — | **停更** |
| 14 | [Tribbtsz/nullclaw-yunhu](https://github.com/Tribbtsz/nullclaw-yunhu) | Go | 渠道插件（Nullclaw） | 3 | 2026-06-09 | MIT | — | 小众，可参考 Go 侧实现 |
| 15 | [shiminli748-netizen/yunhu-local-server](https://github.com/shiminli748-netizen/yunhu-local-server) | Python | **本地模拟服务器** | 2 | 2026-06-16 | — | — | 无真机时可做联调替身（质量未知） |
| 16 | [yh-Tpdev/yhchatAPI](https://github.com/yh-Tpdev/yhchatAPI) | TS/Docs | 反向工程 API 文档库 | **31** | 2026-11-07 | — | — | **最有价值的第三方文档**，含 protobuf/WS |

### 2.2 逐个点评（只保留对我们有决策价值的）

#### ① DaenMax / yhchat-sdk-core（Java）——「中国的非官方标准 SDK」
- **能力【已核实】**：事件侧覆盖普通消息、指令消息、关注/取关、按钮点击、进退群、机器人设置（**未列 shortcut.menu / a2ui**）；接口侧发送、批量、流式、编辑、撤回、消息列表、图/视频/文件上传、看板设置与取消。**不覆盖**：群禁言、移除成员、群标签、群成员列表（新版 API）。
- **活跃度【已核实】**：GitHub 镜像 `v1.4.0 @ 2026-08-17`，Gitee 为主库；Maven Central `cn.daenx:yhchat-sdk-core` **最新只到 1.2.0（2025-05-25）**。
- **优点**：事件插件化（实现接口即可，同事件可多次实现）、Spring Boot 开箱即用、文档/交流群齐全、官方文档背书。
- **缺点**：**Maven Central 版本落后于仓库 1.2.0 vs 1.4.0**，用它得自己 install 或加 Gitee 源；强绑定 Spring 生态；覆盖面落后于 2026 新增的群管 API。
- **对我们的价值**：**参考价值 > 依赖价值**。它的「事件分发器 + 接口封装分层」是很好的结构范本；作为 Java 依赖则不如自己直连 HTTP（我们本来就要做协议转译，不需要 Spring 容器）。

#### ② ErisPulse-YunhuAdapter（Python，PyPI 4.6.3）
- **定位**：ErisPulse 框架的云湖适配器，不是独立 SDK。
- **能力【已核实】**：WebSocket / Webhook **双模**、多 Bot；事件→**OneBot12** 语义映射（message / notice.friend_increase / notice.group_member_increase / notice.yunhu_button_click / notice.yunhu_a2ui_button / notice.yunhu_shortcut_menu / notice.yunhu_bot_setting）；发送侧链式 API（`Send.To(type,id).Text/Image/Video/File`），视频/文件支持流式上传。
- **活跃度**：PyPI **740 下载/月**（四个适配器里最高），GitHub 2 star，主仓在 Codeberg。
- **优点**：**唯一明确按 OneBot12 语义建模的云湖适配器**——事件字段命名、notice 分类可直接抄；双模连接、流式上传实现成熟。
- **缺点**：框架强绑定（ErisPulse 私有事件总线）；OneBot12 ≠ OneBot11，**不能直接当 v11 用**；2 star，bus factor 极低。
- **对我们的价值**：**协议映射的最佳参考物**（尤其 notice 分类与 detail_type 设计），不是依赖。

#### ③ koishi-plugin-adapter-yunhupro（FLY1919，TypeScript）
- **能力**：WS（默认）/Webhook 双模；多开插件多实例；按钮、A2UI、流式消息（实验性）；文件上传超时/重试配置；错误码文档；完整的「云湖官方文档镜像」。
- **活跃度【已核实】**：npm `2.3.3 @ 2026-10-09`，**110 个已发布版本**，**482 下载/月**，Apache-2.0，文档站 <https://fly1919.github.io/adapter-yunhupro/>。
- **优点**：**工程完成度、文档质量、迭代密度三项全场第一**；对平台细节（限流、错误码、Webhook 无鉴权、多实例路径冲突）的记录最诚实、最细。
- **缺点**：Koishi 运行时绑定（cordis 依赖注入），核心逻辑与 Koishi 的 `Adapter/Bot/Session` 抽象耦合，**抽取成本高**；TypeScript。
- **对我们的价值**：**事实标准文档源**（比官方文档更细），实现细节可对照；不适合直接依赖。

#### ④ WindyPear-Team/koishi-plugin-adapter-yunhu
- npm `1.2.0 @ 2025-05-31`，78 下载/月。FLY1919 的 pro 版就是从它 fork 的（README 明确写「基于 @WindyPear 的 adapter-yunhu 二次开发」）。
- **结论**：**已实质停更**，只作历史参考。不要选它。

#### ⑤ nonebot-adapter-yunhu（molanp）
- **能力【已核实】**：事件（基础消息、按钮上报、表单接收）；消息段（Text/Image/At/Buttons/Video/File/HTML/Markdown/Reply ✅；Form/expression/Audio/Post/Tip ⬇️ 未完成）；操作（Send/Recall/Edit ✅，Reaction 🚫）。**仅 Webhook**（回调路径 `/yunhu/{app_id}`）。
- **活跃度**：2026-01-24 最后推送，222 下载/月，MIT。
- **优点**：**「OneBot 风格消息段」建模思路的直接范本**——它已经在做「云湖结构化 content → 消息段数组」的转换，这正是我们要做的事，只是目标是 NoneBot 的 `MessageSegment` 而非 CQ 码。
- **缺点**：Webhook only（要公网）；未实现事件全量；单人维护。
- **对我们的价值**：**消息段 ↔ 云湖 content 的映射表可以直接对照/借鉴**（含 HTML、Markdown、Reply→parentId 的处理）。

#### ⑥ astrbot_plugin_yunhuAdapter（Soffd）
- **能力【已核实，来自 README】**：文本/Markdown/HTML 收发；图/文件/视频上传发送；语音降级为文件；@成员、@全员（可见文本）；引用（Reply ↔ `parentId`）；**流式回复（chunked）**；按钮三态；互动事件（按钮/快捷菜单/A2UI 表单）；系统事件；主动发送；编辑/撤回/批量/消息列表；**看板**；**群管**（禁言、解禁、移除、消息类型限制、群标签）；文件保留库（7 天/512MiB/1000 个）；`/云湖` 指令 + 21 个 Agent Tools。
- **平台细节【已核实】**：明确写「**每个接口每秒 10 次并发限速，失败不自动重发，以避免产生重复消息**」；图片 10MB、文件/视频 20MB 上限；Webhook 可选 `?secret=` 或 `X-Yunhu-Secret` 头（**这是插件自己加的自定义校验，不是云湖官方机制**）；WS 默认 `wss://ws.jwzhd.com/subscribe`，并注明「云湖 v2 文档明确说明 HTTP POST，建议 Webhook；WebSocket 模式沿用旧版实现，实际可用性需自行确认」。
- **活跃度**：2026-10-17，4 star，MIT。
- **对我们的价值**：**平台事实与边界条件的金矿**（限速、大小限制、语音无 audio 类型、@全员无原生参数、Webhook 无鉴权）。它把「云湖能力 → 上层框架能力」的**降级策略**写全了，我们做 OneBot 映射时可逐条对照。

#### ⑦ SiiWay NextBridge `yunhu` driver
- **是什么【已核实】**：NextBridge 是「聊天平台互转桥」，driver 把各平台消息规范化为 `NormalizedMessage` 再按 `rules.json` 转发。yunhu driver 用 **Webhook HTTP 服务端接收 + 开放 API 发送**，配置项 `token` / `webhook_port`（默认 8765）/ `webhook_path`（默认 `/yunhu-webhook`）/ `proxy_host` / `proxy`。
- **平台支持表【已核实】**：云湖「接收 ✅ / 发送 ✅ / 提及 ❌ / 回复 ⚠️ / 媒体 ✅」，备注「**暂未支持原生提及语法**」；QQ 一行明确「支持多种协议：**NapCat、Lagrange、OneBot v11**」。
- **文档时间**：yunhu 页 `最后更新 2026/10/18 · 作者 wyf9`；`zh/v0.4` 下已有 `qq.md` 与 `yunhu.md`。
- **对我们的价值 / 威胁**：**这是目前唯一「云湖 ⇄ OneBot v11」的可用路径，但它是消息桥不是协议桥**——只能按规则转发文本/媒体，不提供 OneBot action API、不提供群管/事件全量、不能让既有 OneBot 插件「无感迁移」。**这恰好是我们差异化的边界**。

#### ⑧ 其他（一句话）
- `nanoyunhu`（AGPL-3.0）：用户账号协议无头端，**AGPL 传染性 + 违反平台 ToS 风险**，不碰。
- `yunhu-local-server`：本地 mock，可作 CI 联调替身（未验证质量）。
- `yhchatAPI`（31 star）：反向工程文档，**含 `wss://chat-ws-go.jwzhd.com/ws` 的 protobuf 客户端协议**——注意这是「登录用户账号」的协议（需要 userId + token + platform/deviceId），**与 bot 开放 API 是两套东西**，不要混淆。

---

## 3. 是否已有「云湖 ↔ OneBot」转译项目？（关键问题）

### 3.1 检索动作与结果【已核实】

| 检索 | 结果 |
|---|---|
| GitHub `yunhu onebot` / `onebot yunhu` | 仅命中 `wsu2059q/YunhuEventToOnebot12`（1 star） |
| GitHub `yhchat onebot` / `yunhu cqhttp` / `yunhu napcat` | **0 结果** |
| GitHub `云湖 onebot` | 仅命中 ErisPulse（框架描述里含「云湖」+ 平台列表） |
| npm 全站搜 `yunhu` | 全部是 Koishi 适配器/工具包/客户端，**无 OneBot 相关** |
| PyPI `yunhu*` | `yunhu`（官方）、`ErisPulse-YunhuAdapter`、`nonebot-adapter-yunhu`、`yunhu2onebot12`（OneBot**12**，无网络层） |
| AstrBot 官方文档「接入消息平台」侧边栏 | **无云湖**（18+ 平台：QQ 官方/OneBot v11/企微/公众号/个人微信/飞书/钉钉/TG/LINE/Slack/Mattermost/Misskey/Discord/KOOK/Satori/Matrix/VoceChat）→ 云湖只能靠第三方插件 |

### 3.2 三个「近似物」的完成度评估

| 项目 | 协议 | 方向 | 传输 | 覆盖度 | OneBot action API | 反向 WS | License | 能否直接用 |
|---|---|---|---|---|---|---|---|---|
| `wsu2059q/YunhuEventToOnebot12` | OneBot **12** | 云湖 → OB12 | **无网络层**（纯函数） | 8 类事件；text/image/video/file/form | ❌ | ❌ | **无 LICENSE**（法律上不可复用） | ❌ 只能当字段映射参考 |
| `ErisPulse-YunhuAdapter` | OneBot **12 语义**（框架内部） | 双向 | WS + Webhook | 10 类事件；发送/上传/群管/看板 | ❌（框架私有 API） | ❌ | MIT | ❌ 绑定框架 |
| `siiway/NextBridge` | 自有 `NormalizedMessage` | 双向 | Webhook + 开放 API | 文本/媒体/@ 不支持/回复部分 | ❌（规则转发） | ❌ | 见仓库 | ⚠️ 只能做「消息互通」 |
| `nonebot-adapter-yunhu` | NoneBot 事件 | 双向 | Webhook | 消息段较全 | ❌ | ❌ | MIT | ❌ 绑定 NoneBot |

### 3.3 结论

**【已核实】截至 2026-11-14，不存在任何「云湖 ↔ OneBot v11（CQHTTP）」的协议转译实现**：没有 CQ 码互转、没有 OneBot action 端点（`/send_msg`、`/get_group_list`…）、没有反向 WS、没有正向 WS 客户端。

**我们的差异化空间（按价值排序）**：
1. **协议而非桥**：对外暴露标准 OneBot v11 **反向 WS**（NapCat/go-cqhttp 同构），让现有 OneBot 生态（NoneBot2、Koishi 的 onebot 适配器、AstrBot 的 aiocqhttp、各种 CQ 码插件）**零改造**接入云湖——这是 NextBridge 做不到的。
2. **CQ 码 ↔ 云湖结构化 content 双向编解码**（含 `[CQ:at,qq=]`、`[CQ:image,file=]`、`[CQ:reply,id=]`、`[CQ:markdown]`/HTML 的降级策略）。目前全网只有「消息段对象」实现，没有 CQ 码实现。
3. **无需公网 IP**：默认走 `wss://ws.jwzhd.com/subscribe`，Webhook 作为兜底。这对个人用户/家宽部署是硬需求（NextBridge/SiiWay 明确要求公网可达）。
4. **平台增强能力的 OneBot 化暴露**：A2UI 表单、按钮点击、快捷菜单、看板 —— 用 OneBot 扩展字段（`yunhu_*`）承载，避免信息丢失（OB12 转换器已经证明这个模式可行）。
5. **安全补齐**：Webhook 无验签 → 我们自己做 `secret` 校验 + `eventId` 幂等去重。

---

## 4. 云湖平台侧事实（逐条附证据）

### 4.1 传输方式：有 WebSocket，但**官方文档只写了 Webhook**

| 项 | 事实 | 证据 |
|---|---|---|
| 官方文档口径 | 「推送是通过 `HTTP` 协议以 `POST` 请求的方式推送 `JSON` 格式的数据」 | [事件订阅详述](https://www.yhchat.com/document-v2/event-subscription/event-subscription) |
| 官方 SDK 口径 | Go SDK 示例：把 `http://ip:8805/sub` 填进控制台 | [bot-go-sdk README](https://github.com/yhchat/bot-go-sdk) |
| **实际存在的 WS 网关** | `wss://ws.jwzhd.com/subscribe`，**无需公网 IP**;Koishi 适配器**默认**走它、AstrBot 插件默认走它 | [adapter-yunhupro 配置项说明](https://fly1919.github.io/adapter-yunhupro/markdown/start/config.html#websocket-%E9%85%8D%E7%BD%AE)、[WebSocket 接入教程](https://fly1919.github.io/adapter-yunhupro/markdown/firstime/start-websocket.html)、[astrbot 插件 README](https://github.com/Soffd/astrbot_plugin_yunhuAdapter) |
| WS 端点性质 | **官方 v2 文档中不存在该端点**；AstrBot 作者注明「沿用旧版实现，云湖实际订阅地址和可用性需自行确认」 | 同上 |
| 另一个 WS | `wss://chat-ws-go.jwzhd.com/ws` 是**客户端（用户账号）protobuf 协议**，需 `login`（userId+token+platform+deviceId），与 bot 无关 | [yhchatAPI: chat-ws-go](https://github.com/yh-Tpdev/yhchatAPI/blob/main/src/api/wss/chat-ws-go.jwzhd.com.md) |

> **【推测】** `ws.jwzhd.com/subscribe` 大概率是云湖为 bot 提供的**半官方**推送通道（多条产品线独立使用、两年未失效），但**不在 SLA 内**。工程上必须双模 + 可切换，不能只依赖 WS。

### 4.2 验签：**没有**

- 官方文档全文（事件订阅页）**未出现任何签名字段**（无 `sign`、`timestamp+nonce`、`X-Signature`），Header 只有 `eventId / eventTime / eventType`。【已核实：文档原文】
- 第三方实现者的明确结论【已核实】：FLY1919 文档在解释「多机器人同路径会消息混乱」时写：**「云湖的 Webhook 推送没有身份验证机制，使用相同路径会导致所有机器人收到相同的消息。」**
- 社区只能自己加：AstrBot 插件提供 `webhook_secret`（`?secret=` 或 `X-Yunhu-Secret`），**是插件自建机制，非平台能力**。
- 风险：回调 URL 泄露 = 任何人可伪造任意用户/群消息注入我们的转译层 → **必须自行鉴权 + 幂等**。

### 4.3 消息内容格式：**结构化 JSON 对象，不是 CQ 码字符串**

**入站（事件）**：`message.content` 是**按 `contentType` 变形的对象**。【已核实：[事件订阅详述](https://www.yhchat.com/document-v2/event-subscription/event-subscription)】

| contentType | content 字段 |
|---|---|
| `text` | `text`、`buttons[]`、`at[]`（userId 列表） |
| `image` | `imageUrl`、`imageName`、`imageKey`、`imageWidth`、`imageHeight` |
| `file` | `fileName`、`fileUrl`、`fileKey`、`fileSize` |
| `video` | `videoUrl`、`videoDuration` |
| `audio` | `audioUrl`、`audioDuration` |
| `markdown` / `html` | `text`（原文） |

**出站（发送）**：**必须先用上传接口换 `imageKey/fileKey/videoKey`**，不能直接传 URL。【已核实：[机器人发送消息](https://www.yhchat.com/document-v2/message-management/send-message)】

```json
{ "recvId": "8888888", "recvType": "group", "contentType": "text",
  "content": { "text": "@张三 请查看", "at": ["7058262"], "buttons": [[{"text":"复制","actionType":2,"value":"x"}]] } }
```

- `buttons` 是**二维数组**（行 × 列），`actionType`：`1` 跳转 URL / `2` 复制 / `3` 点击汇报（→ 触发 `button.report.inline` 事件）。
- `at` 只传 userId 列表，**text 里要自己写 `@昵称`**，客户端再高亮。【已核实：文档「说明」段】
- **降级注意**：官方发送 API **没有 `audio` 类型**（AstrBot 插件把语音转成文件发送）【已核实：插件 README】。

### 4.4 bot 的 self id

- 事件里**没有**「机器人自己是谁」的字段。私聊时 `message.chatId` == **机器人 ID**（文档：「单聊消息，chatId 即机器人ID」），群聊时 `chatId` == 群 ID。【已核实】
- 正确做法：启动时调 `GET /open-apis/v1/bot/info?token=...` 拿 `data.botId`。【已核实：[获取机器人信息](https://www.yhchat.com/document-v2/bot-management/bot-info)】—— 适配器普遍这么做（FLY1919 文档：「新版本会在启动时自动获取机器人 ID，您只需要提供 Token」）。
- **对 OneBot 的影响**：`self_id` 必须异步获取并缓存；反向 WS 建连时 `self_id` 可能尚未就绪 → 需要「先 fetch 再握手」或延迟上报。

### 4.5 ID 形态

- **全部是字符串**（`string`），数值形态：用户 `7058262`、群 `8888888`、bot `987654`、消息 `msg_abc123`（文档示例）。**长度无规范**，实测有 6–9 位纯数字，也有带前缀示例。【已核实：文档字段类型 + 示例】
- **结论**：按**不透明字符串**处理，**禁止**假设长度/纯数字/可自增；OneBot v11 的 `user_id`/`group_id` 是 **int64**，跨语言（JS）会丢精度 → **转译层必须做 string↔int64 的映射与回填**（这是我们最容易被坑的一处）。

### 4.6 频控

| 项 | 事实 | 证据 |
|---|---|---|
| 单接口 | **10 次/秒**（发送、批量、编辑、看板、撤回、消息列表均同一档） | [服务端API列表](https://www.yhchat.com/document-v2/getting-started/server-api-list) |
| 超限错误码 | **1007**（「请求过于频繁时可能返回错误码1007」） | 同上 |
| 实现侧对策 | AstrBot 插件：**按接口独立限流 10/s，失败不自动重发（避免重复消息）** | [插件 README](https://github.com/Soffd/astrbot_plugin_yunhuAdapter) |

> 注意：是「**每个接口** 10/s」而非「每机器人总量 10/s」；但保守起见转译层应按**每 token 每接口**令牌桶实现，并对 1007 做**指数退避**（不能盲目重试导致雪崩）。

### 4.7 公网 / HTTPS

- **Webhook**：必须有公网可达地址。SiiWay 原话：「云湖必须能够从外网访问 Webhook 地址。若运行在 NAT 环境后，请使用反向代理或 ngrok / Cloudflare Tunnel 等隧道工具。」【已核实：[yunhu driver 文档](https://docs.siiway.org/zh-CN/nextbridge/v0.3/drivers/yunhu)】
- **协议**：示例用 `http://1.2.3.4:8765/yunhu-webhook` → **明文 HTTP 也被接受**，不强制 HTTPS。【已核实：同上示例】
- **WebSocket**：**无需公网 IP**（这是它的核心卖点）。【已核实：FLY1919 连接方式对比表】

### 4.8 回调重试策略

- **官方 v2 文档未提及**超时、重试次数、重试间隔、`eventId` 去重语义。【已核实：事件订阅页全文无「重试/timeout/retry」】
- 反向线索：适配器普遍**不假设重投**（AstrBot：「失败不会自动重发，以避免产生重复消息」——这是说发送侧）；**接收侧无任何项目实现 eventId 幂等**。
- **【推测】** 大概率是「一次性 POST，失败即丢」或「有限次重试」，**必须按 at-least-once 设计**：以 `header.eventId`（全局唯一）+ `message.msgId` 做幂等键。

### 4.9 权限与能力边界（影响 OneBot action 覆盖）

| 云湖能力 | 约束 | OneBot 对应 |
|---|---|---|
| 群成员列表 | **需群主/管理员单独授权**，`page/size` 分页 | `get_group_member_list`（可能 1003/无权限） |
| 禁言 | 需 `allowGagMember=1`；`gag` 秒，`-1` 永久 | `set_group_ban` |
| 移除成员 | — | `set_group_kick` |
| 群消息类型控制 | 可限制群内允许的消息类型 | 无对应（扩展） |
| 群标签 | 增删改查 + 用户打标 | **无对应**（云湖特色，只能扩展） |
| 看板 | 用户/群/全局 | **无对应**（扩展） |
| 流式消息 | chunked 传输 | 无对应（可映射为分段 send） |

证据：[服务端API列表](https://www.yhchat.com/document-v2/getting-started/server-api-list)、[用户禁言](https://www.yhchat.com/document-v2/group-management/gag-user)。

---

## 5. 社区热度与迁移需求证据

### 5.1 QQ 官方机器人 / 协议端风控现状（迁移的「推力」）

| 证据 | 内容 | 链接 / 时间 |
|---|---|---|
| **Lagrange V2 官方宣告** | 「由于 OneBot 11 协议的**各种历史遗留问题**，我们最终决定，**LagrangeV2 不再支持 OneBot 11 协议，V1 的 OneBot 11 协议实现也一并不再维护**。」——改用 Lagrange.Milky（Milky 协议） | [Lagrange.Doc v2 首页](https://lagrangedev.github.io/Lagrange.Doc/v2/)【已核实】 |
| NapCat 风控 issue | `[BUG] 最新版本的NapCat/QQ登录会被风控检测导致频繁掉线` #1728 | [NapNeko/NapCatQQ#1728](https://github.com/NapNeko/NapCatQQ/issues/1728)（标题已核实；正文抓取被网络拦截） |
| NapCat 项目仍在高强度维护 | 12,836 star，2026-11-12 有推送 → **协议端没死，但不稳定是常态** | GitHub API【已核实】 |
| AstrBot 把「OneBot v11」与「QQ 官方机器人」并列为两条独立接入路径 | 说明社区对协议端的依赖是结构性的 | [AstrBot 文档侧边栏](https://docs.astrbot.app/what-is-astrbot.html)【已核实】 |
| NextBridge 平台表把「微信」标为 ⏸️「账号封禁风险极高」 | 同一份风控逻辑的旁证 | [platform-support.md](https://github.com/siiway/nb-docs/blob/main/zh/v0.4/platform-support.md)【已核实】 |

> **判断**：OneBot v11 生态正在被**上游协议端主动抛弃**（Lagrange 明确不玩了），但**下游插件生态仍然巨大**（AstrBot 46.9k star、Koishi 6.3k、NoneBot 生态）。这形成一个**「协议标准仍在、实现端萎缩」的窗口**——正是转译层的价值所在。

### 5.2 AstrBot 规模与适配器现状

- **AstrBot：46,909 star / 3,572 fork，AGPL-3.0，2026-11-14 仍在推送**（GitHub API【已核实】）。
- **原生平台列表（18+）中【没有云湖】**；云湖只能通过第三方插件 `astrbot_plugin_yunhuAdapter`（4 star）接入。【已核实：docs.astrbot.app 侧边栏 + 插件 README】
- 含义：**AstrBot 用户想上云湖，目前只有「装第三方插件」一条路**。如果我们做 OneBot v11 反向 WS，AstrBot 用户可以用**内置的 aiocqhttp/OneBot 适配器**直接连——**零插件安装**，这是我们相对 Soffd 插件的天然优势。

### 5.3 云湖自身规模与口碑

| 项 | 内容 | 证据 |
|---|---|---|
| 用户规模 | 「截至 2025 年，云湖已拥有**超过 10 万活跃用户**」 | [关于我们](https://www.yhchat.com/support/about.html)【已核实，官方自述】 |
| 时间线 | 2024-03 立项 → 2025-03 全平台上线 → 2025-06 结束内测进入公测 → **2025-07 开始走向市场** | 同上 |
| 主体 | 北京九万智达科技有限公司；京ICP备20012521号-7；增值电信业务经营许可证 京B2-20261322 | 同上 |
| 开发者侧活跃度 | 第三方客户端至少 4 个（Murexide 22★、Yhchat_MD3 39★、Yunhu.NET、yhchat_web）；反向工程 API 文档 31★；bot PHP 生态也在长（`liutexi/ec-feedback` 11★ 活跃） | GitHub 搜索【已核实】 |
| 「替代 QQ」讨论 | 未检索到成规模、可引用的「云湖替代 QQ」公开讨论帖；能找到的是云湖站内的社群治理讨论（如 <https://www.yhchat.com/c/p/45443>）与零散教程。**此项证据薄弱，不应在对外材料中夸大。** | 【已核实：检索未命中强证据】 |

> **【推测】** 云湖的真实定位更像「AI 社交 + 小圈子社群」，而非「QQ 替代品」；bot 生态的驱动力来自「QQ 协议端不稳 + 云湖开放 API 简单 + 官方鼓励」。**10 万活跃用户 vs AstrBot 4.7 万 star 的量级对比说明：云湖 bot 需求是「供给驱动」而非「需求驱动」——这既是机会（先发）也是风险（盘子小）。**

### 5.4 生态热度量化（下载量，2026-11 采样）

| 包 | 平台 | 月下载 |
|---|---|---|
| `ErisPulse-YunhuAdapter` | PyPI | **740** |
| `koishi-plugin-adapter-yunhupro` | npm | **482** |
| `nonebot-adapter-yunhu` | PyPI | **222** |
| `koishi-plugin-adapter-yunhu`（原版） | npm | 78 |
| **`yunhu`（官方 SDK）** | PyPI | **49** |

> **重要信号【已核实】**：**官方 SDK 的下载量低于所有主流社区适配器一个数量级**。生态事实标准由社区适配器定义，不由官方 SDK 定义。→ 我们做 OneBot 转译层时，**应以社区适配器验证过的行为为准**（尤其 FLY1919 与 Soffd 的实现细节），官方文档只作字段级依据。

---

## 6. 对我们项目的启示

### 6.1 可以直接复用的（省工期）

| 复用对象 | 用法 | 注意 |
|---|---|---|
| **云湖官方 Python SDK（`yunhu`）** | 仅取「事件数据类 + 发送 API 封装」的字段定义作为**字段清单核对表** | 功能太窄（无媒体/群管/验签），**不要做运行时依赖**；PyPI 版本可能落后仓库 |
| **`astrbot_plugin_yunhuAdapter` README** | 当作**平台边界条件清单**：10/s 限流、图片 10MB/文件视频 20MB、无 audio 发送类型、@全员无参数、Webhook 无鉴权 | MIT，可抄实现思路（需保留版权声明） |
| **`adapter-yunhupro` 文档站** | 当作**平台事实标准**（错误码、连接方式对比、多实例路径冲突、A2UI） | Apache-2.0，文档可引用 |
| **`nonebot-adapter-yunhu` 的 message.py** | **消息段 ↔ 云湖 content 映射表**的直接参照（Text/Image/At/Buttons/Video/File/HTML/Markdown/Reply） | MIT |
| **`ErisPulse-YunhuAdapter` 的事件映射表** | **notice 分类与 detail_type 设计**（friend_increase / group_member_increase / yunhu_button_click / yunhu_a2ui_button / yunhu_shortcut_menu / yunhu_bot_setting） | MIT；OB12 语义，需转译到 v11 |
| **`siiway/NextBridge` yunhu driver** | 媒体下载/重传、`proxy_host` 图片防盗链绕过的思路 | 需查 license |
| **`yunhu-local-server`** | CI 里的云湖 mock（若质量可用） | 未验证，建议自建 mock 更可控 |

### 6.2 必须自己写的（核心壁垒）

1. **OneBot v11 协议服务端**：反向 WS（`/api` + `/event`，含 `X-Self-ID`、`Authorization: Bearer`、`X-Client-Role`）、正向 WS 客户端、HTTP POST 上报（可选）。**全网无实现**。
2. **CQ 码编解码器**：`[CQ:at,qq=]` / `[CQ:image,file=]` / `[CQ:reply,id=]` / `[CQ:face]`（云湖无对应 → 降级为 emoji/文本）↔ 云湖结构化 content。**注意 CQ 码转义（`&#91;` 等）必须做对**，否则用户消息会被二次解析。
3. **ID 映射层**：云湖 `string` ↔ OneBot `int64`。建议 **`int64 = 云湖数字 ID 直转`，非纯数字则用稳定哈希 + 持久化映射表**；同时保留 `yunhu_id` 扩展字段供反向查询。
4. **self_id 生命周期**：启动 `GET /bot/info` → 缓存 → 再建连。反向 WS 首帧前必须就绪。
5. **限流与退避**：按 `(token, endpoint)` 维度令牌桶 10/s，1007 → 指数退避 + 抖动；**发送失败不自动重试**（避免重复消息），改为返回 OneBot 失败 retcode。
6. **幂等去重**：`header.eventId` + `msgId` 双键 LRU/Redis，TTL ≥ 重试窗口。
7. **Webhook 安全**：路径内嵌 secret 或 `X-Yunhu-Secret` 头（自建，平台无此能力）+ 来源 IP 白名单（若能确认出口 IP）。
8. **能力降级表**：OneBot 有而云湖没有的 action（`set_group_card`、`set_group_special_title`、`get_stranger_info` 部分字段、`set_friend_add_request`…）必须返回**明确的 retcode + 可读 msg**，而不是静默失败——否则上层插件会以为成功。

### 6.3 推荐架构（**建议，非事实**）

```
[云湖平台]
  ├─(首选) wss://ws.jwzhd.com/subscribe  ← 无需公网 IP
  └─(兜底) HTTP POST webhook             ← 需公网 IP/隧道
        ↓ 统一为内部 YunhuEvent
  ┌──────────────────────────────────────────┐
  │ 转译核心（自研）                          │
  │  · 幂等去重(eventId/msgId)                │
  │  · 事件规范化 → OneBot v11 event          │
  │  · CQ 码 ↔ 云湖 content 编解码            │
  │  · ID 双向映射(string↔int64)              │
  │  · action 路由器 + 限流器(10/s) + 退避     │
  └──────────────────────────────────────────┘
        ↓ OneBot v11 反向 WS(服务端) / 正向 WS(客户端)
  [NoneBot2 / Koishi / AstrBot(aiocqhttp) / CQ 码插件]
```

### 6.4 风险清单

| 风险 | 等级 | 缓解 |
|---|---|---|
| `ws.jwzhd.com/subscribe` **非官方文档化端点**，随时可能变更/下线 | **高** | 双模设计（WS 优先、Webhook 兜底）+ 端点可配置 + 连通性健康检查 + 变更告警 |
| **Webhook 无验签** → 事件伪造/消息注入 | **高** | 自建 secret + 幂等 + 可选 IP 白名单；文档里明确告知用户风险 |
| 平台 API 演进快（2026 新增群管/标签/A2UI/流式），字段可能变 | 中 | 字段解析全部 defensive（缺字段不崩）；契约测试对齐官方文档版本 |
| 云湖盘子小（10 万活跃 vs AstrBot 4.7 万 star），需求可能不足以支撑独立项目 | **中高** | 定位为「AstrBot/NoneBot 的云湖接入方案」而非独立产品；优先做**反向 WS** 以便寄生现有生态 |
| ID 非纯数字 / 超 int64 精度 | 中 | 映射表 + 保留原始字符串；JS 侧用 BigInt/字符串 |
| 官方可能推出自己的「桥」或 Java SDK 完善后生态转向 | 中 | 保持协议层薄、可替换；不与单一 SDK 深度绑定 |
| 内容合规（云湖站内存在涉黄/黑产讨论，见 `yhchat-public-blacklist`、站内举报帖） | 中 | 转译层保持中立管道，不内置内容策略；提供上游过滤钩子 |
| **AGPL 污染** | 中 | **禁止**参考/复制 `nanoyunhu`（AGPL-3.0）代码；AstrBot 本体 AGPL，只读其插件（MIT）实现 |
| 无 license 的项目（`YunhuEventToOnebot12`） | 低 | 只看设计不看代码 |

### 6.5 一句话建议

> **做「云湖 → OneBot v11 反向 WS」的薄转译层，WebSocket 优先、Webhook 兜底，把 CQ 码与 ID 映射做扎实，安全自己补。** 上游用 FLY1919/Soffd 的实现细节当事实标准，协议语义参考 ErisPulse 的 OneBot12 映射，**不要**依赖官方 SDK，**不要**碰 AGPL 的 nanoyunhu，**不要**把 NextBridge 当竞品（它是消息桥，我们是协议桥）。

---

## 附录 A：云湖事件 → OneBot v11 映射建议（设计草案，**非事实**）

| 云湖 eventType | OneBot v11 post_type | 关键字段映射 |
|---|---|---|
| `message.receive.normal` | `message`（`message_type=group/private`） | `sender.senderId`→`user_id`；`chat.chatId`→`group_id`/`user_id`(私聊)；`message.msgId`→`message_id`；`content`→`message`(CQ) + `raw_message`；`senderNickname`→`sender.nickname`；`senderUserLevel`→`sender.role` |
| `message.receive.instruction` | `message` + `yunhu_instruction` 扩展 | `instructionId/instructionName` |
| `bot.followed` / `bot.unfollowed` | `notice`/`friend_add`（或自定义 `yunhu_follow`） | `userId`→`user_id` |
| `group.join` / `group.leave` | `notice`/`group_increase` `group_decrease` | `chatId`→`group_id`（`leave` 无法区分主动退/被踢 → 建议 `sub_type=leave`） |
| `button.report.inline` | `notice`/`yunhu_button` | `msgId`、`userId`、`value` |
| `a2ui.button.report` | `notice`/`yunhu_a2ui` | `actionName`、`formContext`、`interactionJson` |
| `bot.shortcut.menu` | `notice`/`yunhu_menu` | `menuId`、`menuType`、`senderId` |
| `bot.setting` | `notice`/`yunhu_bot_setting` | `settingJson` |

## 附录 B：云湖 API → OneBot v11 action 映射建议（设计草案，**非事实**）

| OneBot v11 action | 云湖接口 | 备注 |
|---|---|---|
| `send_msg` / `send_group_msg` / `send_private_msg` | `POST /open-apis/v1/bot/send` | `recvType=group/user`；媒体需先 `image/upload` 换 key |
| `send_group_forward_msg` | `POST /bot/batch/send`（近似） | **语义不完全等价**，需文档标注 |
| `delete_msg` | `POST /bot/recall` | 群内消息/机器人消息 |
| `get_msg` | `POST /bot/message/list` | 支持「前后消息」 |
| `get_group_list` | `GET /bot/groups` | page/size 分页，需聚合 |
| `get_group_info` | `GET /group/info` | |
| `get_group_member_list` | `GET /group/members` | **需群主/管理员授权**，否则 1003 |
| `set_group_ban` | `POST /group/gag-member` | `gag` 秒，`-1` 永久，需 `allowGagMember` |
| `set_group_kick` | `POST /group/remove-member` | |
| `get_login_info` | `GET /bot/info` | `botId`→`user_id`，`nickname` |
| `get_status` / `get_version_info` | `GET /bot/info`（`isStop`/`isBanned`） | |
| （无对应）看板 | `/bot/board/*` | 用 `yunhu_` 扩展 action |
| （无对应）群标签 | `/group/tag/*` | 用 `yunhu_` 扩展 action |
| （无对应）流式 | `/bot/send/stream` | 映射为多次 `send` 或扩展 |

---

## 附录 C：证据索引（全部为本次实际抓取/调用）

**官方文档**
- 开发须知 <https://www.yhchat.com/document-v2>
- 服务端 API 列表（10/s、1007）<https://www.yhchat.com/document-v2/getting-started/server-api-list>
- 事件订阅详述 <https://www.yhchat.com/document-v2/event-subscription/event-subscription>
- 机器人发送消息 <https://www.yhchat.com/document-v2/message-management/send-message>
- 获取机器人信息 <https://www.yhchat.com/document-v2/bot-management/bot-info>
- 用户禁言 <https://www.yhchat.com/document-v2/group-management/gag-user>
- 官方 Python SDK <https://www.yhchat.com/document-v2/official-sdks/python-sdk>
- 官方 Golang SDK <https://www.yhchat.com/document-v2/official-sdks/golang-sdk>
- 官方 Java SDK（开发中）<https://www.yhchat.com/document-v2/official-sdks/java-sdk>
- 非官方 Python SDK 汇总 <https://www.yhchat.com/document-v2/unofficial-sdks/python-sdk>
- 非官方 Java SDK（Daen）<https://www.yhchat.com/document-v2/unofficial-sdks/java-sdk>
- 非官方 Node SDK（JV）<https://www.yhchat.com/document-v2/unofficial-sdks/node-sdk>
- llms.txt（文档全量目录）<https://www.yhchat.com/document-v2/llms.txt>
- 关于我们（10 万活跃用户 / 时间线）<https://www.yhchat.com/support/about.html>

**第三方文档**
- FLY1919 配置项（WS 端点、无鉴权原话）<https://fly1919.github.io/adapter-yunhupro/markdown/start/config.html>
- FLY1919 首次接入（连接方式对比、公网要求）<https://fly1919.github.io/adapter-yunhupro/markdown/firstime/start.html>
- FLY1919 WebSocket 教程 <https://fly1919.github.io/adapter-yunhupro/markdown/firstime/start-websocket.html>
- SiiWay NextBridge 云湖 driver <https://docs.siiway.org/zh-CN/nextbridge/v0.3/drivers/yunhu>
- NextBridge 平台支持表 <https://github.com/siiway/nb-docs/blob/main/zh/v0.4/platform-support.md>
- Lagrange.Doc v2（放弃 OneBot 11 原话）<https://lagrangedev.github.io/Lagrange.Doc/v2/>
- AstrBot 文档（平台列表无云湖）<https://docs.astrbot.app/what-is-astrbot.html>

**GitHub / 包管理**
- yhchat/bot-python-sdk、yhchat/bot-go-sdk、daenmax/yhchat-sdk-core、molanp/nonebot-adapter-yunhu、Soffd/astrbot_plugin_yunhuAdapter、FLY1919/adapter-yunhupro、WindyPear-Team/koishi-plugin-adapter-yunhu、wsu2059q/YunhuEventToOnebot12、TASA-Ed/nanoyunhu、siiway/NextBridge、yh-Tpdev/yhchatAPI、AstrBotDevs/AstrBot、NapNeko/NapCatQQ（均通过 REST API 取 star/推送时间/README）
- npm registry：`koishi-plugin-adapter-yunhupro@2.3.3`、`koishi-plugin-adapter-yunhu@1.2.0`；下载量 api.npmjs.org
- PyPI：`yunhu@0.0.11`、`ErisPulse-YunhuAdapter@4.6.3`、`nonebot-adapter-yunhu`；下载量 pypistats.org
- Maven Central：`cn.daenx:yhchat-sdk-core@1.2.0`（2025-05-25）
