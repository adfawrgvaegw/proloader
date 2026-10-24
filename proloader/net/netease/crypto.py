"""网易 G79 HTTP 加密层（Python 移植自 g79client/crypto.go + utils/encryption.go）。

包含：
    - G79HttpEncrypt / G79HttpDecrypt：AES-128-CBC + 16 密钥 + 随机 IV + flag 字节
    - CalculateDynamicToken：MD5 + 二进制旋转 + 位反转 XOR + Base64
    - PeAuthSign：自定义 MD4 变体哈希（PE 认证签名）
    - GetValidJSON：从解密数据中提取有效 JSON
"""

from __future__ import annotations

import base64
import hashlib
import secrets
import string
import struct

from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes

from proloader.net.netease.constants import (
    DYNAMIC_TOKEN_SALT,
    G79_HTTP_KEYS,
)

# ── AES 密钥（hex 解码后的 bytes）─────────────────────────
_KEYS: list[bytes] = [bytes.fromhex(k) for k in G79_HTTP_KEYS]

_CHARSET = string.ascii_letters + string.digits


def _random_string(length: int) -> str:
    """生成随机字母数字字符串（对齐 Go 的 randomString）。"""
    return "".join(secrets.choice(_CHARSET) for _ in range(length))


# ═══════════════════════════════════════════════════════════════════════════════
# G79HttpEncrypt / G79HttpDecrypt
# ═══════════════════════════════════════════════════════════════════════════════
def g79_http_encrypt(body: bytes) -> bytes:
    """G79 HTTP 加密。

    格式：IV(16) + AES-CBC(payload + 16 random chars + zero padding) + flag(1)
    flag = (random_nibble << 4) | 0x0C，高 4 位是密钥索引。

    对齐 g79client/crypto.go:G79HttpEncrypt
    """
    block_size = 16
    # 对齐到 16 字节块（body + 16 random chars，向上取整到 block_size 倍数）
    target_len = ((len(body) + block_size + block_size - 1) // block_size) * block_size
    buffer = bytearray(target_len)
    buffer[:len(body)] = body
    # 添加 16 个随机字符
    tail_random = _random_string(16).encode("ascii")
    buffer[len(body):len(body) + 16] = tail_random
    # 其余自动为 0（zero padding）

    # 生成 flag 字节：高 4 位随机(0-14)，低 4 位 = 0xC
    high_nibble = secrets.randbelow(15)  # 0..14
    flag = (high_nibble << 4) | 0x0C

    # 生成 16 字符随机 IV
    iv = _random_string(16).encode("ascii")

    # 选择密钥
    key_index = (flag >> 4) & 0x0F
    key = _KEYS[key_index]

    # AES-CBC 加密
    cipher = Cipher(algorithms.AES(key), modes.CBC(iv))
    encryptor = cipher.encryptor()
    ciphertext = encryptor.update(bytes(buffer)) + encryptor.finalize()

    # 组装结果：IV(16) + ciphertext + flag(1)
    return iv + ciphertext + bytes([flag])


def g79_http_decrypt(payload: bytes) -> bytes:
    """G79 HTTP 解密。

    对齐 g79client/crypto.go:G79HttpDecrypt
    """
    if len(payload) < 18:
        raise ValueError(f"payload too short: {len(payload)} bytes")

    flag = payload[-1]
    iv = payload[0:16]
    ciphertext = payload[16:-1]

    key_index = (flag >> 4) & 0x0F
    key = _KEYS[key_index]

    cipher = Cipher(algorithms.AES(key), modes.CBC(iv))
    decryptor = cipher.decryptor()
    plaintext = decryptor.update(ciphertext) + decryptor.finalize()

    # 移除尾部零字节
    last_non_zero = len(plaintext) - 1
    while last_non_zero >= 0 and plaintext[last_non_zero] == 0:
        last_non_zero -= 1
    if last_non_zero < 0:
        return b""

    return plaintext[:last_non_zero + 1]


# ═══════════════════════════════════════════════════════════════════════════════
# CalculateDynamicToken
# ═══════════════════════════════════════════════════════════════════════════════
def calculate_dynamic_token(path: str, content: str, token: str) -> str:
    """计算动态 token（用于 HTTP API 请求的 user-token 头）。

    对齐 g79client/crypto.go:CalculateDynamicToken

    算法：
        1. tokenMd5 = md5(token)
        2. payload = tokenMd5 + content + salt + path.rstrip('?')
        3. payloadMd5 = md5(payload)
        4. 将 payloadMd5 的每个字节转为 8 位二进制串
        5. 左移（旋转）6 位
        6. 对每个字节：取旋转后对应的 8 位，反转位序，与原字节 XOR
        7. Base64 编码，取前 16 字符，'+' -> 'm'，'/' -> 'o'，末尾加 '1'
    """
    salt = DYNAMIC_TOKEN_SALT

    # 1. token 的 MD5
    token_md5 = hashlib.md5(token.encode()).hexdigest()

    # 2. 构建 payload
    payload = token_md5 + content + salt + path.rstrip("?")

    # 3. payload 的 MD5
    payload_md5 = hashlib.md5(payload.encode()).hexdigest()
    payload_md5_bytes = payload_md5.encode("ascii")

    # 4. 转为二进制字符串
    binary_string = "".join(f"{b:08b}" for b in payload_md5_bytes)

    # 5. 左移 6 位（循环左移）
    binary_string = binary_string[6:] + binary_string[:6]

    # 6. 位反转 + XOR
    transformed = bytearray(payload_md5_bytes)
    for i in range(len(transformed)):
        section = binary_string[i * 8: i * 8 + 8]
        reversed_byte = 0
        for j in range(8):
            if section[7 - j] == "1":
                reversed_byte |= (1 << j)
        transformed[i] = reversed_byte ^ transformed[i]

    # 7. Base64 编码
    b64 = base64.b64encode(bytes(transformed)).decode("ascii")
    token_short = b64[:16].replace("+", "m").replace("/", "o") + "1"

    return token_short


# ═══════════════════════════════════════════════════════════════════════════════
# PeAuthSign（PE 认证签名，自定义 MD4 变体）
# ═══════════════════════════════════════════════════════════════════════════════
# 常量表（80 字节）
_PEAUTH_B = bytes([
    0x62, 0x25, 0x1e, 0xf6, 0x40, 0xb3, 0x40, 0xc0, 0x51, 0x5a, 0x5e, 0x26, 0xaa, 0xc7, 0xb6, 0xe9,
    0x44, 0xea, 0xbe, 0xa4, 0xa9, 0xcf, 0xde, 0x4b, 0x60, 0x4b, 0xbb, 0xf6, 0x70, 0xbc, 0xbf, 0xbe,
    0xc3, 0x59, 0x5b, 0x65, 0x92, 0xcc, 0x0c, 0x8f, 0x7d, 0xf4, 0xef, 0xff, 0xd1, 0x5d, 0x84, 0x85,
    0xc6, 0x7e, 0x9b, 0x28, 0xfa, 0x27, 0xa1, 0xea, 0x85, 0x30, 0xef, 0xd4, 0x05, 0x1d, 0x88, 0x04,
    0xe6, 0xcd, 0xe1, 0x21, 0xd6, 0x07, 0x37, 0xc3, 0x87, 0x0d, 0xd5, 0xf4, 0xed, 0x14, 0x5a, 0x45,
])

# 位移表（20 字节）
_PEAUTH_Q = bytes([
    0x01, 0x06, 0x0a, 0x0d, 0x02, 0x05, 0x09, 0x0e, 0x04, 0x07, 0x0b, 0x03, 0x03, 0x08, 0x0b, 0x05,
    0x01, 0x07, 0x0b, 0x0e,
])

_MASK32 = 0xFFFFFFFF


def _u32(x: int) -> int:
    """截断到 uint32。"""
    return x & _MASK32


def pe_auth_sign(v: str, sp: int, tr: int, *, pad_full_block: bool = False) -> str:
    """PE 认证签名（自定义 MD4 变体哈希）。

    对齐 g79client/utils/encryption.go:PeAuthSign

    Args:
        v: 待签名的字符串（通常是 versionMessage）
        sp: 子页索引，归档参考使用 2，官方 3.9.15 样本使用 3
        tr: 轮数，归档参考使用 9，官方 3.9.15 样本使用 6
        pad_full_block: 官方样本在整块边界也补 64 个字；默认保留归档语义

    Returns:
        Base64 编码的 16 字节签名
    """
    if sp < 0 or tr <= 0:
        raise ValueError("invalid sp/tr parameters")
    if 16 * sp + 16 > len(_PEAUTH_B):
        raise ValueError(f"sp={sp} out of range for constant table")
    if 4 * sp + 4 > len(_PEAUTH_Q):
        raise ValueError(f"sp={sp} out of range for shift table")

    # pad string to 4-byte boundary
    v_bytes = bytearray(v.encode("ascii"))
    if rem := len(v_bytes) % 4:
        v_bytes.extend(b"0" * (4 - rem))

    # 转为 uint32 数组（大端）
    p = []
    for i in range(0, len(v_bytes), 4):
        t = (v_bytes[i] << 24) | (v_bytes[i + 1] << 16) | (v_bytes[i + 2] << 8) | v_bytes[i + 3]
        p.append(t)

    # pad to 64-element boundary
    rem = len(p) % 64
    if rem or pad_full_block:
        p.extend([0xabcde987] * (64 - rem))

    # 选择常量和位移
    c = [
        int.from_bytes(_PEAUTH_B[16 * sp + i * 4: 16 * sp + i * 4 + 4], "little")
        for i in range(4)
    ]
    q = list(_PEAUTH_Q[4 * sp: 4 * sp + 4])

    # 初始化（MD4 初始值）
    r = 0x67452301
    u = 0xefcdab89
    x = 0x98badcfe
    z = 0x10325476

    for _ in range(tr):
        for j in range(0, len(p), 4):
            # Round F: (u & x) | (~u & z)  — ~u 需要 32 位掩码
            a1 = r + ((u & x) | ((~u & _MASK32) & z))
            r = _u32(u + (_u32(a1 + p[j] + c[0]) << q[0]))

            # Round G: (u & z) | (~z & x)
            a2 = r + ((u & z) | ((~z & _MASK32) & x))
            u = _u32(u + (_u32(a2 + p[j] + c[1]) << q[1]))

            # Round H: u ^ x ^ z
            a3 = r + (u ^ x ^ z)
            x = _u32(u + (_u32(a3 + p[j] + c[2]) << q[2]))

            # Round I: x ^ (u | z)
            term = _u32(x ^ (u | z))
            a4 = r + term
            z = _u32(u + (_u32(a4 + p[j] + c[3]) << q[3]))

    # 输出 16 字节（4 个 uint32，小端）
    out = struct.pack("<IIII", r, u, x, z)
    return base64.b64encode(out).decode("ascii")


# ═══════════════════════════════════════════════════════════════════════════════
# GetValidJSON
# ═══════════════════════════════════════════════════════════════════════════════
def get_valid_json(data: bytes) -> bytes:
    """从解密数据中提取有效 JSON（通过花括号配对）。

    对齐 g79client/crypto.go:GetValidJSON
    """
    blocks_count = 0
    readable_length = 0

    for i, b in enumerate(data):
        if b == 0x7B:  # '{'
            blocks_count += 1
        elif b == 0x7D:  # '}'
            blocks_count -= 1

        if blocks_count == 0 and i != 0:
            readable_length = i + 1
            break

    if readable_length != 0:
        return data[:readable_length]
    return data
