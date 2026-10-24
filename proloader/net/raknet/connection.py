"""RakNet 连接握手（OpenConnection 1/2 + Reply 1/2）。

这是建立 RakNet 连接的四步握手，在 Unconnected Ping 之后执行。
握手成功后进入"在线"状态，开始用 FrameSet（0x80）收发可靠帧。

⚠️ 字节序关键规则（Gophertunnel/go-raknet 确认）：
    RakNet 离线消息（OpenConnection 等）**全部大端**！
    这和 Bedrock 层（小端为主）不同，是最容易踩的坑。
    之前版本误用小端，连真实 BDS 会失败。

握手时序（客户端视角）：
    客户端 ─── OpenConnectionRequest1 ──> 服务端   (MTU 探测)
    客户端 <── OpenConnectionReply1 ──── 服务端
    客户端 ─── OpenConnectionRequest2 ──> 服务端
    客户端 <── OpenConnectionReply2 ──── 服务端
    ── 进入在线帧阶段（ConnectionRequest 等）──

包格式（全大端，参考 go-raknet/internal/message/）：

    OCR1:  0x05 | MAGIC(16) | protocol(u8=11) | padding(填充到 MTU-28 字节)
    Reply1: 0x06 | MAGIC(16) | server_guid(i64 BE) | security(u8) | [cookie(u32 BE)] | MTU(u16 BE)
    OCR2:  0x07 | MAGIC(16) | [cookie(u32 BE)+security response(u8)] | server_addr(SystemAddress) | MTU(u16 BE) | client_guid(i64 BE)
    Reply2: 0x08 | server_guid(i64 BE) | client_addr(SystemAddress) | MTU(u16 BE) | security(u8)

SystemAddress 格式（go-raknet quirk：IP 字节取反 + 端口大端）：
    version(u8=4) | ip[4](每字节 ^0xFF) | port(u16 BE)

学习要点：
    - MTU 协商：OCR1 包大小 = MTU-28（IP头20+UDP头8），靠 padding 填充
    - client_guid 必须是负数（vanilla 客户端行为）
    - Cookie 安全机制：BDS 默认开启，Reply1 的 security=1 时带 cookie，OCR2 要回带
"""

from __future__ import annotations

import asyncio
import logging
import random
import socket
import struct
import time
from dataclasses import dataclass
from typing import List, Optional, Tuple

from proloader.net.raknet.constants import (
    DEFAULT_PORT,
    MAGIC,
    OfflinePacketID,
)
from proloader.net.raknet.wire import ByteReader, ByteWriter

logger = logging.getLogger(__name__)

# RakNet 协议版本（go-raknet/conn.go: protocolVersion = 11）
# 注意：这是 RakNet 协议版本，不是 Bedrock 游戏协议版本（2168）
RAKNET_PROTOCOL_VERSION: int = 11

# MTU 候选（go-raknet/dial.go: {1492, 1200, 576}，从大到小尝试）
MTU_CANDIDATES: Tuple[int, ...] = (1492, 1200, 576)
DEFAULT_MTU: int = 1400

# 离线握手可能的错误响应包 ID（BDS 在 OCR1/OCR2 阶段可能回这些而不是 Reply）
# 这些都是带 MAGIC 的离线消息
ID_ALREADY_CONNECTED: int = 0x12           # 该客户端已被占用（残留连接）
ID_CONNECTION_ATTEMPT_FAILED: int = 0x11   # 服务端拒绝（满员等）
ID_INCOMPATIBLE_PROTOCOL_VERSION: int = 0x19  # RakNet 协议版本不匹配


# ═══════════════════════════════════════════════════════════════════════════════
# SystemAddress：RakNet 地址三元组（IP 字节取反 + 端口大端）
# ═══════════════════════════════════════════════════════════════════════════════
@dataclass
class SystemAddress:
    """RakNet 系统地址。

    ⚠️ go-raknet 的地址编码有两个 quirk：
        1. IPv4 的每个字节取反：^ip[i]（即 255-ip[i]）
        2. 端口是大端（和其它大端数值一致）
    """

    ip: str
    port: int
    version: int = 4  # 4=IPv4

    def encode(self, w: ByteWriter) -> None:
        """写入字节流。"""
        w.write_byte(self.version)
        if self.version == 4:
            # IP 字节取反（go-raknet quirk）
            for b in socket.inet_aton(self.ip):
                w.write_byte(b ^ 0xFF)
        else:
            raise NotImplementedError("IPv6 SystemAddress 暂未实现")
        # 端口大端
        w.write(struct.pack(">H", self.port & 0xFFFF))

    @classmethod
    def decode(cls, r: ByteReader) -> "SystemAddress":
        version = r.read_byte()
        if version == 4 or version == 0:  # go-raknet 兼容 version=0
            raw = [r.read_byte() for _ in range(4)]
            ip = ".".join(str(b ^ 0xFF) for b in raw)  # 取反还原
            port = struct.unpack(">H", r.read(2))[0]  # 大端
            return cls(ip=ip, port=port, version=4)
        raise NotImplementedError(f"SystemAddress version {version} 暂未实现")


# ═══════════════════════════════════════════════════════════════════════════════
# OpenConnection 包编解码（全大端）
# ═══════════════════════════════════════════════════════════════════════════════
def build_open_connection_request_1(
    protocol: int = RAKNET_PROTOCOL_VERSION, mtu: int = DEFAULT_MTU
) -> bytes:
    """构造 OpenConnectionRequest1。

    格式：0x05 | MAGIC(16) | protocol(u8) | padding
    包总长度 = MTU - 28（IP头20 + UDP头8），padding 填充剩余字节。
    服务端通过收到的包总长度反推 MTU。
    """
    w = ByteWriter()
    w.write_byte(OfflinePacketID.OPEN_CONNECTION_REQUEST_1)
    w.write_magic()
    w.write_byte(protocol)
    # padding：包总长 = MTU - 28，已用 1+16+1=18 字节，padding = MTU-28-18 = MTU-46
    padding_len = mtu - 28 - 18
    if padding_len < 0:
        raise ValueError(f"MTU {mtu} 太小（最小 46）")
    w.write(b"\x00" * padding_len)
    return w.to_bytes()


@dataclass
class OpenConnectionReply1:
    server_guid: int
    use_security: bool
    mtu: int
    cookie: int = 0  # 仅当 use_security=True 时有效


def parse_open_connection_reply_1(data: bytes) -> OpenConnectionReply1:
    """解析 OpenConnectionReply1（全大端）。"""
    r = ByteReader(data)
    pid = r.read_byte()
    if pid != OfflinePacketID.OPEN_CONNECTION_REPLY_1:
        raise ValueError(f"不是 OpenConnectionReply1: 0x{pid:02x}")
    if r.read_magic() != MAGIC:
        raise ValueError("MAGIC 不匹配")
    server_guid = struct.unpack(">q", r.read(8))[0]  # 大端 i64
    use_security = r.read_bool()
    cookie = 0
    if use_security:
        cookie = struct.unpack(">I", r.read(4))[0]  # 大端 u32
    mtu = struct.unpack(">H", r.read(2))[0]  # 大端 u16
    return OpenConnectionReply1(
        server_guid=server_guid, use_security=use_security, mtu=mtu, cookie=cookie
    )


def build_open_connection_request_2(
    server_addr: SystemAddress,
    mtu: int,
    client_guid: int,
    cookie: int = 0,
    has_security: bool = False,
) -> bytes:
    """构造 OpenConnectionRequest2（全大端）。

    格式：0x07 | MAGIC(16) | [cookie(u32 BE) + security response(u8=0)] | server_addr | MTU(u16 BE) | client_guid(i64 BE)
    cookie 仅当服务端 Reply1 的 use_security=True 时带上。
    """
    w = ByteWriter()
    w.write_byte(OfflinePacketID.OPEN_CONNECTION_REQUEST_2)
    w.write_magic()
    if has_security:
        w.write(struct.pack(">I", cookie & 0xFFFFFFFF))  # 大端 u32
        # 参考 OpenConnectionRequest2.MarshalBinary 的 cookieOffset=5：
        # cookie后还有1字节clientSupportsSecurity，非4字节cookie即结束。
        w.write_byte(0)
    server_addr.encode(w)
    w.write(struct.pack(">H", mtu & 0xFFFF))  # 大端 u16
    w.write(struct.pack(">q", client_guid))  # 大端 i64
    return w.to_bytes()


@dataclass
class OpenConnectionReply2:
    server_guid: int
    client_addr: SystemAddress
    mtu: int
    use_encryption: bool


def parse_open_connection_reply_2(data: bytes) -> OpenConnectionReply2:
    """解析 OpenConnectionReply2（全大端，带 MAGIC）。

    格式：0x08 | MAGIC(16) | server_guid(i64 BE) | client_addr(SystemAddress) | MTU(u16 BE) | use_encryption(u8)
    ⚠️ Reply2 和 Reply1 一样带 MAGIC（都是离线消息），解析时必须先读 MAGIC 再读 server_guid，
       否则 MAGIC 的字节会被误当 server_guid，后续 client_addr.version 读到 MAGIC[8]=0xFD=253 报错。
    """
    r = ByteReader(data)
    pid = r.read_byte()
    if pid != OfflinePacketID.OPEN_CONNECTION_REPLY_2:
        raise ValueError(f"不是 OpenConnectionReply2: 0x{pid:02x}")
    if r.read_magic() != MAGIC:
        raise ValueError("MAGIC 不匹配")
    server_guid = struct.unpack(">q", r.read(8))[0]  # 大端 i64
    client_addr = SystemAddress.decode(r)
    mtu = struct.unpack(">H", r.read(2))[0]  # 大端 u16
    use_encryption = r.read_bool()
    return OpenConnectionReply2(
        server_guid=server_guid,
        client_addr=client_addr,
        mtu=mtu,
        use_encryption=use_encryption,
    )


# ═══════════════════════════════════════════════════════════════════════════════
# RakNetHandshake：完整的四步握手
# ═══════════════════════════════════════════════════════════════════════════════
@dataclass
class RakNetConnection:
    """握手成功后的连接信息。"""

    server_guid: int
    client_guid: int
    mtu: int
    sock: socket.socket
    server_addr: Tuple[str, int]
    cookie: int = 0
    has_security: bool = False


class RakNetHandshakeError(RuntimeError):
    """RakNet 握手失败。"""


class AlreadyConnectedError(RakNetHandshakeError):
    """BDS 认为该客户端已连接（残留连接）。

    通常是上一次连接没断干净，BDS 还记着这个 client_guid / 源地址。
    处理：换新 client_guid + 新 socket（新源端口）重试整个握手。
    """


def _new_client_guid() -> int:
    """生成负数 client_guid（vanilla 客户端行为：dialerID = -rand.Int64()）。"""
    return -(random.getrandbits(63) + 1)


async def _handshake_once(
    host: str,
    port: int,
    client_guid: int,
    mtu_candidates: Tuple[int, ...],
    timeout: float,
    protocol: int = RAKNET_PROTOCOL_VERSION,
) -> RakNetConnection:
    """执行一次完整的 RakNet 四步握手（不重试）。

    遇到 AlreadyConnected 抛 AlreadyConnectedError（让上层换 GUID/源端口重试）。
    遇到其它致命错误（协议不匹配、拒绝）抛 RakNetHandshakeError。
    """
    loop = asyncio.get_event_loop()
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)

    try:
        # ── 步骤 1+2：MTU 探测，发 OCR1，等 Reply1 ──────────────────────────
        reply1: Optional[OpenConnectionReply1] = None
        for mtu in mtu_candidates:
            req1 = build_open_connection_request_1(protocol=protocol, mtu=mtu)
            logger.debug("发 OCR1 (MTU=%d, %d bytes) → %s:%d", mtu, len(req1), host, port)
            await loop.run_in_executor(None, sock.sendto, req1, (host, port))
            try:
                sock.settimeout(timeout)
                data, _ = await loop.run_in_executor(None, sock.recvfrom, 4096)
            except socket.timeout:
                logger.debug("MTU=%d 无响应，尝试更小", mtu)
                continue

            pid = data[0]
            if pid == OfflinePacketID.OPEN_CONNECTION_REPLY_1:
                reply1 = parse_open_connection_reply_1(data)
                logger.info(
                    "收到 Reply1: server_guid=%d security=%s cookie=%d mtu=%d",
                    reply1.server_guid, reply1.use_security, reply1.cookie, reply1.mtu,
                )
                break
            if pid == ID_INCOMPATIBLE_PROTOCOL_VERSION:
                raise RakNetHandshakeError(
                    f"RakNet 协议版本不匹配（客户端={protocol}）。"
                    f"检查服务器是否基岩版。"
                )
            if pid == ID_ALREADY_CONNECTED:
                # OCR1 阶段就回 AlreadyConnected：残留连接，让上层重试
                raise AlreadyConnectedError(
                    f"BDS 在 OCR1 阶段回 AlreadyConnected（残留连接），需换 GUID/源端口重试"
                )
            logger.debug("OCR1 阶段收到非 Reply1 包 0x%02x，忽略", pid)

        if reply1 is None:
            raise RakNetHandshakeError(
                f"RakNet 握手失败：所有 MTU 候选 {mtu_candidates} 均未收到 Reply1\n"
                f"检查 {host}:{port} 是否在线、是否基岩版服务器"
            )

        # ── 步骤 3+4：发 OCR2（带 cookie 如果有），等 Reply2 ─────────────────
        server_addr = SystemAddress(ip=host, port=port)
        req2 = build_open_connection_request_2(
            server_addr=server_addr,
            mtu=reply1.mtu,
            client_guid=client_guid,
            cookie=reply1.cookie,
            has_security=reply1.use_security,
        )
        logger.debug("发 OCR2 (%d bytes, security=%s)", len(req2), reply1.use_security)
        await loop.run_in_executor(None, sock.sendto, req2, (host, port))

        deadline = time.monotonic() + timeout
        next_send = time.monotonic() + 0.5
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise RakNetHandshakeError("等待 Reply2 超时")
            sock.settimeout(min(remaining, max(0.001, next_send - time.monotonic())))
            try:
                data, _ = await loop.run_in_executor(None, sock.recvfrom, 4096)
            except socket.timeout:
                if time.monotonic() >= deadline:
                    raise RakNetHandshakeError("等待 Reply2 超时")
                # 对齐参考 dial.go request2：UDP 离线消息每 500ms 重发，
                # 保留同一 socket/GUID/cookie，并受总 deadline 限制。
                await loop.run_in_executor(None, sock.sendto, req2, (host, port))
                next_send = time.monotonic() + 0.5
                continue

            pid = data[0]
            if pid == OfflinePacketID.OPEN_CONNECTION_REPLY_2:
                reply2 = parse_open_connection_reply_2(data)
                logger.info(
                    "收到 Reply2: server_guid=%d client_addr=%s:%d mtu=%d enc=%s",
                    reply2.server_guid, reply2.client_addr.ip,
                    reply2.client_addr.port, reply2.mtu, reply2.use_encryption,
                )
                return RakNetConnection(
                    server_guid=reply2.server_guid,
                    client_guid=client_guid,
                    mtu=reply2.mtu,
                    sock=sock,
                    server_addr=(host, port),
                    cookie=reply1.cookie,
                    has_security=reply1.use_security,
                )
            if pid == ID_ALREADY_CONNECTED:
                raise AlreadyConnectedError(
                    "BDS 在 OCR2 阶段回 AlreadyConnected（残留连接），需换 GUID/源端口重试"
                )
            if pid == ID_CONNECTION_ATTEMPT_FAILED:
                raise RakNetHandshakeError(
                    "BDS 回 ConnectionAttemptFailed（服务端拒绝，可能满员或被 ban）"
                )
            if pid == ID_INCOMPATIBLE_PROTOCOL_VERSION:
                raise RakNetHandshakeError("RakNet 协议版本不匹配")
            logger.debug("等待 Reply2 时收到 0x%02x，忽略继续等", pid)
    except Exception:
        sock.close()
        raise


async def handshake(
    host: str,
    port: int = DEFAULT_PORT,
    timeout: float = 10.0,
    client_guid: Optional[int] = None,
    mtu_candidates: Tuple[int, ...] = MTU_CANDIDATES,
    max_retries: int = 2,
    protocol: int = RAKNET_PROTOCOL_VERSION,
) -> RakNetConnection:
    """执行完整的 RakNet 四步握手（带 AlreadyConnected 自动重试）。

    client_guid 不传时随机生成负数（vanilla 客户端行为：dialerID = -rand.Int64()）。
    遇到 AlreadyConnected（残留连接）时，换新 client_guid + 新 socket（新源端口）重试，
    最多重试 max_retries 次。若仍失败建议重启 BDS 清理连接表。
    """
    if client_guid is None:
        client_guid = _new_client_guid()

    last_err: Optional[Exception] = None
    for attempt in range(max_retries + 1):
        try:
            return await _handshake_once(host, port, client_guid, mtu_candidates, timeout, protocol)
        except AlreadyConnectedError as e:
            last_err = e
            if attempt < max_retries:
                logger.warning(
                    "AlreadyConnected（残留连接），换新 GUID + 新源端口重试 %d/%d",
                    attempt + 1, max_retries,
                )
                client_guid = _new_client_guid()
                await asyncio.sleep(0.5)
                continue
            raise RakNetHandshakeError(
                f"重试 {max_retries} 次仍收到 AlreadyConnected："
                f"BDS 连接表里有残留，请重启 BDS 清理。最后错误：{e}"
            ) from e
    # 理论上不会到这
    raise RakNetHandshakeError(f"RakNet 握手失败：{last_err}")
