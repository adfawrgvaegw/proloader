"""端到端测试：登录 → 游戏初始化 → 放一个方块（/setblock）。

用法：
    python3 -m tools.test_place_block <host> <port>

验收标准（阶段 1-G）：
    连一个 online-mode=false 的原版 BDS，登录 + 初始化 + 放一个方块，
    服务器控制台出现 "Set block at" 或 /testforblock 能查到该方块即通过。

前置条件：
    1. BDS 已启动，server.properties 设 online-mode=false
    2. permissions.json 允许 visitor 进服并使用命令
    3. 本脚本连 host:port（默认 19132）
"""

from __future__ import annotations

import asyncio
import logging
import sys

from proloader.net.bedrock.session import BedrockSession, BedrockLoginError
from proloader.net.raknet.connection import handshake
from proloader.net.raknet.session import RakNetSession


async def run(host: str, port: int) -> int:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s | %(message)s",
        datefmt="%H:%M:%S",
    )

    # ── 阶段 1：RakNet 离线握手 ──────────────────────────────
    print(f"=== [1/4] RakNet 离线握手 {host}:{port} ===")
    try:
        conn = await handshake(host, port, timeout=10.0)
    except Exception as e:
        print(f"✗ RakNet 离线握手失败：{e}")
        return 1
    print(f"✓ server_guid={conn.server_guid} mtu={conn.mtu}")

    # ── 阶段 2：RakNet 在线握手 ──────────────────────────────
    print("=== [2/4] RakNet 在线握手 ===")
    raknet = RakNetSession(conn)
    await raknet.start()
    try:
        await raknet.connect(timeout=10.0)
    except Exception as e:
        print(f"✗ RakNet 在线握手失败：{e}")
        await raknet.stop()
        return 2
    print("✓ RakNet 在线握手完成")

    # ── 阶段 3：Bedrock 登录 + 游戏初始化 ────────────────────
    print("=== [3/4] Bedrock 登录 + 游戏初始化 ===")
    bedrock = BedrockSession(raknet)
    try:
        await bedrock.login(server_addr=f"{host}:{port}", timeout=15.0)
        print("✓ LOGIN_SUCCESS")
        start_game = await bedrock.initialize_game(timeout=20.0)
        print(f"✓ 游戏初始化完成: runtime_id={start_game.entity_runtime_id}")
    except BedrockLoginError as e:
        print(f"✗ 登录/初始化失败：{e}")
        await bedrock.close()
        return 3
    except Exception as e:
        print(f"✗ 登录/初始化异常：{e}")
        import traceback
        traceback.print_exc()
        await bedrock.close()
        return 3

    # ── 阶段 4：放一个方块 ──────────────────────────────────
    # 用 /setblock 在玩家附近放一个金块（gold_block=41）
    # 坐标用相对玩家位置的偏移；这里直接用固定坐标 0, 10, 0
    #
    # ⚠️ 给 BDS 1~2 秒处理 SetLocalPlayerAsInitialised（玩家真正落地的最后一步）。
    #    Gophertunnel 官方测试在 DoSpawn 后也 sleep 2s 再发命令，否则可能收到
    #    PacketViolationWarning（玩家尚未完全生成）。
    print("=== [4/4] 放一个方块（/setblock 0 10 0 gold_block）===")
    await asyncio.sleep(1.5)
    try:
        result = await bedrock.run_command("setblock 0 10 0 gold_block", timeout=5.0)
        if result:
            print(f"✓ 收到 CommandOutput: {result[:64]}...")
        else:
            print("✓ 命令已发（无输出，可能正常）")
    except Exception as e:
        print(f"✗ 放方块异常：{e}")
        import traceback
        traceback.print_exc()

    # 等 2 秒看有没有服务端回包
    print("=== 等 2 秒收服务端回包 ===")
    try:
        for i in range(5):
            pkt = await asyncio.wait_for(bedrock.recv_packet(), timeout=2.0)
            from proloader.net.raknet.wire import ByteReader
            r = ByteReader(pkt)
            value = r.read_unsigned_varint()
            real_pid = value & 0x3FF
            print(f"  [{i+1}] packet_id={real_pid} {len(pkt)} bytes")
    except asyncio.TimeoutError:
        print("  （2s 内收包完毕）")

    print("=== 测试完成，断开 ===")
    await bedrock.close()
    return 0


def main() -> None:
    if len(sys.argv) < 3:
        print("用法: python3 -m tools.test_place_block <host> <port>")
        print("示例: python3 -m tools.test_place_block 127.0.0.1 19132")
        sys.exit(1)
    host = sys.argv[1]
    port = int(sys.argv[2])
    rc = asyncio.run(run(host, port))
    sys.exit(rc)


if __name__ == "__main__":
    main()
