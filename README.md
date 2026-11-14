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

工程基建与文本消息的收发链路已经完成：配置与日志、到云湖的事件订阅连接、到机器人框架的反向 WebSocket 连接、文本消息的双向转换，以及发送与查询类接口。当前版本只处理文本，图片、文件与语音降级为占位文本；通知类事件与心跳尚未接入。

完整的进度与验收状态记录在[项目状态](docs/design/status.md)中，模块划分与数据流见[架构说明](docs/design/architecture.md)。

## 文档

- [开发计划](docs/design/roadmap.md)
- [项目状态](docs/design/status.md)
- [架构说明](docs/design/architecture.md)
- [文档索引](docs/README.md)
- [调研材料](docs/research/)

架构设计与协议映射说明会随实现同步补充。

## 许可证

本项目使用 GNU General Public License v3.0，详见 [LICENSE](LICENSE)。
