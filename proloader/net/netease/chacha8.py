"""ChaCha8 流密码引擎（Python 移植自 database64128/chacha8-go）。

网易 Link 连接使用的 ChaCha8 变体：
    key:   32 字节
    nonce: 12 字节（固定 b"163 NetEase\\n"）
    rounds: 8（4 次双轮 = 8 轮）

对齐 g79client/utils/chacha8.go → database64128/chacha8-go/chacha8。
状态布局（IETF 格式）：
    word  0-3:  常量 "expand 32-byte k"
    word  4-11: 密钥（8 个 uint32，小端）
    word    12: 计数器（初始 0，每 64 字节块 +1）
    word 13-15: nonce（3 个 uint32，小端）
"""

from __future__ import annotations

import struct

# ChaCha 常量 "expand 32-byte k"
_CONSTANTS = (0x61707865, 0x3320646e, 0x79622d32, 0x6b206574)
_BLOCK_SIZE = 64


def _rotl32(v: int, n: int) -> int:
    return ((v << n) | (v >> (32 - n))) & 0xFFFFFFFF


def _quarter_round(s: list[int], a: int, b: int, c: int, d: int) -> None:
    s[a] = (s[a] + s[b]) & 0xFFFFFFFF
    s[d] = _rotl32(s[d] ^ s[a], 16)
    s[c] = (s[c] + s[d]) & 0xFFFFFFFF
    s[b] = _rotl32(s[b] ^ s[c], 12)
    s[a] = (s[a] + s[b]) & 0xFFFFFFFF
    s[d] = _rotl32(s[d] ^ s[a], 8)
    s[c] = (s[c] + s[d]) & 0xFFFFFFFF
    s[b] = _rotl32(s[b] ^ s[c], 7)


class ChaCha8Engine:
    """ChaCha8 流密码引擎（有状态，多次 process 调用行为像连续流）。

    对齐 chacha8-go 的 Cipher.XORKeyStream。
    """

    def __init__(self, key: bytes, nonce: bytes) -> None:
        if len(key) != 32:
            raise ValueError(f"key must be 32 bytes, got {len(key)}")
        if len(nonce) != 12:
            raise ValueError(f"nonce must be 12 bytes, got {len(nonce)}")

        self._key = struct.unpack("<8I", key)
        self._nonce = struct.unpack("<3I", nonce)
        self._counter = 0
        self._keystream = self._block(0)
        self._pos = 0

    def _block(self, counter: int) -> bytes:
        state = [
            *_CONSTANTS,
            *self._key,
            counter,
            *self._nonce,
        ]
        original = list(state)

        # 8 轮 = 4 次双轮（每次双轮 = 4 column QR + 4 diagonal QR）
        for _ in range(4):
            _quarter_round(state, 0, 4, 8, 12)
            _quarter_round(state, 1, 5, 9, 13)
            _quarter_round(state, 2, 6, 10, 14)
            _quarter_round(state, 3, 7, 11, 15)
            _quarter_round(state, 0, 5, 10, 15)
            _quarter_round(state, 1, 6, 11, 12)
            _quarter_round(state, 2, 7, 8, 13)
            _quarter_round(state, 3, 4, 9, 14)

        out = [(state[i] + original[i]) & 0xFFFFFFFF for i in range(16)]
        return struct.pack("<16I", *out)

    def process(self, data: bytes) -> bytes:
        """加密/解密（XORKeyStream，同一操作）。多次调用行为像连续流。"""
        if not data:
            return b""

        result = bytearray(len(data))
        for i, b in enumerate(data):
            if self._pos >= _BLOCK_SIZE:
                self._counter += 1
                self._keystream = self._block(self._counter)
                self._pos = 0
            result[i] = b ^ self._keystream[self._pos]
            self._pos += 1

        return bytes(result)
