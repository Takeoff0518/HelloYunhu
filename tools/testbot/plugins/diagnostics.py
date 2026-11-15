"""诊断命令。

回复事件的关键字段与消息段类型，用于排查协议映射问题。在群里或私聊发送
`/diag` 触发。

字段含义：
- `self_id` 是机器人标识，来自云湖的 botId；
- `message_id` 是映射后的整数，应与 OneBot 侧的引用、撤回一致；
- `group_id` 为 0 表示私聊；
- `segments` 列出消息段类型，用于确认 @ 与图片是否被正确拆分。
"""

from nonebot import on_command
from nonebot.adapters.onebot.v11 import Bot, MessageEvent

diag = on_command("diag", priority=5, block=True)


@diag.handle()
async def _(bot: Bot, event: MessageEvent) -> None:
    segments = ", ".join(segment.type for segment in event.get_message())
    lines = [
        f"self_id={event.self_id}",
        f"message_id={event.message_id}",
        f"user_id={event.user_id}",
        f"group_id={event.group_id}",
        f"message_type={event.message_type}",
        f"raw_message={event.raw_message}",
        f"segments={segments}",
    ]
    await bot.send(event, "\n".join(lines))
