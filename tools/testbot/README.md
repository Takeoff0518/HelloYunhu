# 测试机器人

这个目录是一个基于 NoneBot2 的测试机器人，用于在没有 AstrBot 的情况下验证 HelloYunhu：它作为机器人框架接收事件、回显消息，并在收到 `/diag` 时输出事件的关键字段。

它属于测试工具，不参与主程序的构建，也不引入 Go 依赖。主程序的依赖清单与交叉编译不受影响。

## 环境要求

Python 3.10 或更高版本。本机用 3.14 跑通过，NoneBot2 2.5.0 与 adapter-onebot 2.4.6 可正常安装与加载。

## 启动

```powershell
cd tools\testbot
.\run.ps1
```

首次运行会创建 `.venv`、安装 `requirements.txt` 中的依赖，并根据 `.env.example` 生成 `.env`。之后直接运行 `.\run.ps1` 即可。

也可以手动操作：

```powershell
python -m venv .venv
.\.venv\Scripts\python.exe -m pip install -r requirements.txt
Copy-Item .env.example .env
.\.venv\Scripts\python.exe bot.py
```

启动后本进程监听 `127.0.0.1:8080`，反向 WebSocket 端点是 `ws://127.0.0.1:8080/onebot/v11/ws`，把它填进 `config.yaml` 的 `onebot.url`。

启动顺序是先本机器人，再 HelloYunhu：HelloYunhu 是主动连接的一方。

## 插件

| 插件 | 作用 |
|---|---|
| `plugins/echo.py` | 回复收到消息的纯文本，默认对所有消息生效 |
| `plugins/diagnostics.py` | `/diag` 回复 `self_id`、`message_id`、`group_id`、消息段类型；`/quote` 引用当前消息再回复，用于验证 `parentId` |
| `plugins/m2accept.py` | 主动调一组接口核对查询与发送路径，默认关闭，需要 `M2_CHECK_GROUP` 与 `M2_CHECK_USER`；`/m2check` 手动重跑 |

`/diag` 的输出用于确认映射结果，例如群里 @ 机器人时 `segments` 应当包含 `at`，发图片时应当包含 `image`。

`/quote` 用于验证引用链路：框架会把引用段转成 `get_msg` 与带 `parentId` 的发送请求，云湖侧应当显示为引用回复。

## 扩展方式

新增测试场景时在 `plugins/` 下加文件即可，`bot.py` 会加载整个目录。建议按场景拆分，不要把逻辑堆进 `echo.py`：例如引用回复单独一个插件、图片往返单独一个插件。

新增依赖写进 `requirements.txt`，然后删掉 `.venv` 重装，或者手动执行一次 `pip install -r requirements.txt`。

## 注意

- `.env`、`.venv`、`__pycache__` 与验收脚本生成的 `.m2check/` 都不进入版本库。
- 第一轮测试把访问令牌留空，确认协议通后再同时设置两边的令牌。
- 撤回、禁言、踢人、关注与入群等通知事件在 HelloYunhu 侧尚未实现，测试这些不会有反应。
- `m2accept` 会真的往群里发消息，只在测试群启用。
