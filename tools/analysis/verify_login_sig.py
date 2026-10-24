#!/usr/bin/env python3
"""验证网易 Login 的 JWT 签名：前插 JWT 和 RawToken 是否能用客户端公钥验签。

用法: python3 -m tools.verify_login_sig
"""
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.hazmat.primitives.asymmetric.utils import encode_dss_signature

from proloader.net.bedrock.crypto import (
    generate_keypair,
    serialize_public_key_b64,
    serialize_public_key_spki,
    load_public_key_spki,
)
from proloader.net.bedrock.login import (
    ClientData,
    build_login_from_jwt_chain,
    parse_jwt,
    sign_jwt,
)
from proloader.net.netease.client import NeteaseClient


def verify_jwt_signature(jwt_str: str, pub_key) -> bool:
    """用公钥验证 JWT 签名（ES384）。"""
    header_b64, payload_b64, sig_b64 = jwt_str.split(".")
    signing_input = f"{header_b64}.{payload_b64}".encode("ascii")

    # base64url 解码签名
    import base64
    padding = 4 - len(sig_b64) % 4
    if padding != 4:
        sig_b64 += "=" * padding
    signature = base64.urlsafe_b64decode(sig_b64)

    # JOSE (r||s) → DER
    if len(signature) != 96:
        print(f"  ⚠️ 签名长度 {len(signature)} != 96（P-384 应为 96）")
        return False
    r = int.from_bytes(signature[:48], "big")
    s = int.from_bytes(signature[48:], "big")
    der_sig = encode_dss_signature(r, s)

    try:
        pub_key.verify(der_sig, signing_input, ec.ECDSA(hashes.SHA384()))
        return True
    except Exception as e:
        print(f"  ✗ 验签失败: {e}")
        return False


def main():
    # 1. 获取 jwt_chain
    cookie_path = Path(__file__).resolve().parents[1] / "cache" / "netease" / "cookie.json"
    cookie_str = cookie_path.read_text()

    client = NeteaseClient(platform="ios")
    client.bootstrap()
    client.authenticate_with_cookie(cookie_str)

    server_code = "12345678"
    server_entity = client.search_rental_server(server_code)
    host, port = client.enter_rental_server_world(server_entity.entity_id, "")
    client.connect_link_and_send_game_start(server_code, server_entity)

    priv = generate_keypair()
    client_key = serialize_public_key_b64(priv)
    authv2 = client.generate_rental_game_auth_v2(server_entity.entity_id, client_key)
    jwt_chain = client.send_auth_v2_request(authv2)

    print(f"\n{'='*70}")
    print("JWT 签名验证")
    print(f"{'='*70}")

    # 2. 构建 ConnectionRequest
    client_data = ClientData()
    client_data.GameVersion = "1.21.50"
    client_data.ServerAddress = f"{host}:{port}"

    conn_req = build_login_from_jwt_chain(jwt_chain, client_data, priv)

    # 3. 解析 ConnectionRequest
    import struct
    chain_len = struct.unpack("<i", conn_req[:4])[0]
    chain_json = conn_req[4:4+chain_len].decode()
    new_chain = json.loads(chain_json)["chain"]

    raw_token_offset = 4 + chain_len
    raw_token_len = struct.unpack("<i", conn_req[raw_token_offset:raw_token_offset+4])[0]
    raw_token = conn_req[raw_token_offset+4:raw_token_offset+4+raw_token_len].decode()

    print(f"chain 包含 {len(new_chain)} 个 JWT")
    print(f"RawToken 长度: {len(raw_token)} 字符")
    print()

    # 4. 验证每个 JWT 的签名
    client_pub = priv.public_key()

    # chain[0] = 前插 JWT（客户端签名），用客户端公钥验签
    print("--- chain[0]（前插 JWT，客户端签名）---")
    header, payload, _ = parse_jwt(new_chain[0])
    print(f"  header.x5u = {header.get('x5u', 'N/A')[:40]}...")
    print(f"  payload.identityPublicKey = {payload.get('identityPublicKey', 'N/A')[:40]}...")
    ok = verify_jwt_signature(new_chain[0], client_pub)
    print(f"  {'✓ 签名验证通过' if ok else '✗ 签名验证失败'}（用客户端公钥）")

    # chain[1] = 网易 CA（Mojang key 签名），用 Mojang key 验签
    print("\n--- chain[1]（网易 CA，Mojang key 签名）---")
    header1, payload1, _ = parse_jwt(new_chain[1])
    print(f"  header.x5u = {header1.get('x5u', 'N/A')[:40]}...")
    print(f"  payload.identityPublicKey = {payload1.get('identityPublicKey', 'N/A')[:40]}...")
    # 用 chain[0].identityPublicKey（Mojang key）验签
    mojang_key_b64 = payload.get("identityPublicKey")
    if mojang_key_b64:
        import base64
        mojang_key_der = base64.b64decode(mojang_key_b64)
        mojang_key = load_public_key_spki(mojang_key_der)
        ok1 = verify_jwt_signature(new_chain[1], mojang_key)
        print(f"  {'✓ 签名验证通过' if ok1 else '✗ 签名验证失败'}（用 Mojang key）")

    # chain[2] = 玩家身份（网易中间公钥签名），用网易中间公钥验签
    print("\n--- chain[2]（玩家身份，网易中间公钥签名）---")
    header2, payload2, _ = parse_jwt(new_chain[2])
    print(f"  header.x5u = {header2.get('x5u', 'N/A')[:40]}...")
    print(f"  payload.identityPublicKey = {payload2.get('identityPublicKey', 'N/A')[:40]}...")
    netease_mid_b64 = payload1.get("identityPublicKey")
    if netease_mid_b64:
        import base64
        netease_mid_der = base64.b64decode(netease_mid_b64)
        netease_mid_key = load_public_key_spki(netease_mid_der)
        ok2 = verify_jwt_signature(new_chain[2], netease_mid_key)
        print(f"  {'✓ 签名验证通过' if ok2 else '✗ 签名验证失败'}（用网易中间公钥）")

    # RawToken（客户端签名），用 chain[2].identityPublicKey（客户端公钥）验签
    print("\n--- RawToken（ClientData JWT，客户端签名）---")
    rt_header, rt_payload, _ = parse_jwt(raw_token)
    print(f"  header.x5u = {rt_header.get('x5u', 'N/A')[:40]}...")
    print(f"  payload 字段数: {len(rt_payload)}")
    print(f"  payload 有 exp/nbf: {'exp' in rt_payload or 'nbf' in rt_payload}")
    # chain[2].identityPublicKey 应该是客户端公钥
    final_identity_pub_b64 = payload2.get("identityPublicKey")
    if final_identity_pub_b64:
        import base64
        final_identity_pub_der = base64.b64decode(final_identity_pub_b64)
        final_identity_pub = load_public_key_spki(final_identity_pub_der)
        # 验证是否等于客户端公钥
        client_pub_der = serialize_public_key_spki(priv)
        if final_identity_pub_der == client_pub_der:
            print(f"  ✓ chain[2].identityPublicKey == 客户端公钥")
        else:
            print(f"  ✗ chain[2].identityPublicKey != 客户端公钥")
        ok3 = verify_jwt_signature(raw_token, final_identity_pub)
        print(f"  {'✓ 签名验证通过' if ok3 else '✗ 签名验证失败'}（用 chain[2].identityPublicKey）")

    # 5. 总结
    print(f"\n{'='*70}")
    print("签名验证总结:")
    print(f"  chain[0] (前插, 客户端签名): {'✓' if ok else '✗'}")
    print(f"  chain[1] (网易CA, Mojang签名): {'✓' if ok1 else '✗'}")
    print(f"  chain[2] (玩家身份, 网易签名): {'✓' if ok2 else '✗'}")
    print(f"  RawToken (客户端签名): {'✓' if ok3 else '✗'}")
    if ok and ok1 and ok2 and ok3:
        print("\n  ✓ 所有签名验证通过！问题不在签名，可能在 ClientData 字段值或 chain 结构。")
    else:
        print("\n  ✗ 有签名验证失败！这就是 'Connection Request invalid' 的根因。")


if __name__ == "__main__":
    main()
