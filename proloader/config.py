"""proloader 配置系统（对齐 fatalder defines/control_config.go + utils/config.go）。

功能：
1. 数据目录（storage/）：服务器配置、任务组、上次使用配置
2. 服务器配置 ControlConfig：租赁服编号/密码/地址/Token
3. 自动 Cookie 读取：启动时从网易客户端沙盒 Cache.db 自动提取 sauth_json
4. JSON 持久化

目录结构（对齐 fatalder GetStoragePath = ~/.proloader/）：
    ~/.proloader/
        config.json                # 全局配置
        server_configs/            # 服务器配置（json 文件，按名称保存）
        tasks/                     # 任务组
        last_server.json           # 上次使用的服务器配置
        cookie.json                # 最新 cookie（auto-refresh 后写入）
"""

from __future__ import annotations

import base64
import json
import sqlite3
import struct
import urllib.parse
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any, Dict, List, Optional


# ═══════════════════════════════════════════════════════════════════════════════
# 数据类
# ═══════════════════════════════════════════════════════════════════════════════
@dataclass
class ControlConfig:
    """服务器连接配置（对齐 fatalder defines.ControlConfig）。"""
    rental_server_code: str = ""           # 租赁服编号
    rental_server_passcode: str = ""       # 租赁服密码
    auth_server_address: str = ""          # 认证服务器地址（可选，默认自动发现）
    auth_server_token: str = ""            # 认证 Token（可选）

    def to_json(self) -> str:
        return json.dumps(asdict(self), ensure_ascii=False)

    @classmethod
    def from_json(cls, s: str) -> "ControlConfig":
        data = json.loads(s)
        return cls(**{k: data.get(k, "") for k in cls.__dataclass_fields__})


@dataclass
class CookieData:
    """登录 Cookie 数据（对齐 proloader netease.CookieData）。"""
    sauth_json: str = ""
    mac_addr: str = "02:00:00:00:00:00"
    ram: str = "4294967296"
    rom: str = ""
    is_guest: bool = False
    emulator: int = 0

    def to_cookie_str(self) -> str:
        """序列化为 authenticate_with_cookie 需要的 JSON 字符串。"""
        return json.dumps(asdict(self), ensure_ascii=False)

    @classmethod
    def from_json(cls, s: str) -> "CookieData":
        data = json.loads(s)
        fields = {k: data.get(k, v) for k, v in cls.__dataclass_fields__.items()}
        # 默认值处理
        if fields.get("mac_addr") in (None, ""):
            fields["mac_addr"] = "02:00:00:00:00:00"
        if fields.get("ram") in (None, ""):
            fields["ram"] = "4294967296"
        return cls(**fields)


# ═══════════════════════════════════════════════════════════════════════════════
# 存储路径
# ═══════════════════════════════════════════════════════════════════════════════
STORAGE_ROOT = Path.home() / ".proloader"
SERVER_CONFIGS_DIR = STORAGE_ROOT / "server_configs"
TASKS_DIR = STORAGE_ROOT / "tasks"
LAST_SERVER_FILE = STORAGE_ROOT / "last_server.json"
COOKIE_FILE = STORAGE_ROOT / "cookie.json"
GLOBAL_CONFIG_FILE = STORAGE_ROOT / "config.json"

# 网易 macOS 客户端沙盒路径（自动 Cookie 提取）
NETEASE_CONTAINER = Path.home() / "Library/Containers/com.netease.mc/Data"
NETEASE_COOKIES = NETEASE_CONTAINER / "Library/Cookies/Cookies.binarycookies"
NETEASE_CACHE_DB = NETEASE_CONTAINER / "Library/Caches/com.netease.mc/Cache.db"


def ensure_storage_dirs() -> None:
    """创建所有必要的存储目录。"""
    SERVER_CONFIGS_DIR.mkdir(parents=True, exist_ok=True)
    TASKS_DIR.mkdir(parents=True, exist_ok=True)


# ═══════════════════════════════════════════════════════════════════════════════
# 服务器配置持久化
# ═══════════════════════════════════════════════════════════════════════════════
def save_server_config(cfg: ControlConfig, name: str) -> Path:
    """保存服务器配置到 server_configs/{name}.json。"""
    ensure_storage_dirs()
    path = SERVER_CONFIGS_DIR / f"{name}.json"
    path.write_text(cfg.to_json(), encoding="utf-8")
    return path


def load_server_config(name: str) -> Optional[ControlConfig]:
    """从 server_configs/{name}.json 加载。"""
    path = SERVER_CONFIGS_DIR / f"{name}.json"
    if not path.exists():
        return None
    return ControlConfig.from_json(path.read_text(encoding="utf-8"))


def list_server_configs() -> List[str]:
    """列出所有已保存的服务器配置名（不含 .json 后缀）。"""
    ensure_storage_dirs()
    return [p.stem for p in SERVER_CONFIGS_DIR.glob("*.json")]


def save_last_server(cfg: ControlConfig) -> None:
    """记住上次使用的服务器配置。"""
    ensure_storage_dirs()
    LAST_SERVER_FILE.write_text(cfg.to_json(), encoding="utf-8")


def load_last_server() -> Optional[ControlConfig]:
    """读取上次使用的服务器配置。"""
    if not LAST_SERVER_FILE.exists():
        return None
    try:
        return ControlConfig.from_json(LAST_SERVER_FILE.read_text(encoding="utf-8"))
    except (json.JSONDecodeError, KeyError):
        return None


# ═══════════════════════════════════════════════════════════════════════════════
# 自动 Cookie 读取（从网易客户端沙盒自动提取 sauth_json）
# ═══════════════════════════════════════════════════════════════════════════════
def _parse_binary_cookies(data: bytes) -> list:
    """解析 Safari Cookies.binarycookies（提取 mac_addr/ram 等）。"""
    if data[:4] != b"cook":
        return []
    num_pages = struct.unpack(">I", data[4:8])[0]
    page_sizes = [struct.unpack(">I", data[8 + i * 4:12 + i * 4])[0] for i in range(num_pages)]
    pages_off = 8 + num_pages * 4
    out = []
    for ps in page_sizes:
        page = data[pages_off:pages_off + ps]
        pages_off += ps
        if len(page) < 8:
            continue
        num_cookies = struct.unpack("<I", page[4:8])[0]
        offsets = [struct.unpack("<I", page[8 + j * 4:12 + j * 4])[0] for j in range(num_cookies)]
        for off in offsets:
            if off + 24 > len(page):
                continue
            uo = struct.unpack("<I", page[off + 8:off + 12])[0]
            no = struct.unpack("<I", page[off + 12:off + 16])[0]
            po = struct.unpack("<I", page[off + 16:off + 20])[0]
            vo = struct.unpack("<I", page[off + 20:off + 24])[0]

            def rs(rel: int) -> str:
                if rel <= 0: return ""
                s = off + rel
                if s >= len(page): return ""
                e = page.find(b"\x00", s)
                if e < 0: e = len(page)
                return page[s:e].decode("utf-8", errors="replace")

            out.append({"url": rs(uo), "name": rs(no), "path": rs(po), "value": rs(vo)})
    return out


def _extract_from_cache_db(existing_sauth: Optional[dict] = None) -> Dict[str, Any]:
    """从 Cache.db 提取 sauth_json / access_token / sessionid。

    逻辑对齐 tools/extract_netease_cookie.py。
    """
    result: Dict[str, Any] = {"sauth_json": "", "access_token": "", "sessionid": ""}
    if not NETEASE_CACHE_DB.exists():
        return result

    try:
        conn = sqlite3.connect(str(NETEASE_CACHE_DB))
        conn.text_factory = bytes
        cur = conn.cursor()

        # (a) 最新 unisdk_login_json 条目 → access_token + chl_sdk_json
        cur.execute(
            "SELECT entry_ID, receiver_data FROM cfurl_cache_receiver_data "
            "WHERE CAST(receiver_data AS BLOB) LIKE '%unisdk_login_json%' "
            "ORDER BY entry_ID DESC"
        )
        rows = cur.fetchall()
    except (sqlite3.OperationalError, sqlite3.DatabaseError):
        return result

    access_token = ""
    chl_sdk: Dict[str, Any] = {}
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
            break

    # (b) 从 URL 提取 sessionid（MPay token=1-...）
    sessionid = ""
    try:
        cur.execute(
            "SELECT request_key FROM cfurl_cache_response "
            "WHERE request_key LIKE '%token=1-%' "
            "ORDER BY rowid DESC LIMIT 5"
        )
        for (rk,) in cur.fetchall():
            rk_str = rk.decode("utf-8", errors="replace") if isinstance(rk, bytes) else str(rk)
            if "token=1-" in rk_str:
                qs = urllib.parse.parse_qs(urllib.parse.urlparse(rk_str).query)
                if "token" in qs:
                    sessionid = qs["token"][0]
                    break
    except sqlite3.OperationalError:
        pass
    conn.close()

    result["access_token"] = access_token
    result["sessionid"] = sessionid

    # (c) 组装 sauth_json（保留 existing_sauth 设备字段 + 新鲜值覆盖）
    if chl_sdk or existing_sauth:
        sauth_obj: Dict[str, Any] = {}
        if existing_sauth:
            sauth_obj.update(existing_sauth)
        for k in ("gameid", "login_channel", "sdkuid", "udid"):
            if chl_sdk.get(k):
                sauth_obj[k] = chl_sdk[k]
        sauth_obj.setdefault("app_channel", "app_store")
        sauth_obj.setdefault("platform", "ios")
        sauth_obj.setdefault("source_platform", "ios")
        sauth_obj.setdefault("sdk_version", "5.17.0")
        if sessionid:
            sauth_obj["sessionid"] = sessionid
        if access_token:
            sauth_obj["access_token"] = access_token
        result["sauth_json"] = json.dumps(sauth_obj, ensure_ascii=False)

    return result


def auto_refresh_cookie() -> CookieData:
    """自动从网易客户端沙盒提取 Cookie。

    流程：
    1. 读已有 cookie.json（保留设备字段 mac_addr/ram/rom 等）
    2. macOS: 解析 Cookies.binarycookies → 补 mac_addr/ram
    3. macOS: 读 Cache.db → 抽 sauth_json/access_token/sessionid 新鲜值
    4. Windows/Linux: 如果沙盒不存在，回退使用 cookie.json 中已有值
    5. 写回 cookie.json，返回 CookieData

    如果沙盒不存在或无数据，回退使用 cookie.json 中已有值。
    Windows 用户需手动配置 cookie.json（通过 TUI 或 set_cookie_manual）。
    """
    ensure_storage_dirs()

    # 1. 读已有 cookie
    existing_cookie: Dict[str, Any] = {}
    existing_sauth: Optional[Dict[str, Any]] = None
    if COOKIE_FILE.exists():
        try:
            existing_cookie = json.loads(COOKIE_FILE.read_text(encoding="utf-8"))
            if existing_cookie.get("sauth_json"):
                existing_sauth = json.loads(existing_cookie["sauth_json"])
        except (json.JSONDecodeError, KeyError):
            pass

    # 2. 解析 Cookies.binarycookies
    cookie_list: list = []
    if NETEASE_COOKIES.exists():
        try:
            cookie_list = _parse_binary_cookies(NETEASE_COOKIES.read_bytes())
        except Exception:
            pass
    cookie_map = {c["name"]: c["value"] for c in cookie_list}

    # 3. Cache.db 提取
    if NETEASE_CACHE_DB.exists():
        cache_data = _extract_from_cache_db(existing_sauth)
    else:
        cache_data = {"sauth_json": "", "access_token": "", "sessionid": ""}

    # 4. 组装
    sauth_json_str = cache_data["sauth_json"] or existing_cookie.get("sauth_json", "")
    mac_addr = cookie_map.get("mac_addr") or existing_cookie.get("mac_addr") or "02:00:00:00:00:00"
    ram = cookie_map.get("ram") or existing_cookie.get("ram") or "4294967296"
    rom = existing_cookie.get("rom", "")
    is_guest = existing_cookie.get("is_guest", False)
    emulator = existing_cookie.get("emulator", 0)

    # ★ 修复: deviceid 不在 Cache.db 里，如果 sauth_json 缺少 deviceid，用 udid 补上
    if sauth_json_str:
        sauth_obj = json.loads(sauth_json_str)
        if not sauth_obj.get("deviceid"):
            udid = sauth_obj.get("udid", "")
            if udid:
                sauth_obj["deviceid"] = udid
                sauth_json_str = json.dumps(sauth_obj, ensure_ascii=False)
                print(f"[Cookie] deviceid 缺失，已用 udid 补上: {udid}")

    cookie = CookieData(
        sauth_json=sauth_json_str,
        mac_addr=mac_addr,
        ram=ram,
        rom=rom,
        is_guest=bool(is_guest),
        emulator=int(emulator) if emulator is not None else 0,
    )

    # 写回 cookie.json（供后续复用）
    COOKIE_FILE.write_text(json.dumps(asdict(cookie), ensure_ascii=False, indent=2))

    return cookie


def check_cookie_valid(cookie: CookieData) -> tuple:
    """快速检查 Cookie 字段完整性。

    Returns:
        (is_valid: bool, fields: dict)
            fields: {field: bool} True=存在，False=缺失
    """
    if not cookie.sauth_json:
        return False, {}
    try:
        sauth_obj = json.loads(cookie.sauth_json)
    except json.JSONDecodeError:
        return False, {}
    required = ("gameid", "sdkuid", "sessionid", "platform")
    fields = {k: bool(sauth_obj.get(k)) for k in required}
    fields["access_token"] = bool(sauth_obj.get("access_token"))
    return all(fields.values()), fields
