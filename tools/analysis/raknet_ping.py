"""最小 RakNet Unconnected Ping 诊断脚本（纯 socket，不依赖 proloader 包）。

用于区分"网络层 UDP 不通"还是"协议层包格式错"：
    - 本地连远程：若超时 → UDP 被安全组/防火墙拦
    - 远程连 127.0.0.1：若超时 → BDS 协议层问题（几乎不会）

Unconnected Ping 格式（全大端）:
    0x01 | time(u64 BE) | MAGIC(16) | client_guid(i64 BE)   共 33 字节
Unconnected Pong 响应:
    0x1C | time(u64 BE) | server_guid(i64 BE) | MAGIC(16) | name_len(u16 BE) | motd
"""

from __future__ import annotations

import socket
import struct
import sys
import time

MAGIC = bytes.fromhex("00ffff00fefefefefdfdfdfd12345678")


def ping(host: str, port: int, timeout: float = 5.0) -> int:
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    s.settimeout(timeout)
    t = int(time.time() * 1000) & 0xFFFFFFFFFFFFFFFF
    pkt = bytes([0x01]) + struct.pack(">Q", t) + MAGIC + struct.pack(">q", -123456789)
    print(f"→ 发 Unconnected Ping ({len(pkt)} bytes) 到 {host}:{port}")
    print(f"  原始: {pkt.hex()}")
    s.sendto(pkt, (host, port))
    try:
        data, addr = s.recvfrom(4096)
        print(f"← 收到响应 {len(data)} 字节 from {addr}, 首字节 0x{data[0]:02x}")
        print(f"  原始前 40: {data[:40].hex()}")
        if data[0] == 0x1C:
            pong_time = struct.unpack(">Q", data[1:9])[0]
            server_guid = struct.unpack(">q", data[9:17])[0]
            magic_ok = data[17:33] == MAGIC
            name_len = struct.unpack(">H", data[33:35])[0]
            motd = data[35:35 + name_len].decode("utf-8", errors="replace")
            print(f"  ✓ PONG! server_guid={server_guid} magic_ok={magic_ok}")
            print(f"  MOTD({name_len}): {motd[:120]}")
            return 0
        print(f"  ✗ 非 Pong 响应 0x{data[0]:02x}")
        return 2
    except socket.timeout:
        print(f"✗ {timeout}s 超时，无响应（UDP 可能被防火墙/安全组拦截）")
        return 1
    finally:
        s.close()


if __name__ == "__main__":
    if len(sys.argv) < 3:
        print("用法: python3 raknet_ping.py <host> <port>")
        sys.exit(1)
    sys.exit(ping(sys.argv[1], int(sys.argv[2])))
