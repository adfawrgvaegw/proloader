#!/usr/bin/env python3
"""解析网易客户端 Cookies.binarycookies + 提取 Cache.db 中的 sauth 数据。

输出 cache/netease/cookie.json（供 test_netease_auth.py 使用）。

用法: python3 -m tools.extract_netease_cookie
"""
from __future__ import annotations

import base64
import json
import sqlite3
import struct
import sys
import urllib.parse
from pathlib import Path

CONTAINER = (Path.home() / "Library/Containers/com.netease.mc/Data")
BINARY_COOKIES = CONTAINER / "Library/Cookies/Cookies.binarycookies"
CACHE_DB = CONTAINER / "Library/Caches/com.netease.mc/Cache.db"
OUT_PATH = Path(__file__).resolve().parents[1] / "cache/netease/cookie.json"


# ──────────────────────────────────────────────────────────────
# 1. 解析 Cookies.binarycookies（Safari 二进制格式）
# ──────────────────────────────────────────────────────────────
def parse_binary_cookies(data: bytes) -> list[dict]:
    if data[:4] != b"cook":
        raise ValueError("不是有效的 binarycookies 文件（magic 错误）")
    num_pages = struct.unpack(">I", data[4:8])[0]
    page_sizes = []
    for i in range(num_pages):
        page_sizes.append(struct.unpack(">I", data[8 + i * 4 : 12 + i * 4])[0])

    pages_offset = 8 + num_pages * 4
    cookies: list[dict] = []
    for ps in page_sizes:
        page = data[pages_offset : pages_offset + ps]
        pages_offset += ps
        if len(page) < 8:
            continue
        num_cookies = struct.unpack("<I", page[4:8])[0]
        offsets = []
        for j in range(num_cookies):
            off = struct.unpack("<I", page[8 + j * 4 : 12 + j * 4])[0]
            offsets.append(off)
        for off in offsets:
            cookie = _parse_cookie(page, off)
            if cookie:
                cookies.append(cookie)
    return cookies


def _parse_cookie(page: bytes, offset: int) -> dict | None:
    if offset + 24 > len(page):
        return None
    url_off = struct.unpack("<I", page[offset + 8 : offset + 12])[0]
    name_off = struct.unpack("<I", page[offset + 12 : offset + 16])[0]
    path_off = struct.unpack("<I", page[offset + 16 : offset + 20])[0]
    value_off = struct.unpack("<I", page[offset + 20 : offset + 24])[0]

    def read_str(rel_off: int) -> str:
        if rel_off <= 0:
            return ""
        start = offset + rel_off
        if start >= len(page):
            return ""
        end = page.find(b"\x00", start)
        if end < 0:
            end = len(page)
        return page[start:end].decode("utf-8", errors="replace")

    return {
        "url": read_str(url_off),
        "name": read_str(name_off),
        "path": read_str(path_off),
        "value": read_str(value_off),
    }


# ──────────────────────────────────────────────────────────────
# 2. 从 Cache.db 提取 sauth_json / access_token / sessionid
# ──────────────────────────────────────────────────────────────
def extract_from_cache_db(existing_sauth: dict | None = None) -> dict:
    """从 HTTP 缓存数据库提取登录相关数据。

    - cfurl_cache_receiver_data: 响应体（uni_sauth → unisdk_login_json → oauth2.access_token + chl_sdk_json）
    - cfurl_cache_response: 请求 URL（含 token=1-... 的 MPay 请求 → sessionid）

    existing_sauth: 现有 sauth_json 解析后的 dict，用于保留设备字段（deviceid/sdk_version/aim_info 等
                    不在 Cache.db 里的字段）。若为 None 则仅返回 Cache.db 能提取的字段。
    """
    result: dict = {"sauth_json": "", "access_token": "", "sessionid": ""}
    if not CACHE_DB.exists():
        print(f"⚠️  Cache.db 不存在: {CACHE_DB}")
        return result

    conn = sqlite3.connect(str(CACHE_DB))
    conn.text_factory = bytes  # 避免解码问题
    cur = conn.cursor()

    # (a) 找 receiver_data 里含 unisdk_login_json 的最新条目（uni_sauth 响应）
    try:
        cur.execute(
            "SELECT entry_ID, receiver_data FROM cfurl_cache_receiver_data "
            "WHERE CAST(receiver_data AS BLOB) LIKE '%unisdk_login_json%' "
            "ORDER BY entry_ID DESC"
        )
        rows = cur.fetchall()
    except sqlite3.OperationalError as e:
        print(f"⚠️  查询 receiver_data 失败: {e}")
        rows = []

    access_token = ""
    chl_sdk: dict = {}
    for entry_id, data in rows:
        if not isinstance(data, (bytes, bytearray)):
            continue
        try:
            outer = json.loads(data.decode("utf-8", errors="replace"))
        except json.JSONDecodeError:
            continue
        ulj_b64 = outer.get("unisdk_login_json", "")
        if not ulj_b64:
            continue
        try:
            ulj = json.loads(base64.b64decode(ulj_b64).decode("utf-8"))
        except Exception:
            continue
        oauth2 = ulj.get("oauth2", {})
        if oauth2.get("access_token") and not access_token:
            access_token = oauth2["access_token"]
        if ulj.get("chl_sdk_json") and not chl_sdk:
            chl_sdk = ulj["chl_sdk_json"]
        if access_token and chl_sdk:
            print(f"  从 entry_ID={entry_id} 提取到 unisdk_login_json")
            break

    # (b) 从请求 URL 提取 sessionid（MPay token，格式 token=1-...）
    sessionid = ""
    try:
        cur.execute(
            "SELECT request_key FROM cfurl_cache_response "
            "WHERE request_key LIKE '%token=1-%' "
            "ORDER BY rowid DESC LIMIT 5"
        )
        import urllib.parse
        for (rk,) in cur.fetchall():
            rk_str = rk.decode("utf-8", errors="replace") if isinstance(rk, bytes) else str(rk)
            if "token=1-" in rk_str:
                parsed = urllib.parse.urlparse(rk_str)
                qs = urllib.parse.parse_qs(parsed.query)
                if "token" in qs:
                    sessionid = qs["token"][0]
                    break
    except sqlite3.OperationalError as e:
        print(f"⚠️  查询 cache_response 失败: {e}")

    conn.close()

    result["access_token"] = access_token
    result["sessionid"] = sessionid

    # (c) 组装 sauth_json：chl_sdk_json 提供 gameid/login_channel/sdkuid/udid，
    #     保留 existing_sauth 里的设备字段（deviceid/sdk_version/aim_info 等）
    if chl_sdk or existing_sauth:
        sauth_obj: dict = {}
        if existing_sauth:
            sauth_obj.update(existing_sauth)  # 保留现有字段
        # 用 chl_sdk_json 的新鲜值覆盖
        if chl_sdk.get("gameid"):
            sauth_obj["gameid"] = chl_sdk["gameid"]
        if chl_sdk.get("login_channel"):
            sauth_obj["login_channel"] = chl_sdk["login_channel"]
        if chl_sdk.get("sdkuid"):
            sauth_obj["sdkuid"] = chl_sdk["sdkuid"]
        if chl_sdk.get("udid"):
            sauth_obj["udid"] = chl_sdk["udid"]
        # iOS 平台字段（若 existing 没有则补默认）
        sauth_obj.setdefault("app_channel", "app_store")
        sauth_obj.setdefault("platform", "ios")
        sauth_obj.setdefault("source_platform", "ios")
        sauth_obj.setdefault("sdk_version", "5.17.0")
        # 刷新 sessionid / access_token
        if sessionid:
            sauth_obj["sessionid"] = sessionid
        if access_token:
            sauth_obj["access_token"] = access_token
        result["sauth_json"] = json.dumps(sauth_obj, ensure_ascii=False)

    return result


# ──────────────────────────────────────────────────────────────
# 3. 主流程
# ──────────────────────────────────────────────────────────────
def main():
    print("=== 1. 解析 Cookies.binarycookies ===")
    all_cookies: list[dict] = []
    if BINARY_COOKIES.exists():
        data = BINARY_COOKIES.read_bytes()
        print(f"  文件大小: {len(data)} 字节")
        try:
            all_cookies = parse_binary_cookies(data)
            print(f"  解析到 {len(all_cookies)} 个 cookie:")
            for c in all_cookies:
                val_preview = c["value"][:60] + ("..." if len(c["value"]) > 60 else "")
                print(f"    [{c['url']}] {c['name']} = {val_preview}")
        except Exception as e:
            print(f"  ❌ 解析失败: {e}")
    else:
        print(f"  ⚠️  文件不存在: {BINARY_COOKIES}")

    # 读取现有 cookie.json，保留设备字段（deviceid/sdk_version/aim_info 等不在 Cache.db 里）
    existing_sauth: dict | None = None
    existing_cookie: dict = {}
    if OUT_PATH.exists():
        try:
            existing_cookie = json.loads(OUT_PATH.read_text())
            if existing_cookie.get("sauth_json"):
                existing_sauth = json.loads(existing_cookie["sauth_json"])
                print(f"\n  📌 已有 cookie.json，保留设备字段: "
                      f"deviceid={existing_sauth.get('deviceid', '无')}, "
                      f"sdk_version={existing_sauth.get('sdk_version', '无')}")
        except (json.JSONDecodeError, KeyError):
            pass

    print("\n=== 2. 从 Cache.db 提取 sauth 数据 ===")
    cache_data = extract_from_cache_db(existing_sauth)
    print(f"  sauth_json: {'✅ ' + str(len(cache_data['sauth_json'])) + ' 字符' if cache_data['sauth_json'] else '❌ 未找到'}")
    print(f"  access_token: {'✅ ' + cache_data['access_token'][:40] + '...' if cache_data['access_token'] else '❌ 未找到'}")
    print(f"  sessionid: {'✅ ' + cache_data['sessionid'][:40] + '...' if cache_data['sessionid'] else '❌ 未找到'}")

    print("\n=== 3. 组装 cookie.json ===")
    sauth_json_str = cache_data["sauth_json"]

    # 如果 Cache.db 没拼出 sauth_json，回退用现有 cookie.json 的
    if not sauth_json_str and existing_cookie.get("sauth_json"):
        sauth_json_str = existing_cookie["sauth_json"]
        print("  ⚠️  Cache.db 无新数据，沿用现有 cookie.json 的 sauth_json")

    if not sauth_json_str:
        print("\n❌ 无法获取 sauth_json，请先在网易客户端完成登录后再运行本脚本。")
        sys.exit(1)

    # 解析 sauth_json 看完整性
    try:
        sauth_obj = json.loads(sauth_json_str)
        print(f"\n  sauth_json 字段:")
        for k in ("gameid", "platform", "app_channel", "sdkuid", "sessionid",
                  "udid", "deviceid", "access_token", "sdk_version"):
            v = sauth_obj.get(k, "")
            print(f"    {k}: {'✅ ' + str(v)[:50] if v else '❌ 缺失'}")
    except json.JSONDecodeError as e:
        print(f"  ⚠️  sauth_json 解析失败: {e}")

    # 组装完整 cookie（对齐 CookieData 结构）
    cookie_obj = {
        "sauth_json": sauth_json_str,
        "mac_addr": existing_cookie.get("mac_addr", "02:00:00:00:00:00"),
        "ram": existing_cookie.get("ram", "4294967296"),
        "rom": existing_cookie.get("rom", ""),
        "is_guest": existing_cookie.get("is_guest", False),
        "emulator": existing_cookie.get("emulator", 0),
    }

    # 从 binarycookies 补充 mac_addr 等（如果有）
    cookie_map = {c["name"]: c["value"] for c in all_cookies}
    if "mac_addr" in cookie_map:
        cookie_obj["mac_addr"] = cookie_map["mac_addr"]
    if "ram" in cookie_map:
        cookie_obj["ram"] = cookie_map["ram"]

    OUT_PATH.parent.mkdir(parents=True, exist_ok=True)
    OUT_PATH.write_text(json.dumps(cookie_obj, ensure_ascii=False, indent=2))
    print(f"\n✅ 已写入: {OUT_PATH}")
    print(f"   共 {len(json.dumps(cookie_obj))} 字符")


if __name__ == "__main__":
    main()
