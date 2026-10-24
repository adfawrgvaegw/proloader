"""机器人上下线控制示例（对齐用户要求：脚本里让机器人上线和下线）。

用法：
    python3 -m examples.bot_control 12345678
    python3 -m examples.bot_control 12345678 --passcode SECRET

流程：
    1. 机器人上线（自动读取 Cookie）
    2. 执行 list 命令查看在线玩家
    3. 执行 gamemode creative
    4. 机器人下线
"""

from __future__ import annotations

import argparse
import asyncio
import sys
from pathlib import Path

# 让 examples/ 能 import proloader
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from proloader.config import ControlConfig, auto_refresh_cookie, check_cookie_valid
from proloader.control import Control


async def main(code: str, passcode: str = "", commands: list = None) -> int:
    """机器人上线 → 执行命令 → 下线。"""
    cfg = ControlConfig(rental_server_code=code, rental_server_passcode=passcode)
    ctl = Control(cfg)

    # ── 1. 上线 ──
    print("[1/3] 机器人上线中...")
    cookie = auto_refresh_cookie()
    ok, fields = check_cookie_valid(cookie)
    if not ok:
        print(f"  ❌ Cookie 无效: {fields}")
        print("  请先在 Mac 网易客户端登录并进入租赁服一次。")
        return 1

    try:
        rep = await ctl.bot_online(cookie)
    except Exception as e:
        print(f"  ❌ 上线失败: {e}")
        import traceback; traceback.print_exc()
        return 1

    print(f"  ✅ 上线成功!")
    print(f"     假人昵称:  {rep.user_name}")
    print(f"     用户 ID:   {rep.user_id}")
    print(f"     RuntimeID: {rep.runtime_id}")
    print(f"     服务器:    {rep.host}:{rep.port}")
    print(f"     耗时:      {rep.elapsed_sec:.1f}s")

    # ── 2. 执行命令 ──
    print("\n[2/3] 执行命令...")
    cmd_list = commands or ["list", "gamemode creative"]
    for cmd in cmd_list:
        try:
            # aiCommand 通道（绕过 MCP），自动用假人昵称做相对坐标
            result = await ctl.run_command_ai(cmd, timeout=8.0)
            status = "✅" if result is not None else "⚠️ (无响应)"
            print(f"  {status} {cmd}")
            if result:
                print(f"      → {str(result)[:100]}")
        except Exception as e:
            print(f"  ❌ {cmd}: {e}")

    # ── 3. 下线 ──
    print("\n[3/3] 机器人下线...")
    try:
        await ctl.bot_offline()
        print("  ✅ 已下线")
    except Exception as e:
        print(f"  ⚠️ 下线异常: {e}")

    return 0


def app() -> None:
    parser = argparse.ArgumentParser(description="proloader 机器人上下线控制示例")
    parser.add_argument("code", help="租赁服编号")
    parser.add_argument("--passcode", default="", help="租赁服密码（无密码则省略）")
    parser.add_argument("--cmd", action="append", default=None,
                        help="要执行的命令（可重复），默认 list + gamemode creative")
    args = parser.parse_args()
    rc = asyncio.run(main(args.code, args.passcode, args.cmd))
    sys.exit(rc)


if __name__ == "__main__":
    app()
