"""诊断：发 setblock 后抓完整的 PacketViolationWarning 包并逐字段解析。

用法：python3 -m tools.diag_violation <host> <port>
"""

from __future__ import annotations

import asyncio
import logging
import sys

from proloader.net.bedrock.session import BedrockSession
from proloader.net.bedrock.packets import PacketID
from proloader.net.bedrock.login import build_command_request
from proloader.net.raknet.connection import handshake
from proloader.net.raknet.session import RakNetSession
from proloader.net.raknet.wire import ByteReader


async def run(host: str, port: int) -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s | %(message)s", datefmt="%H:%M:%S")

    conn = await handshake(host, port, timeout=10.0)
    raknet = RakNetSession(conn)
    await raknet.start()
    await raknet.connect(timeout=10.0)

    bedrock = BedrockSession(raknet)
    await bedrock.login(server_addr=f"{host}:{port}", timeout=15.0)
    sg = await bedrock.initialize_game(timeout=20.0)
    print(f"✓ 初始化完成: runtime_id={sg.entity_runtime_id} unique_id={sg.entity_unique_id}")

    # 发 setblock
    pkt = build_command_request(
        command="setblock 0 10 0 gold_block",
        request_id="proloader_1",
        player_runtime_id=sg.entity_runtime_id,
        entity_unique_id=sg.entity_unique_id,
        origin=0,
    )
    print(f"\n→ 发 CommandRequest [{len(pkt)}B]: {pkt.hex()}")
    await bedrock.send_packet(pkt)

    # 收包，找 PacketViolationWarning (156)
    print("\n=== 收包（找 PacketViolationWarning id=156）===")
    for _ in range(200):
        try:
            data = await asyncio.wait_for(bedrock.recv_packet(), timeout=5.0)
        except asyncio.TimeoutError:
            print("（5s 超时，结束）")
            break
        r = ByteReader(data)
        pid = r.read_unsigned_varint() & 0x3FF
        if pid == 156:
            print(f"\n★★★ PacketViolationWarning 完整 [{len(data)}B]: {data.hex()}")
            # 解析（Gophertunnel 格式：varuint(156)|varint32(type)|varint32(sev)|varint32(pktid)|string(ctx)）
            r2 = ByteReader(data)
            r2.read_unsigned_varint()  # packet id
            vtype = r2.read_varint()
            vsev = r2.read_varint()
            vpid = r2.read_varint()
            vctx = r2.read_string()
            print(f"  Gophertunnel 格式解析: type={vtype} severity={vsev} packet_id={vpid}")
            print(f"  context: {vctx}")
            print(f"  remaining: {r2.remaining}")
            # 也试 LeviLamina 格式（带 protocol_version）
            r3 = ByteReader(data)
            r3.read_unsigned_varint()
            proto = r3.read_varint()
            t2 = r3.read_varint()
            s2 = r3.read_varint()
            p2 = r3.read_varint()
            print(f"  LeviLamina 格式尝试: proto={proto} type={t2} sev={s2} pktid={p2} remaining={r3.remaining}")
            break
        elif pid == 79:
            print(f"  ← CommandOutput [{len(data)}B]: {data.hex()}")
        else:
            print(f"  ← id={pid} [{len(data)}B]")

    await bedrock.close()
    return 0


def main() -> None:
    if len(sys.argv) < 3:
        print("用法: python3 -m tools.diag_violation <host> <port>")
        sys.exit(1)
    rc = asyncio.run(run(sys.argv[1], int(sys.argv[2])))
    sys.exit(rc)


if __name__ == "__main__":
    main()
