"""Offline Login checks. Returned data contains no tokens, keys or player identity.

Signature linkage starts at the first token's advertised key. This checks internal
consistency, not trust in a NetEase CA or acceptance by a game server.
"""
import base64
import json
import math
import struct
import time

from cryptography.exceptions import InvalidSignature
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.hazmat.primitives.asymmetric.utils import encode_dss_signature

from .crypto import load_public_key_spki
from .login import parse_jwt


def _key(value):
    key = load_public_key_spki(base64.b64decode(value, validate=True))
    if not isinstance(key, ec.EllipticCurvePublicKey) or not isinstance(key.curve, ec.SECP384R1):
        raise ValueError("P-384 key required")
    return key


def _verify(token, key):
    header, claims, sig = parse_jwt(token)
    if header.get("alg") != "ES384" or len(sig) != 96:
        return False
    der = encode_dss_signature(int.from_bytes(sig[:48], "big"), int.from_bytes(sig[48:], "big"))
    try:
        key.verify(der, token.rsplit(".", 1)[0].encode("ascii"), ec.ECDSA(hashes.SHA384()))
        return True
    except InvalidSignature:
        return False


def _validity(claims, now):
    for name in ("nbf", "exp"):
        if name in claims and (type(claims[name]) not in (int, float) or not math.isfinite(claims[name])):
            return "invalid"
    if "nbf" in claims and now < claims["nbf"]:
        return "not_yet_valid"
    if "exp" in claims and now >= claims["exp"]:
        return "expired"
    return "valid" if "nbf" in claims and "exp" in claims else "bounds_missing"


def audit_connection_request(request: bytes, *, now=None) -> dict:
    """Check length framing, chain signatures, RawToken and skin consistency.

    Malformed input raises a fixed error, without echoing credential content.
    Geometry texture dimensions are reported separately: differing dimensions
    can describe scaled skins and do not alone establish an invalid Login.
    """
    try:
        offset = 0

        def read_string():
            nonlocal offset
            length = struct.unpack_from("<i", request, offset)[0]
            offset += 4
            if length < 0 or length > len(request) - offset:
                raise ValueError
            value = request[offset:offset + length].decode("utf-8")
            offset += length
            return value

        envelope = json.loads(read_string())
        if "AuthenticationType" in envelope and type(envelope["AuthenticationType"]) is not int:
            raise ValueError
        raw_token = read_string()
        if offset != len(request):
            raise ValueError
        certificate = json.loads(envelope["Certificate"]) if "Certificate" in envelope else envelope
        chain = certificate["chain"]
        if not isinstance(chain, list) or not chain:
            raise ValueError
        decoded = [parse_jwt(token) for token in chain]
        key = _key(decoded[0][0]["x5u"])
        signatures, advertised_keys, validity = [], [], []
        at = time.time() if now is None else now
        for token, (header, claims, _) in zip(chain, decoded):
            signatures.append(_verify(token, key))
            advertised_keys.append(_key(header["x5u"]).public_numbers() == key.public_numbers()
                                   if "x5u" in header else None)
            validity.append(_validity(claims, at))
            key = _key(claims["identityPublicKey"])
        raw_header, claims, _ = parse_jwt(raw_token)
        raw_valid = _verify(raw_token, key)
        raw_key_matches = _key(raw_header["x5u"]).public_numbers() == key.public_numbers()

        skin = base64.b64decode(claims["SkinData"], validate=True)
        width, height = claims["SkinImageWidth"], claims["SkinImageHeight"]
        if type(width) is not int or type(height) is not int:
            raise ValueError
        patch = json.loads(base64.b64decode(claims["SkinResourcePatch"], validate=True))
        geometry = json.loads(base64.b64decode(claims["SkinGeometryData"], validate=True))
        default = patch["geometry"]["default"]
        descriptions = [g["description"] for g in geometry.get("minecraft:geometry", [])]
        matches = [d for d in descriptions if d.get("identifier") == default]
        schema = "modern" if "minecraft:geometry" in geometry else "unrecognized_or_empty"
        if default in geometry and isinstance(geometry[default], dict):
            schema = "legacy"
            legacy = geometry[default]
            matches = [{"texture_width": legacy.get("texturewidth"),
                        "texture_height": legacy.get("textureheight")}]
        return {
            "envelope": "certificate" if "Certificate" in envelope else "legacy",
            "authentication_type": envelope.get("AuthenticationType"),
            "chain_count": len(chain), "chain_signatures": signatures,
            "chain_advertised_keys_match": advertised_keys, "chain_validity": validity,
            "raw_signature": raw_valid, "raw_key_matches": raw_key_matches,
            "skin_dimensions": [width, height], "skin_bytes": len(skin),
            "skin_size_matches": type(width) is int and type(height) is int and
                                 width > 0 and height > 0 and len(skin) == width * height * 4,
            "skin_geometry_default_exists": bool(matches),
            "geometry_schema": schema,
            "geometry_texture_dimensions": [[d.get(k) if type(d.get(k)) is int else None
                                             for k in ("texture_width", "texture_height")]
                                            for d in matches],
            "ca_trust": "not_checked",
        }
    except (ValueError, TypeError, KeyError, IndexError, AttributeError, struct.error):
        raise ValueError("malformed Login audit input") from None
