# HelloYunhu

HelloYunhu 是一个协议转换程序，在云湖（Yunhu）机器人开放平台与 OneBot v11 协议之间进行双向转换。

## 项目要解决的问题

OneBot v11 是目前中文聊天机器人社区使用最广的接口标准。AstrBot、NoneBot2、Koishi 等框架，以及围绕它们编写的大量插件，都通过这套标准工作。基于它编写的机器人通常只能连接 QQ 的协议端实现，例如 NapCat、Lagrange 或 go-cqhttp。

云湖提供了自己的机器人开放接口，但没有提供 OneBot 兼容层。要把已有的机器人迁移到云湖，只能重写框架的适配层或插件，迁移成本很高。

HelloYunhu 补上的就是这一层。对于机器人框架，它表现为一个标准的 OneBot v11 协议端；对于云湖，它表现为一个普通的机器人客户端。框架和插件不需要知道自己连接的实际是云湖。

## 工作方式

程序同时维护两条连接。

第一条是与云湖平台之间的连接。HelloYunhu 作为客户端订阅消息事件，并在需要回复时调用云湖的开放接口发送消息。

第二条是与机器人框架之间的连接。HelloYunhu 作为客户端，通过反向 WebSocket 连接到框架监听的端口，按照 OneBot v11 规定的格式上送事件、接收接口调用。反向 WebSocket 也是 NapCat、Lagrange 与 AstrBot 配合时使用的方式，因此框架侧的配置方法与用户已有的使用习惯一致。

## 当前状态

项目处于开发早期，尚未发布可用版本。

文本、图片、文件、视频与语音已经双向可用：接收方向把媒体下载后交给框架，发送方向先上传再发送；图文混排会拆成多条消息。群管理（撤回、禁言、踢人）、接口限流与只读诊断接口已完成。通知类事件与心跳尚未接入。

完整的进度与验收状态记录在[项目状态](docs/design/status.md)中，模块划分与数据流见[架构说明](docs/design/architecture.md)。

## 功能清单

图例：`[x]` 已完成，`[ ]` 计划内，`[ ] ✗` 平台能力受限，无法实现。

### 事件接收

- [x] 群聊文本消息
- [x] 私聊文本消息
- [x] 图片接收，下载后以 base64 交给框架
- [x] 文件接收，带文件名与体积
- [x] 视频接收
- [x] 语音接收，转为 `record` 段
- [x] @ 提及拆成 `at` 段并保留位置
- [x] 指令消息，指令名拼回正文
- [x] 自身消息过滤
- [x] 生命周期事件
- [ ] 关注、入群、退群通知事件
- [ ] ✗ 撤回通知（平台不推送撤回事件）
- [ ] 心跳事件

### 消息发送

- [x] 文本，含 @ 与引用
- [x] 图文混排拆成多条消息
- [x] 图片发送，先上传再发送
- [x] 文件发送
- [x] 视频发送
- [x] 私聊发送
- [x] CQ 码字符串入口
- [x] 语音按文件发送
- [ ] ✗ 语音气泡（平台没有语音发送接口）
- [ ] 事件以 CQ 码字符串输出

### OneBot 接口

- [x] `send_msg`、`send_group_msg`、`send_private_msg`
- [x] `get_login_info`、`get_status`、`get_version_info`
- [x] `get_group_info`、`get_group_list`
- [x] `get_group_member_info`、`get_group_member_list`
- [x] `get_msg`，回放进程内缓存
- [x] `delete_msg`
- [x] `set_group_ban`，换算为平台档位
- [x] `set_group_kick`
- [x] `get_stranger_info`（平台无接口，返回空昵称）
- [x] 未实现的接口返回 1404 与说明
- [ ] `get_image`，下载到本地
- [ ] ✗ 全体禁言、设管理员、改群名、群公告、精华消息、戳一戳（平台未开放）
- [ ] ✗ 拒绝再加群（`set_group_kick` 的 `reject_add_request` 没有对应参数）
- [ ] ✗ 任意历史消息查询（平台没有按标识查询历史的接口）

### 稳定性

- [x] 两条连接断线重连
- [x] 消息标识与会话归属落盘
- [x] 接口调用限流，可配置
- [x] 只读诊断接口
- [x] 自检命令 `selftest`
- [x] 原始事件调试日志
- [ ] 并发 10 条消息不乱序的验收
- [ ] 双实例互不影响的验收
- [ ] ✗ 撤回超出保留期的消息（记录随映射表清理）

### 工程与工具

- [x] YAML 配置与校验
- [x] SQLite 持久化
- [x] 三平台持续集成
- [x] 仓库内测试机器人
- [x] 协议映射与测试指南
- [ ] Docker 镜像
- [ ] Webhook 接收模式
- [ ] 配置项完整说明
- [ ] 公开仓库前的凭据与许可证审查

## 文档

- [开发计划](docs/design/roadmap.md)
- [项目状态](docs/design/status.md)
- [架构说明](docs/design/architecture.md)
- [文档索引](docs/README.md)
- [调研材料](docs/research/)

架构设计与协议映射说明会随实现同步补充。

## 许可证

本项目使用 GNU General Public License v3.0，详见 [LICENSE](LICENSE)。
