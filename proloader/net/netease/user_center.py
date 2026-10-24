"""Fatalder 用户中心的显式配置适配器，不内置服务地址、API Key 或共享私钥。

对齐 cache/fatalder/extracted/utils/user_center/{operator,crypto}.go。
领取的是 sauth cookie，后续仍须完成 PE、进入世界、Link 和 JWT chain 申请。
"""
from __future__ import annotations

import base64
import ipaddress
import json
import secrets
from dataclasses import dataclass, field
from urllib.parse import urlsplit

import requests
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.asymmetric import padding, rsa
from cryptography.hazmat.primitives.ciphers.aead import AESGCM


class UserCenterError(RuntimeError):
    """只包含可记录的错误类别，不回显请求/响应凭据。"""


def _encode(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b'=').decode('ascii')


def _decode(data: str) -> bytes:
    return base64.b64decode(data + '=' * (-len(data) % 4), altchars=b'-_', validate=True)


def seal_envelope(plaintext: bytes, recipient: rsa.RSAPublicKey) -> dict:
    key, iv = secrets.token_bytes(32), secrets.token_bytes(12)
    encrypted = AESGCM(key).encrypt(iv, plaintext, None)
    encrypted_key = recipient.encrypt(key, padding.OAEP(
        mgf=padding.MGF1(hashes.SHA256()), algorithm=hashes.SHA256(), label=None,
    ))
    return {'k': _encode(encrypted_key), 'iv': _encode(iv), 'ct': _encode(encrypted[:-16]),
            'tag': _encode(encrypted[-16:]), 'alg': 'AES-256-GCM', 'rsa': 'RSA-OAEP-SHA256'}


def open_envelope(envelope: dict, recipient: rsa.RSAPrivateKey) -> bytes:
    # Go 响应允许省略 alg/rsa，但显式声明其他算法时不默默接受。
    try:
        if not isinstance(envelope, dict):
            raise ValueError
        if envelope.get('alg', 'AES-256-GCM') != 'AES-256-GCM' or envelope.get('rsa', 'RSA-OAEP-SHA256') != 'RSA-OAEP-SHA256':
            raise ValueError
        key = recipient.decrypt(_decode(envelope['k']), padding.OAEP(
            mgf=padding.MGF1(hashes.SHA256()), algorithm=hashes.SHA256(), label=None,
        ))
        iv, ct, tag = (_decode(envelope[k]) for k in ('iv', 'ct', 'tag'))
        if len(key) != 32 or len(iv) != 12 or len(tag) != 16:
            raise ValueError
        return AESGCM(key).decrypt(iv, ct + tag, None)
    except Exception:
        raise UserCenterError('用户中心响应加密封包无效') from None


@dataclass(frozen=True)
class CookieGrant:
    cookie: str = field(repr=False)
    random_mode: bool = False


class UserCenterCookieProvider:
    """调用已授权服务的 /api/phoenix/sauth；传入的 Session 由调用者管理。

    RSA 密钥是用户中心传输密钥，与 Bedrock P-384 登录密钥用途不同。
    """

    def __init__(self, base_url: str, api_key: str, server_key: rsa.RSAPublicKey,
                 client_key: rsa.RSAPrivateKey, *, session=None, timeout: float = 15):
        url = urlsplit(base_url)
        loopback = url.hostname == 'localhost'
        try:
            loopback = loopback or ipaddress.ip_address(url.hostname or '').is_loopback
        except ValueError:
            pass
        if (not url.hostname or url.username or url.password or url.query or url.fragment or
                url.path not in ('', '/') or
                not (url.scheme == 'https' or (url.scheme == 'http' and loopback))):
            raise ValueError('用户中心需要 HTTPS 根地址，或本机 HTTP 测试地址')
        if not api_key.strip() or timeout <= 0:
            raise ValueError('用户中心需要 API Key 和正数超时')
        if not isinstance(server_key, rsa.RSAPublicKey) or not isinstance(client_key, rsa.RSAPrivateKey):
            raise ValueError('用户中心传输密钥必须为 RSA')
        self._base_url = base_url.rstrip('/')
        self._api_key = api_key
        self._server_key, self._client_key = server_key, client_key
        self._session = session
        self._timeout = timeout

    def fetch_cookie(self) -> CookieGrant:
        envelope = seal_envelope(b'{}', self._server_key)
        # 无业务负载的 GET 仍含加密 {}，严格保留参考实现的行为。
        session = self._session if self._session is not None else requests.Session()
        try:
            response = session.request(
                'GET', self._base_url + '/api/phoenix/sauth',
                data=json.dumps(envelope, separators=(',', ':')),
                headers={'Content-Type': 'application/json', 'X-API-Key': self._api_key},
                timeout=self._timeout, allow_redirects=False,
            )
            try:
                if not 200 <= response.status_code < 300:
                    raise UserCenterError(f'用户中心 HTTP {response.status_code}')
                payload = json.loads(open_envelope(response.json(), self._client_key))
                if not isinstance(payload, dict) or payload.get('success') is not True:
                    raise UserCenterError('用户中心未授权或未能分配登录态')
                data = payload['data']
                cookie = data['login_token']
                random_mode = data.get('random_mode', False)
                if not isinstance(cookie, str) or not cookie or type(random_mode) is not bool:
                    raise ValueError
                # Auth() 把该字符串直接送 G79AuthenticateWithCookie，它必须是 cookie JSON。
                obj = json.loads(cookie)
                sauth = json.loads(obj['sauth_json'])
                if not all(isinstance(sauth.get(k), str) and sauth[k].strip()
                           for k in ('sdkuid', 'sessionid', 'udid', 'deviceid')):
                    raise ValueError
                return CookieGrant(cookie, random_mode)
            finally:
                response.close()
        except UserCenterError:
            raise
        except Exception:
            raise UserCenterError('用户中心请求失败或登录态格式无效') from None
        finally:
            if self._session is None:
                session.close()
