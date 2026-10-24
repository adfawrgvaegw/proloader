#!/usr/bin/env python3
"""诊断网易 JWT chain 结构：解析每个 JWT 的 header 和 payload。

用法: python3 -m tools.diag_jwt_chain
"""
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from proloader.net.bedrock.crypto import generate_keypair, serialize_public_key_b64
from proloader.net.bedrock.login import parse_jwt, build_login_from_jwt_chain, ClientData
from proloader.net.netease.client import NeteaseClient


def main():
    # 1. 读取 cookie + authv2 获取 jwt chain
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
    print(f"JWT chain 原始字符串长度: {len(jwt_chain)}")
    print(f"{'='*70}")
    print(f"原始字符串前 200 字符: {jwt_chain[:200]}...")

    # 2. 解析 jwt_chain
    parsed = json.loads(jwt_chain)
    if isinstance(parsed, list):
        chain_list = parsed
    elif isinstance(parsed, dict) and "chain" in parsed:
        chain_list = parsed["chain"]
    else:
        print(f"未知格式: {type(parsed)}")
        return

    print(f"\nchain 包含 {len(chain_list)} 个 JWT")
    print(f"{'='*70}")

    # 3. 逐个解析 JWT
    for i, jwt_str in enumerate(chain_list):
        print(f"\n--- chain[{i}] ---")
        print(f"JWT 长度: {len(jwt_str)} 字符")
        try:
            header, payload, signature = parse_jwt(jwt_str)
            print(f"header:  {json.dumps(header, indent=2, ensure_ascii=False)}")
            print(f"payload: {json.dumps(payload, indent=2, ensure_ascii=False)}")
            print(f"signature 长度: {len(signature)} bytes")
            # 检查关键字段
            has_x5u = "x5u" in header
            has_identity_pub = "identityPublicKey" in payload
            has_extra_data = "extraData" in payload
            has_cert_auth = "certificateAuthority" in payload
            print(f"\n关键字段检查:")
            print(f"  header.x5u: {'✓' if has_x5u else '✗'}")
            print(f"  payload.identityPublicKey: {'✓' if has_identity_pub else '✗'}")
            print(f"  payload.extraData: {'✓' if has_extra_data else '✗'}")
            print(f"  payload.certificateAuthority: {'✓' if has_cert_auth else '✗'}")
            if has_identity_pub:
                print(f"  identityPublicKey = {payload['identityPublicKey'][:40]}...")
        except Exception as e:
            print(f"❌ 解析失败: {e}")

    # 4. 构建前插后的 chain，看最终结构
    print(f"\n{'='*70}")
    print("构建 ConnectionRequest 后的 chain 结构:")
    print(f"{'='*70}")

    client_data = ClientData()
    client_data.GameVersion = "1.21.50"
    client_data.ServerAddress = f"{host}:{port}"

    try:
        conn_req = build_login_from_jwt_chain(jwt_chain, client_data, priv)
        print(f"ConnectionRequest 总长度: {len(conn_req)} bytes")

        # 重新解析前插后的 chain
        import struct
        chain_len = struct.unpack("<i", conn_req[:4])[0]
        chain_json = conn_req[4:4+chain_len].decode()
        new_chain = json.loads(chain_json)["chain"]
        print(f"前插后 chain 包含 {len(new_chain)} 个 JWT")

        for i, jwt_str in enumerate(new_chain):
            header, payload, _ = parse_jwt(jwt_str)
            has_identity_pub = "identityPublicKey" in payload
            has_cert_auth = "certificateAuthority" in payload
            has_extra_data = "extraData" in payload
            x5u = header.get("x5u", "N/A")[:30]
            identity_pub = payload.get("identityPublicKey", "N/A")[:30] if has_identity_pub else "N/A"
            print(f"  chain[{i}]: x5u={x5u}... identityPublicKey={identity_pub}... "
                  f"extraData={'✓' if has_extra_data else '✗'} "
                  f"certAuth={'✓' if has_cert_auth else '✗'}")

        # 检查最后一个 JWT 的 identityPublicKey
        last_header, last_payload, _ = parse_jwt(new_chain[-1])
        last_identity_pub = last_payload.get("identityPublicKey")
        print(f"\n最后一个 JWT (chain[{len(new_chain)-1}]) 的 identityPublicKey: "
              f"{last_identity_pub[:40] if last_identity_pub else 'None (无此字段)'}")

        # 对比客户端公钥
        client_pub = serialize_public_key_b64(priv)
        print(f"客户端公钥 (authv2 传入): {client_pub[:40]}...")
        print(f"前插 JWT header.x5u (客户端公钥): {parse_jwt(new_chain[0])[0].get('x5u', 'N/A')[:40]}...")

        if last_identity_pub:
            if last_identity_pub == client_pub:
                print("✓ 最后 JWT 的 identityPublicKey == 客户端公钥（RawToken 可验签）")
            else:
                print("✗ 最后 JWT 的 identityPublicKey != 客户端公钥")
                print(f"  → RawToken 用此 key 验签，但我们用客户端私钥签名 → 验签失败！")
                print(f"  → 这可能就是 'Connection Request invalid' 的根因")
        else:
            print("⚠️ 最后 JWT 无 identityPublicKey 字段")
            print(f"  → RawToken 验签 key 保持为上一个 JWT 的 identityPublicKey")
            # 找上一个有 identityPublicKey 的
            for j in range(len(new_chain)-2, -1, -1):
                _, p, _ = parse_jwt(new_chain[j])
                if "identityPublicKey" in p:
                    print(f"  → chain[{j}].identityPublicKey = {p['identityPublicKey'][:40]}...")
                    if p["identityPublicKey"] == client_pub:
                        print(f"  ✓ chain[{j}].identityPublicKey == 客户端公钥")
                    else:
                        print(f"  ✗ chain[{j}].identityPublicKey != 客户端公钥 → 验签失败")
                    break

    except Exception as e:
        import traceback
        traceback.print_exc()


if __name__ == "__main__":
    main()
