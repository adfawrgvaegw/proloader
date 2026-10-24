"""Bedrock Batch 包：多包合并 + 压缩。

所有 Bedrock 游戏包都先塞进 Batch，再交给 RakNet 当帧 body 发。
Batch 是省带宽的关键——多个小包合并 + 压缩后再发。

格式有四种（Gophertunnel encoder.go + fatalder compression.go + 网易抓包确认）：
    1) vanilla 未启用压缩（compression == nil，RequestNetworkSettings 阶段）：
        0xfe | varuint(len)+packet | varuint(len)+packet | ...   （无 compression byte！）
    2) vanilla 已启用压缩（NetworkSettings 协商后）：
        0xfe | compression(u8) | compressed( payload )
        compression 取值: 0=FLATE, 1=SNAPPY, 255=NopCompression(不压但带byte)
        payload 内部: varuint(len1)+packet1 | varuint(len2)+packet2 | ...
    3) 网易模式未启用压缩（NetworkSettings 前）：
        0xfe | varuint(len)+packet | ...                          （无 compression byte，不压缩）
    4) 网易模式已启用压缩（NetworkSettings 协商后，algorithm=2/NetEase）：
        0xfe | netease_deflate( payload )                          （无 compression byte，总是压缩）
        netease_deflate = raw DEFLATE blocks + sync flush，但**无 final block**（end tag 被去掉）

⚠️ 网易压缩算法（CompressionAlgorithmNetEase=2）关键差异（fatalder compression.go）：
    - 压缩：用 flate.Writer Write + Flush，记录长度后 Close，只取 Flush 后的部分（去掉 final block）
    - 解压：手动补回 final empty stored block {0x01,0x00,0x00,0xFF,0xFF}，且维护跨 batch 的
      32KB 滑动窗口字典（上一个 batch 解压数据的最后 32KB 作为下一个 batch 的 flate dictionary）
    - **无 threshold 判断**：fatalder encoder.go 在 compression!=nil 时总是压缩（与 vanilla 不同）

加密：0xfe 头不加密，[compression + payload] 或 [payload] 整体被 AES-CTR 加密。
"""

from __future__ import annotations

import zlib
from typing import List, Optional

from proloader.net.bedrock.packets import CompressionAlgorithm, ID_BATCH
from proloader.net.raknet.wire import ByteReader, ByteWriter


def _raw_deflate_compress(data: bytes, level: int = 6) -> bytes:
    """Raw DEFLATE 压缩（RFC 1951，无 zlib 头/校验）。

    ⚠️ Gophertunnel 用 klauspost/compress/flate（raw DEFLATE），不是 zlib 格式！
    Python zlib.compress() 输出 zlib 格式（0x78 头 + Adler32），BDS 无法解压。
    必须用 wbits=-15 的 raw DEFLATE 才能和 BDS 互操作。
    """
    co = zlib.compressobj(level, zlib.DEFLATED, -15)
    return co.compress(data) + co.flush(zlib.Z_FINISH)


def _raw_deflate_decompress(data: bytes) -> bytes:
    """Raw DEFLATE 解压（兼容 zlib 格式，自动检测）。"""
    try:
        return zlib.decompress(data, -15)  # raw DEFLATE
    except zlib.error:
        return zlib.decompress(data)  # 回退到 zlib 格式


# ═══════════════════════════════════════════════════════════════════════════════
# 网易定制压缩算法（fatalder compression.go: neteaseCompression）
# ═══════════════════════════════════════════════════════════════════════════════
def _netease_compress(data: bytes, level: int = 6) -> bytes:
    """网易定制 DEFLATE 压缩（无 final block）。

    对齐 fatalder compression.go:neteaseCompression.Compress：
        1. flate.Writer.Write(data) → 压缩数据
        2. flate.Writer.Flush()     → sync flush（产生 0x00 0x00 0xFF 0xFF）
        3. 记录当前长度 validLen
        4. flate.Writer.Close()     → 写 final block（0x03 0x00）
        5. 返回 [:validLen]          → 去掉 final block！

    Python 等价：compressobj(level, DEFLATED, -15) + compress + flush(Z_SYNC_FLUSH)
    不调用 flush(Z_FINISH)，所以没有 final block。

    注意：压缩端无滑动窗口（fatalder Compress 不用 dict），每个 batch 独立压缩。
    """
    co = zlib.compressobj(level, zlib.DEFLATED, -15)
    # Z_SYNC_FLUSH 产生 sync flush 标记（0x00 0x00 0xFF 0xFF），不调用 Z_FINISH
    return co.compress(data) + co.flush(zlib.Z_SYNC_FLUSH)


# 网易解压需要补回的 final empty stored block（end-of-stream 标记）
# DEFLATE: BFINAL=1 + BTYPE=00(stored) + padding + LEN=0x0000 + NLEN=0xFFFF
_NETEASE_FINAL_BLOCK = b"\x01\x00\x00\xff\xff"

# 滑动窗口最大长度（32KB，DEFLATE 规范）
_NETEASE_MAX_WINDOW = 32768


class NeteaseDecompressor:
    """网易定制 DEFLATE 解压器（带跨 batch 滑动窗口字典）。

    对齐 fatalder compression.go:neteaseCompression.Decompress：
        1. 给压缩数据补回 final empty stored block {0x01,0x00,0x00,0xFF,0xFF}
        2. 用上一个 batch 的最后 32KB 解压数据作为 flate dictionary 创建新 reader
        3. 解压
        4. 更新 dict = (旧 dict + 新解压数据)[-32768:]

    ⚠️ 滑动窗口是必须的：网易服务端的压缩数据可以引用前一个 batch 的数据（跨 batch 压缩）。
       如果不维护 dict，解压会失败或产生乱码。
    """

    def __init__(self) -> None:
        self._dict: bytes = b""

    def decompress(self, compressed: bytes) -> bytes:
        """解压一个 batch 的网易压缩数据。"""
        # 1. 补回 final block 标记
        data = compressed + _NETEASE_FINAL_BLOCK
        # 2. 用滑动窗口字典创建新的 decompressobj
        #    zdict 设置 flate reader 的初始历史窗口
        d = zlib.decompressobj(-15, zdict=self._dict)
        decompressed = d.decompress(data)
        d.flush()
        # 3. 更新滑动窗口字典（保留最后 32KB）
        merged = self._dict + decompressed
        if len(merged) <= _NETEASE_MAX_WINDOW:
            self._dict = merged
        else:
            self._dict = merged[-_NETEASE_MAX_WINDOW:]
        return decompressed


# ═══════════════════════════════════════════════════════════════════════════════
# Batch 编解码
# ═══════════════════════════════════════════════════════════════════════════════
def encode_batch(
    packets: List[bytes],
    algorithm: int = CompressionAlgorithm.NONE,
    threshold: int = 0,
    compression_enabled: bool = True,
    netease_mode: bool = False,
) -> bytes:
    """把多个 packet 合并成 Batch 包。

    Args:
        packets: 各 packet 的完整字节（含 packet header）
        algorithm: 压缩算法（0=flate, 1=snappy, 2=netease, 255=none）
        threshold: 压缩阈值（vanilla 用，网易忽略——总是压缩）
        compression_enabled: 是否已启用压缩（协商前 False → 无 compression byte）
        netease_mode: 网易模式（True=始终不写 compression byte）

    Returns:
        完整 Batch 包

    网易模式规则（fatalder encoder.go:105-114 确认）：
        - compression != nil 时**总是压缩**（无 threshold 判断！）
        - NetEase 算法不写 compression byte
        - 其他算法写 compression byte
    """
    # payload: varuint(len)+packet 重复
    w = ByteWriter()
    for p in packets:
        w.write_unsigned_varint(len(p))
        w.write(p)
    payload = w.to_bytes()

    out = ByteWriter()
    out.write_byte(ID_BATCH)  # 0xfe

    # 网易模式：始终不写 compression byte
    if netease_mode:
        if compression_enabled:
            # 协商后总是压缩（fatalder 无 threshold 判断）
            if algorithm == CompressionAlgorithm.NETEASE:
                out.write(_netease_compress(payload))
            elif algorithm == CompressionAlgorithm.FLATE:
                # 兜底：FLATE 也不写 byte（不应出现在网易服务器，但防御性处理）
                out.write(_raw_deflate_compress(payload))
            else:
                # 未知算法：不压缩（防御性）
                out.write(payload)
        else:
            # 协商前：不压缩，无 byte
            out.write(payload)
        return out.to_bytes()

    # vanilla 模式
    if not compression_enabled:
        # 未协商压缩（RequestNetworkSettings 阶段）：无 compression byte，直接 payload
        # Gophertunnel: compression == nil 时不写 byte
        out.write(payload)
        return out.to_bytes()

    # 已协商：写 compression byte
    should_compress = algorithm != CompressionAlgorithm.NONE and (
        threshold == 0 or len(payload) >= threshold
    )
    if not should_compress:
        # 包小：用 NopCompression（写 0xff byte，但不实际压缩）
        out.write_byte(CompressionAlgorithm.NONE & 0xFF)
        out.write(payload)
    elif algorithm == CompressionAlgorithm.FLATE:
        out.write_byte(CompressionAlgorithm.FLATE & 0xFF)
        out.write(_raw_deflate_compress(payload))  # raw DEFLATE（非 zlib）
    elif algorithm == CompressionAlgorithm.SNAPPY:
        raise NotImplementedError("Snappy 压缩暂未实现（BDS 默认 flate，够用）")
    else:
        raise ValueError(f"未知压缩算法: {algorithm}")
    return out.to_bytes()


def decode_batch(
    data: bytes,
    netease_mode: bool = False,
    netease_decompressor: Optional[NeteaseDecompressor] = None,
) -> List[bytes]:
    """解析 Batch 包，返回内部 packet 列表。

    Args:
        data: 完整 Batch 包 [0xfe, ...]（若已解密，传入完整含 0xfe 头的字节）
        netease_mode: 网易模式（True=无 compression byte）
        netease_decompressor: 网易解压器实例（有状态，维护滑动窗口字典）。
            传入时用滑动窗口解压；不传时网易模式按明文处理（用于协商前的原始数据）。

    Returns:
        各 packet 的完整字节列表（含 packet header）
    """
    r = ByteReader(data)
    head = r.read_byte()
    if head != ID_BATCH:
        raise ValueError(f"不是 Batch 包: 0x{head:02x}")
    rest = r.read(r.remaining)
    if len(rest) < 1:
        raise ValueError("Batch 包过短")

    # 网易模式 + 有解压器 → 用滑动窗口解压
    if netease_decompressor is not None:
        try:
            payload = netease_decompressor.decompress(rest)
        except Exception:
            # 解压失败：可能是未压缩的原始数据（防御性回退）
            payload = rest
        return _split_payload(payload)

    # 网易模式 + 无解压器 → 协商前的原始数据，不压缩
    if netease_mode:
        return _split_payload(rest)

    # vanilla 模式：判断 rest[0] 是否是合法 compression byte
    if rest[0] in (CompressionAlgorithm.FLATE, CompressionAlgorithm.SNAPPY, CompressionAlgorithm.NONE):
        compression = rest[0]
        compressed = rest[1:]
        if compression == CompressionAlgorithm.FLATE:
            payload = _raw_deflate_decompress(compressed)  # 兼容 raw DEFLATE 和 zlib
        elif compression == CompressionAlgorithm.SNAPPY:
            raise NotImplementedError("Snappy 解压暂未实现")
        else:  # NONE (NopCompression)
            payload = compressed
    else:
        # 无 compression byte（旧格式）：rest 直接是 payload
        payload = rest
    return _split_payload(payload)


def _split_payload(payload: bytes) -> List[bytes]:
    """拆 payload: varuint(len)+packet 重复。"""
    packets: List[bytes] = []
    r = ByteReader(payload)
    while r.remaining > 0:
        n = r.read_unsigned_varint()
        packets.append(r.read(n))
    return packets
