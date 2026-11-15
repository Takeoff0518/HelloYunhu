"""最小回声插件。

把收到的消息以纯文本回显，用于确认文本往返与 @ 解析：回显内容直接来自
`get_plaintext()`，因此文本段是否正确可以一眼看出来。
"""

from nonebot import on_message
from nonebot.adapters.onebot.v11 import Bot, MessageEvent

echo = on_message(priority=10, block=False)


@echo.handle()
async def _(bot: Bot, event: MessageEvent) -> None:
    await bot.send(event, f"收到：{event.get_plaintext()}")
