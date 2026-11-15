"""诊断与引用命令。

在群里或私聊发送 `/diag` 会回复事件的关键字段与消息段类型，用于排查协议映射问题。
发送 `/quote` 会引用当前消息再回复，用于验证发送请求里的 `parentId`。
发送 `/friends` 会调用 `get_friend_list` 并回复数量，用于确认返回的是空数组。
发送 `/forward` 会发一条两层嵌套的合并转发，用于确认展开成文本的效果。

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
forward = on_command("forward", priority=5, block=True)


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


@forward.handle()
async def _(bot: Bot, event: MessageEvent) -> None:
    # 发一条两层嵌套的合并转发，验证展开排版。
    if not event.group_id:
        await bot.send(event, "请在群里使用 /forward")
        return

    nickname = event.sender.card or event.sender.nickname or "发起人"
    nodes = [
        {
            "type": "node",
            "data": {
                "user_id": event.user_id,
                "nickname": nickname,
                "content": [{"type": "text", "data": {"text": "外层第一条"}}],
            },
        },
        {
            "type": "node",
            "data": {
                "user_id": event.user_id,
                "nickname": nickname,
                "content": [
                    {
                        "type": "forward",
                        "data": {
                            "content": [
                                {
                                    "type": "node",
                                    "data": {
                                        "user_id": event.user_id,
                                        "nickname": "嵌套节点",
                                        "content": [
                                            {"type": "text", "data": {"text": "嵌套第二条"}}
                                        ],
                                    },
                                }
                            ]
                        },
                    }
                ],
            },
        },
    ]

    await bot.call_api("send_group_forward_msg", group_id=event.group_id, messages=nodes)
    await bot.send(event, "已发送合并转发")
