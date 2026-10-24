"""端到端登录测试：RakNet 握手 + 在线会话 + Bedrock 登录全链路。

用法：
    python3 -m tools.test_bedrock_login <host> <port>

测试目标（阶段 1-C 验收）：
    连一个 online-mode=false 的原版 BDS，看到 PlayStatus(LOGIN_SUCCESS) 即通过。

怎么起一个测试 BDS：
    1. 从 minecraft.net 下载 BDS（Bedrock Dedicated Server）对应版本
    2. server.properties 设：online-mode=false
    3. permissions.json 允许 visitor 进服
    4. ./bedrock_server 启动
    5. 跑本脚本连 127.0.0.1:19132

注意：公共基岩服（CubeCraft 等）都是 online-mode=true，需要 Xbox Live 登录，
本脚本离线 JWT 连不上，只能连自己起的 online-mode=false BDS。
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

    print(f"=== 阶段1：RakNet 离线握手 {host}:{port} ===")
    try:
        conn = await handshake(host, port, timeout=10.0)
    except Exception as e:
        print(f"✗ RakNet 离线握手失败：{e}")
        return 1
    print(f"✓ RakNet 离线握手完成: server_guid={conn.server_guid} mtu={conn.mtu}")

    print("=== 阶段2：RakNet 在线握手（ConnectionRequest → Accepted → NewIncoming）===")
    raknet = RakNetSession(conn)
    await raknet.start()
    try:
        await raknet.connect(timeout=10.0)
    except Exception as e:
        print(f"✗ RakNet 在线握手失败：{e}")
        await raknet.stop()
        return 2
    print("✓ RakNet 在线握手完成")

    print("=== 阶段3：Bedrock 登录（RequestNetworkSettings → Login → 加密握手 → PlayStatus）===")
    bedrock = BedrockSession(raknet)
    try:
        await bedrock.login(server_addr=f"{host}:{port}", timeout=15.0)
    except BedrockLoginError as e:
        print(f"✗ Bedrock 登录失败：{e}")
        await bedrock.close()
        return 3
    except Exception as e:
        print(f"✗ Bedrock 登录异常：{e}")
        await bedrock.close()
        return 3

    print("=== ✓✓✓ Bedrock 登录成功（LOGIN_SUCCESS）===")
    print("=== 收 5 个游戏包看看（StartGame / ResourcePacksInfo 等）===")
    try:
        for i in range(5):
            packet = await asyncio.wait_for(bedrock.recv_packet(), timeout=10.0)
            pid = packet[0] if packet else -1
            # 读 varuint 头拿真实 packet id
            from proloader.net.bedrock.packets import PacketID
            from proloader.net.raknet.wire import ByteReader
            r = ByteReader(packet)
            value = r.read_unsigned_varint()
            real_pid = value & 0x3FF
            name = {v: k for k, v in vars(PacketID).items() if isinstance(v, int)}.get(real_pid, f"UNKNOWN_{real_pid}")
            print(f"  [{i+1}] packet_id={real_pid} ({name}) {len(packet)} bytes")
    except asyncio.TimeoutError:
        print("  （10s 内没收到更多包，可能是服务端在等 ResourcePack 响应，正常）")
    except Exception as e:
        print(f"  收包异常：{e}")

    print("=== 测试完成，断开连接 ===")
    await bedrock.close()
    return 0


def main() -> None:
    if len(sys.argv) < 3:
        print("用法: python3 -m tools.test_bedrock_login <host> <port>")
        print("示例: python3 -m tools.test_bedrock_login 127.0.0.1 19132")
        sys.exit(1)
    host = sys.argv[1]
    port = int(sys.argv[2])
    rc = asyncio.run(run(host, port))
    sys.exit(rc)


if __name__ == "__main__":
    main()
