# 项目状态

本文件记录项目的当前进度，供上下文被压缩后恢复工作现场使用。每次工作结束时更新它，写清楚完成了什么、停在哪里、下一步做什么。

最后更新：2026-11-15。

## 当前阶段

M2 消息与身份的代码基本完成：CQ 码编解码、SQLite 持久化标识映射、文本与图片双向收发、文件与视频的上下行、群信息与群成员查询、消息查询、CQ 码入口与协议映射说明都已实现。撤回、禁言与踢人属于 M3 范围，心跳事件与 CQ 码出口格式尚未实现。真实环境验收仍未进行，缺少云湖机器人令牌与 AstrBot 环境。仓库仍未推送。

## 已完成

### 规划材料

- 协议调研，产出三份材料，保存在 `docs/research/`。
- 仓库整理：GPL-3.0 许可证、README、文档目录结构、`.gitignore`。
- 开发计划：`docs/design/roadmap.md`。
- 协作约定：`AGENTS.md`。

### M0 工程基建

- 模块：`go.mod` 声明模块路径 `github.com/Takeoff0518/HelloYunhu` 与 Go 1.27.4。
- 命令行入口：`run`、`version`、`selftest`、`help` 四个子命令。
- 配置：`internal/config` 实现 YAML 配置的结构、默认值、加载与校验，示例在 `examples/config.yaml`。
- 日志：`internal/logging` 封装 `log/slog`，支持文本与 JSON 两种输出。
- 工程配套：`Makefile`、`.golangci.yml`、`.gitattributes`、`.github/workflows/ci.yml`。

### M1 文本消息闭环

- `internal/yunhu/model`：事件外壳、消息事件、文本内容与发送请求的数据结构。
- `internal/yunhu/api`：机器人信息与发送消息，令牌追加、响应外壳解析、错误码包装。
- `internal/yunhu/subscribe`：订阅连接，令牌以查询参数传递，断线按指数退避重连。
- `internal/onebot/model`：事件、消息段与响应结构，按 aiocqhttp 的硬性要求组织。
- `internal/onebot/client`：反向 WebSocket 客户端，握手头、回显原样返回、写入互斥与超时。
- `internal/bridge/identity`：标识映射，消息标识用递增整数，内存实现，接口按持久化设计。
- `internal/bridge/message`：消息段与云湖消息的相互转换，不支持的类型降级为占位文本。
- `internal/bridge/event`：云湖事件转 OneBot 事件，含自身消息过滤与生命周期事件。
- `internal/onebot/action`：发送与查询类接口，未实现的接口返回 1404。
- `internal/app`：按实例装配两条连接与转发协程，任一实例失败即整体退出。
- `internal/app` 的集成测试覆盖完整链路：模拟云湖回机器人信息、推送群消息、接收发送请求，模拟框架校验握手、收事件、发调用、收响应。
- 设计文档：`docs/design/architecture.md` 记录模块划分、数据流、并发模型与错误处理。

### M2 消息与身份（基本完成）

- `internal/bridge/cqcode`：CQ 码与消息段数组的相互转换，含转义与还原，未闭合的片段按文本处理。
- `internal/storage`：SQLite 持久化，实现消息标识映射接口，重启后映射关系保持不变，并按保留时长清理过期记录。
- `internal/bridge/cache`：最近消息缓存，供 `get_msg` 回放，按插入顺序淘汰。
- `internal/app`：启动时打开数据库，所有实例共用一张映射表，退出时关闭，按小时清理过期记录；消息事件写入缓存。
- `internal/bridge/media`：读取 base64、网络地址与本地路径的媒体，下载有体积上限。
- `internal/bridge/message`：把一条 OneBot 消息拆成多条云湖消息，文本与 @ 合并、媒体段单列；接收方向构造文本、图片、文件、视频与语音段。
- `internal/bridge/event`：图片先下载再以 base64 交给框架，失败时退化为地址；文件与视频、语音给出地址。
- `internal/yunhu/api`：群信息、群列表、群成员列表与图片、文件、视频上传。
- `internal/onebot/action`：图片、文件、视频与语音的发送，群类查询，消息查询，CQ 码字符串入口；群成员权限不足时返回空数据，保住 @ 唤醒。
- 设计文档：新增 `docs/design/protocol-mapping.md`，记录事件、接口、消息段与标识的实际映射以及尚未确认的行为。

M2 范围之外的内容按开发计划留给 M3：撤回、禁言、踢人、限流与诊断接口。尚未实现的小项：心跳事件、CQ 码的出口格式（事件固定用消息段数组）。

### 提交历史

| 提交 | 内容 |
|---|---|
| `docs:` | 导入项目文档、调研材料与许可证 |
| `chore:` | 初始化 Go 模块与命令行骨架 |
| `feat(logging):` | 增加日志初始化 |
| `feat(config):` | 增加配置加载与校验 |
| `feat(app):` | 配置与日志接入进程生命周期 |
| `build:` | 增加构建脚本与持续集成 |
| `docs(design):` | 记录 M0 完成情况 |
| `feat(yunhu):` | 增加事件数据结构与接口客户端 |
| `feat(onebot):` | 增加反向 WebSocket 客户端与协议结构 |
| `feat(yunhu):` | 增加事件订阅客户端 |
| `feat(bridge):` | 增加标识映射与事件、消息转换 |
| `feat(onebot):` | 实现发送与查询类接口调用 |
| `feat(app):` | 接入订阅连接与框架连接，打通文本消息闭环 |
| `docs(design):` | 补充架构说明并更新 M1 状态 |
| `chore:` | 合并 M1 文本消息闭环到 main |
| `feat(bridge):` | 增加 CQ 码编解码 |
| `feat(storage):` | 增加 SQLite 持久化标识映射 |
| `feat(app):` | 使用持久化映射并定期清理过期记录 |
| `docs(design):` | 记录 M2 进度并更新标识映射说明 |
| `feat(yunhu):` | 增加群信息、群成员与媒体上传接口 |
| `feat(bridge):` | 增加媒体读取与消息拆分 |
| `feat(onebot):` | 增加图片发送与群类查询接口 |
| `feat(bridge):` | 增加消息缓存与文件视频段转换 |
| `feat(onebot):` | 支持文件视频发送、消息查询与 CQ 码入口 |
| `feat(yunhu):` | 补充文件、视频与语音的内容结构 |

## 已确认的决定

| 事项 | 结论 |
|---|---|
| 许可证 | GNU General Public License v3.0 |
| 开发语言 | Go |
| Go 模块路径 | `github.com/Takeoff0518/HelloYunhu` |
| Go 版本 | 1.27.4 |
| 仓库地址 | `https://github.com/Takeoff0518/HelloYunhu`，当前为私有仓库 |
| 首个连接方式 | 反向 WebSocket，角色为 Universal |
| 首个版本的消息范围 | 先做群聊，私聊在 M2 补齐 |
| Web 管理界面 | 首个版本不提供，只提供只读诊断接口 |
| 文档语言 | 中文优先，发布后再补英文摘要 |
| 验收框架 | AstrBot 加 aiocqhttp 适配器 |
| 可执行文件名称 | `helloyunhu` |
| 配置文件格式 | YAML，默认路径 `config.yaml`，示例在 `examples/` |
| 配置项命名 | 小写下划线，例如 `api_base`、`message_ttl` |
| 日志 | `log/slog`，默认级别 info，默认格式 text |
| 构建入口 | `Makefile`，持续集成直接调用 `go` 命令 |
| 持续集成 | GitHub Actions，三平台为 linux/amd64、windows/amd64、darwin/arm64 |
| 重连策略 | 订阅连接与框架连接均为 1 秒到 30 秒指数退避，连接建立过后重置 |
| 事件通道容量 | 64，写满时背压到订阅连接的读循环 |
| 消息内容范围 | 首版只处理文本，媒体与其它类型降级为占位文本 |
| 标识映射实现 | M1 为内存表，M2 换成 SQLite，接口不变 |

## 验收对照

M0 的三个验收项：

| 验收项 | 状态 |
|---|---|
| `go build ./...` 通过 | 已通过 |
| 三个平台的构建任务成功 | 未验证，需要推送后由持续集成执行 |
| 没有配置文件时给出明确错误并退出 | 已通过，退出码为 1，错误信息包含文件路径 |

M1 的两个验收项：

| 验收项 | 状态 |
|---|---|
| 云湖群内 @机器人 发送文本，框架收到并回复 | 实现完成，集成测试覆盖；真实环境未验证 |
| 断开任一连接后 30 秒内自动恢复 | 重连实现与模拟断开的单元测试通过；真实网络未验证 |

## 本地验证记录

- `gofmt -l .` 无输出，`go vet ./...` 通过，`go test ./...` 全部通过。
- 集成测试覆盖两条连接与一次完整收发，模拟服务分别扮演云湖与框架，不依赖外网。
- 存储测试覆盖同一标识分配稳定、重启后映射不变、过期记录清理，数据库文件放在临时目录。
- 媒体测试覆盖 base64、本地文件、网络下载、体积上限与非 200 响应；发送测试覆盖上传后再发送图片。
- 图片接收测试覆盖三种情况：没有媒体读取器时给出地址、下载成功时内联 base64、下载失败时退化为地址。
- `go test -race ./...` 在本机不可用：竞态检测需要 cgo，本机 MinGW 8.1 运行时版本过旧，检测二进制以 `0xc0000139` 退出。竞态检测由持续集成在 Linux 上执行。
- `golangci-lint v2.15.0` 通过 `go run` 方式运行，结果为 0 个问题。
- 手动验证：`selftest -config examples/config.yaml` 通过；`run` 在缺少配置时报错退出。

## 环境

- 操作系统：Windows，工作目录 `E:\code\HelloYunhu`。
- Go：1.27.4，安装在 `D:\golang`，`GOTOOLCHAIN` 为 `auto`。
- 模块代理：本机无法直接连接 `sum.golang.org`，下载依赖时需要设置 `GOPROXY=https://goproxy.cn,direct` 与 `GOSUMDB=sum.golang.google.cn`。这只影响本机，不属于项目配置。
- 未安装 GNU make 与 golangci-lint 可执行文件。lint 已通过 `go run` 验证，`Makefile` 尚未实际执行。
- Git 2.37.0，当前分支 `feat/m2-message-and-identity`，`origin` 已指向 GitHub，仓库尚未推送。

## 恢复上下文的步骤

如果上下文被压缩，按下面的顺序恢复：

1. 读 `AGENTS.md`，确认写作要求、工作流与禁忌。
2. 读本文件，确认当前阶段与下一步。
3. 读 `docs/design/roadmap.md` 的第 4 节与第 5 节，确认结构与里程碑。
4. 需要实现细节时读 `docs/design/architecture.md`。
5. 需要协议细节时查 `docs/research/feasibility-report.md`，不要通读。

## 下一步

1. 有云湖令牌与 AstrBot 环境时做一次真实链路验收：M1 的文本往返、M2 的图片与文件往返、@ 与引用，结果补进本文件。
2. 合并 `feat/m2-message-and-identity` 到 `main`，随后开始 M3：撤回、禁言、踢人、限流与诊断接口。
3. 推送与持续集成由你确认后再做。

## 已知的未决问题

- 云湖侧的 14 项待实测内容（见调研报告第 11 节）尚未开始验证。其中接口响应外壳的字段名、发送接口返回的消息标识位置、私聊接收类型、媒体下载是否需要 Referer、文件与视频发送的字段组合会直接影响 M2 的收敛。
- 通知类事件尚未转换，当前记录日志后跳过。
- 心跳事件尚未发送，不影响首个验收框架。
- CQ 码只做了入口方向，事件固定以消息段数组发出。
- 持续集成尚未在 GitHub 上运行过，工作流文件没有经过实际执行验证。
- 仓库目前是私有状态。转为公开之前，需要确认所有提交中没有凭据，并检查依赖许可证与 GPL-3.0 的兼容性。
