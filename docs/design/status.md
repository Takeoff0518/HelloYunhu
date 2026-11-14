# 项目状态

本文件记录项目的当前进度，供上下文被压缩后恢复工作现场使用。每次工作结束时更新它，写清楚完成了什么、停在哪里、下一步做什么。

最后更新：2026-11-14。

## 当前阶段

M0 工程基建，尚未开始。等 Go 版本与模块路径确认后开工。

## 已完成

- 协议调研，产出三份材料，保存在 `docs/research/`。
- 仓库整理：GPL-3.0 许可证、README、文档目录结构、`.gitignore`。
- 开发计划：`docs/design/roadmap.md`。
- 协作约定：`AGENTS.md`。
- 本状态文件。

仓库尚未有任何提交，所有文件处于未跟踪状态。

## 已确认的决定

| 事项 | 结论 |
|---|---|
| 许可证 | GNU General Public License v3.0 |
| 开发语言 | Go |
| 首个连接方式 | 反向 WebSocket，角色为 Universal |
| 首个版本的消息范围 | 先做群聊，私聊在 M2 补齐 |
| Web 管理界面 | 首个版本不提供，只提供只读诊断接口 |
| 文档语言 | 中文优先，发布后再补英文摘要 |
| 验收框架 | AstrBot 加 aiocqhttp 适配器 |
| 可执行文件名称 | `helloyunhu` |
| Go 模块路径 | `github.com/Takeoff0518/HelloYunhu` |
| 仓库地址 | `https://github.com/Takeoff0518/HelloYunhu`，当前为私有仓库 |
| Go 版本 | 1.27.4 |

## 下一步：M0 任务清单

按顺序执行，每完成一项就在本文件中勾掉。

1. 创建 `go.mod`，模块路径使用 `github.com/Takeoff0518/HelloYunhu`。
2. 创建目录骨架与 `cmd/helloyunhu/main.go`，支持 `run`、`version`、`selftest` 三个子命令，其中 `selftest` 先留空实现。
3. 实现 `internal/config`：配置结构、默认值、加载、校验，并提供 `examples/config.yaml`。
4. 实现 `internal/logging`：初始化 `log/slog`，支持文本与 JSON 两种输出。
5. 添加 `Makefile`、`.golangci.yml`、`.github/workflows/ci.yml`（格式检查、静态检查、测试、三平台构建）。
6. 创建首次提交。

## M0 验收标准

- `go build ./...` 通过。
- 三个平台的构建任务在持续集成中成功。
- 在没有任何配置文件的情况下启动，程序给出明确错误并退出，而不是崩溃或静默使用默认值。

## 环境

- 操作系统：Windows。
- 工作目录：`E:\code\HelloYunhu`。
- Go：1.27.4，安装在 `D:\golang`，`GOTOOLCHAIN` 为 `auto`。
- Git：2.37.0。

## 恢复上下文的步骤

如果上下文被压缩，按下面的顺序恢复：

1. 读 `AGENTS.md`，确认写作要求、工作流与禁忌。
2. 读本文件，确认当前阶段与下一步。
3. 读 `docs/design/roadmap.md` 的第 4 节与第 5 节，确认结构与里程碑。
4. 从「下一步」中第一项未完成的任务继续。
5. 需要协议细节时查 `docs/research/feasibility-report.md`，不要通读。

## 已知的未决问题

- 云湖侧的 14 项待实测内容（见调研报告第 11 节）尚未开始验证，其中发送接口返回的消息标识位置、私聊接收类型、媒体下载是否需要 Referer 三项会直接影响 M2 的设计。
- 仓库目前是私有状态。转为公开之前，需要确认所有提交中没有凭据，并检查依赖许可证与 GPL-3.0 的兼容性。
