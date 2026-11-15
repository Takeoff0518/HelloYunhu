"""诊断与引用命令。

在群里或私聊发送 `/diag` 会回复事件的关键字段与消息段类型，用于排查协议映射问题。
发送 `/quote` 会引用当前消息再回复，用于验证发送请求里的 `parentId`。
发送 `/friends` 会调用 `get_friend_list` 并回复数量，用于确认返回的是空数组。

字段含义：
- `self_id` 是机器人标识，来自云湖的 botId；
- `message_id` 是映射后的整数，应与 OneBot 侧的引用、撤回一致；
- `group_id` 为 0 表示私聊；
- `segments` 列出消息段类型，用于确认 @ 与图片是否被正确拆分。
"""

from nonebot import on_command
from nonebot.adapters.onebot.v11 import Bot, Message, MessageEvent, MessageSegment

diag = on_command("diag", priority=5, block=True)
quote = on_command("quote", priority=5, block=True)
friends = on_command("friends", priority=5, block=True)


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


@quote.handle()
async def _(bot: Bot, event: MessageEvent) -> None:
    # 引用收到的消息再回复，用于验证发送请求里的 parentId 是否正确。
    message = MessageSegment.reply(event.message_id) + Message("引用回复测试")
    await bot.send(event, message)


@friends.handle()
async def _(bot: Bot, event: MessageEvent) -> None:
    # 云湖没有用户列表接口，这里验证返回的是空数组，而不是失败或 null。
    friend_list = await bot.get_friend_list()
    await bot.send(event, f"friends={len(friend_list)}")
