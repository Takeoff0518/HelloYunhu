# 项目状态

本文件记录项目的当前进度，供上下文被压缩后恢复工作现场使用。每次工作结束时更新它，写清楚完成了什么、停在哪里、下一步做什么。

最后更新：2026-11-15。

## 当前阶段

M0 工程基建的代码部分已完成，等待推送到 GitHub 后由持续集成验证。M1 文本消息闭环尚未开始。

## 已完成

### 规划材料

- 协议调研，产出三份材料，保存在 `docs/research/`。
- 仓库整理：GPL-3.0 许可证、README、文档目录结构、`.gitignore`。
- 开发计划：`docs/design/roadmap.md`。
- 协作约定：`AGENTS.md`。

### M0 工程基建

- 模块：`go.mod` 声明模块路径 `github.com/Takeoff0518/HelloYunhu` 与 Go 1.27.4，依赖只有 `gopkg.in/yaml.v3`。
- 命令行入口：`cmd/helloyunhu` 提供 `run`、`version`、`selftest`、`help` 四个子命令。`version` 优先使用链接参数注入的信息，未注入时回退到构建信息中的提交与时间。
- 配置：`internal/config` 实现 YAML 配置的结构、默认值、加载与校验，示例在 `examples/config.yaml`。校验会一次性返回全部问题。
- 日志：`internal/logging` 封装 `log/slog`，支持文本与 JSON 两种输出，级别与格式的解析只此一处。
- 装配：`internal/app` 加载配置、创建日志、遍历实例并等待退出信号。连接逻辑在 M1 接入。
- 工程配套：`Makefile`、`.golangci.yml`、`.gitattributes`、`.github/workflows/ci.yml`（格式检查、静态检查、竞态测试、三平台构建）。

### 提交历史

| 提交 | 内容 |
|---|---|
| `docs:` | 导入项目文档、调研材料与许可证 |
| `chore:` | 初始化 Go 模块与命令行骨架 |
| `feat(logging):` | 增加日志初始化 |
| `feat(config):` | 增加配置加载与校验 |
| `feat(app):` | 配置文件与日志接入进程生命周期 |
| `build:` | 增加构建脚本与持续集成 |

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

## M0 验收对照

| 验收项 | 状态 |
|---|---|
| `go build ./...` 通过 | 已通过 |
| 三个平台的构建任务成功 | 未验证，需要推送后由持续集成执行 |
| 没有配置文件时给出明确错误并退出 | 已通过，`run` 退出码为 1，错误信息包含缺少的文件名 |

## 本地验证记录

- `gofmt -l .` 无输出，`go vet ./...` 通过，`go test ./...` 全部通过。
- `go test -race ./...` 在本机不可用：竞态检测需要 cgo，本机的 MinGW 8.1 运行时版本过旧，检测二进制启动时以 `0xc0000139` 退出。竞态检测改由持续集成在 Linux 上执行，本地只跑普通测试。
- `golangci-lint v2.15.0` 通过 `go run` 方式运行，结果为 0 个问题，配置可用。
- 手动验证：`selftest -config examples/config.yaml` 输出通过；`run` 在缺少 `config.yaml` 时报错退出。

## 环境

- 操作系统：Windows，工作目录 `E:\code\HelloYunhu`。
- Go：1.27.4，安装在 `D:\golang`，`GOTOOLCHAIN` 为 `auto`。
- 模块代理：本机无法直接连接 `sum.golang.org`，下载依赖时需要设置 `GOPROXY=https://goproxy.cn,direct` 与 `GOSUMDB=sum.golang.google.cn`。这只影响本机，不属于项目配置，不要写进代码或脚本。
- 未安装 GNU make 与 golangci-lint 可执行文件。`Makefile` 与 lint 配置均已按标准写法给出，lint 已通过 `go run` 验证，`Makefile` 尚未实际执行。
- Git 2.37.0，分支 `main`，`origin` 已指向 GitHub，仓库尚未推送。

## 恢复上下文的步骤

如果上下文被压缩，按下面的顺序恢复：

1. 读 `AGENTS.md`，确认写作要求、工作流与禁忌。
2. 读本文件，确认当前阶段与下一步。
3. 读 `docs/design/roadmap.md` 的第 4 节与第 5 节，确认结构与里程碑。
4. 从「下一步」中第一项未完成的任务继续。
5. 需要协议细节时查 `docs/research/feasibility-report.md`，不要通读。

## 下一步

1. 把 `main` 推送到 GitHub，确认持续集成通过。通过后 M0 即完成，在开发计划第 5 节标记。
2. 从 `main` 创建 `feat/m1-subscribe-client` 之类的分支，开始 M1：云湖订阅客户端与反向 WebSocket 客户端。
3. M1 开工前先读调研报告第 11 节，把订阅连接的实测项排进计划。

## 已知的未决问题

- 云湖侧的 14 项待实测内容（见调研报告第 11 节）尚未开始验证，其中发送接口返回的消息标识位置、私聊接收类型、媒体下载是否需要 Referer 三项会直接影响 M2 的设计。
- 持续集成尚未在 GitHub 上运行过，工作流文件没有经过实际执行验证。
- 仓库目前是私有状态。转为公开之前，需要确认所有提交中没有凭据，并检查依赖许可证与 GPL-3.0 的兼容性。
