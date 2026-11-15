"""HelloYunhu 测试用机器人。

连接方式是反向 WebSocket：本进程监听端口，HelloYunhu 主动连过来。因此启动顺序
必须是先运行本机器人，再启动 helloyunhu。

配置见同目录的 .env（由 .env.example 复制而来），插件见 plugins/ 目录。
"""

import nonebot
from nonebot.adapters.onebot.v11 import Adapter

nonebot.init()
nonebot.get_driver().register_adapter(Adapter)
nonebot.load_plugins("plugins")
nonebot.run()
