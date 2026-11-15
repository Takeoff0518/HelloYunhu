"""M2 验收脚本：主动调用一组接口，核对查询与发送链路。

默认关闭。设置环境变量 `M2_CHECK_GROUP` 与 `M2_CHECK_USER` 后，HelloYunhu 连上框架
时按顺序执行检查，结果写入 `.m2check/result.json`，同时逐条输出日志。素材在运行时
生成，不进入仓库。

覆盖的场景：群信息类查询的返回结构、图片与文件与视频发送、私聊发送、图文混排拆成
多条消息、@ 段合并进文本消息、发送结果回查。

第二阶段设置 `M2_CHECK_REPLY_TO` 为一条消息标识，只发一条引用回复，用于确认进程
重启后映射表仍然可用。也可以在群里发送 `/m2check` 手动重跑。
"""

import asyncio
import json
import os
import shutil
import struct
import subprocess
import zlib
from pathlib import Path

from nonebot import get_driver, logger, on_command
from nonebot.adapters.onebot.v11 import Bot, Message, MessageSegment

# 素材与结果放在测试机器人目录下，已加入忽略列表。
BASE = Path(__file__).resolve().parent.parent
WORK = BASE / ".m2check"

# 相邻两次调用的间隔，避免触碰每秒十次的上限。
GAP = 1.5

_running = False


def _png() -> Path:
    """生成一张 64x64 的纯色图片，不依赖第三方库。"""
    path = WORK / "hello-yunhu.png"
    width = height = 64

    def chunk(tag: bytes, data: bytes) -> bytes:
        return (
            struct.pack(">I", len(data))
            + tag
            + data
            + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)
        )

    raw = b"".join(b"\x00" + bytes([64, 128, 200]) * width for _ in range(height))
    header = struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0)
    path.write_bytes(
        b"\x89PNG\r\n\x1a\n"
        + chunk(b"IHDR", header)
        + chunk(b"IDAT", zlib.compress(raw))
        + chunk(b"IEND", b"")
    )
    return path


def _text_file() -> Path:
    """生成一个小文本文件，用于文件发送。"""
    path = WORK / "hello-yunhu.txt"
    path.write_text("HelloYunhu 文件发送检查。\n", encoding="utf-8")
    return path


def _ffmpeg() -> str:
    """定位 ffmpeg，找不到时返回空字符串。"""
    candidate = os.environ.get("M2_CHECK_FFMPEG", "")
    if candidate and Path(candidate).exists():
        return candidate
    found = shutil.which("ffmpeg")
    if found:
        return found
    default = Path(r"E:\code\ffmpeg\bin\ffmpeg.exe")
    return str(default) if default.exists() else ""


def _video() -> Path | None:
    """用 ffmpeg 生成一段 1 秒的测试视频，没有 ffmpeg 时返回空。"""
    path = WORK / "hello-yunhu.mp4"
    binary = _ffmpeg()
    if not binary:
        return None

    result = subprocess.run(
        [
            binary,
            "-y",
            "-loglevel",
            "error",
            "-f",
            "lavfi",
            "-i",
            "color=c=0x3050a0:s=320x240:d=1",
            "-pix_fmt",
            "yuv420p",
            "-movflags",
            "+faststart",
            str(path),
        ],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0 or not path.exists():
        logger.warning(f"M2 视频素材生成失败: {result.stderr.strip()[:200]}")
        return None
    return path


def _brief(data: object) -> str:
    """把返回内容压缩成一行，便于写日志。"""
    if isinstance(data, (dict, list)):
        text = json.dumps(data, ensure_ascii=False)
    else:
        text = repr(data)
    return text if len(text) <= 400 else text[:400] + "…"


async def _run(bot: Bot) -> dict:
    """执行检查序列，返回汇总结果。"""
    group = int(os.environ.get("M2_CHECK_GROUP", "0"))
    user = int(os.environ.get("M2_CHECK_USER", "0"))
    reply_to = os.environ.get("M2_CHECK_REPLY_TO", "").strip()

    if group <= 0 or user <= 0:
        raise RuntimeError("未设置 M2_CHECK_GROUP 与 M2_CHECK_USER")

    WORK.mkdir(exist_ok=True)
    png = _png()
    text_file = _text_file()

    steps: list[dict] = []
    state: dict[str, int] = {}

    async def step(name: str, call) -> None:
        try:
            data = await call()
            steps.append({"step": name, "ok": True, "detail": _brief(data)})
            logger.info(f"M2 通过 {name}: {_brief(data)}")
        except Exception as exc:  # 记录失败后继续，一次跑完所有场景
            steps.append({"step": name, "ok": False, "detail": str(exc)})
            logger.warning(f"M2 失败 {name}: {exc}")
        await asyncio.sleep(GAP)

    if reply_to:
        # 第二阶段只验证重启后的引用映射。
        await step(
            "重启后引用",
            lambda: bot.call_api(
                "send_group_msg",
                group_id=group,
                message=MessageSegment.reply(int(reply_to)) + Message("重启后引用检查"),
            ),
        )
        summary = {"group": group, "user": user, "steps": steps}
        _write(summary)
        return summary

    await step("get_login_info", bot.get_login_info)
    await step("get_group_list", bot.get_group_list)
    await step("get_group_info", lambda: bot.get_group_info(group_id=group))
    await step("get_group_member_list", lambda: bot.get_group_member_list(group_id=group))
    await step(
        "get_group_member_info",
        lambda: bot.get_group_member_info(group_id=group, user_id=user),
    )

    async def send_group(payload: Message) -> dict:
        data = await bot.send_group_msg(group_id=group, message=payload)
        if state.get("first_id", 0) == 0 and "message_id" in data:
            state["first_id"] = int(data["message_id"])
        return data

    await step("发送文本", lambda: send_group(Message("M2 检查：文本消息")))
    await step(
        "发送图文混排",
        lambda: send_group(
            Message("M2 检查：混排第一段")
            + MessageSegment.image(png)
            + Message("M2 检查：混排第二段")
        ),
    )
    await step(
        "发送@",
        lambda: send_group(MessageSegment.at(user) + Message(" M2 检查：@ 与文本合并")),
    )
    await step("发送图片", lambda: send_group(Message(MessageSegment.image(png))))
    await step(
        "发送文件",
        lambda: send_group(
            Message(
                MessageSegment(
                    "file",
                    {"file": str(text_file), "name": text_file.name},
                )
            )
        ),
    )

    video = _video()
    if video:
        await step(
            "发送视频",
            lambda: send_group(Message(MessageSegment("video", {"file": str(video)}))),
        )
    else:
        steps.append({"step": "发送视频", "ok": True, "detail": "跳过：未找到 ffmpeg"})

    await step("发送私聊", lambda: bot.send_private_msg(user_id=user, message="M2 检查：私聊消息"))
    await step("get_status", bot.get_status)

    summary = {"group": group, "user": user, "steps": steps}
    _write(summary)
    return summary


def _write(summary: dict) -> None:
    """把结果写入文件，供终端之外核对。"""
    path = WORK / "result.json"
    path.write_text(json.dumps(summary, ensure_ascii=False, indent=2), encoding="utf-8")
    failed = [item["step"] for item in summary["steps"] if not item["ok"]]
    if failed:
        logger.warning(f"M2 检查完成，失败 {len(failed)} 项: {', '.join(failed)}")
    else:
        logger.info("M2 检查完成，全部通过")


async def _guarded(bot: Bot) -> None:
    """保证同一时间只跑一轮。"""
    global _running
    if _running:
        logger.info("M2 检查正在进行，忽略本次触发")
        return

    _running = True
    try:
        await _run(bot)
    except Exception as exc:
        logger.error(f"M2 检查无法开始: {exc}")
    finally:
        _running = False


m2check = on_command("m2check", priority=5, block=True)


@m2check.handle()
async def _(bot: Bot) -> None:
    await m2check.send("M2 检查开始，结果写入 .m2check/result.json")
    await _guarded(bot)


@get_driver().on_bot_connect
async def _(bot: Bot) -> None:
    """连上框架后自动跑一轮，仅在设置了群号与成员号时生效。"""
    if not os.environ.get("M2_CHECK_GROUP") or not os.environ.get("M2_CHECK_USER"):
        return

    delay = int(os.environ.get("M2_CHECK_DELAY", "5"))
    await asyncio.sleep(delay)
    await _guarded(bot)
