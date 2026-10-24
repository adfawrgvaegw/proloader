import json
from unittest.mock import AsyncMock

import pytest
from cryptography.hazmat.primitives import serialization

from proloader.net.bedrock.crypto import generate_keypair, serialize_public_key_b64
from proloader.net.bedrock.login import sign_jwt
from tools.diagnostics import diag_netease_session as diag


@pytest.fixture
def args(tmp_path):
    key = generate_keypair()
    pem = key.private_bytes(serialization.Encoding.PEM, serialization.PrivateFormat.PKCS8,
                            serialization.NoEncryption())
    private = tmp_path / 'key.pem'; private.write_bytes(pem)
    token = sign_jwt(key, {'alg': 'ES384', 'x5u': serialize_public_key_b64(key)},
                     {'identityPublicKey': serialize_public_key_b64(key)})
    chain = tmp_path / 'chain.json'; chain.write_text(json.dumps([token]))
    return diag.parser().parse_args(['--chain-file', str(chain), '--private-key', str(private),
                                     '--host', '127.0.0.1', '--port', '19132', '--uid', '42'])


async def test_default_is_offline_and_no_credentials_printed(args, monkeypatch, capsys):
    connect = AsyncMock()
    monkeypatch.setattr(diag, 'handshake', connect)
    assert await diag.run(args) == 0
    connect.assert_not_called()
    result = json.loads(capsys.readouterr().out)
    assert result == {'stage': '本地认证材料校验', 'ok': True, 'protocol': 860}


async def test_online_handshake_failure_reports_stage_and_closes(args, monkeypatch, capsys):
    args.connect = True
    monkeypatch.setattr(diag, 'handshake', AsyncMock(return_value=object()))
    raknet = AsyncMock()
    raknet.connect.side_effect = TimeoutError('SECRET_DETAIL')
    monkeypatch.setattr(diag, 'RakNetSession', lambda conn: raknet)
    assert await diag.run(args) == 1
    raknet.stop.assert_awaited_once()
    output = capsys.readouterr().out
    assert 'SECRET_DETAIL' not in output
    assert json.loads(output.splitlines()[-1]) == {
        'stage': 'RakNet 在线握手', 'ok': False, 'error': 'TimeoutError',
    }
