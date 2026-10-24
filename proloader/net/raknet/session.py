"""RakNet 在线会话：收包循环 + ACK/NACK + 重组 + 在线握手 + 心跳。

这是把"单包编解码"串成"能真连 BDS"的核心层。OpenConnection 握手成功后，
所有 Bedrock 包都封装在 Frame（放进 FrameSet 0x80）里传输，本模块负责：
    1. 异步收 UDP 包，按第一字节分发：FrameSet / ACK / NACK / Disconnection
    2. 收到 FrameSet 立刻记下要回 ACK，按 sequence_number 排序投递 frame
    3. reliable 帧按 frame_index 去重，ordered 帧按 order_index+channel 保序
    4. split 帧按 split_id 重组
    5. 在线握手：ConnectionRequest → Accepted → NewIncomingConnection
    6. ConnectedPing 心跳（保活，否则服务端踢人）

⚠️ 地址编码（抓包 + Gophertunnel go-raknet 双重确认）：
    在线和离线的 SystemAddress 编码【完全相同】：
        IPv4: version(1=4) + ip[4](每字节 ^0xFF 取反) + port(2 大端)
        IPv6: version(1=6) + addrFamily(2 LE=10) + port(2 BE) + flowInfo(4 BE) + ip[16] + scopeId(4 BE)
    空槽位用 IPv4 0.0.0.0:0（version=4, ip=ffffffff, port=0），不是 version=0。
    本模块 OnlineAddress 就是按这个编码实现的。

在线控制包（都是 frame body，第一字节是包 ID）：
    0x09 ConnectionRequest:          clientGUID(u64 BE) | time(u64 BE) | useSecurity(u8)
    0x10 ConnectionRequestAccepted:  clientAddr | SystemIndex(u16 BE) | [20 SystemAddress] | requestTime(u64 BE) | replyTime(u64 BE)
        ⚠️ 没有 clientGUID 字段！开头直接是 clientAddr（用抓包 188B 实测 + Gophertunnel 源码确认）
    0x13 NewIncomingConnection:      serverAddr | [20 SystemAddress] | requestTime(u64 BE) | serverReplyTime(u64 BE)
    0x00 ConnectedPing:              time(u64 BE)
    0x03 ConnectedPong:              pingTime(u64 BE) | pongTime(u64 BE)
"""

from __future__ import annotations

import asyncio
from collections import deque
import logging
import socket
import struct
import time
from dataclasses import dataclass, field
from typing import Dict, List, Optional, Tuple

from proloader.net.raknet.connection import RakNetConnection
from proloader.net.raknet.constants import FramePacketID, Reliability
from proloader.net.raknet.frames import (
    AckRange,
    Frame,
    FrameSet,
    build_ack,
    build_nack,
    parse_ack,
    parse_nack,
)
from proloader.net.raknet.wire import ByteReader, ByteWriter

logger = logging.getLogger(__name__)

# 在线 SystemAddress 固定 20 个槽位（Gophertunnel 写死）
SYSTEM_ADDRESS_COUNT: int = 20

# 心跳间隔（秒）。BDS 默认 15 秒没收到任何东西就踢，留余量。
PING_INTERVAL: float = 5.0

# 待 ACK 超时（秒），超时重传
ACK_TIMEOUT: float = 4.0

# 在线控制包 ID（frame body 第一字节）
class OnlinePacketID:
    CONNECTED_PING = 0x00
    CONNECTED_PONG = 0x03
    CONNECTION_REQUEST = 0x09
    CONNECTION_REQUEST_ACCEPTED = 0x10
    NEW_INCOMING_CONNECTION = 0x13
    DISCONNECTION_NOTIFICATION = 0x15
    ID_INCOMPATIBLE_PROTOCOL = 0x19


# ═══════════════════════════════════════════════════════════════════════════════
# OnlineAddress：在线阶段的地址编码（IPv4 取反+BE port / IPv6 完整）
# ═══════════════════════════════════════════════════════════════════════════════
@dataclass
class OnlineAddress:
    """在线阶段 SystemAddress（用于 ConnectionRequestAccepted / NewIncomingConnection）。

    ⚠️ 之前误以为"在线版 IP 直传 + 端口小端"，实际抓包 + go-raknet 源码 +
    RakNetProtocolDoc 确认：在线和离线地址编码**相同**：
        IPv4: version(1=4) + ip[4](每字节 ^0xFF 取反) + port(2 大端)
        IPv6: version(1=6) + addrFamily(2 小端=10) + port(2 大端) + flowInfo(4 大端)
              + ip[16] + scopeId(4 大端)   共 29 字节
    空槽位用 IPv4 0.0.0.0:0（version=4, ip=ffffffff, port=0），不是 version=0。
    """

    ip: str = "0.0.0.0"
    port: int = 0
    version: int = 4  # 4=IPv4, 6=IPv6

    def encode(self, w: ByteWriter) -> None:
        w.write_byte(self.version)
        if self.version == 4:
            for b in socket.inet_aton(self.ip):
                w.write_byte(b ^ 0xFF)  # IP 取反
            w.write(struct.pack(">H", self.port & 0xFFFF))  # port 大端
        elif self.version == 6:
            import ipaddress
            w.write(struct.pack("<H", 10))  # AF_INET6 小端
            w.write(struct.pack(">H", self.port & 0xFFFF))  # port 大端
            w.write(struct.pack(">I", 0))  # flow info 大端
            w.write(ipaddress.IPv6Address(self.ip).packed)  # 16 bytes
            w.write(struct.pack(">I", 0))  # scope id 大端
        else:
            raise NotImplementedError(f"OnlineAddress encode version {self.version}")

    @classmethod
    def decode(cls, r: ByteReader) -> "OnlineAddress":
        import ipaddress
        version = r.read_byte()
        if version == 4:
            raw = [r.read_byte() for _ in range(4)]
            ip = ".".join(str(b ^ 0xFF) for b in raw)  # IP 取反还原
            port = struct.unpack(">H", r.read(2))[0]  # port 大端
            return cls(ip=ip, port=port, version=4)
        if version == 6:
            r.read(2)  # addr family (LE，忽略)
            port = struct.unpack(">H", r.read(2))[0]  # port 大端
            r.read(4)  # flow info
            ip_bytes = r.read(16)
            r.read(4)  # scope id
            ip = str(ipaddress.IPv6Address(ip_bytes))
            return cls(ip=ip, port=port, version=6)
        raise ValueError(f"未知 OnlineAddress version: {version}")


# ═══════════════════════════════════════════════════════════════════════════════
# 在线控制包编解码
# ═══════════════════════════════════════════════════════════════════════════════
def build_connected_ping(time_ms: int) -> bytes:
    """ConnectedPing: 0x00 | time(u64 BE)，对齐 RakNet message。"""
    w = ByteWriter()
    w.write_byte(OnlinePacketID.CONNECTED_PING)
    w.write(struct.pack(">Q", time_ms & 0xFFFFFFFFFFFFFFFF))
    return w.to_bytes()


@dataclass
class ConnectedPong:
    ping_time: int
    pong_time: int


def parse_connected_pong(data: bytes) -> ConnectedPong:
    r = ByteReader(data)
    pid = r.read_byte()
    if pid != OnlinePacketID.CONNECTED_PONG:
        raise ValueError(f"不是 ConnectedPong: 0x{pid:02x}")
    ping_time = struct.unpack(">Q", r.read(8))[0]
    pong_time = struct.unpack(">Q", r.read(8))[0]
    return ConnectedPong(ping_time=ping_time, pong_time=pong_time)


def build_connection_request(client_guid: int, time_ms: int, use_security: bool = False) -> bytes:
    """ConnectionRequest: 0x09 | clientGUID(u64 BE) | time(u64 BE) | useSecurity(u8)。"""
    w = ByteWriter()
    w.write_byte(OnlinePacketID.CONNECTION_REQUEST)
    w.write(struct.pack(">Q", client_guid & 0xFFFFFFFFFFFFFFFF))
    w.write(struct.pack(">Q", time_ms & 0xFFFFFFFFFFFFFFFF))
    w.write_byte(1 if use_security else 0)
    return w.to_bytes()


@dataclass
class ConnectionRequestAccepted:
    """ConnectionRequestAccepted 解析结果。

    ⚠️ 没有 client_guid 字段！抓包 188B 实测 + Gophertunnel connection_request_accepted.go 确认，
       开头直接是 clientAddress，然后是 SystemIndex，然后 20 个 SystemAddress。
    """

    client_address: OnlineAddress
    system_index: int
    system_addresses: List[OnlineAddress]
    request_time: int
    reply_time: int


def parse_connection_request_accepted(data: bytes) -> ConnectionRequestAccepted:
    """解析 ConnectionRequestAccepted。

    格式: 0x10 | clientAddr(OnlineAddress) | SystemIndex(u16 BE) | [20 OnlineAddress] | requestTime(u64 BE) | replyTime(u64 BE)

    实测抓包（BDS 188B）拆解：
        10                              包头
        04 80ffff fea1f4 00              clientAddr IPv4: ip=127.0.0.1(取反), port=62464(BE)
        00 00                           SystemIndex=0
        06 0a00 4abc 00000000 fe80...   第1个 SystemAddr IPv6: port=19132(BDS端口)
        04 ffffffff 0000 × 19           19 个空槽 (0.0.0.0:0)
        9eec30e5 9f010000               requestTime(u64 BE)
        00000000 00000f40               replyTime(u64 BE)
        = 1+7+2+29+(19×7)+8+8 = 188 ✓
    """
    r = ByteReader(data)
    pid = r.read_byte()
    if pid != OnlinePacketID.CONNECTION_REQUEST_ACCEPTED:
        raise ValueError(f"不是 ConnectionRequestAccepted: 0x{pid:02x}")
    # 开头直接是 clientAddress（不是 clientGUID！）
    client_address = OnlineAddress.decode(r)
    # SystemIndex（u16 BE，message/connection_request_accepted.go）
    system_index = struct.unpack(">H", r.read(2))[0]
    # 20 个 SystemAddress（第 1 个是真实地址，后 19 个是 0.0.0.0:0 空槽）
    addrs: List[OnlineAddress] = []
    for _ in range(SYSTEM_ADDRESS_COUNT):
        if r.remaining == 16:
            break  # 参考允许只发送10个地址的实现。
        addrs.append(OnlineAddress.decode(r))
    request_time = struct.unpack(">Q", r.read(8))[0]
    reply_time = struct.unpack(">Q", r.read(8))[0]
    return ConnectionRequestAccepted(
        client_address=client_address,
        system_index=system_index,
        system_addresses=addrs,
        request_time=request_time,
        reply_time=reply_time,
    )


def build_new_incoming_connection(
    server_address: OnlineAddress,
    system_addresses: List[OnlineAddress],
    request_time: int,
    server_reply_time: int,
) -> bytes:
    """NewIncomingConnection: 0x13 | address | addresses | PingTime(BE) | PongTime(BE)。

    request_time 参数为要回显的 Accepted.reply_time；server_reply_time 为本机当前时间。
    保留参数名供旧调用方兼容。
    """
    w = ByteWriter()
    w.write_byte(OnlinePacketID.NEW_INCOMING_CONNECTION)
    server_address.encode(w)
    # 20 个 SystemAddress（用 Accepted 里带回来的，不足补 0.0.0.0:0 空槽）
    # ⚠️ 空槽用 version=4 + 0.0.0.0:0（抓包 04 ffffffff 0000 实测），不是 version=0
    for i in range(SYSTEM_ADDRESS_COUNT):
        addr = system_addresses[i] if i < len(system_addresses) else OnlineAddress()
        addr.encode(w)
    w.write(struct.pack(">Q", request_time & 0xFFFFFFFFFFFFFFFF))
    w.write(struct.pack(">Q", server_reply_time & 0xFFFFFFFFFFFFFFFF))
    return w.to_bytes()


# ═══════════════════════════════════════════════════════════════════════════════
# 待 ACK 帧条目（用于超时重传）
# ═══════════════════════════════════════════════════════════════════════════════
@dataclass
class _PendingFrame:
    """已发出但还没收到 ACK 的 reliable 帧，用于重传。"""

    frame: Frame
    send_time: float


@dataclass
class _SplitGroup:
    """分片重组缓冲：split_id -> 收到的分片。"""

    split_count: int
    fragments: Dict[int, bytes] = field(default_factory=dict)

    def is_complete(self) -> bool:
        return len(self.fragments) == self.split_count

    def assemble(self) -> bytes:
        return b"".join(self.fragments[i] for i in range(self.split_count))


# ═══════════════════════════════════════════════════════════════════════════════
# RakNetSession：在线会话主控
# ═══════════════════════════════════════════════════════════════════════════════
class RakNetSession:
    """RakNet 在线会话。

    用法：
        conn = await handshake(host, port)        # 离线握手（connection.py）
        session = RakNetSession(conn)
        await session.start()
        await session.connect()                    # 在线握手
        await session.send_reliable(bedrock_packet)
        data = await session.recv()                # 已保序重组的帧 body
    """

    def __init__(self, conn: RakNetConnection) -> None:
        # 与参考 newConn 一致：服务端 MTU 必须钳制到支持区间。
        # 异常大值会让 body bit-length(u16)溢出，破坏 Login 分片。
        conn.mtu = min(max(conn.mtu, 576), 1492)
        self._conn = conn
        self._sock = conn.sock
        self._server_addr = conn.server_addr  # (host, port)
        self._loop = asyncio.get_event_loop()

        # ── 发送侧 ──────────────────────────────────────────────
        self._send_sequence: int = 0          # 下一个 FrameSet 的 sequence_number
        self._frame_index: int = 0            # 下一个 reliable frame 的 frame_index
        self._order_index: int = 0            # 下一个 ordered frame 的 order_index (channel 0)
        self._next_split_id: int = 0          # 下一个分片组的 split_id
        self._pending_acks: Dict[int, _PendingFrame] = {}  # frame_index -> 待 ACK
        self._seq_to_frames: Dict[int, List[int]] = {}  # FrameSet seq -> [frame_index, ...]（ACK 用）
        self._send_queue: asyncio.Queue[Frame] = asyncio.Queue()  # 待发送帧队列

        # ── 接收侧 ──────────────────────────────────────────────
        self._missing_sequences = {}
        self._rtt_samples = deque()
        self._recv_sequence: int = 0          # 下一个期望的 FrameSet sequence_number
        self._ack_needed: List[int] = []      # 收到待回 ACK 的 sequence_number
        # 可靠编号用于去重，不能以“最高已到编号”丢弃晚到帧。
        self._reliable_seen: set[int] = set()
        self._reliable_history = deque()
        self._order_recv: Dict[int, int] = {}  # channel -> 下一个期望的 order_index
        self._order_buffer: Dict[int, Dict[int, bytes]] = {}
        self._split_buffer: Dict[int, _SplitGroup] = {}  # split_id -> 重组缓冲

        # ── 上层投递 ─────────────────────────────────────────────
        self._inbox: asyncio.Queue[bytes] = asyncio.Queue()  # 已重组保序的帧 body

        # ── 在线握手 ─────────────────────────────────────────────
        self._handshake_done = asyncio.Event()
        self._accepted: Optional[ConnectionRequestAccepted] = None

        # ── 心跳/重传任务 ────────────────────────────────────────
        self._tasks: List[asyncio.Task] = []
        self._closed = False
        self._stop_started = False
        self._boot_time = time.monotonic()

    # ──────────────────────────────────────────────────────────────
    # 生命周期
    # ──────────────────────────────────────────────────────────────
    async def start(self) -> None:
        """启动收包 / 发送 / ACK / 心跳 / 重传五个后台循环。"""
        # ⚠️ 清除离线握手阶段设置的 socket 超时！
        # _handshake_once 中 sock.settimeout(timeout/remaining) 用于离线握手，
        # 但握手完成后 socket 传给 RakNetSession，超时设置不会被自动清除。
        # 如果不清除，在线阶段 recvfrom 会在超时后抛 socket.timeout（OSError 子类），
        # 导致连接断开。在线阶段应该用阻塞模式，连接状态由 RakNet 心跳/重传管理。
        self._sock.setblocking(False)
        self._sock.connect(self._server_addr)
        self._tasks = [
            asyncio.create_task(self._recv_loop(), name="raknet-recv"),
            asyncio.create_task(self._flush_loop(), name="raknet-flush"),
            asyncio.create_task(self._ack_loop(), name="raknet-ack"),
            asyncio.create_task(self._ping_loop(), name="raknet-ping"),
            asyncio.create_task(self._resend_loop(), name="raknet-resend"),
        ]

    async def stop(self) -> None:
        """优雅关闭：发 Disconnection，取消任务，关 socket。"""
        if self._stop_started:
            return
        self._stop_started = True
        try:
            # 发 DisconnectionNotification（reliable，尽力而为）
            if not self._closed:
                disc = bytes([OnlinePacketID.DISCONNECTION_NOTIFICATION])
                await self.send_reliable(disc, ordered=False)
                await asyncio.sleep(0.1)  # 给点时间发出去
        except Exception:
            pass
        self._closed = True
        tasks = [t for t in self._tasks if t is not asyncio.current_task()]
        for t in tasks:
            t.cancel()
        await asyncio.gather(*tasks, return_exceptions=True)
        self._sock.close()
        logger.info("RakNet 会话已关闭")

    # ──────────────────────────────────────────────────────────────
    # 在线握手
    # ──────────────────────────────────────────────────────────────
    async def connect(self, timeout: float = 10.0) -> None:
        """完成在线握手：发 ConnectionRequest → 等 Accepted → 发 NewIncoming。

        必须在 start() 之后调用。握手成功后才能 send/recv 游戏包。
        """
        # 在 Bedrock 里，时间戳用"启动后毫秒数"（vanilla 行为）
        now_ms = int((time.monotonic() - self._boot_time) * 1000)
        req = build_connection_request(self._conn.client_guid, now_ms, use_security=False)
        # 官方抓包：ConnectionRequest为reliable unordered，不消耗游戏有序编号。
        await self.send_reliable(req, ordered=False)

        try:
            await asyncio.wait_for(self._handshake_done.wait(), timeout=timeout)
        except asyncio.TimeoutError:
            raise RuntimeError("在线握手超时：未收到 ConnectionRequestAccepted")

        # _accepted 由 _handle_frame 在收到 Accepted 时设置
        accepted = self._accepted
        assert accepted is not None
        new_incoming = build_new_incoming_connection(
            server_address=OnlineAddress(ip=self._server_addr[0], port=self._server_addr[1]),
            system_addresses=accepted.system_addresses,
            request_time=accepted.reply_time,
            server_reply_time=int((time.monotonic() - self._boot_time) * 1000),
        )
        logger.debug(
            "NewIncomingConnection [%d B]: %s | server_addr=%s:%d request_time=%d reply_time=%d sysaddrs=%d",
            len(new_incoming), new_incoming.hex(),
            self._server_addr[0], self._server_addr[1],
            accepted.request_time, accepted.reply_time, len(accepted.system_addresses),
        )
        await self.send_reliable(new_incoming, ordered=True)
        logger.info(
            "RakNet 在线握手完成: server_guid=%d mtu=%d",
            self._conn.server_guid, self._conn.mtu,
        )

    # ──────────────────────────────────────────────────────────────
    # 发送 API
    # ──────────────────────────────────────────────────────────────
    async def send_reliable(self, data: bytes, ordered: bool = True) -> None:
        """可靠发送一帧（默认 reliable ordered，游戏包用这个）。"""
        frame = Frame(body=data, reliability=Reliability.RELIABLE_ORDERED if ordered else Reliability.RELIABLE)
        await self._enqueue_frame(frame)

    async def send_unreliable(self, data: bytes) -> None:
        """不可靠发送（不保证送达，省 ACK 开销）。"""
        frame = Frame(body=data, reliability=Reliability.UNRELIABLE)
        await self._enqueue_frame(frame)

    async def _enqueue_frame(self, frame: Frame) -> None:
        """分配索引字段并入队。body 超过单帧上限时自动分片。

        RakNet 分片（go-raknet）：大包拆成多个 split frame，每片独立 frame_index，
        共享同一 order_index（同一有序消息）+ split_id。接收方按 split_id 重组。
        每个分片放进独立 FrameSet，保证单 UDP 包不超过 MTU。
        """
        # 单帧 body 上限 = MTU - FrameSet头(4) - Frame头
        # Frame头 = 1(flags) + 2(body_length) + [3 if reliable] + [3 if sequenced] + [4 if ordered]
        # 协商 MTU 包括 IPv4/UDP 头；对齐参考 effectiveMTU()。
        mtu = self._conn.mtu - 28
        frameset_header = 4  # 0x80 + seq u24
        frame_header = 1 + 2  # flags + body_length(u16 BE)
        if frame.is_reliable:
            frame_header += 3
        if frame.is_sequenced:
            frame_header += 3
        if frame.is_ordered or frame.is_sequenced:
            frame_header += 4  # order_index u24 + order_channel u8
        max_body = mtu - frameset_header - frame_header

        if len(frame.body) <= max_body:
            # 不分片
            if frame.is_reliable:
                frame.frame_index = self._frame_index
                self._frame_index += 1
            if frame.is_ordered:
                frame.order_index = self._order_index
                frame.order_channel = 0
                self._order_index += 1
            await self._send_queue.put(frame)
            return

        # 分片：每片再加 split 头 = 4(count u32) + 2(id u16) + 4(index u32) = 10
        split_header = 10
        max_body_split = mtu - frameset_header - frame_header - split_header
        if max_body_split < 64:
            raise ValueError(f"MTU {mtu} 太小，无法分片")

        split_id = self._next_split_id
        self._next_split_id = (self._next_split_id + 1) & 0xFFFF

        body = frame.body
        split_count = (len(body) + max_body_split - 1) // max_body_split

        # 所有分片共享同一 order_index（同一有序消息）
        order_index = self._order_index if frame.is_ordered else 0
        if frame.is_ordered:
            self._order_index += 1

        logger.debug(
            "分片发送: body=%d B → %d 片 (每片≤%d B, split_id=%d)",
            len(body), split_count, max_body_split, split_id,
        )
        for i in range(split_count):
            chunk = body[i * max_body_split : (i + 1) * max_body_split]
            split_frame = Frame(
                body=chunk,
                reliability=frame.reliability,
                is_split=True,
                split_count=split_count,
                split_id=split_id,
                split_index=i,
                order_index=order_index,
                order_channel=0,
            )
            if frame.is_reliable:
                split_frame.frame_index = self._frame_index
                self._frame_index += 1
            await self._send_queue.put(split_frame)

    # ──────────────────────────────────────────────────────────────
    # 接收 API
    # ──────────────────────────────────────────────────────────────
    async def recv(self) -> bytes:
        """收一个已保序重组的帧 body（Block 直到有数据）。"""
        return await self._inbox.get()

    # ═══════════════════════════════════════════════════════════════════════════
    # 后台循环
    # ═══════════════════════════════════════════════════════════════════════════
    async def _recv_loop(self) -> None:
        """收包主循环：读 UDP，按第一字节分发。"""
        while not self._closed:
            try:
                data = await self._loop.sock_recv(self._sock, 65535)
            except (OSError, socket.error) as e:
                if not self._closed:
                    logger.warning("recv socket 异常 (%s)，退出收包循环", e)
                break
            if not data:
                continue
            try:
                self._dispatch(data)
            except Exception:
                logger.exception("分发包异常 (0x%02x, %d bytes)", data[0], len(data))

    def _dispatch(self, data: bytes) -> None:
        """按 UDP 包第一字节分发（用位 flag 判断，go-raknet 风格）。

        ⚠️ RakNet datagram ID 是 0x80-0x8F（高4位 0x8，低4位是 flags）。
        BDS 发的 FrameSet 常带 0x04 (needsBAndAS) flag → 0x84，不能用 == 0x80 精确匹配！
        判断顺序（go-raknet）：ACK 0x40 bit set；NACK 0x20 bit set；datagram 0x80 set 且无前两者。
        """
        pid = data[0]
        logger.debug("← UDP 包 0x%02x [%d B]: %s", pid, len(data), data[:20].hex())
        if pid & 0x40:  # ACK（0xC0 等）
            self._handle_ack(data)
        elif pid & 0x20:  # NACK（0xA0 等）
            self._handle_nack(data)
        elif pid & 0x80:  # datagram（0x80-0x8F）
            self._handle_frame_set(data)
        elif pid == OnlinePacketID.DISCONNECTION_NOTIFICATION:
            logger.info("服务端发来 DisconnectionNotification，断开")
            self._closed = True
        else:
            logger.debug("未处理的在线包 0x%02x", pid)

    # ── FrameSet 处理 ────────────────────────────────────────────
    def _handle_frame_set(self, data: bytes) -> None:
        try:
            fs = FrameSet.decode(data)
        except Exception:
            logger.exception("FrameSet 解码失败")
            return
        # A gap requests transport recovery after a reordering grace period.
        # Reference: FunShuttler/core/raknet/conn.go receiveDatagram + win.missing.
        seq = fs.sequence_number
        distance = (seq - self._recv_sequence) & 0xFFFFFF
        if distance < 0x800000:
            if distance > 4096:
                self._closed = True
                return
            now = time.monotonic()
            for missing in range(distance):
                self._missing_sequences.setdefault((self._recv_sequence+missing) & 0xFFFFFF, now)
            self._recv_sequence = (seq+1) & 0xFFFFFF
        self._missing_sequences.pop(seq, None)
        # 记下要回 ACK
        self._ack_needed.append(fs.sequence_number)
        # 投递每个 frame
        for frame in fs.frames:
            try:
                self._handle_frame(frame)
            except Exception:
                logger.exception("frame 处理异常 (reliability=%d)", frame.reliability)

    def _handle_frame(self, frame: Frame) -> None:
        # 每个可靠分片有独立编号；在重组前去重，避免重传重建旧 split group。
        if frame.is_reliable:
            if frame.frame_index in self._reliable_seen:
                return
            self._reliable_seen.add(frame.frame_index)
            self._reliable_history.append(frame.frame_index)
            # 有界近期去重窗口，同时容许 u24 编号在长会话中回绕。
            if len(self._reliable_history) > 8192:
                self._reliable_seen.remove(self._reliable_history.popleft())

        # 分片重组
        if frame.is_split:
            group = self._split_buffer.setdefault(frame.split_id, _SplitGroup(split_count=frame.split_count))
            group.fragments[frame.split_index] = frame.body
            if not group.is_complete():
                return
            body = group.assemble()
            del self._split_buffer[frame.split_id]
            frame = Frame(
                body=body,
                reliability=frame.reliability,
                frame_index=frame.frame_index,
                order_index=frame.order_index,
                order_channel=frame.order_channel,
            )

        # 对齐参考 conn.receivePacket：完整消息按 channel/order_index 排队，
        # 在线控制消息也必须消耗其有序编号，然后才处理正文。
        if frame.is_ordered:
            ch = frame.order_channel
            expect = self._order_recv.get(ch, 0)
            distance = (frame.order_index - expect) & 0xFFFFFF
            if distance >= 0x800000:
                return  # 已投递的旧消息（含 u24 回绕）。
            pending = self._order_buffer.setdefault(ch, {})
            pending.setdefault(frame.order_index, frame.body)
            while expect in pending:
                body = pending.pop(expect)
                expect = (expect + 1) & 0xFFFFFF
                self._order_recv[ch] = expect
                self._deliver_body(body)
        else:
            self._deliver_body(frame.body)

    def _deliver_body(self, body: bytes) -> None:
        """投递已经完成分片重组与排序的正文。"""
        if body:
            pid = body[0]
            if pid in (OnlinePacketID.CONNECTION_REQUEST_ACCEPTED,
                       OnlinePacketID.CONNECTED_PONG,
                       OnlinePacketID.CONNECTED_PING,
                       OnlinePacketID.DISCONNECTION_NOTIFICATION):
                self._handle_online_packet(body)
                return
        self._inbox.put_nowait(body)

    def _handle_online_packet(self, body: bytes) -> None:
        pid = body[0]
        if pid == OnlinePacketID.CONNECTION_REQUEST_ACCEPTED:
            try:
                accepted = parse_connection_request_accepted(body)
            except Exception:
                logger.exception("ConnectionRequestAccepted 解析失败")
                return
            self._accepted = accepted
            self._handshake_done.set()
            logger.debug("收到 ConnectionRequestAccepted")
        elif pid == OnlinePacketID.CONNECTED_PING:
            # BDS 发 ConnectedPing 保活，必须回 ConnectedPong，否则 BDS 认为连接不活跃
            # ⚠️ pingTime 原样回传（避免端序争议：实测 BDS 的 time 用 BE 更合理，
            #    但标准是 LE，原样回传 8 字节 BDS 收到的就是它发的，必然匹配）
            try:
                ping_time_bytes = body[1:9]
                now_ms = int((time.monotonic() - self._boot_time) * 1000)
                w = ByteWriter()
                w.write_byte(OnlinePacketID.CONNECTED_PONG)
                w.write(ping_time_bytes)  # pingTime 原样回传
                w.write(struct.pack(">Q", now_ms & 0xFFFFFFFFFFFFFFFF))
                asyncio.create_task(self.send_unreliable(w.to_bytes()))
                logger.debug("收到 ConnectedPing，回 ConnectedPong")
            except Exception:
                logger.debug("ConnectedPing 处理失败")
        elif pid == OnlinePacketID.CONNECTED_PONG:
            try:
                pong = parse_connected_pong(body)
                rtt_ms = (time.monotonic() - self._boot_time) * 1000 - pong.ping_time
                logger.debug("收到 ConnectedPong, RTT≈%.1fms", rtt_ms)
            except Exception:
                logger.debug("ConnectedPong 解析失败")
        elif pid == OnlinePacketID.DISCONNECTION_NOTIFICATION:
            self._closed = True

    # ── ACK/NACK 处理 ────────────────────────────────────────────
    def _handle_ack(self, data: bytes) -> None:
        logger.debug("收到 ACK raw: %s", data[:24].hex())
        try:
            records = parse_ack(data)
        except Exception:
            logger.exception("ACK 解析失败")
            return
        logger.debug("收到 ACK: %s", records)
        # ACK 确认的是 FrameSet sequence_number，不是 frame_index！
        # 通过 _seq_to_frames 映射找到该 FrameSet 包含的所有 frame_index，全部移除待 ACK
        # ⚠️ is_single=True 时 end 是默认值 0，不能用 range(start, end+1)（会得到空 range）！
        for rec in records:
            seqs = [rec.start] if rec.is_single else range(rec.start, rec.end + 1)
            for seq in seqs:
                fidxs = self._seq_to_frames.pop(seq, [])
                if fidxs:
                    for fidx in fidxs:
                        pending = self._pending_acks.pop(fidx, None)
                        if pending:
                            self._rtt_samples.append((time.monotonic(), time.monotonic()-pending.send_time))
                    logger.debug("ACK seq=%d 移除 frame_index=%s", seq, fidxs)
                else:
                    logger.debug("ACK seq=%d 在 _seq_to_frames 中不存在（keys=%s）", seq, list(self._seq_to_frames.keys()))

    def _handle_nack(self, data: bytes) -> None:
        try:
            records = parse_nack(data)
        except Exception:
            logger.exception("NACK 解析失败")
            return
        # NACK 指明的是 FrameSet sequence_number，重传该 FrameSet 里所有 reliable frame
        for rec in records:
            seqs = [rec.start] if rec.is_single else range(rec.start, rec.end + 1)
            for seq in seqs:
                for fidx in self._seq_to_frames.get(seq, []):
                    pf = self._pending_acks.pop(fidx, None)
                    if pf is not None:
                        self._rtt_samples.append((time.monotonic(), 2*(time.monotonic()-pf.send_time)))
                        asyncio.create_task(self._resend_frame(pf.frame))

    # ── 发送循环：把待发帧打包成 FrameSet 发出 ────────────────────
    async def _flush_loop(self) -> None:
        """从 _send_queue 取帧，打包 FrameSet 发出。reliable 帧登记待 ACK。

        合并多个 frame 到一个 FrameSet 时，保证总大小不超过 MTU（否则 IP 分片丢包）。
        分片后的大 frame 每片接近 MTU，自然单独成包。
        """
        while not self._closed:
            try:
                first = await asyncio.wait_for(self._send_queue.get(), timeout=0.5)
            except asyncio.TimeoutError:
                continue
            frames = [first]
            # 暂不合并多个 frame（合并后部分 BDS 版本不 ACK，待排查）；每帧单独 FrameSet
            # 后续如需合并优化，需确保总大小不超 MTU 且 BDS 正常 ACK

            seq = self._send_sequence
            self._send_sequence += 1
            fs = FrameSet(sequence_number=seq, frames=frames)
            packet = fs.encode()

            # reliable 帧登记待 ACK（用于重传）+ 记录 seq→frame_index 映射（ACK/NACK 用）
            now = time.monotonic()
            seq_frames: List[int] = []
            for f in frames:
                if f.is_reliable:
                    self._pending_acks[f.frame_index] = _PendingFrame(frame=f, send_time=now)
                    seq_frames.append(f.frame_index)
            if seq_frames:
                self._seq_to_frames[seq] = seq_frames
                logger.debug("发送 FrameSet seq=%d 含 frame_index=%s (packet=%dB)", seq, seq_frames, len(packet))

            try:
                await self._loop.sock_sendall(self._sock, packet)
            except OSError as e:
                logger.warning("发送 FrameSet 失败: %s", e)

    @staticmethod
    def _frame_wire_size(frame: Frame) -> int:
        """计算单个 frame 编码后的字节数（不含 FrameSet 头）。"""
        size = 1 + 2  # flags + body_length(u16)
        if frame.is_reliable:
            size += 3
        if frame.is_sequenced:
            size += 3
        if frame.is_ordered or frame.is_sequenced:
            size += 4  # order_index u24 + order_channel u8
        if frame.is_split:
            size += 10  # split_count u32 + split_id u16 + split_index u32
        size += len(frame.body)
        return size

    async def _resend_frame(self, frame: Frame) -> None:
        """重传保留 reliable index；仅 FrameSet 的 sequence number 更新。

        对齐参考 resend→sendDatagram。改变 reliable index 会制造可靠序列空洞，
        并让接收端把重复数据当成新消息。
        """
        await self._send_queue.put(frame)

    # ── ACK 循环：周期把 _ack_needed 回给服务端 ──────────────────
    async def _flush_acks(self) -> None:
        """把积攒的 ACK 一次性发回。"""
        if not self._ack_needed:
            return
        # 合并连续的 sequence 成 range，省字节
        seqs = sorted(set(self._ack_needed))
        self._ack_needed.clear()
        records: List[AckRange] = []
        start = prev = seqs[0]
        for s in seqs[1:]:
            if s == prev + 1:
                prev = s
                continue
            records.append(AckRange.single(start) if start == prev else AckRange.range(start, prev))
            start = prev = s
        records.append(AckRange.single(start) if start == prev else AckRange.range(start, prev))
        packet = build_ack(records)
        try:
            await self._loop.sock_sendall(self._sock, packet)
        except OSError as e:
            logger.warning("发送 ACK 失败: %s", e)

    def _recovery_delay(self):
        now = time.monotonic()
        while self._rtt_samples and now-self._rtt_samples[0][0] > 5:
            self._rtt_samples.popleft()
        average = sum(delay for _,delay in self._rtt_samples)/len(self._rtt_samples) if self._rtt_samples else 0.05
        return min(4.0, max(0.075, average*1.5))

    async def _flush_nacks(self):
        now = time.monotonic()
        missing = [seq for seq, last in self._missing_sequences.items() if now-last >= self._recovery_delay()][:256]
        if missing:
            packet = build_nack([AckRange.single(seq) for seq in missing])
            await self._loop.sock_sendall(self._sock, packet)
            for seq in missing:
                self._missing_sequences.pop(seq, None)

    async def _ack_loop(self) -> None:
        """周期性 flush ACK（合并到 flush_loop 的节奏里，这里单独跑便于理解）。"""
        while not self._closed:
            await asyncio.sleep(0.1)
            if self._ack_needed:
                await self._flush_acks()
            await self._flush_nacks()

    # ── 心跳循环 ─────────────────────────────────────────────────
    async def _ping_loop(self) -> None:
        """周期发 ConnectedPing，保活 + 测 RTT。"""
        while not self._closed:
            await asyncio.sleep(PING_INTERVAL)
            now_ms = int((time.monotonic() - self._boot_time) * 1000)
            await self.send_unreliable(build_connected_ping(now_ms))

    # ── 重传循环 ─────────────────────────────────────────────────
    async def _resend_loop(self) -> None:
        """扫描待 ACK 帧，超时的重传。"""
        while not self._closed:
            await asyncio.sleep(0.05)
            now = time.monotonic()
            stale = [idx for idx, pf in self._pending_acks.items() if now - pf.send_time > self._recovery_delay()]
            for idx in stale:
                pf = self._pending_acks.pop(idx, None)
                if pf is not None:
                    logger.debug("帧 frame_index=%d 超时未 ACK，重传", idx)
                    await self._resend_frame(pf.frame)
