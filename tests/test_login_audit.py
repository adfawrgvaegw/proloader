import base64
import json
import struct

import pytest

from proloader.net.bedrock.crypto import generate_keypair, serialize_public_key_b64
from proloader.net.bedrock.login import ClientData, build_login_from_jwt_chain, parse_jwt, sign_jwt
from proloader.net.bedrock.login_audit import audit_connection_request


def make_request(*, expires=200, not_before=0, broken_link=False, corrupt_raw=False):
    root, intermediate, client = (generate_keypair() for _ in range(3))
    chain = []
    for signer, target in ((root, intermediate), (intermediate, client)):
        chain.append(sign_jwt(signer, {'alg': 'ES384', 'x5u': serialize_public_key_b64(signer)},
                              {'identityPublicKey': serialize_public_key_b64(target),
                               'nbf': not_before, 'exp': expires}))
    if broken_link:
        claims = parse_jwt(chain[1])[1]
        wrong = generate_keypair()
        chain[1] = sign_jwt(wrong, {'alg': 'ES384', 'x5u': serialize_public_key_b64(wrong)}, claims)
    request = build_login_from_jwt_chain(json.dumps(chain), ClientData(), client)
    if corrupt_raw:
        length = struct.unpack_from('<i', request)[0]
        prefix = request[:length + 4]
        wrong = generate_keypair()
        raw = sign_jwt(wrong, {'alg': 'ES384', 'x5u': serialize_public_key_b64(wrong)},
                       ClientData().to_jwt_claims()).encode()
        request = prefix + struct.pack('<i', len(raw)) + raw
    return request


def test_valid_chain_skin_and_safe_report():
    request = make_request()
    result = audit_connection_request(request, now=100)
    assert result['chain_signatures'] == [True, True]
    assert result['chain_validity'] == ['valid', 'valid']
    assert result['raw_signature'] and result['raw_key_matches']
    assert result['skin_size_matches'] and result['skin_geometry_default_exists']
    assert result['ca_trust'] == 'not_checked'
    assert 'eyJ' not in json.dumps(result)


@pytest.mark.parametrize('kwargs,expected', [({'expires': 99}, 'expired'),
                                           ({'not_before': 101}, 'not_yet_valid')])
def test_token_time_bounds(kwargs, expected):
    assert audit_connection_request(make_request(**kwargs), now=100)['chain_validity'] == [expected] * 2


def test_broken_intermediate_is_detected_even_when_terminal_key_matches():
    result = audit_connection_request(make_request(broken_link=True), now=100)
    assert result['chain_signatures'] == [True, False]
    assert result['chain_advertised_keys_match'] == [True, False]
    assert result['raw_signature']


def test_raw_token_signed_by_different_key():
    result = audit_connection_request(make_request(corrupt_raw=True), now=100)
    assert not result['raw_signature'] and not result['raw_key_matches']


def test_legacy_geometry_and_scaled_texture_are_not_rejected():
    key = generate_keypair()
    token = sign_jwt(key, {'alg': 'ES384', 'x5u': serialize_public_key_b64(key)},
                     {'identityPublicKey': serialize_public_key_b64(key)})
    data = ClientData()
    data.SkinGeometryData = base64.b64encode(json.dumps({
        'geometry.humanoid.custom': {'texturewidth': 64, 'textureheight': 64},
    }).encode()).decode()
    result = audit_connection_request(build_login_from_jwt_chain(json.dumps([token]), data, key))
    assert result['geometry_schema'] == 'legacy'
    assert result['skin_geometry_default_exists']
    assert result['geometry_texture_dimensions'] == [[64, 64]]
    assert result['skin_dimensions'] == [256, 256]
    assert result['chain_validity'] == ['bounds_missing']


@pytest.mark.parametrize('mutate', [lambda b: b[:-1], lambda b: b + b'private-extra',
                                    lambda b: struct.pack('<i', -1) + b[4:]])
def test_bad_lengths_have_fixed_private_error(mutate):
    with pytest.raises(ValueError, match='^malformed Login audit input$'):
        audit_connection_request(mutate(make_request()))
