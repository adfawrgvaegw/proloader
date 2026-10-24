"""用 iOS 真实版本号测试 PE 认证。

从 Cache.db 提取的 iOS 客户端真实版本：
    engine_ver = 3.9.20.297905
    patch_ver  = 3.9.21.297996
"""

import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from proloader.net.netease.client import NeteaseClient

import logging
logging.basicConfig(level=logging.INFO, format="\033[36m[%(name)s]\033[0m %(message)s")


def main():
    cookie_path = Path(__file__).resolve().parents[1] / "cache" / "netease" / "cookie.json"
    cookie_str = cookie_path.read_text()
    print(f"✅ Cookie 已加载 ({len(cookie_str)} 字符)")

    client = NeteaseClient()
    # 跳过 bootstrap，手动设置 iOS 真实版本号
    client.engine_version = "3.9.20.297905"
    client.g79_latest_version = "3.9.21.297996"
    client.patch_resources_hash = "ef94c4a55bd40c43965f8ed8b52bc103"

    print(f"\n--- PE 认证（iOS 版本号）---")
    print(f"  engine_version: {client.engine_version}")
    print(f"  g79_latest_version: {client.g79_latest_version}")
    print(f"  patch_resources_hash: {client.patch_resources_hash}")

    try:
        client.authenticate_with_cookie(cookie_str)
        print(f"\n✅ PE 认证成功！")
        print(f"  user_id: {client.user_id}")
        print(f"  user_token: {client.user_token[:20]}...")
    except Exception as e:
        print(f"\n❌ PE 认证失败: {e}")


if __name__ == "__main__":
    main()
