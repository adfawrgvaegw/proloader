"""无需游戏客户端、账号或网络的网易登录格式与会话回归测试。"""
import asyncio
import base64
import json
import struct
from unittest.mock import AsyncMock

import pytest
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.hazmat.primitives.asymmetric.utils import encode_dss_signature

from proloader.net.bedrock.crypto import generate_keypair, serialize_public_key_b64
from proloader.net.bedrock.login import (
    ClientData, build_login_from_jwt_chain, parse_jwt, sign_jwt,
)
from proloader.net.bedrock.packets import PacketID
from proloader.net.bedrock.session import BedrockSession
from proloader.net.raknet.wire import ByteReader, ByteWriter


@pytest.fixture
def auth_chain():
    # 模拟认证 API 的 CA→玩家链，末端绑定客户端公钥。
    client, ca, intermediate = (generate_keypair() for _ in range(3))
    chain = []
    for signer, target in ((ca, intermediate), (intermediate, client)):
        chain.append(sign_jwt(signer, {"alg": "ES384", "x5u": serialize_public_key_b64(signer)},
                              {"identityPublicKey": serialize_public_key_b64(target), "iss": "NetEase"}))
    return client, chain


def unpack_request(data):
    r = ByteReader(data)
    chain = json.loads(r.read(struct.unpack("<i", r.read(4))[0]))
    raw = r.read(struct.unpack("<i", r.read(4))[0]).decode()
    assert r.pos == len(data)
    return chain, raw


def verify(token, private):
    _, claims, sig = parse_jwt(token)
    der = encode_dss_signature(int.from_bytes(sig[:48], "big"), int.from_bytes(sig[48:], "big"))
    private.public_key().verify(der, token.rsplit(".", 1)[0].encode(), ec.ECDSA(hashes.SHA384()))
    return claims


@pytest.mark.parametrize("protocol", [686, 860])
@pytest.mark.parametrize("as_object", [False, True])
def test_chain_layout_and_signatures(auth_chain, protocol, as_object):
    key, original = auth_chain
    request = build_login_from_jwt_chain(
        json.dumps({"chain": original} if as_object else original),
        ClientData(ServerAddress="localhost:19132"), key, protocol=protocol,
    )
    envelope, raw = unpack_request(request)
    if protocol == 686:
        assert set(envelope) == {"chain"}
        assert envelope["chain"][1:] == original
        bridge = verify(envelope["chain"][0], key)
        assert bridge["certificateAuthority"] is True
        assert bridge["identityPublicKey"] == parse_jwt(original[0])[0]["x5u"]
    else:
        assert envelope["AuthenticationType"] == 0
        assert envelope["Token"] == ""
        assert json.loads(envelope["Certificate"])["chain"] == original
    claims = verify(raw, key)
    assert claims["ServerAddress"] == "localhost:19132"
    assert "exp" not in claims and "nbf" not in claims


@pytest.mark.parametrize("bad", ["[]", '{}', '{"chain":"abc"}', '[1]', '[""]', '["bad.jwt.data"]'])
def test_reject_malformed_chain(bad):
    with pytest.raises(ValueError):
        build_login_from_jwt_chain(bad, ClientData(), generate_keypair())


def test_reject_wrong_key(auth_chain):
    _, chain = auth_chain
    with pytest.raises(ValueError, match="私钥"):
        build_login_from_jwt_chain(json.dumps(chain), ClientData(), generate_keypair())


@pytest.mark.parametrize("kwargs", [
    {"netease_mode": True},
    {"netease_mode": True, "netease_jwt_chain": "[]"},
    {"netease_jwt_chain": "[]"},
])
def test_require_explicit_netease_credentials(kwargs):
    with pytest.raises(ValueError):
        BedrockSession(object(), **kwargs)


async def test_invalid_chain_sends_nothing(auth_chain):
    _, chain = auth_chain
    raknet = AsyncMock()
    session = BedrockSession(raknet, netease_mode=True, netease_jwt_chain=json.dumps(chain),
                             priv_key=generate_keypair())
    with pytest.raises(ValueError):
        await session.login("localhost:19132")
    raknet.send_reliable.assert_not_called()
    assert session._recv_task is None


@pytest.mark.parametrize("protocol,version,os", [(686, "1.21.2", 1), (860, "1.21.120", 2)])
async def test_session_login_sequence(auth_chain, protocol, version, os):
    key, chain = auth_chain
    session = BedrockSession(AsyncMock(), netease_mode=True, netease_jwt_chain=json.dumps(chain),
                             netease_protocol=protocol, priv_key=key)
    session._send_packet = AsyncMock()
    # 模拟服务端握手，真实走 P-384 ECDH 派生；阻止启动网络收包。
    session._recv_loop = AsyncMock()
    server = generate_keypair()
    jwt = sign_jwt(server, {"alg": "ES384", "x5u": serialize_public_key_b64(server)},
                   {"salt": base64.b64encode(b"0123456789abcdef").decode()})
    w = ByteWriter()
    w.write_unsigned_varint(PacketID.SERVER_TO_CLIENT_HANDSHAKE)
    w.write_string(jwt)
    session._wait_packet = AsyncMock(side_effect=[
        b'\x8f\x01' + struct.pack('<HH', 0, 0) + b'\0' * 6,
        w.to_bytes(), b'\x02' + struct.pack('>i', 0),
    ])
    await session.login("localhost:19132")
    await asyncio.sleep(0)
    sent = [call.args[0] for call in session._send_packet.call_args_list]
    assert [session._read_packet_id(p)[0] for p in sent] == [193, 1, 4, 129]
    assert struct.unpack('>i', sent[0][-4:])[0] == protocol
    r = ByteReader(sent[1]); assert r.read_unsigned_varint() == 1
    assert struct.unpack('>i', r.read(4))[0] == protocol
    envelope, raw = unpack_request(r.read(r.read_unsigned_varint()))
    assert ("chain" in envelope) == (protocol == 686)
    claims = verify(raw, key)
    assert (claims["GameVersion"], claims["DeviceOS"]) == (version, os)
    assert session._crypto is not None
    await session.close()
