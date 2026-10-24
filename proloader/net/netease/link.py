"""网易 Link 连接（Python 移植自 g79client/service/link_connection）。

完整流程：
    TCP 连接 → RSA-2048 握手 → ChaCha8 密钥派生 → LoginV2 → GameStart → 关闭

Link 连接用于通知网易后端"玩家要进入游戏"，是 Bedrock 连接的前置步骤。
（fatalder login.go:454-496 在 SendAuthV2Request 之前先做 Link+GameStart）

用法：
    conn = dial_link(session, user_id, user_token)
    conn.send_game_start(payload)
    conn.close()
"""

from __future__ import annotations

import base64
import hashlib
import json
import logging
import secrets
import socket
from typing import Any, Optional

from cryptography.hazmat.primitives.asymmetric import padding as asym_padding
from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes
from cryptography.hazmat.primitives.serialization import (
    load_der_private_key,
    load_der_public_key,
)

from proloader.net.netease.chacha8 import ChaCha8Engine
from proloader.net.netease.constants import LINK_SERVER_LIST_URL

logger = logging.getLogger(__name__)

# ─── RSA 密钥常量（来自 g79client/service/link_connection/connection.go:40-42）───
_SERVER_PUBLIC_KEY_B64 = (
    "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEArht9ioo6tc3Z7On/80iYjNI+HpxnEpSc"
    "0tXC9JLykvwkxluZiLPrlvO6sgkkPsQMBXudGRu335dBCVdwfMefY7wswrQG51U+Nw3xfSSRgSpt"
    "NV8PcmNjh6EYAluRSy7AZLcWc6+qJ6fJFOeABGYxNwMVvbpDC0R+t7BtcmQCk+4uXP2dsRjJSs6AL"
    "lfT7iEs8IL7iRfu1IvomTAc6eJarStgxEBTWdV/d2XfoIshbNYQ9ziBk0iWzHoI15UXFWLL+jZwhQ"
    "YwzB0f+ilckgFT0IFKU4msUUQ7io4CBY2iI1G0BnSQYwpm84WR0HrgKL7uoxtJQ98iPk2GbZgWFv1"
    "OTwIDAQAB"
)
_CLIENT_PRIVATE_KEY_B64 = (
    "MIIEpAIBAAKCAQEAx/Ipxphk+HO16oMY8HBOpauKgrdBnuQpH3LhJzsEkgYVK88ObW8w5ts3e0+U"
    "WCRJYoCwxPT2CPKeUvJr86gt3ajaOSXEej9iudOUcU6C5bARmkET5EXGvRLH7dnqJt0X2oGUG7H4O"
    "kit8+4UydcbjLs3VfXZba73+zqVUWhJPnpm5bnVzrsqu/mDo/EuClH9zp8oWz2RQfyCdVPtCwmY8wg"
    "/nrrUZNVV/AbgchuTedUHgs0IQ7eqUEZTVWrUFkkv82OvWUlu2RFxfFw4e9cTzEJJVPGxLmowFpFNK"
    "zp206jP3uL23VdnHrI8m4CGOqIlyfXgfCfIu/NxbLnzRnbQpQIDAQABAoIBAEvGXciy1olGKOpAVsJ"
    "Afb3RfgO9+bOC2obdnbClcDz66ykYJmqY2hqTd7pW1Wx2DA21ochy4Y9Qi2n6D6le0ksQA+vmgUinH"
    "v43zikGzRrJGFKyWRyIySG8rWJZ1KB35+NaekvorZ9BDhPE5cH8sKcsCHOeYZFs3vQqJo6cjC2NwBS"
    "C+8HzeO+Btcgz88FX46tFvNokeGuJqZEEcfhss5dggEdHiUKTJsRXIdxwXqnYv08uPEzn/ogsnCBc5"
    "+ujwwds2qxiua599J8lY5KbB3/kBVqtea2MYUX/JUNEAIQRm7ygJ0DJJKRjPO0vYnkxXWC2waY5cRB"
    "pKQiTmJVwvf0CgYEA7xA8816Y42zRk5eU1ngmVX1GEoVkK6wf0obIgSMHlrwKl5u4rb3Vu8qGIPZ/OP"
    "w6zIr+sfAmsR2hFy6mAjQrLsGoGmegU1s6Ogr5fqMw55/OWv7tEhfI+myX2N9xw9IpodBsFfvd8yDD"
    "MmByiFtFx1oGu41VbbAJZdvsdlBIyV8CgYEA1hx5DspkRth44zIzgV+7EmCRSghCJ0c85MJwohpXX2"
    "wKg09gWm9AuuH74DEo0akIZGIRUZqxelMh3dc/mKmQmoMRbfMJojb7fytw3CtZEDNiXXDBtrZW2FFr"
    "wixK8XzFCKjYxwqOCt+kYbQcCz7MABWUb+heUxH+QMYtXTRr8HsCgYEAqup6GSktt5NKNvItmDQofA"
    "Bng8BYgJy716FDYogv2cWw8PmFTLonP+6ofJKfHJfAVhKdy4u9re1YCaHxUCwKH5CW5eHmjxHvDCZi"
    "f/aedUsclpQh3EijCN9wpL4DsRPlben8DK+Y3EU1KSQpXnGa7s7fd2GxjQ1JesiEQ4Zcs5MCgYEAkd"
    "fpOgLw1TUk+xU58jkkMztmG/iOHzUuLGCp2jF5LH1ql9Ecv90iSWofaLHzrQSnu8D1LRHjLICuA+9X"
    "2YQ/BJCc8bjn6f/rxc7wXHiGfTuTGDTzLqL7evPTI/uJvP6RM/nXV5U/9fYqgYbux1YqHTCV4Lh2b7"
    "1E5BhZ1DAeCjsCgYA5taKd1CCN1KyflOpiDJPskQSBdF9wO+P+dUdmFZDNccUFk2rNQ63idcH/n2L2"
    "58Wf9SiI2dDjaaxUEetP3Ei75eHf4ixw22zlZ/kl+/BPWBdmTb+22xzoVMnt/ya/kvEC27/GAbrV4v"
    "+c5WFTs+ExWWkYbrpYJ6LnxAHiNf7/Uw=="
)
_CLIENT_PUBLIC_KEY_MD5_HEX = "3f17d36cec77e47f1d301c0cc41cd73f"
_NETEASE_CHACHA_NONCE = b"163 NetEase\n"

# RSA-2048 密钥大小（字节）
_RSA_KEY_SIZE = 256  # 2048 / 8


# ═══════════════════════════════════════════════════════════════════════════
# VarInt（无符号 LEB128，int32 当 uint32 处理）
# 对齐 g79client/service/link_connection/connection.go:writeVarInt/readVarInt
# ═══════════════════════════════════════════════════════════════════════════

def _write_varint(value: int) -> bytes:
    v = value & 0xFFFFFFFF
    out = bytearray()
    while True:
        b = v & 0x7F
        v >>= 7
        if v != 0:
            out.append(b | 0x80)
        else:
            out.append(b)
            break
    return bytes(out)


def _read_varint_from_buf(buf: bytes, offset: int) -> tuple[int, int]:
    """从缓冲区读取 VarInt，返回 (value, bytes_consumed)。"""
    result = 0
    shift = 0
    pos = offset
    while pos < len(buf):
        b = buf[pos]
        pos += 1
        result |= (b & 0x7F) << shift
        if not (b & 0x80):
            return result, pos - offset
        shift += 7
        if shift >= 35:
            raise ValueError("VarInt too large")
    raise ValueError("Incomplete VarInt")


# ═══════════════════════════════════════════════════════════════════════════
# RSA 密钥加载（懒加载）
# ═══════════════════════════════════════════════════════════════════════════

_server_pub_key = None
_client_priv_key = None


def _get_server_pub_key():
    global _server_pub_key
    if _server_pub_key is None:
        der = base64.b64decode(_SERVER_PUBLIC_KEY_B64)
        _server_pub_key = load_der_public_key(der)
    return _server_pub_key


def _get_client_priv_key():
    global _client_priv_key
    if _client_priv_key is None:
        der = base64.b64decode(_CLIENT_PRIVATE_KEY_B64)
        _client_priv_key = load_der_private_key(der, password=None)
    return _client_priv_key


# ═══════════════════════════════════════════════════════════════════════════
# TCP 辅助
# ═══════════════════════════════════════════════════════════════════════════

def _recv_exact(sock: socket.socket, n: int) -> bytes:
    """从 socket 精确读取 n 字节。"""
    data = bytearray()
    while len(data) < n:
        chunk = sock.recv(n - len(data))
        if not chunk:
            raise ConnectionError(f"连接已关闭（已读 {len(data)}/{n} 字节）")
        data.extend(chunk)
    return bytes(data)


def _recv_varint(sock: socket.socket) -> int:
    """从 socket 逐字节读取 VarInt。"""
    result = 0
    shift = 0
    while True:
        b = _recv_exact(sock, 1)[0]
        result |= (b & 0x7F) << shift
        if not (b & 0x80):
            break
        shift += 7
        if shift >= 35:
            raise ValueError("VarInt too large")
    return result


# ═══════════════════════════════════════════════════════════════════════════
# Link 服务器列表
# ═══════════════════════════════════════════════════════════════════════════

def fetch_link_servers(session) -> list[dict]:
    """获取 Link 服务器列表。

    对齐 g79client/globals.go:fetchG79LinkServers。
    GET LinkServerURL → JSON 数组 [{ip, port, ...}, ...]
    """
    resp = session.get(
        LINK_SERVER_LIST_URL,
        headers={
            "User-Agent": "libhttpclient/1.0.0.0",
            "Accept-Encoding": "gzip",
            "content-type": "text/plain",
        },
        timeout=15,
    )
    resp.raise_for_status()
    return resp.json()


# ═══════════════════════════════════════════════════════════════════════════
# LinkConnection
# ═══════════════════════════════════════════════════════════════════════════

class LinkConnection:
    """网易 Link 连接。

    完整流程：TCP 连接 → RSA 握手 → ChaCha8 → LoginV2 → GameStart → 关闭

    用法：
        conn = LinkConnection(host, port, user_id, user_token)
        conn.login()
        conn.send_game_start(payload)
        conn.close()
    """

    def __init__(self, host: str, port: int, user_id: str, user_token: str) -> None:
        self._host = host
        self._port = port
        self._user_id = user_id
        self._user_token = user_token
        self._sock: Optional[socket.socket] = None
        self._encrypt: Optional[ChaCha8Engine] = None
        self._decrypt: Optional[ChaCha8Engine] = None
        self._cipher_r1: bytes = b""
        self._handshake_done = False
        self._online = False

    # ── 连接与握手 ──────────────────────────────────────────

    def connect(self) -> None:
        """TCP 连接 + RSA 握手 + ChaCha8 密钥派生。"""
        self._sock = socket.create_connection((self._host, self._port), timeout=10)
        self._sock.settimeout(15)
        self._send_handshake()
        self._complete_handshake()
        logger.info("Link 握手完成: %s:%d", self._host, self._port)

    def _send_handshake(self) -> None:
        """发送 RSA 加密的 cipherR1 + 客户端公钥 MD5。

        对齐 connection.go:sendHandshake。
        """
        # 1. 生成 16 字节随机 cipherR1
        self._cipher_r1 = secrets.token_bytes(16)

        # 2. RSA-2048 加密 cipherR1（用服务器公钥，PKCS1v15）
        pub_key = _get_server_pub_key()
        encrypted_r1 = pub_key.encrypt(self._cipher_r1, asym_padding.PKCS1v15())

        # 3. 发送加密的 cipherR1（256 字节）
        self._sock.sendall(encrypted_r1)

        # 4. 发送客户端公钥 MD5（16 字节）
        md5_bytes = bytes.fromhex(_CLIENT_PUBLIC_KEY_MD5_HEX)
        self._sock.sendall(md5_bytes)

        logger.debug("Link 握手: 已发送 cipherR1 (%d bytes) + MD5 (%d bytes)",
                     len(encrypted_r1), len(md5_bytes))

    def _complete_handshake(self) -> None:
        """接收并解密 cipherR2，派生 ChaCha8 密钥。

        对齐 connection.go:completeHandshake。
        """
        # 1. 接收 RSA 加密的 cipherR2（256 字节）
        encrypted_r2 = _recv_exact(self._sock, _RSA_KEY_SIZE)

        # 2. RSA 解密（用客户端私钥，PKCS1v15）
        priv_key = _get_client_priv_key()
        plain = priv_key.decrypt(encrypted_r2, asym_padding.PKCS1v15())

        # 3. 去掉尾部 \x00
        plain = plain.rstrip(b"\x00")
        if len(plain) != 16:
            raise ValueError(f"cipherR2 长度异常: {len(plain)}（期望 16）")

        cipher_r2 = plain

        # 4. 派生 ChaCha8 密钥
        encrypt_key = self._cipher_r1 + cipher_r2  # 32 字节
        decrypt_key = cipher_r2 + self._cipher_r1  # 32 字节

        self._encrypt = ChaCha8Engine(encrypt_key, _NETEASE_CHACHA_NONCE)
        self._decrypt = ChaCha8Engine(decrypt_key, _NETEASE_CHACHA_NONCE)
        self._handshake_done = True

    # ── 消息收发 ────────────────────────────────────────────

    def _send_message(self, server_id: int, method: str, payload: bytes) -> None:
        """发送加密的 Link 消息。

        对齐 connection.go:sendMessage。
        帧格式: VarInt(len(encrypted)) + encrypted
        body 格式: VarInt(serverID) + flag(0) + len(method) + method + payload
        """
        if self._encrypt is None:
            raise RuntimeError("加密引擎未初始化")

        # 构建 body
        body = bytearray()
        body.extend(_write_varint(server_id))
        body.append(0)  # flag
        body.append(len(method))
        body.extend(method.encode("ascii"))
        body.extend(payload)

        # 加密
        encrypted = self._encrypt.process(bytes(body))

        # 发送帧
        frame = _write_varint(len(encrypted)) + encrypted
        self._sock.sendall(frame)

        logger.debug("Link 发送: method=%s, serverID=%d, body=%d bytes",
                      method, server_id, len(body))

    def _recv_message(self) -> dict[str, Any]:
        """接收并解密一条 Link 消息。

        对齐 connection.go:decodeMessage。
        """
        # 1. 读取帧长度 + 加密 body
        length = _recv_varint(self._sock)
        encrypted = _recv_exact(self._sock, length)

        # 2. 解密
        plain = self._decrypt.process(encrypted)

        # 3. 解析: VarInt(serverID) + flag + len(method) + method + payload
        server_id, off = _read_varint_from_buf(plain, 0)
        flag = plain[off]
        off += 1
        method_len = plain[off]
        off += 1
        method = plain[off:off + method_len].decode("ascii")
        off += method_len
        payload = plain[off:]

        logger.debug("Link 接收: method=%s, serverID=%d, payload=%d bytes",
                      method, server_id, len(payload))
        return {
            "server_id": server_id,
            "flag": flag,
            "method": method,
            "payload": payload,
        }

    # ── LoginV2 ─────────────────────────────────────────────

    def login(self) -> None:
        """执行 LoginV2 登录（握手 + 发送 LoginV2 + 等待响应）。

        对齐 connection.go:sendLoginRequest + waitForLogin。
        """
        if not self._handshake_done:
            self.connect()

        # 1. 构建 LoginV2 payload
        uid = int(self._user_id)

        # s1: 16 字节随机数（hex）
        rand_block = secrets.token_bytes(16)
        s1 = rand_block.hex()

        # s2: AES-ECB 加密 rand_block（key = MD5(user_token)，16 字节）
        token_hash = hashlib.md5(self._user_token.encode()).digest()
        s2 = self._aes_ecb_encrypt_block(rand_block, token_hash).hex()

        payload = json.dumps({
            "uid": uid,
            "s1": s1,
            "s2": s2,
            "is_zip": False,
        }, separators=(",", ":")).encode()

        # 2. 发送 LoginV2
        self._send_message(0, "LoginV2", payload)

        # 3. 等待 LoginV2 响应（可能收到其他消息，跳过）
        while True:
            msg = self._recv_message()
            if msg["method"] == "LoginV2":
                resp = json.loads(msg["payload"])
                code = resp.get("code", -1)
                if code != 0:
                    raise RuntimeError(f"Link LoginV2 失败 (code={code}): {resp}")
                self._online = True
                logger.info("Link LoginV2 成功")
                return
            logger.debug("Link 等待 LoginV2 响应，收到 %s", msg["method"])

    @staticmethod
    def _aes_ecb_encrypt_block(plaintext: bytes, key: bytes) -> bytes:
        """AES-ECB 单块加密（无填充）。

        对齐 g79client/utils/aes.go:AesECBEncryptBlock。
        """
        cipher = Cipher(algorithms.AES(key), modes.ECB())
        encryptor = cipher.encryptor()
        return encryptor.update(plaintext) + encryptor.finalize()

    # ── GameStart ───────────────────────────────────────────

    def send_game_start(self, payload: dict[str, Any]) -> None:
        """发送 GameStart 消息。

        对齐 connection.go:SendGameStart。
        发送后不需要等待响应（fatalder 发完就关闭连接）。
        """
        if not self._online:
            raise RuntimeError("Link 连接未登录")
        data = json.dumps(payload, separators=(",", ":")).encode()
        self._send_message(0, "GameStart", data)
        logger.info("Link GameStart 已发送")

    # ── 关闭 ────────────────────────────────────────────────

    def close(self) -> None:
        if self._sock:
            try:
                self._sock.close()
            except Exception:
                pass
            self._sock = None
        self._online = False


# ═══════════════════════════════════════════════════════════════════════════
# 入口函数
# ═══════════════════════════════════════════════════════════════════════════

def dial_link(
    session,
    user_id: str,
    user_token: str,
) -> LinkConnection:
    """建立 Link 连接（选服务器 + 握手 + LoginV2）。

    Args:
        session: requests.Session（用于获取 Link 服务器列表）
        user_id: PE 认证返回的 user_id
        user_token: PE 认证返回的 user_token

    Returns:
        已登录的 LinkConnection
    """
    servers = fetch_link_servers(session)
    if not servers:
        raise RuntimeError("没有可用的 Link 服务器")

    entry = secrets.choice(servers)
    host = entry["ip"]
    port = int(entry["port"])
    logger.info("Link 服务器: %s:%d（共 %d 个）", host, port, len(servers))

    conn = LinkConnection(host, port, user_id, user_token)
    conn.login()
    return conn
