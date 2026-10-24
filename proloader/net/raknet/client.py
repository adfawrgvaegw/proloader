"""RakNet 客户端 —— UDP 连接 + 离线握手。

本模块实现 RakNet 的离线阶段（最常用、最易验证的部分）：
    - Unconnected Ping: 服务器列表查询（"这个服在线吗？MOTD 是啥？"）
    - Open Connection Request 1/2: 建立连接（后续接在线帧阶段）

学习要点：
    - RakNet 跑在 UDP 上，用 Python socket.SOCK_DGRAM
    - Unconnected Ping 是最轻量的真实协议交互：发一个包、收一个包
      能 ping 通真实公共基岩服，就证明我们的协议编解码是对的
    - time 字段是客户端发出去再被服务器原样回显，用于测往返时延 RTT
    - clientGUID 是本地随机 i64，标识本连接

验证方法：
    python3 -m proloader.net.raknet.client ping <host> <port>
    例: python3 -m proloader.net.raknet.client ping mco.cubecraft.net 19132
"""

from __future__ import annotations

import asyncio
import logging
import random
import socket
import struct
import time
from dataclasses import dataclass
from typing import Optional, Tuple

from proloader.net.raknet.constants import (
    DEFAULT_PORT,
    OfflinePacketID,
    MAGIC,
)
from proloader.net.raknet.wire import ByteReader, ByteWriter, parse_unconnected_pong

logger = logging.getLogger(__name__)


@dataclass
class ServerInfo:
    """Unconnected Ping 解析出的服务器信息。"""

    guid: int
    protocol_version: int
    edition: str  # MCPE / MCEE
    motd_line1: str
    motd_line2: str
    version_name: str
    player_count: int
    max_players: int
    gamemode: str
    port_ipv4: int
    port_ipv6: int
    raw: str  # 原始 MOTD 串
    rtt_ms: float  # 往返时延

    @classmethod
    def from_motd(cls, guid: int, protocol: int, motd: str, rtt_ms: float) -> "ServerInfo":
        """从 MOTD 字符串解析。

        格式: MCPE;<name>;<protocol>;<version>;<players>;<max>;<serverID>;<motd2>;<gamemode>;<gamemodeNum>;<port4>;<port6>
        """
        fields = motd.split(";")
        # 字段数可能因版本不同有出入，做防御性解析
        def _get(i: int, default: str = "") -> str:
            return fields[i] if i < len(fields) else default

        try:
            players = int(_get(4, "0"))
        except ValueError:
            players = 0
        try:
            max_players = int(_get(5, "20"))
        except ValueError:
            max_players = 20
        try:
            port4 = int(_get(10, "0"))
        except ValueError:
            port4 = 0
        try:
            port6 = int(_get(11, "0"))
        except ValueError:
            port6 = 0

        return cls(
            guid=guid,
            protocol_version=protocol or int(_get(2, "0") or 0),
            edition=_get(0, "MCPE"),
            motd_line1=_get(1),
            version_name=_get(3),
            player_count=players,
            max_players=max_players,
            gamemode=_get(8),
            port_ipv4=port4,
            port_ipv6=port6,
            motd_line2=_get(7),
            raw=motd,
            rtt_ms=rtt_ms,
        )


class RakNetClient:
    """RakNet 客户端。

    目前实现离线阶段（Unconnected Ping + Open Connection Request 1/2）。
    在线帧阶段（Frame Set、ACK、分片重组）留作后续扩展——
    对接网易需要走 NeOmega 核心（见 backend/neomega_backend.py），
    所以这里聚焦于"能验证协议正确"的离线握手。
    """

    def __init__(self) -> None:
        self._guid: int = random.getrandbits(63)

    @property
    def guid(self) -> int:
        return self._guid

    def _build_unconnected_ping(self, timestamp_ms: int) -> bytes:
        """构造 Unconnected Ping 包。

        格式: 0x01 | time(u64 BE) | MAGIC(16) | clientGUID(i64 BE)
        ⚠️ RakNet 离线消息全大端（go-raknet 确认）。
        """
        w = ByteWriter()
        w.write_byte(OfflinePacketID.UNCONNECTED_PING)
        w.write(struct.pack(">Q", timestamp_ms & 0xFFFFFFFFFFFFFFFF))  # 大端 u64
        w.write_magic()
        w.write(struct.pack(">q", self._guid))  # 大端 i64
        return w.to_bytes()

    async def ping(self, host: str, port: int = DEFAULT_PORT, timeout: float = 5.0) -> ServerInfo:
        """Unconnected Ping：查询服务器 MOTD。

        这是验证 RakNet 协议实现是否正确的最简方法——
        发一个 UDP 包，收一个 UDP 包，解析 Pong。

        Args:
            host: 服务器地址
            port: 端口（默认 19132）
            timeout: 超时秒数

        Returns:
            ServerInfo: 服务器信息

        Raises:
            TimeoutError: 服务器无响应
        """
        loop = asyncio.get_event_loop()
        sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        try:
            sock.settimeout(timeout)

            timestamp = int(time.time() * 1000)
            ping_packet = self._build_unconnected_ping(timestamp)
            logger.debug("ping %s:%d (%d bytes)", host, port, len(ping_packet))

            t0 = time.monotonic()
            await loop.run_in_executor(None, sock.sendto, ping_packet, (host, port))

            # 收 Pong（可能收到多个无关包，循环找 0x1C）
            deadline = t0 + timeout
            while True:
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    raise TimeoutError(f"ping {host}:{port} 超时（{timeout}s 无响应）")
                sock.settimeout(remaining)
                try:
                    data, _ = await loop.run_in_executor(None, sock.recvfrom, 4096)
                except socket.timeout:
                    raise TimeoutError(f"ping {host}:{port} 超时（{timeout}s 无响应）")

                if not data:
                    continue
                if data[0] == OfflinePacketID.UNCONNECTED_PONG:
                    rtt_ms = (time.monotonic() - t0) * 1000.0
                    guid, proto, motd = parse_unconnected_pong(data)
                    return ServerInfo.from_motd(guid, proto, motd, rtt_ms)
                # 不是 Pong，继续等
                logger.debug("收到非 Pong 包 0x%02x，继续等待", data[0])
        finally:
            sock.close()

    def _build_open_connection_request_1(self, protocol_version: int, mtu_size: int) -> bytes:
        """构造 Open Connection Request 1。

        格式: 0x05 | MAGIC(16) | protocol(u8) | MTU padding
        MTU 协商：客户端用 padding 把包撑到期望 MTU，服务器回 Reply 1 确认。

        Note: 完整在线握手需后续 Reply 1/2，这里只构造 Request 1 供学习。
        """
        w = ByteWriter()
        w.write_byte(OfflinePacketID.OPEN_CONNECTION_REQUEST_1)
        w.write_magic()
        w.write_byte(protocol_version)
        # padding 到 MTU（MTU - 头部开销）。常见 MTU: 1400/1492/1500
        # 这里只示意，实际需精确填充
        padding_len = max(0, mtu_size - len(w.to_bytes()) - 1)
        w.write(b"\x00" * padding_len)
        return w.to_bytes()


# ═══════════════════════════════════════════════════════════════════════════════
# CLI 入口：直接 ping 真实服务器验证协议
# ═══════════════════════════════════════════════════════════════════════════════
async def _cli_ping(host: str, port: int) -> None:
    client = RakNetClient()
    print(f"正在 ping {host}:{port} ...")
    info = await client.ping(host, port)
    print(f"✓ 收到响应（RTT {info.rtt_ms:.1f} ms）")
    print(f"  版本:     {info.edition} 协议 v{info.protocol_version} ({info.version_name})")
    print(f"  MOTD:     {info.motd_line1}")
    print(f"  在线/上限: {info.player_count}/{info.max_players}")
    print(f"  游戏模式:  {info.gamemode}")
    print(f"  端口:     IPv4={info.port_ipv4} IPv6={info.port_ipv6}")
    print(f"  服务器 GUID: {info.guid}")
    print(f"  原始:     {info.raw}")


def main() -> None:
    import sys

    logging.basicConfig(level=logging.INFO, format="%(message)s")
    if len(sys.argv) < 4 or sys.argv[1] != "ping":
        print("用法: python3 -m proloader.net.raknet.client ping <host> <port>")
        print("示例: python3 -m proloader.net.raknet.client ping mco.cubecraft.net 19132")
        sys.exit(1)
    host = sys.argv[2]
    port = int(sys.argv[3])
    asyncio.run(_cli_ping(host, port))


if __name__ == "__main__":
    main()
