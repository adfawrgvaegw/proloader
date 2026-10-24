#!/usr/bin/env python3
"""诊断 ConnectionRequest 结构：解析 JWT chain 信任链 + RawToken 签名验证。

模拟 fatalder request.go:Parse 的验证逻辑，检查：
1. chain 长度（必须 1 或 3）
2. 每个 JWT 的 header.x5u 和 payload.identityPublicKey
3. 信任链：chain[0] 自签 → chain[1] 用 chain[0].identityPublicKey 验 → ...
4. RawToken 用 chain[last].identityPublicKey 验签

用法: python3 -m tools.diag_login_request
"""
import base64
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from proloader.net.bedrock.crypto import (
    generate_keypair,
    load_public_key_spki,
    serialize_public_key_b64,
    serialize_public_key_spki,
)
from proloader.net.bedrock.login import (
    ClientData,
    build_login_from_jwt_chain,
    parse_jwt,
)
from proloader.net.netease.client import NeteaseClient


def b64url_decode(s: str) -> bytes:
    padding = 4 - len(s) % 4
    if padding != 4:
        s += "=" * padding
    return base64.urlsafe_b64decode(s)


def verify_es384(jwt_str: str, pub_key_der_b64: str) -> bool:
    """用 base64 SPKI 公钥验证 ES384 JWT 签名。"""
    from cryptography.hazmat.primitives.asymmetric import ec
    from cryptography.hazmat.primitives.asymmetric.utils import encode_dss_signature
    from cryptography.hazmat.primitives import hashes

    header_b64, payload_b64, sig_b64 = jwt_str.split(".")
    signing_input = f"{header_b64}.{payload_b64}".encode("ascii")
    signature = b64url_decode(sig_b64)

    # JOSE 原始编码 (r||s) → DER
    coord_len = 48  # P-384
    r = int.from_bytes(signature[:coord_len], "big")
    s = int.from_bytes(signature[coord_len:], "big")
    der_sig = encode_dss_signature(r, s)

    pub_der = base64.b64decode(pub_key_der_b64)
    pub_key = load_public_key_spki(pub_der)
    try:
        pub_key.verify(der_sig, signing_input, ec.ECDSA(hashes.SHA384()))
        return True
    except Exception as e:
        return False


def main():
    # 1. 获取 JWT chain
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
    print("网易 JWT chain 原始结构")
    print(f"{'='*70}")
    parsed = json.loads(jwt_chain)
    if isinstance(parsed, list):
        chain_list = parsed
    else:
        chain_list = parsed.get("chain", [])

    for i, jwt_str in enumerate(chain_list):
        header, payload, sig = parse_jwt(jwt_str)
        print(f"\n--- 网易 JWT[{i}] ---")
        print(f"  header:  {json.dumps(header)}")
        print(f"  payload keys: {list(payload.keys())}")
        if "identityPublicKey" in payload:
            print(f"  identityPublicKey: {payload['identityPublicKey'][:40]}...")
        if "extraData" in payload:
            print(f"  extraData: {json.dumps(payload['extraData'])}")
        if "exp" in payload:
            print(f"  exp: {payload['exp']}")

    # 2. 构建 ConnectionRequest（前插模式）
    print(f"\n{'='*70}")
    print("构建 ConnectionRequest（prepend_client_jwt=True）")
    print(f"{'='*70}")

    cd = ClientData()
    cd.GameVersion = "1.21.70"
    cd.ServerAddress = f"{host}:{port}"
    conn_req = build_login_from_jwt_chain(jwt_chain, cd, priv, prepend_client_jwt=True)

    # 3. 解析 ConnectionRequest
    import struct
    r = conn_req
    chain_len = struct.unpack("<i", r[:4])[0]
    chain_json = r[4:4+chain_len]
    rest = r[4+chain_len:]
    raw_token_len = struct.unpack("<i", rest[:4])[0]
    raw_token = rest[4:4+raw_token_len].decode("ascii")

    chain_data = json.loads(chain_json)
    new_chain = chain_data["chain"]

    print(f"chain 长度: {len(new_chain)}")
    print(f"chainJSON 长度: {chain_len}")
    print(f"RawToken 长度: {raw_token_len}")

    # 4. 验证信任链（模拟 fatalder Parse case 3）
    print(f"\n{'='*70}")
    print("信任链验证（模拟 fatalder Parse）")
    print(f"{'='*70}")

    current_key_b64 = None
    for i, jwt_str in enumerate(new_chain):
        header, payload, sig = parse_jwt(jwt_str)
        header_x5u = header.get("x5u", "")
        print(f"\n--- chain[{i}] ---")
        print(f"  header: {json.dumps(header)}")
        print(f"  header.x5u: {header_x5u[:50]}...")

        # 验签：chain[0] 用自身 header.x5u，后续用前一个的 identityPublicKey
        verify_key = header_x5u if i == 0 else current_key_b64
        if verify_key:
            ok = verify_es384(jwt_str, verify_key)
            print(f"  验签（key={verify_key[:30]}...）: {'✅ 成功' if ok else '❌ 失败'}")
        else:
            print(f"  验签: ⚠️ 无验签 key")

        # 更新 current_key 为此 JWT 的 identityPublicKey
        idpk = payload.get("identityPublicKey")
        if idpk:
            current_key_b64 = idpk
            print(f"  payload.identityPublicKey: {idpk[:50]}...")
        else:
            print(f"  payload.identityPublicKey: (无)")

    # 5. 验证 RawToken
    print(f"\n--- RawToken ---")
    header, payload, sig = parse_jwt(raw_token)
    print(f"  header: {json.dumps(header)}")
    print(f"  header.x5u: {header.get('x5u', '')[:50]}...")
    print(f"  payload keys: {list(payload.keys())}")
    if current_key_b64:
        ok = verify_es384(raw_token, current_key_b64)
        print(f"  验签（key=chain[last].identityPublicKey={current_key_b64[:30]}...）: {'✅ 成功' if ok else '❌ 失败'}")

    # 检查 RawToken 是否有 exp/nbf（fatalder 不应有）
    if "exp" in payload:
        print(f"  ⚠️ RawToken 含 exp 字段（fatalder 不应有）")
    if "nbf" in payload:
        print(f"  ⚠️ RawToken 含 nbf 字段（fatalder 不应有）")


if __name__ == "__main__":
    main()
