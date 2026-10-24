"""在线握手诊断：dump ConnectionRequest 发送 + 所有响应包。

用于排查"发了 ConnectionRequest 但收不到 ConnectionRequestAccepted"。
跑：python3 -m tools.raknet_online_debug 127.0.0.1 19132
"""

from __future__ import annotations

import asyncio
import socket
import sys
import time

from proloader.net.raknet.connection import handshake
from proloader.net.raknet.constants import Reliability
from proloader.net.raknet.frames import Frame, FrameSet
from proloader.net.raknet.session import build_connection_request


async def main(host: str, port: int) -> int:
    conn = await handshake(host, port, timeout=10.0)
    print(f"离线握手 OK: server_guid={conn.server_guid} mtu={conn.mtu} client_guid={conn.client_guid}")
    sock = conn.sock
    sock.settimeout(2.0)

    # 构造 ConnectionRequest
    now_ms = int(time.time() * 1000) & 0xFFFFFFFFFFFFFFFF
    req_body = build_connection_request(conn.client_guid, now_ms, use_security=False)
    print(f"\nConnectionRequest body [{len(req_body)} bytes]: {req_body.hex()}")

    # 打包成 Frame (RELIABLE_ORDERED=3) + FrameSet
    frame = Frame(
        body=req_body, reliability=Reliability.RELIABLE_ORDERED,
        frame_index=0, order_index=0, order_channel=0,
    )
    fs = FrameSet(sequence_number=0, frames=[frame])
    packet = fs.encode()
    print(f"\n发送 FrameSet [{len(packet)} bytes]: {packet.hex()}")
    # 拆解字段以便核对
    print(f"  [0] 0x{packet[0]:02x} (FrameSet 标志)")
    seq = packet[1] | (packet[2] << 8) | (packet[3] << 16)
    print(f"  [1-3] sequence_number={seq} (LE u24)")
    print(f"  [4] flags=0x{packet[4]:02x} (reliability<<5={packet[4]>>5})")
    blen = (packet[5] << 8) | packet[6]
    print(f"  [5-6] body_length_field=0x{blen:04x} → >>3 = {blen>>3} 字节 (body 真实长度={len(req_body)})")

    sock.sendto(packet, (host, port))
    print(f"已发送 → {host}:{port}")

    # 收所有包 8 秒
    print("\n=== 监听响应 (8s) ===")
    deadline = time.time() + 8
    count = 0
    while time.time() < deadline:
        try:
            data, addr = sock.recvfrom(4096)
        except socket.timeout:
            continue
        count += 1
        print(f"\n← 收到 #{count} [{len(data)} bytes] from {addr}")
        print(f"  首字节 0x{data[0]:02x}")
        print(f"  hex[:64]: {data[:64].hex()}")
        if data[0] == 0x80 or (data[0] & 0xF0) == 0x80:  # FrameSet (0x80-0x8F)
            try:
                fs2 = FrameSet.decode(data)
                print(f"  FrameSet seq={fs2.sequence_number} frames={len(fs2.frames)}")
                for i, f in enumerate(fs2.frames):
                    print(f"    frame[{i}] rel={f.reliability} body[{len(f.body)}]:")
                    print(f"      完整 hex: {f.body.hex()}")
                    if f.body and f.body[0] == 0x10:
                        print(f"      → 这是 ConnectionRequestAccepted，逐字段解析见下")
                        b = f.body
                        print(f"      [0] 0x10")
                        print(f"      [1:9] clientGUID候选 = {b[1:9].hex()}")
                        print(f"      [9] version候选 = 0x{b[9]:02x} ({b[9]})")
            except Exception as e:
                print(f"  FrameSet 解析失败: {e}")
        elif data[0] == 0xC0:
            print("  → ACK")
        elif data[0] == 0xA0:
            print("  → NACK")
    print(f"\n=== 共收到 {count} 个包 ===")
    sock.close()
    return 0


if __name__ == "__main__":
    h = sys.argv[1] if len(sys.argv) > 1 else "127.0.0.1"
    p = int(sys.argv[2]) if len(sys.argv) > 2 else 19132
    sys.exit(asyncio.run(main(h, p)))
