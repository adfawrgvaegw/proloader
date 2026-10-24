"""RakNet 可靠帧层（Frame / FrameSet / ACK / NACK）。

这是 RakNet 连接建立后（OpenConnection 握手之后）的数据传输层。
所有 Bedrock 游戏包都封装在 Frame 里，通过 FrameSet（0x80）发送。

帧结构（Gophertunnel/go-raknet，字节序混合，容易踩坑）：

    Frame:
        flags: u8
            bits 7-5: reliability (3 bit)
            bit 4: is_split
            bits 3-0: 保留
        body_length: u16 BIG-endian  ← 注意大端！
        [reliable]   frame_index: u24 LE   (reliability in 2,3,4,6,7)
        [sequenced]  sequence_index: u24 LE (reliability in 1,4)
        [sequenced/ordered] order_index: u24 LE + order_channel: u8 (reliability in 1,3,4,7)
        [split]      split_count: u32 BE + split_id: u16 BE + split_index: u32 BE
        body: bytes[body_length]

    FrameSet (0x80):
        0x80 | sequence_number: u24 LE | frames[]

    ACK (0xC0) / NACK (0xA0):
        0xC0/0xA0 | record_count: u16 LE | records[]
        record: is_single: u8 | [single: seq u24 LE | range: start u24 LE + end u24 LE]

学习要点：
    - 字节序混合是 RakNet 最大坑：body_length/split 字段大端，index 字段小端
    - reliability 决定帧有哪些字段（reliable/sequenced/ordered 是独立的）
    - ACK 告知对方"这些 sequence 收到了"，NACK 告知"这些没收到，重传"
    - 分片（split）用于大包：一个包拆成多个 Frame 发，对方按 split_id 重组
"""

from __future__ import annotations

import struct
from dataclasses import dataclass, field
from typing import List, Tuple

from proloader.net.raknet.constants import (
    FramePacketID,
    Reliability,
    is_ordered,
    is_reliable,
    is_sequenced,
)
from proloader.net.raknet.wire import ByteReader, ByteWriter


# ═══════════════════════════════════════════════════════════════════════════════
# u24 工具（小端，RakNet index 字段用）
# ═══════════════════════════════════════════════════════════════════════════════
def _read_lu24(r: ByteReader) -> int:
    """读小端 u24（3 字节）。"""
    b = r.read(3)
    return b[0] | (b[1] << 8) | (b[2] << 16)


def _write_lu24(w: ByteWriter, value: int) -> None:
    """写小端 u24（3 字节）。"""
    w.write(bytes([value & 0xFF, (value >> 8) & 0xFF, (value >> 16) & 0xFF]))


# ═══════════════════════════════════════════════════════════════════════════════
# Frame：单个帧
# ═══════════════════════════════════════════════════════════════════════════════
@dataclass
class Frame:
    """RakNet 帧。

    Attributes:
        reliability: 可靠性等级（决定有哪些 index 字段）
        body: 帧体（Bedrock 包数据）
        frame_index: reliable 帧的序号（用于 ACK）
        sequence_index: sequenced 帧的序号（旧帧丢弃）
        order_index / order_channel: ordered 帧的排序序号 + 通道
        split_count / split_id / split_index: 分片信息（大包拆分）
    """

    body: bytes = b""
    reliability: int = Reliability.UNRELIABLE
    # 索引字段（编解码时按 reliability 决定是否读写）
    frame_index: int = 0
    sequence_index: int = 0
    order_index: int = 0
    order_channel: int = 0
    # 分片字段
    is_split: bool = False
    split_count: int = 0
    split_id: int = 0
    split_index: int = 0

    @property
    def is_reliable(self) -> bool:
        return is_reliable(self.reliability)

    @property
    def is_sequenced(self) -> bool:
        return is_sequenced(self.reliability)

    @property
    def is_ordered(self) -> bool:
        return is_ordered(self.reliability)

    def encode(self, w: ByteWriter) -> None:
        """编码单个帧。"""
        # flags: reliability << 5 | is_split << 4
        flags = (self.reliability & 0x07) << 5
        if self.is_split:
            flags |= 0x10
        w.write_byte(flags)
        # body_length: 大端 u16，值 = len << 3（标准 RakNet：低3位为 flags，高13位为长度）
        # go-raknet packet.go 确认：writeUint16(len<<3)，read 时 >>3 还原。BDS 同此实现。
        w.write(struct.pack(">H", (len(self.body) << 3) & 0xFFFF))
        # reliable: frame_index u24 LE
        if self.is_reliable:
            _write_lu24(w, self.frame_index)
        # sequenced: sequence_index u24 LE
        if self.is_sequenced:
            _write_lu24(w, self.sequence_index)
        # ordered: order_index u24 LE + order_channel u8
        if self.is_ordered or self.is_sequenced:
            _write_lu24(w, self.order_index)
            w.write_byte(self.order_channel)
        # split: split_count u32 BE + split_id u16 BE + split_index u32 BE
        if self.is_split:
            w.write(struct.pack(">I", self.split_count))
            w.write(struct.pack(">H", self.split_id & 0xFFFF))
            w.write(struct.pack(">I", self.split_index))
        # body
        w.write(self.body)

    @classmethod
    def decode(cls, r: ByteReader) -> "Frame":
        """解码单个帧。"""
        flags = r.read_byte()
        reliability = (flags >> 5) & 0x07
        is_split = bool(flags & 0x10)
        # body_length: 大端 u16，值 = len << 3（读取时 >> 3 还原字节数）
        body_length = struct.unpack(">H", r.read(2))[0] >> 3
        frame = Frame(reliability=reliability, is_split=is_split)
        if frame.is_reliable:
            frame.frame_index = _read_lu24(r)
        if frame.is_sequenced:
            frame.sequence_index = _read_lu24(r)
        if frame.is_ordered or frame.is_sequenced:
            frame.order_index = _read_lu24(r)
            frame.order_channel = r.read_byte()
        if is_split:
            frame.split_count = struct.unpack(">I", r.read(4))[0]
            frame.split_id = struct.unpack(">H", r.read(2))[0]
            frame.split_index = struct.unpack(">I", r.read(4))[0]
        frame.body = r.read(body_length)
        return frame


# ═══════════════════════════════════════════════════════════════════════════════
# FrameSet：帧集合（0x80 包）
# ═══════════════════════════════════════════════════════════════════════════════
@dataclass
class FrameSet:
    """FrameSet（0x80）：一次发送的帧集合。

    每个 FrameSet 有一个 sequence_number，接收方用它生成 ACK。
    一个 FrameSet 可以含多个 Frame（合并发送省包头开销）。
    """

    sequence_number: int = 0
    frames: List[Frame] = field(default_factory=list)

    def encode(self) -> bytes:
        """编码为完整 FrameSet 包（含 0x80 头）。"""
        w = ByteWriter()
        w.write_byte(FramePacketID.FRAME_SET)
        _write_lu24(w, self.sequence_number)
        for frame in self.frames:
            frame.encode(w)
        return w.to_bytes()

    @classmethod
    def decode(cls, data: bytes) -> "FrameSet":
        """从完整包解码 FrameSet（接受 0x80-0x8F，低4位是 flags）。"""
        r = ByteReader(data)
        pid = r.read_byte()
        # datagram ID 是 0x80-0x8F（高4位 0x8）。BDS 常发 0x84（带 needsBAndAS flag）。
        if (pid & 0xF0) != 0x80:
            raise ValueError(f"不是 FrameSet: 0x{pid:02x}")
        seq = _read_lu24(r)
        frames: List[Frame] = []
        while r.remaining > 0:
            frames.append(Frame.decode(r))
        return cls(sequence_number=seq, frames=frames)


# ═══════════════════════════════════════════════════════════════════════════════
# ACK / NACK：确认 / 否认（0xC0 / 0xA0）
# ═══════════════════════════════════════════════════════════════════════════════
@dataclass
class AckRange:
    """ACK/NACK 记录（单个或范围）。"""

    is_single: bool
    start: int  # single 时就是序号；range 时是起点
    end: int = 0  # range 时的终点

    @classmethod
    def single(cls, seq: int) -> "AckRange":
        return cls(is_single=True, start=seq)

    @classmethod
    def range(cls, start: int, end: int) -> "AckRange":
        return cls(is_single=False, start=start, end=end)


def _encode_ack_records(packet_id: int, records: List[AckRange]) -> bytes:
    """编码 ACK 或 NACK 包。record_count 大端（go-raknet）。"""
    w = ByteWriter()
    w.write_byte(packet_id)
    w.write(struct.pack(">H", len(records) & 0xFFFF))  # 大端 u16
    for rec in records:
        w.write_byte(1 if rec.is_single else 0)
        _write_lu24(w, rec.start)
        if not rec.is_single:
            _write_lu24(w, rec.end)
    return w.to_bytes()


def _decode_ack_records(data: bytes, expected_id: int) -> List[AckRange]:
    """解码 ACK 或 NACK 包。record_count 大端（go-raknet）。"""
    r = ByteReader(data)
    pid = r.read_byte()
    if pid != expected_id:
        raise ValueError(f"包 ID 不匹配: 期望 0x{expected_id:02x} 实际 0x{pid:02x}")
    count = struct.unpack(">H", r.read(2))[0]  # 大端 u16
    records: List[AckRange] = []
    for _ in range(count):
        is_single = r.read_bool()
        start = _read_lu24(r)
        if is_single:
            records.append(AckRange.single(start))
        else:
            end = _read_lu24(r)
            records.append(AckRange.range(start, end))
    return records


def build_ack(records: List[AckRange]) -> bytes:
    """构造 ACK 包（0xC0）。"""
    return _encode_ack_records(FramePacketID.ACK, records)


def build_nack(records: List[AckRange]) -> bytes:
    """构造 NACK 包（0xA0）。"""
    return _encode_ack_records(FramePacketID.NACK, records)


def parse_ack(data: bytes) -> List[AckRange]:
    """解析 ACK 包。"""
    return _decode_ack_records(data, FramePacketID.ACK)


def parse_nack(data: bytes) -> List[AckRange]:
    """解析 NACK 包。"""
    return _decode_ack_records(data, FramePacketID.NACK)
