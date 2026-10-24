import json
from types import SimpleNamespace
from unittest.mock import Mock

import pytest
from cryptography.hazmat.primitives.asymmetric import rsa

from proloader.net.bedrock.crypto import generate_keypair, serialize_public_key_b64
from proloader.net.bedrock.login import sign_jwt
from proloader.net.netease.rental_login import prepare_rental_login
from proloader.net.netease.user_center import (
    CookieGrant, UserCenterCookieProvider, UserCenterError, seal_envelope, open_envelope,
)


@pytest.fixture(scope='module')
def keys():
    return [rsa.generate_private_key(public_exponent=65537, key_size=2048) for _ in range(2)]


def cookie():
    return json.dumps({'sauth_json': json.dumps(dict.fromkeys(
        ['sdkuid', 'sessionid', 'udid', 'deviceid'], 'TEST_SECRET'))})


def provider(keys, payload, *, status=200, tamper=False):
    server, client = keys
    transport = Mock()
    def request(method, url, **kw):
        assert method == 'GET' and url == 'https://example.test/api/phoenix/sauth'
        assert open_envelope(json.loads(kw['data']), server) == b'{}'
        assert kw['headers']['X-API-Key'] == 'API_SECRET'
        assert kw['allow_redirects'] is False
        response = Mock(status_code=status)
        envelope = seal_envelope(json.dumps(payload).encode(), client.public_key())
        if tamper:
            envelope['tag'] = 'AAAAAAAAAAAAAAAAAAAAAA'
        response.json.return_value = envelope
        return response
    transport.request.side_effect = request
    return UserCenterCookieProvider('https://example.test', 'API_SECRET', server.public_key(), client,
                                    session=transport), transport


def test_encrypted_cookie_request_and_random_mode(keys):
    p, transport = provider(keys, {'success': True, 'data': {'login_token': cookie(), 'random_mode': True}})
    grant = p.fetch_cookie()
    assert grant.cookie == cookie() and grant.random_mode is True
    assert 'TEST_SECRET' not in repr(grant)
    assert 'API_SECRET' not in repr(p)
    transport.request.assert_called_once()


@pytest.mark.parametrize('payload', [
    {'success': False, 'data': {'login_token': cookie()}},
    {'success': True, 'data': {'login_token': 'not-json'}},
    {'success': True, 'data': {'login_token': '{}'}},
    {'success': True, 'data': {'login_token': cookie(), 'random_mode': 'true'}},
])
def test_invalid_grants_rejected(keys, payload):
    p, _ = provider(keys, payload)
    with pytest.raises(UserCenterError) as exc:
        p.fetch_cookie()
    assert 'TEST_SECRET' not in str(exc.value)


@pytest.mark.parametrize('status', [301, 401, 403, 500])
def test_http_failure_does_not_use_body_or_follow_redirect(keys, status):
    p, _ = provider(keys, {'secret': 'PRIVATE_RESPONSE'}, status=status)
    with pytest.raises(UserCenterError, match=f'HTTP {status}'):
        p.fetch_cookie()


def test_modified_gcm_tag_rejected(keys):
    p, _ = provider(keys, {'success': True, 'data': {'login_token': cookie()}}, tamper=True)
    with pytest.raises(UserCenterError, match='封包无效'):
        p.fetch_cookie()


@pytest.mark.parametrize('url', ['http://example.test', 'https://user:pass@example.test',
                                  'https://example.test/api', 'https://example.test?key=secret'])
def test_reject_ambiguous_or_unprotected_remote_address(keys, url):
    with pytest.raises(ValueError):
        UserCenterCookieProvider(url, 'key', keys[0].public_key(), keys[1])


@pytest.mark.parametrize('protocol', [686, 860])
def test_provider_to_chain_to_session_preserves_key_binding(protocol):
    events = []
    p = Mock()
    p.fetch_cookie.side_effect = lambda: (events.append('grant') or CookieGrant(cookie(), True))
    client = Mock(user_id='42')
    server = SimpleNamespace(entity_id='server-id')
    client.authenticate_with_cookie.side_effect = lambda value: events.append('PE')
    client.search_rental_server.side_effect = lambda value: (events.append('search') or server)
    client.enter_rental_server_world.side_effect = lambda *a: (events.append('world') or ('127.0.0.1', 19132))
    client.connect_link_and_send_game_start.side_effect = lambda *a: events.append('link')
    ca = generate_keypair()
    def auth(server_id, public_key):
        events.append('authv2')
        return public_key
    def chain(public_key):
        events.append('chain')
        jwt = sign_jwt(ca, {'alg': 'ES384', 'x5u': serialize_public_key_b64(ca)},
                        {'identityPublicKey': public_key})
        return json.dumps([jwt])
    client.generate_rental_game_auth_v2.side_effect = auth
    client.send_auth_v2_request.side_effect = chain
    login = prepare_rental_login(client, p, '123456', 'PASS', protocol=protocol)
    assert events == ['grant', 'PE', 'search', 'world', 'link', 'authv2', 'chain']
    assert login.uid == 42 and login.random_mode is True
    assert login.make_session(None)._priv is login.private_key
    assert login.jwt_chain not in repr(login)
    client.enter_rental_server_world.assert_called_once_with('server-id', 'PASS')


def test_unavailable_provider_cannot_fall_back_to_old_cookie():
    p = Mock(); p.fetch_cookie.side_effect = UserCenterError('unavailable')
    client = Mock()
    with pytest.raises(UserCenterError):
        prepare_rental_login(client, p, '123456')
    client.authenticate_with_cookie.assert_not_called()
