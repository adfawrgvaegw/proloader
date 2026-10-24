"""诊断：登录成功后 dump 服务器发的所有包（按顺序记录 packet_id + 大小 + 前 64 字节）。

用法：
    python3 -m tools.dump_login_packets <host> <port>

目的：搞清 protocol 2168 的初始化包序列，特别是 ResourcePackStack 之前/之后有什么。
"""

from __future__ import annotations

import asyncio
import logging
import sys

from proloader.net.bedrock.session import BedrockSession
from proloader.net.bedrock.packets import PacketID
from proloader.net.raknet.connection import handshake
from proloader.net.raknet.session import RakNetSession
from proloader.net.raknet.wire import ByteReader

# 已知 packet_id → 名称
PID_NAMES = {v: k for k, v in vars(PacketID).items() if isinstance(v, int)}


def pid_name(pid: int) -> str:
    return PID_NAMES.get(pid, f"UNKNOWN_{pid}")


async def run(host: str, port: int) -> int:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s | %(message)s",
        datefmt="%H:%M:%S",
    )

    conn = await handshake(host, port, timeout=10.0)
    raknet = RakNetSession(conn)
    await raknet.start()
    await raknet.connect(timeout=10.0)

    bedrock = BedrockSession(raknet)
    await bedrock.login(server_addr=f"{host}:{port}", timeout=15.0)
    print("✓ LOGIN_SUCCESS，开始 dump 服务器发的包（15 秒）\n")

    # 发 HAVE_ALL 响应 ResourcePacksInfo
    from proloader.net.bedrock.login import build_resource_pack_client_response
    # ResourcePacksInfo 应该已经在 inbox 里了
    # 先尝试收 ResourcePacksInfo
    try:
        rpi = await asyncio.wait_for(bedrock.recv_packet(), timeout=3.0)
        r = ByteReader(rpi)
        pid = r.read_unsigned_varint() & 0x3FF
        print(f"→ 收到 {pid_name(pid)} (id={pid}) [{len(rpi)}B], 发 HAVE_ALL")
        await bedrock.send_packet(build_resource_pack_client_response(status=2))
        print("→ 已发 ResourcePackClientResponse(HAVE_ALL)\n")
    except asyncio.TimeoutError:
        print("（3s 内没收到 ResourcePacksInfo，可能已在 login 中消费）")

    # 收 15 秒所有包
    print("=== 收包 dump（15 秒）===")
    count = 0
    while True:
        try:
            pkt = await asyncio.wait_for(bedrock.recv_packet(), timeout=15.0)
            count += 1
            r = ByteReader(pkt)
            value = r.read_unsigned_varint()
            pid = value & 0x3FF
            rest = pkt[r.pos:]
            print(f"  [{count:3d}] id={pid:4d} ({pid_name(pid):30s}) [{len(pkt):5d}B] {rest[:48].hex()}")
        except asyncio.TimeoutError:
            print(f"\n=== 15 秒内共收到 {count} 个包 ===")
            break
        except Exception as e:
            print(f"  收包异常: {e}")
            break

    await bedrock.close()
    return 0


def main() -> None:
    if len(sys.argv) < 3:
        print("用法: python3 -m tools.dump_login_packets <host> <port>")
        sys.exit(1)
    rc = asyncio.run(run(sys.argv[1], int(sys.argv[2])))
    sys.exit(rc)


if __name__ == "__main__":
    main()
