"""把可替换的 cookie 来源接到已有租赁服认证与 Bedrock 会话。

同步认证方法；调用异步程序时可放入 asyncio.to_thread，避免阻塞事件循环。
"""
from __future__ import annotations

from dataclasses import dataclass, field
from typing import Protocol

from cryptography.hazmat.primitives.asymmetric import ec

from proloader.net.bedrock.crypto import generate_keypair, serialize_public_key_b64
from proloader.net.bedrock.session import BedrockSession
from proloader.net.netease.user_center import CookieGrant


class CookieProvider(Protocol):
    def fetch_cookie(self) -> CookieGrant: ...


@dataclass(frozen=True)
class RentalLogin:
    host: str
    port: int
    uid: int
    random_mode: bool
    jwt_chain: str = field(repr=False)
    private_key: ec.EllipticCurvePrivateKey = field(repr=False)
    protocol: int = 860

    def make_session(self, raknet) -> BedrockSession:
        return BedrockSession(
            raknet, netease_mode=True, netease_jwt_chain=self.jwt_chain,
            netease_uid=self.uid, netease_protocol=self.protocol, priv_key=self.private_key,
        )


def prepare_rental_login(client, provider: CookieProvider, server_code: str,
                         server_password: str = '', *, protocol: int = 860) -> RentalLogin:
    """一次领取身份，顺序执行 PE→查询→进入世界→Link→chain。

    provider 返回 random_mode 仅作元数据保留，不将其解释为已创建新账号。
    HTTP client 的 bootstrap 及关闭由调用者负责，未获取来源时不回退旧 cookie。
    """
    if not server_code.strip() or protocol not in (686, 860):
        raise ValueError('需要服务器编号以及支持的协议版本')
    grant = provider.fetch_cookie()
    client.authenticate_with_cookie(grant.cookie)
    uid = int(client.user_id)
    if uid <= 0:
        raise ValueError('认证返回的网易 UID 无效')
    server = client.search_rental_server(server_code)
    host, port = client.enter_rental_server_world(server.entity_id, server_password)
    if not isinstance(host, str) or not host or type(port) is not int or not 0 < port < 65536:
        raise ValueError('进入世界接口返回无效地址')
    client.connect_link_and_send_game_start(server_code, server)
    private_key = generate_keypair()
    request = client.generate_rental_game_auth_v2(server.entity_id, serialize_public_key_b64(private_key))
    chain = client.send_auth_v2_request(request)
    result = RentalLogin(host, port, uid, grant.random_mode, chain, private_key, protocol)
    # 在交出上下文前复用实际登录编码器检查公钥绑定，防止认证来源返回别人的 chain。
    session = result.make_session(None)
    session._client_data.ServerAddress = f'{host}:{port}'
    session._build_connection_request()
    return result
