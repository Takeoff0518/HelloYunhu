"""M3 群管理与成员查询的测试命令。

群里发送命令即可调用对应接口，结果直接回复在群里。命令只对测试群有意义，正式使用
时不要放进生产机器人。

- `/m3members` 查询群成员列表，回复总数与前几位成员的昵称、角色与群名片；
- `/m3ban 用户号 秒数` 禁言，秒数为 0 表示解禁；
- `/m3kick 用户号` 移出群成员；
- `/m3recall 消息号` 撤回一条消息。

这里的用户号是云湖的用户标识，不是 QQ 号。禁言与踢人需要机器人在群后台获得对应
权限，没有权限时平台返回 1003，命令会如实回复错误。
"""

from nonebot import on_command
from nonebot.adapters.onebot.v11 import Bot, Message
from nonebot.params import CommandArg


def _brief(data: object) -> str:
    """把返回内容压缩成一行，便于在群里查看。"""
    text = repr(data)
    return text if len(text) <= 300 else text[:300] + "…"


members = on_command("m3members", priority=5, block=True)
ban = on_command("m3ban", priority=5, block=True)
kick = on_command("m3kick", priority=5, block=True)
recall = on_command("m3recall", priority=5, block=True)


@members.handle()
async def _(bot: Bot) -> None:
    data = await bot.get_group_list()
    if not data:
        await members.finish("没有可用的群")

    group = int(data[0]["group_id"])
    people = await bot.get_group_member_list(group_id=group)

    lines = [f"群 {group} 成员数={len(people)}"]
    for person in people[:5]:
        lines.append(
            f"{person.get('user_id')} {person.get('nickname', '')} "
            f"角色={person.get('role', '')} 名片={person.get('card', '')}"
        )
    await members.finish(Message("\n".join(lines)))


@ban.handle()
async def _(bot: Bot, args: Message = CommandArg()) -> None:
    parts = args.extract_plain_text().split()
    if len(parts) < 2:
        await ban.finish("用法：/m3ban 用户号 秒数，0 表示解禁")

    group = int((await bot.get_group_list())[0]["group_id"])
    try:
        await bot.set_group_ban(
            group_id=group,
            user_id=int(parts[0]),
            duration=int(parts[1]),
        )
    except Exception as exc:  # 权限不足或参数被平台拒绝时如实回复
        await ban.finish(f"禁言失败：{exc}")
    await ban.finish(f"禁言已提交：用户 {parts[0]}，{parts[1]} 秒")


@kick.handle()
async def _(bot: Bot, args: Message = CommandArg()) -> None:
    parts = args.extract_plain_text().split()
    if not parts:
        await kick.finish("用法：/m3kick 用户号")

    group = int((await bot.get_group_list())[0]["group_id"])
    try:
        await bot.set_group_kick(group_id=group, user_id=int(parts[0]))
    except Exception as exc:
        await kick.finish(f"移除失败：{exc}")
    await kick.finish(f"移除已提交：用户 {parts[0]}")


@recall.handle()
async def _(bot: Bot, args: Message = CommandArg()) -> None:
    parts = args.extract_plain_text().split()
    if not parts:
        await recall.finish("用法：/m3recall 消息号")

    try:
        await bot.delete_msg(message_id=int(parts[0]))
    except Exception as exc:
        await recall.finish(f"撤回失败：{exc}")
    await recall.finish(f"撤回已提交：消息 {parts[0]}")
