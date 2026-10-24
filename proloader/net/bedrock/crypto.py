"""Bedrock 加密层：AES-256-CTR + SHA256 校验和 + ECDH P-384 密钥交换。

⚠️ 关键技术点（Gophertunnel minecraft/protocol/packet/encryption.go 确认）：
    1. 加密算法是 AES-256-**CTR**（不是 CFB8，老资料有误）
    2. key = keyBytes（32 字节），IV = keyBytes[:12] + [0,0,0,2]（16 字节）
    3. keyBytes = SHA256(salt + sharedSecret)，**salt 在前**
    4. ECDH 用 P-384 曲线，sharedSecret = 共享点 X 坐标（48 字节）
    5. 每包维护递增的 sendCounter（u64 LE），收发两端独立计数
    6. 校验和 = SHA256(sendCounter_LE8 + payload + keyBytes)[:8]，追加到包尾
    7. 加密时 0xfe 头不加密，只加密 payload + checksum
    8. CTR 是流密码，encryptor/decryptor 实例要复用（stream 状态连续）

Login 加密握手流程：
    客户端生成 P-384 密钥对 → 发 Login（含公钥）
    → 服务端回 ServerToClientHandshake（JWT 含服务端公钥 + salt）
    → 客户端 ECDH 派生 keyBytes → 启用加密
    → 发 ClientToServerHandshake（第一个加密包，空 payload）
"""

from __future__ import annotations

import base64
import hashlib
import struct
from dataclasses import dataclass
from typing import Optional

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes


# ═══════════════════════════════════════════════════════════════════════════════
# ECDH P-384 密钥交换
# ═══════════════════════════════════════════════════════════════════════════════
def generate_keypair() -> ec.EllipticCurvePrivateKey:
    """生成 ECDSA P-384 密钥对（用于 Login JWT 签名 + ECDH 密钥交换）。"""
    return ec.generate_private_key(ec.SECP384R1())


def serialize_public_key_spki(priv: ec.EllipticCurvePrivateKey) -> bytes:
    """公钥序列化为 SPKI DER 格式（JWT x5u 字段用）。"""
    return priv.public_key().public_bytes(
        encoding=serialization.Encoding.DER,
        format=serialization.PublicFormat.SubjectPublicKeyInfo,
    )


def serialize_public_key_b64(priv: ec.EllipticCurvePrivateKey) -> str:
    """公钥 SPKI DER → base64 字符串（JWT x5u 字段格式）。"""
    return base64.b64encode(serialize_public_key_spki(priv)).decode("ascii")


def load_public_key_spki(der: bytes) -> ec.EllipticCurvePublicKey:
    """从 SPKI DER 加载公钥。"""
    return serialization.load_der_public_key(der)  # type: ignore


def derive_key_bytes(
    my_priv: ec.EllipticCurvePrivateKey,
    server_pub: ec.EllipticCurvePublicKey,
    salt: bytes,
) -> bytes:
    """ECDH 派生 keyBytes = SHA256(salt + sharedSecret)。

    Args:
        my_priv: 客户端 P-384 私钥
        server_pub: 服务端 P-384 公钥（从 ServerToClientHandshake JWT 的 x5u 解析）
        salt: 16 字节盐（从 JWT 的 salt claim，base64 解码）

    Returns:
        32 字节 keyBytes（AES-256 密钥）
    """
    shared_secret = my_priv.exchange(ec.ECDH(), server_pub)  # 48 字节（P-384）
    return hashlib.sha256(salt + shared_secret).digest()


# ═══════════════════════════════════════════════════════════════════════════════
# BedrockCrypto：AES-256-CTR 加密/解密 + SHA256 校验和
# ═══════════════════════════════════════════════════════════════════════════════
@dataclass
class BedrockCrypto:
    """Bedrock 加密会话。

    一个实例同时管理发送加密和接收解密（两端的 sendCounter 独立递增）。
    CTR 流密码的 encryptor/decryptor 实例必须复用（stream 状态连续）。

    用法：
        crypto = BedrockCrypto(key_bytes)
        encrypted_packet = crypto.encrypt(b'\\xfe' + payload)  # 加密完整包（含 0xfe 头）
        payload = crypto.decrypt(encrypted_tail)                # 解密（已去 0xfe 头）
    """

    key_bytes: bytes  # 32 字节
    _send_counter: int = 0   # 发送计数（加密用）
    _recv_counter: int = 0   # 接收计数（解密用）
    _encryptor: object = None
    _decryptor: object = None

    def __post_init__(self) -> None:
        if len(self.key_bytes) != 32:
            raise ValueError(f"keyBytes 必须 32 字节，得到 {len(self.key_bytes)}")
        # AES-256-CTR，IV = keyBytes[:12] + [0,0,0,2]
        iv = self.key_bytes[:12] + b"\x00\x00\x00\x02"
        cipher = Cipher(algorithms.AES(self.key_bytes), modes.CTR(iv))
        self._encryptor = cipher.encryptor()
        self._decryptor = cipher.decryptor()

    def encrypt(self, data: bytes) -> bytes:
        """加密一个 batch 包。

        Args:
            data: 完整包 [0xfe, payload...]

        Returns:
            加密包 [0xfe, encrypted(payload + checksum)]
            0xfe 头保持明文，payload + checksum 加密。
        """
        if not data or data[0] != 0xFE:
            raise ValueError("encrypt 的 data 必须以 0xfe 开头")
        payload = data[1:]
        # 校验和 = SHA256(sendCounter_LE8 + payload + keyBytes)[:8]
        counter_bytes = struct.pack("<Q", self._send_counter)
        checksum = hashlib.sha256(counter_bytes + payload + self.key_bytes).digest()[:8]
        self._send_counter += 1
        # 加密 payload + checksum（0xfe 头不加密）
        encrypted_tail = self._encryptor.update(payload + checksum)  # type: ignore
        return bytes([0xFE]) + encrypted_tail

    def decrypt(self, data: bytes) -> bytes:
        """解密一个 batch 包的 payload（已去 0xfe 头）。

        Args:
            data: 加密的 payload（不含 0xfe 头）= encrypted(payload + checksum)

        Returns:
            解密的 payload（不含校验和）

        Raises:
            ValueError: 校验和不匹配
        """
        if len(data) < 8:
            raise ValueError(f"加密包太短：{len(data)} 字节（最少 8）")
        decrypted = self._decryptor.update(data)  # type: ignore
        # 后 8 字节是校验和
        checksum = decrypted[-8:]
        payload = decrypted[:-8]
        # 验证校验和
        counter_bytes = struct.pack("<Q", self._recv_counter)
        expected = hashlib.sha256(counter_bytes + payload + self.key_bytes).digest()[:8]
        self._recv_counter += 1
        if checksum != expected:
            raise ValueError(
                f"加密包校验和不匹配：recv_counter={self._recv_counter - 1} "
                f"expected={expected.hex()} got={checksum.hex()}"
            )
        return payload
