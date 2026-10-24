#!/usr/bin/env python3
"""端到端测试：cookie → PE 认证 → 搜索租赁服 → 进入世界 → Link+GameStart → JWT chain

用法: python3 -m tools.test_netease_auth
"""
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from proloader.net.netease.client import NeteaseClient

LOG_FMT = "\033[36m[%(name)s]\033[0m %(message)s"

import logging
logging.basicConfig(level=logging.INFO, format=LOG_FMT)


def main():
    # 1. 读取 cookie
    cookie_path = Path(__file__).resolve().parents[1] / "cache" / "netease" / "cookie.json"
    if not cookie_path.exists():
        print(f"❌ Cookie 文件不存在: {cookie_path}")
        print("   请先运行网易客户端登录，然后提取 sauth_json")
        sys.exit(1)

    cookie_str = cookie_path.read_text()
    print(f"✅ Cookie 已加载 ({len(cookie_str)} 字符)")

    # 2. 创建客户端并 bootstrap
    print("\n--- Bootstrap ---")
    client = NeteaseClient(platform="ios")
    client.bootstrap()
    print(f"  engine_version: {client.engine_version}")
    print(f"  g79_latest_version: {client.g79_latest_version}")
    print(f"  patch_resources_hash: {client.patch_resources_hash[:32]}...")

    # 3. PE 认证
    print("\n--- PE 认证 ---")
    try:
        client.authenticate_with_cookie(cookie_str)
        print(f"  ✅ user_id: {client.user_id}")
        print(f"  ✅ user_token: {client.user_token[:20]}...")
    except Exception as e:
        print(f"  ❌ PE 认证失败: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)

    # 4. 搜索租赁服
    print("\n--- 搜索租赁服 ---")
    server_code = "12345678"
    try:
        server_entity = client.search_rental_server(server_code)
        print(f"  ✅ serverID: {server_entity.entity_id}")
        print(f"  ✅ owner_id: {server_entity.owner_id or '(空)'}")
        print(f"  ✅ min_level: {server_entity.min_level}")
    except Exception as e:
        print(f"  ❌ 搜索失败: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)

    # 5. 进入租赁服世界
    print("\n--- 进入租赁服世界 ---")
    try:
        host, port = client.enter_rental_server_world(server_entity.entity_id, "")
        print(f"  ✅ IP:port = {host}:{port}")
    except Exception as e:
        print(f"  ❌ 进入失败: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)

    # 6. Link 连接 + GameStart（Bedrock 连接前置，通知网易后端玩家要进游戏）
    print("\n--- Link 连接 + GameStart ---")
    try:
        client.connect_link_and_send_game_start(server_code, server_entity)
        print(f"  ✅ Link+GameStart 成功")
    except Exception as e:
        print(f"  ❌ Link+GameStart 失败: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)

    # 7. JWT Chain（需要客户端公钥，这里用测试值）
    print("\n--- JWT Chain 获取 ---")
    try:
        # 生成临时 ECDH 密钥对用于测试
        from proloader.net.bedrock.crypto import generate_keypair, serialize_public_key_b64
        priv = generate_keypair()
        client_key = serialize_public_key_b64(priv)
        print(f"  client_key: {client_key[:40]}...")

        authv2 = client.generate_rental_game_auth_v2(server_entity.entity_id, client_key)
        print(f"  authv2: {authv2.decode()[:100]}...")

        chain_info = client.send_auth_v2_request(authv2)
        print(f"  ✅ JWT chain ({len(chain_info)} 字符)")
        print(f"  前 100 字符: {chain_info[:100]}...")

        # 保存 JWT chain
        chain_path = Path(__file__).resolve().parents[1] / "cache" / "netease" / "jwt_chain.json"
        chain_path.write_text(chain_info)
        print(f"  已保存到: {chain_path}")
    except Exception as e:
        print(f"  ❌ JWT chain 获取失败: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)

    print("\n🎉 端到端测试成功！")
    print(f"  user_id: {client.user_id}")
    print(f"  server: {host}:{port}")
    print(f"  jwt_chain: {len(chain_info)} 字符")


if __name__ == "__main__":
    main()
