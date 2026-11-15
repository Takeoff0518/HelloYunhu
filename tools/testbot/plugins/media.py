"""媒体消息诊断。

群里发图片、文件或视频时，直接回复这条消息的段信息，不需要再补一条 `/diag`。
云湖一条消息只能有一个内容类型，媒体消息本身带不了文字，`/diag` 只能挂在文字消息
上，所以媒体单独处理。
"""

from nonebot import on_message
from nonebot.adapters.onebot.v11 import Bot, MessageEvent
from nonebot.rule import Rule

# 需要单独报告的消息段类型。
MEDIA_TYPES = {"image", "file", "video", "record"}


async def _has_media(event: MessageEvent) -> bool:
    """规则：消息里至少有一个媒体段。"""
    return any(segment.type in MEDIA_TYPES for segment in event.get_message())


media_report = on_message(rule=Rule(_has_media), priority=3, block=True)


@media_report.handle()
async def _(bot: Bot, event: MessageEvent) -> None:
    lines = [
        f"message_id={event.message_id}",
        f"user_id={event.user_id}",
        f"group_id={event.group_id}",
    ]

    for segment in event.get_message():
        if segment.type not in MEDIA_TYPES:
            continue
        # 只保留前 160 个字符，base64 内容没必要整段打印。
        data = {
            key: (value if len(str(value)) <= 160 else str(value)[:160] + "…")
            for key, value in segment.data.items()
        }
        lines.append(f"segment={segment.type} {data}")

    await bot.send(event, "\n".join(lines))
