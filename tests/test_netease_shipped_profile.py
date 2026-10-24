"""Sample-bound wire regression and independent ARM64 static golden vectors."""
import json
from unittest.mock import Mock

import pytest

from proloader.net.netease.client import NeteaseClient
from proloader.net.netease.crypto import pe_auth_sign, g79_http_encrypt, g79_http_decrypt, get_valid_json


def test_native_block_boundary_is_explicit():
    # Register transcription of shipped function 0x97e9f24, documented in
    # cache/rea-loader-20261018/auth-sign-static-validation.json.
    text = ('proloader-offline-vector-' * 20)[:256]
    assert pe_auth_sign(text, 3, 6, pad_full_block=True) == 'QYmHkolivDiJElg+KQArSQ=='
    assert pe_auth_sign(text, 3, 6) == 'uVn8d4mCP9+Jaib4Ca2tHw=='


def _client():
    c = NeteaseClient(pe_auth_profile='official-android-3915')
    c.engine_version = '3.9.15.297907'
    c.g79_latest_version = '3.9.41.300665'
    c.patch_resources_hash = '2d2833df3518783e52130aa8951c548f'
    return c


@pytest.mark.parametrize('field,value', [
    ('platform', 'ios'), ('engine_version', '3.10.0.300878'),
    ('g79_latest_version', '3.9.42.999999'), ('patch_resources_hash', '0' * 32),
])
def test_shipped_profile_rejects_unexamined_metadata_before_post(field, value):
    c = _client()
    setattr(c, field, value)
    c._session.post = Mock()
    try:
        with pytest.raises(ValueError, match='inspected metadata'):
            c.authenticate_with_cookie('{}')
        c._session.post.assert_not_called()
    finally:
        c._session.close()


def test_shipped_profile_wire_uses_artifact_hash_and_native_parameters(monkeypatch):
    c = _client()
    monkeypatch.setattr('proloader.net.netease.client.uuid.uuid4', lambda: '00000000-0000-0000-0000-000000000000')
    captured = {}
    def post(url, *, data, **kwargs):
        captured.update(json.loads(get_valid_json(g79_http_decrypt(bytes.fromhex(data)))))
        return Mock(text=g79_http_encrypt(b'{"code":32}').hex())
    c._session.post = post
    cookie = json.dumps({'sauth_json': json.dumps(dict(sdkuid='U', sessionid='T', udid='D', deviceid='X'))})
    try:
        with pytest.raises(RuntimeError, match='code: 32'):
            c.authenticate_with_cookie(cookie)
        assert captured['message'] == (
            '3.9.15.297907260a8b0499822c3f4fb4e35e8051afab'
            '3.9.41.3006652d2833df3518783e52130aa8951c548f'
            '2b3e7ca013bb30a74d822579860c042b'
            '00000000-0000-0000-0000-000000000000'
        )
        assert captured['sign'] == 'Qc02bYlmqueJFoZ3KQSCng=='
    finally:
        c._session.close()


def test_verified_3942_patch_uses_fresh_resource_hash_in_signed_message():
    c=_client()
    c.g79_latest_version='3.9.42.301055'
    c.patch_resources_hash='65ce57607cdb2f70a040a1db7149279d'
    captured={}
    def post(url,*,data,**kwargs):
        captured.update(json.loads(get_valid_json(g79_http_decrypt(bytes.fromhex(data)))))
        return Mock(text=g79_http_encrypt(b'{"code":32}').hex())
    c._session.post=post
    cookie=json.dumps({'sauth_json':json.dumps(dict(sdkuid='U',sessionid='T',udid='D',deviceid='X'))})
    try:
        with pytest.raises(RuntimeError,match='code: 32'):c.authenticate_with_cookie(cookie)
        assert captured['patch_version']=='3.9.42.301055'
        assert '3.9.42.30105565ce57607cdb2f70a040a1db7149279d' in captured['message']
    finally:c._session.close()
