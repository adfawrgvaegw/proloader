"""NeOmega 接入点连接测试 —— 真机验证脚本。

用法（前提：fastbuilder 已运行，假人已进服，接入点在 tcp://localhost:24013）：
    python3 tools/test_neomega_connect.py
    python3 tools/test_neomega_connect.py --x 10 --y 64 --z 20 --block stone

脚本做三件事：
    1. 加载 NeOmega 原生库
    2. 连接接入点
    3. 发 /setblock 放一个方块

成功标志：
    - 脚本打印 "✓ 方块已放置"
    - 游戏里对应坐标出现该方块（肉眼确认）
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

# 把项目根目录加到 sys.path，方便直接运行
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from proloader.backend.neomega.config import NeOmegaConfig
from proloader.backend.neomega.core import NeOmegaCore, NeOmegaCoreError


def main() -> int:
    parser = argparse.ArgumentParser(description="NeOmega 接入点连接测试")
    parser.add_argument("--lib", default="bin/neomega_macos_arm64.dylib", help="原生库路径")
    parser.add_argument("--addr", default="tcp://localhost:24013", help="接入点地址")
    parser.add_argument("--x", type=int, default=0, help="放置坐标 X")
    parser.add_argument("--y", type=int, default=64, help="放置坐标 Y")
    parser.add_argument("--z", type=int, default=0, help="放置坐标 Z")
    parser.add_argument("--block", default="stone", help="方块名（默认 stone）")
    parser.add_argument("--via", default="websocket",
                        choices=["websocket", "player", "settings"], help="命令通道")
    parser.add_argument("--need-response", action="store_true", help="等命令回执（更可靠但可能超时）")
    args = parser.parse_args()

    lib_path = args.lib
    if not Path(lib_path).exists():
        # 尝试相对项目根目录
        alt = Path(__file__).resolve().parent.parent / lib_path
        if alt.exists():
            lib_path = str(alt)
        else:
            print(f"✗ 原生库不存在: {lib_path}")
            print("  从 https://github.com/ToolDelta-Basic/DependencyLibrary 下载")
            return 1

    cfg = NeOmegaConfig(
        lib_path=lib_path,
        access_point_addr=args.addr,
        place_via=args.via,
        need_response=args.need_response,
    )

    core = NeOmegaCore(cfg)

    # 1. 加载库
    print(f"[1/3] 加载原生库: {lib_path}")
    try:
        core.load()
    except NeOmegaCoreError as e:
        print(f"✗ 加载失败: {e}")
        return 1
    print("  ✓ 库加载成功")

    # 2. 连接接入点
    print(f"[2/3] 连接接入点: {args.addr}")
    print("  （若卡住：确认 fastbuilder 已运行 + 假人已进服）")
    try:
        core.connect(retries=3, interval=3.0)
    except NeOmegaCoreError as e:
        print(f"✗ 连接失败: {e}")
        print("\n排查清单:")
        print("  1. fastbuilder 是否在运行？（终端窗口没关）")
        print("  2. 假人是否已进服？（fastbuilder 显示入服成功）")
        print("  3. 接入点地址是否对？（默认 tcp://localhost:24013）")
        return 1
    print("  ✓ 已连接接入点")

    # 检查在线状态
    if core.available():
        print("  ✓ 假人在线")
    else:
        print("  ⚠ OmegaAvailable() 返回 0（假人可能未完全就绪，仍尝试发命令）")

    # 3. 放方块
    cmd = f"setblock {args.x} {args.y} {args.z} {args.block}"
    print(f"[3/3] 发命令: /{cmd}  (通道={args.via}, 回执={args.need_response})")
    try:
        if args.via == "settings":
            core.send_settings_command(cmd)
            print(f"  ✓ 命令已发送（settings 通道无回执，请游戏内肉眼确认 {args.block} 出现在 ({args.x},{args.y},{args.z})）")
        else:
            result = core.send_websocket_command(cmd, need_response=args.need_response) \
                if args.via == "websocket" \
                else core.send_player_command(cmd, need_response=args.need_response)
            if result is not None:
                print(f"  ✓ 回执: SuccessCount={result.SuccessCount}, success={result.success}")
                if result.OutputMessages:
                    for m in result.OutputMessages:
                        print(f"    - success={m.Success} msg={m.Message!r} params={m.Parameters}")
            else:
                print(f"  ✓ 命令已发送（fire-and-forget，请游戏内肉眼确认 {args.block} 出现在 ({args.x},{args.y},{args.z})）")
    except Exception as e:
        print(f"✗ 命令失败: {e}")
        core.disconnect()
        return 1

    print("\n" + "=" * 50)
    print("测试完成。进游戏看看方块有没有出现。")
    print("=" * 50)
    core.disconnect()
    return 0


if __name__ == "__main__":
    sys.exit(main())
