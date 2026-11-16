# 文档索引

## 设计文档

- [开发计划](design/roadmap.md)：项目目标、范围、设计取舍、仓库结构、里程碑与工程规范。
- [项目状态](design/status.md)：当前进度、下一步任务与未决问题，每次工作结束时更新。
- [架构说明](design/architecture.md)：模块划分、数据流、并发模型与错误处理，随实现同步更新。
- [协议映射说明](design/protocol-mapping.md)：事件、接口、消息段与标识的映射，以及尚未确认的行为。
- 标识映射说明（`design/id-mapping.md`）：映射算法、表结构与恢复方式，随后续里程碑补充。

## 协作约定

- [AGENTS.md](../AGENTS.md)：写作要求、工作流、代码规范与禁忌，参与开发前必读。

## 使用文档

- [快速开始](guides/quickstart.md)：安装、配置与首次接入，十分钟跑通第一句话。
- [配置说明](guides/configuration.md)：全部配置项的取值、默认值与常见组合。
- [扩展字段](guides/extensions.md)：本程序在 OneBot v11 之外增加的字段、自定义通知类型与非标准行为，写给编写插件的人。
- [常见问题](guides/troubleshooting.md)：按故障阶段组织的现象、原因与处理步骤。
- [测试指南](guides/testing.md)：自检、启动顺序、用测试机器人跑一轮测试与采集现场信息。
- [测试机器人](../tools/testbot/README.md)：基于 NoneBot2 的测试框架，提供回声与诊断命令，不参与主程序构建。

## 调研材料

这三份文档是设计决策的依据，记录的是 2026 年 11 月的调研结论，之后不再修改。

- [可行性调研](research/feasibility-report.md)：云湖开放接口与事件结构、平台限制、OneBot 侧的要求、映射草案、风险与待验证事项。
- [生态调研](research/ecosystem-report.md)：云湖相关 SDK、框架适配器与同类项目的现状。
- [参考项目分析](research/gensokyo-notes.md)：Gensokyo 的架构、标识映射算法与工程经验。
