# HelloYunhu

[![License](https://img.shields.io/badge/license-GPL--3.0--or--later-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.27.4-00ADD8.svg?logo=go)](go.mod)
[![CI](https://github.com/Takeoff0518/HelloYunhu/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/Takeoff0518/HelloYunhu/actions/workflows/ci.yml)
[![Release](https://img.shields.io/badge/release-v0.3.0-blue.svg)](https://github.com/Takeoff0518/HelloYunhu/releases)

[快速开始](docs/guides/quickstart.md)　[配置说明](docs/guides/configuration.md)　[常见问题](docs/guides/troubleshooting.md)　[功能支持情况](docs/guides/compatibility.md)

HelloYunhu 是一个协议转换程序，在[云湖（Yunhu）](https://www.yhchat.com/)机器人开放平台与 [OneBot v11](https://github.com/botuniverse/onebot-11) 协议之间进行双向转换。

## 项目要解决的问题

OneBot v11 是目前中文聊天机器人社区使用最广的接口标准。[AstrBot](https://github.com/AstrBotDevs/AstrBot)、[NoneBot2](https://github.com/nonebot/nonebot2)、[Koishi](https://github.com/koishijs/koishi) 等框架，以及围绕它们编写的大量插件，都通过这套标准工作。基于它编写的机器人通常只能连接 QQ 的协议端实现，例如 [NapCat](https://github.com/NapNeko/NapCatQQ)、[Lagrange](https://github.com/LagrangeDev/Lagrange.Core) 或 [go-cqhttp](https://github.com/Mrs4s/go-cqhttp)。

云湖提供了自己的机器人开放接口，但没有提供 OneBot 兼容层。要把已有的机器人迁移到云湖，只能重写框架的适配层或插件，迁移成本很高。

HelloYunhu 补上的就是这一层。对于机器人框架，它表现为一个标准的 OneBot v11 协议端；对于云湖，它表现为一个普通的机器人客户端。框架和插件不需要知道自己连接的实际是云湖。

## 工作方式

程序同时维护两条连接。

第一条是与云湖平台之间的连接。HelloYunhu 作为客户端订阅消息事件，并在需要回复时调用云湖的开放接口发送消息。

第二条是与机器人框架之间的连接。HelloYunhu 作为客户端，通过反向 WebSocket 连接到框架监听的端口，按照 OneBot v11 规定的格式上送事件、接收接口调用。反向 WebSocket 也是 NapCat、Lagrange 与 AstrBot 配合时使用的方式，因此框架侧的配置方法与用户已有的使用习惯一致。

## 当前状态

0.3.0 已发布，可从 [Releases](https://github.com/Takeoff0518/HelloYunhu/releases) 下载 linux/amd64、windows/amd64 与 darwin/arm64 三个发行包。首个验收对象是 AstrBot 加 aiocqhttp 适配器，其他 OneBot v11 框架按协议层兼容。

完整的进度与验收状态记录在[项目状态](docs/design/status.md)中，模块划分与数据流见[架构说明](docs/design/architecture.md)。

## 功能概览

| 维度 | 状态 |
|---|---|
| 消息类型 | 文本、图片、文件、视频、语音双向可用，支持 @、引用与图文混排拆分；合并转发降级为文本 |
| 事件 | 群聊与私聊的消息、指令、生命周期与通知事件；心跳未接入 |
| 接口 | 发送、查询、撤回、禁言、踢人与快速操作可用，未实现的接口返回 1404 |
| 群管理 | 撤回、禁言（四个档位）、踢人，以及群标签与专属头衔 |
| 稳定性 | 多条框架连接独立重连、标识与会话归属落库、接口限流、只读诊断接口 |
| 媒体与头像 | 可选的 `proxy` 服务，把云湖媒体与头像转成客户端可下载的地址；`compat.gensokyo` 为只认该名字的客户端补上头像接口 |
| 发行 | 0.2.0 提供三平台发行包与校验和；Webhook 接收已支持，Docker 镜像未开始 |

单条能力的状态、限制与原因见[功能支持情况](docs/guides/compatibility.md)。那份表是唯一的清单，随代码一起更新。

## 开发文档

- [开发计划](docs/design/roadmap.md)
- [项目状态](docs/design/status.md)
- [架构说明](docs/design/architecture.md)
- [扩展字段与非标准行为](docs/guides/extensions.md)
- [更新记录](CHANGELOG.md)
- [文档索引](docs/README.md)
- [调研材料](docs/research/)

## 许可证

本项目使用 GNU General Public License v3.0，详见 [LICENSE](LICENSE)。
