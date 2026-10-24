"""RakNet 二进制编解码工具。

RakNet/Bedrock 用小端字节序，大量使用 VarInt（变长整数）压缩。
这个模块提供字节流读写 + VarInt 编解码，是所有包序列化的基础。

学习要点：
    - VarInt：每字节低 7 位是数据，最高位是"是否还有后续字节"标志
      小整数（0-127）只需 1 字节，32767 只需 2 字节，省带宽
    - VarLong 同理，最多 10 字节
    - 字符串：VarInt 长度前缀 + UTF-8 字节
    - GUID：8 字节小端 i64（连接标识）

参考：minecraft.wiki 的 Bedrock 协议数据类型
"""

from __future__ import annotations

import struct
from typing import Tuple


# ═══════════════════════════════════════════════════════════════════════════════
# 字节流读写器（光标式，向前推进）
# ═══════════════════════════════════════════════════════════════════════════════
class ByteReader:
    """只读字节流，带光标。用于解码收到的包。"""

    __slots__ = ("data", "pos")

    def __init__(self, data: bytes) -> None:
        self.data = data
        self.pos = 0

    @property
    def remaining(self) -> int:
        return len(self.data) - self.pos

    def read(self, n: int) -> bytes:
        """读 n 字节。"""
        if self.pos + n > len(self.data):
            raise EOFError(f"读取越界: 需要 {n} 字节，仅剩 {self.remaining}")
        chunk = self.data[self.pos : self.pos + n]
        self.pos += n
        return chunk

    def read_byte(self) -> int:
        return self.read(1)[0]

    def read_bool(self) -> bool:
        return self.read_byte() != 0

    def read_u16(self) -> int:
        """小端无符号 16 位。"""
        return struct.unpack("<H", self.read(2))[0]

    def read_i16(self) -> int:
        return struct.unpack("<h", self.read(2))[0]

    def read_u24(self) -> int:
        """小端无符号 24 位（RakNet 地址用）。"""
        b = self.read(3)
        return b[0] | (b[1] << 8) | (b[2] << 16)

    def read_u32(self) -> int:
        return struct.unpack("<I", self.read(4))[0]

    def read_i64(self) -> int:
        """小端有符号 64 位（GUID 用）。"""
        return struct.unpack("<q", self.read(8))[0]

    def read_u64(self) -> int:
        return struct.unpack("<Q", self.read(8))[0]

    def read_varint(self) -> int:
        """读有符号 VarInt（ZigZag + 无符号 LEB128）。

        ⚠️ Gophertunnel Varint32 用 **ZigZag** 编码（writer.go 确认）：
            ux = uint32(u) << 1; if u < 0 { ux = ^ux }
        不是标准 LEB128 符号扩展！之前用符号扩展是错的，只是因为大部分值是 0（两种编码 0 都一样）才没暴露。
        """
        raw = self.read_unsigned_varint()
        # ZigZag 解码: (raw >> 1) ^ -(raw & 1)
        return (raw >> 1) ^ -(raw & 1)

    def read_unsigned_varint(self) -> int:
        """读无符号 VarInt。"""
        result = 0
        shift = 0
        for _ in range(5):  # VarInt 最多 5 字节（32 位）
            if self.remaining < 1:
                raise EOFError("VarInt 不完整")
            byte = self.read_byte()
            result |= (byte & 0x7F) << shift
            if (byte & 0x80) == 0:
                return result
            shift += 7
        raise ValueError("VarInt 过长（>5 字节）")

    def read_varlong(self) -> int:
        """读有符号 VarLong（ZigZag + 无符号 LEB128，非符号扩展）。"""
        raw = self.read_unsigned_varlong()
        # ZigZag 解码: (raw >> 1) ^ -(raw & 1)
        return (raw >> 1) ^ -(raw & 1)

    def read_unsigned_varlong(self) -> int:
        result = 0
        shift = 0
        for _ in range(10):  # VarLong 最多 10 字节
            if self.remaining < 1:
                raise EOFError("VarLong 不完整")
            byte = self.read_byte()
            result |= (byte & 0x7F) << shift
            if (byte & 0x80) == 0:
                return result
            shift += 7
        raise ValueError("VarLong 过长（>10 字节）")

    def read_string(self) -> str:
        """读字符串：VarInt 长度 + UTF-8 字节。"""
        length = self.read_unsigned_varint()
        return self.read(length).decode("utf-8", errors="replace")

    def read_uuid(self) -> bytes:
        """读 Bedrock UUID（16 字节，每半 8 字节各自反转）。

        Gophertunnel Reader.UUID: 读 16 字节后反转 [0:8] 和 [8:16] 各自。
        返回标准 big-endian UUID 字节。
        """
        raw = self.read(16)
        # 反转两个半段还原成标准 UUID
        return raw[:8][::-1] + raw[8:][::-1]

    def read_magic(self) -> bytes:
        """读 16 字节 MAGIC。"""
        return self.read(16)


class ByteWriter:
    """只写字节流。用于编码要发送的包。"""

    __slots__ = ("parts",)

    def __init__(self) -> None:
        self.parts: list[bytes] = []

    def write(self, data: bytes) -> "ByteWriter":
        self.parts.append(data)
        return self

    def write_byte(self, value: int) -> "ByteWriter":
        self.parts.append(bytes([value & 0xFF]))
        return self

    def write_bool(self, value: bool) -> "ByteWriter":
        return self.write_byte(1 if value else 0)

    def write_u16(self, value: int) -> "ByteWriter":
        self.parts.append(struct.pack("<H", value & 0xFFFF))
        return self

    def write_i16(self, value: int) -> "ByteWriter":
        self.parts.append(struct.pack("<h", value))
        return self

    def write_u24(self, value: int) -> "ByteWriter":
        self.parts.append(bytes([value & 0xFF, (value >> 8) & 0xFF, (value >> 16) & 0xFF]))
        return self

    def write_u32(self, value: int) -> "ByteWriter":
        self.parts.append(struct.pack("<I", value & 0xFFFFFFFF))
        return self

    def write_i64(self, value: int) -> "ByteWriter":
        self.parts.append(struct.pack("<q", value))
        return self

    def write_u64(self, value: int) -> "ByteWriter":
        self.parts.append(struct.pack("<Q", value & 0xFFFFFFFFFFFFFFFF))
        return self

    def write_varint(self, value: int) -> "ByteWriter":
        """写有符号 VarInt（ZigZag + 无符号 LEB128）。

        Gophertunnel Varint32: ux = uint32(u) << 1; if u < 0 { ux = ^ux }
        例如 0 → 0x00, 1 → 0x02, -1 → 0x01, 4 → 0x08。
        """
        # ZigZag 编码: (n << 1) ^ (n >> 31)，>> 是算术右移（Python 对负数自动符号扩展）
        zz = (value << 1) ^ (value >> 31)
        return self.write_unsigned_varint(zz & 0xFFFFFFFF)

    def write_unsigned_varint(self, value: int) -> "ByteWriter":
        """写无符号 VarInt。"""
        if value < 0:
            raise ValueError("无符号 VarInt 不能为负")
        while True:
            byte = value & 0x7F
            value >>= 7
            if value == 0:
                self.parts.append(bytes([byte]))
                return self
            self.parts.append(bytes([byte | 0x80]))

    def write_varlong(self, value: int) -> "ByteWriter":
        """写有符号 VarLong（ZigZag + 无符号 LEB128）。"""
        # ZigZag 编码: (n << 1) ^ (n >> 63)
        zz = (value << 1) ^ (value >> 63)
        return self.write_unsigned_varlong(zz & 0xFFFFFFFFFFFFFFFF)

    def write_unsigned_varlong(self, value: int) -> "ByteWriter":
        if value < 0:
            raise ValueError("无符号 VarLong 不能为负")
        while True:
            byte = value & 0x7F
            value >>= 7
            if value == 0:
                self.parts.append(bytes([byte]))
                return self
            self.parts.append(bytes([byte | 0x80]))

    def write_string(self, value: str) -> "ByteWriter":
        encoded = value.encode("utf-8")
        self.write_unsigned_varint(len(encoded))
        self.parts.append(encoded)
        return self

    def write_uuid(self, uuid_bytes: bytes) -> "ByteWriter":
        """写 Bedrock UUID（16 字节，每半 8 字节各自反转）。

        Gophertunnel Writer.UUID: b = u[8:]+u[:8] 然后整体反转
        = u[:8][::-1] + u[8:][::-1]（每半各自反转）。
        """
        if len(uuid_bytes) != 16:
            raise ValueError(f"UUID 必须 16 字节，得到 {len(uuid_bytes)}")
        self.parts.append(uuid_bytes[:8][::-1] + uuid_bytes[8:][::-1])
        return self

    def write_magic(self) -> "ByteWriter":
        from proloader.net.raknet.constants import MAGIC

        self.parts.append(MAGIC)
        return self

    def to_bytes(self) -> bytes:
        return b"".join(self.parts)


# ── 便捷函数 ──────────────────────────────────────────────────────────────────
def encode_unsigned_varint(value: int) -> bytes:
    return ByteWriter().write_unsigned_varint(value).to_bytes()


def encode_varint(value: int) -> bytes:
    return ByteWriter().write_varint(value).to_bytes()


def parse_unconnected_pong(payload: bytes) -> Tuple[int, int, str]:
    """解析 Unconnected Pong 的 MOTD 字段。

    Pong 格式（wiki.bedrock.dev）：
        0x1C | time(u64 LE) | serverGUID(i64 LE) | MAGIC(16) | string(MOTD)

    注意：RakNet 的数值字段是小端，但**字符串长度前缀是 u16 大端**
    （这是 RakNet 的历史遗留 quirk，容易踩坑）。

    MOTD 是分号分隔的字段串：
        MCPE;<name>;<protocol>;<version>;<players>;<max>;<serverID>;<motd2>;<gamemode>;<gamemodeNum>;<port4>;<port6>

    Returns:
        (server_guid, protocol_version, motd_string)
    """
    r = ByteReader(payload)
    pid = r.read_byte()  # 0x1C
    if pid != 0x1C:
        raise ValueError(f"不是 Unconnected Pong: 0x{pid:02x}")
    # RakNet 离线消息全大端（go-raknet 确认）
    _time = struct.unpack(">Q", r.read(8))[0]  # 大端 u64
    guid = struct.unpack(">q", r.read(8))[0]  # 大端 i64
    _magic = r.read_magic()
    # 字符串长度前缀是 u16 大端（RakNet quirk）
    length = struct.unpack(">H", r.read(2))[0]
    motd = r.read(length).decode("utf-8", errors="replace")
    # MOTD 字段: MCPE;name;proto;version;players;max;...
    fields = motd.split(";")
    protocol = int(fields[2]) if len(fields) > 2 and fields[2].isdigit() else 0
    return guid, protocol, motd
