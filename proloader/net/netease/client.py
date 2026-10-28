"""网易 G79 客户端（Python 移植自 g79client）。

完整登录链路：
    cookie → PE 认证 → 搜索租赁服 → 进入世界 → Link+GameStart → JWT chain

用法：
    client = NeteaseClient()
    client.bootstrap()
    client.authenticate_with_cookie(cookie_json_str)
    server_entity = client.search_rental_server("12345678")
    ip, port = client.enter_rental_server_world(server_entity.entity_id, "")
    client.connect_link_and_send_game_start("12345678", server_entity)
    authv2 = client.generate_rental_game_auth_v2(server_entity.entity_id, client_public_key_b64)
    jwt_chain = client.send_auth_v2_request(authv2)
"""

from __future__ import annotations

import hashlib
import io
import json
import logging
import re
from urllib.parse import urlsplit
import secrets
import uuid
import zipfile
from dataclasses import dataclass, field
from typing import Any, Optional

import requests

from .pe_profiles import OFFICIAL_ENGINE, OFFICIAL_PATCHES, PEMetadataMismatchError, PEAuthenticationError


class RentalWorldEnterError(RuntimeError):
    """Safe world-enter failure; never retain the response or credentials."""
    def __init__(self, code=None):
        self.code = code if type(code) is int else None
        super().__init__('rental_world_enter_rejected')


class RentalWorldPending(RuntimeError):
    def __init__(self, state=None, progress=None, total=None):
        self.state = state if type(state) is int else None
        self.progress = progress if type(progress) is int and progress >= 0 else None
        self.total = total if type(total) is int and total > 0 else None
        super().__init__('rental_world_starting')

from proloader.net.netease.constants import (
    API_GATEWAY_URL,
    AUTH_SERVER_URL,
    CORE_SERVER_URL,
    ENGINE_VERSION,
    G79_ANDROID_MESSAGE_TAG,
    G79_ANDROID_STEP,
    G79_ANDROID_STEP2,
    G79_IOS_LIBRARY_HASH,
    G79_IOS_MESSAGE_TAG,
    G79_IOS_SIGNATURE_HASH,
    G79_LIBRARY_HASH,
    G79_SIGNATURE_HASH,
    LINK_SERVER_LIST_URL,
    PACK_LIST_URL,
    PATCH_LIST_URL,
    RELEASE_JSON_URL,
    WEB_SERVER_URL,
)
from proloader.net.netease.crypto import (
    calculate_dynamic_token,
    g79_http_decrypt,
    g79_http_encrypt,
    get_valid_json,
    pe_auth_sign,
)

logger = logging.getLogger(__name__)


# ═══════════════════════════════════════════════════════════════════════════════
# 数据类型
# ═══════════════════════════════════════════════════════════════════════════════
@dataclass
class SauthData:
    """sauth_json 解析后的数据（从 cookie 中提取）。"""
    is_unisdk_guest: int = 0
    gameid: str = ""
    login_channel: str = ""
    app_channel: str = ""
    platform: str = ""
    sdkuid: str = ""
    sessionid: str = ""
    sdk_version: str = ""
    udid: str = ""
    deviceid: str = ""
    aim_info: str = ""
    client_login_sn: str = ""
    gas_token: str = ""
    source_platform: str = ""
    ip: str = ""
    # 从 sdklog 解析的设备信息（iOS cookie 的 sauth_json 含 sdklog）
    device_model: str = ""
    os_name: str = ""
    os_ver: str = ""
    # OAuth2 access_token（iOS PE 认证需要，来自 unisdk_login_json.oauth2.access_token）
    access_token: str = ""


@dataclass
class CookieData:
    """cookie JSON 解析后的数据。"""
    sauth_json: str = ""
    mac_addr: str = ""
    ram: str = ""
    rom: str = ""
    is_guest: Optional[bool] = None
    emulator: Optional[int] = None

    @property
    def mac_address(self) -> str:
        return self.mac_addr.strip().upper() if self.mac_addr else "00:00:00:00:00:00"

    @property
    def ram_value(self) -> str:
        return self.ram if self.ram else "3072"

    @property
    def rom_value(self) -> str:
        return self.rom if self.rom else "32768"

    @property
    def is_guest_flag(self) -> int:
        return 1 if self.is_guest else 0

    @property
    def emulator_flag(self) -> int:
        return self.emulator if self.emulator is not None else 0


@dataclass
class RentalServerEntity:
    """租赁服搜索结果。"""
    entity_id: str = ""
    owner_id: str = ""
    min_level: int = 0


@dataclass
class RentalServerSearchResult:
    """租赁服搜索响应。"""
    entities: list[RentalServerEntity] = field(default_factory=list)


@dataclass
class RentalEnterWorldEntity:
    """进入租赁服世界响应。"""
    mcserver_host: str = ""
    mcserver_port: int = 0


# ═══════════════════════════════════════════════════════════════════════════════
# NeteaseClient
# ═══════════════════════════════════════════════════════════════════════════════
class NeteaseClient:
    """网易 G79 客户端。

    用法：
        client = NeteaseClient()
        client.bootstrap()
        client.authenticate_with_cookie(cookie_str)
        # 之后可以调用 search_rental_server / enter_rental_server_world / ...
    """

    def __init__(self, platform: str = "android", *, pe_auth_profile: str = "reference") -> None:
        if pe_auth_profile not in ("reference", "official-android-3915"):
            raise ValueError("unknown PE auth profile")
        self.pe_auth_profile = pe_auth_profile
        self.user_id: str = ""
        self.user_token: str = ""
        self.seed: str = ""
        self.platform: str = platform  # "android" 或 "ios"
        self.engine_version: str = ENGINE_VERSION
        self.g79_latest_version: str = ""
        self.patch_resources_hash: str = ""
        self.user_detail: Optional[dict] = None
        self.cookie: str = ""
        self._session: requests.Session = requests.Session()
        # 服务器 URL（默认硬编码，bootstrap 后可能更新）
        self.auth_server_url: str = AUTH_SERVER_URL
        self.core_server_url: str = CORE_SERVER_URL
        self.web_server_url: str = WEB_SERVER_URL
        self.api_gateway_url: str = API_GATEWAY_URL

    # ──────────────────────────────────────────────────────────────
    # Bootstrap：获取版本和补丁元数据
    # ──────────────────────────────────────────────────────────────
    def bootstrap(self, *, engine_source: str = "pack-list") -> None:
        """获取 G79 最新版本和补丁元数据（PE 认证需要）。

        Android: 从 pack list + patch list + manifest 获取版本和 resources_hash。
        iOS: 从客户端 stream/version 文件读取 engine_version，patch_version 为空
            （iOS 不使用 Android 的 RN 补丁体系）。
        """
        if self.platform == "ios":
            self._bootstrap_ios()
            return
        try:
            if engine_source == "pack-list":
                self._fetch_engine_version()
            elif engine_source == "official-download":
                self._fetch_official_download_version()
            else:
                raise ValueError("unknown engine source")
            self._fetch_patch_metadata()
            logger.info("Bootstrap 成功: engine=%s, version=%s, resources_hash=%s",
                        self.engine_version, self.g79_latest_version,
                        self.patch_resources_hash[:16] + "...")
        except Exception:
            # Missing metadata cannot be replaced with unrelated patches/zero hashes.
            self.g79_latest_version = ""
            self.patch_resources_hash = ""
            raise

    def _bootstrap_ios(self) -> None:
        """iOS bootstrap：从客户端 stream/version 读取 engine_version，从 iOS 补丁列表获取 patch_version。

        iOS 客户端的 stream/version 文件格式：
            {"patch_version": "", "engine_version": "3.9.20.297905"}

        engine_version 用 stream/version 的值（app 完整版本号）。
        patch_version 从 iOS 补丁列表选取（stream/version 里为空，因为 RN 补丁独立管理）。
        resources_hash 从 iOS manifest 计算（路径用 ios_ 前缀）。
        """
        # 1. engine_version 从 stream/version 读取
        from pathlib import Path
        stream_version_path = str(Path.home() / "Library/Containers/com.netease.mc/Data/Library/games/com.netease/storge/stream/version")
        try:
            import os
            if os.path.exists(stream_version_path):
                with open(stream_version_path) as f:
                    sv = json.load(f)
                self.engine_version = sv.get("engine_version", "") or self.engine_version
                logger.info("iOS engine_version 从 stream/version 读取: %s", self.engine_version)
            else:
                self.engine_version = "3.9.20.297905"
                logger.warning("stream/version 不存在，使用回退值: %s", self.engine_version)
        except Exception as e:
            logger.warning("读取 stream/version 失败: %s，使用回退值", e)
            self.engine_version = "3.9.20.297905"

        # 2. patch_version 从 iOS 补丁列表选取（按 engine 版本前缀匹配）
        try:
            self._fetch_ios_patch_metadata()
        except Exception as e:
            logger.warning("iOS 补丁元数据获取失败: %s", e)
            if not self.g79_latest_version:
                self.g79_latest_version = ""
            if not self.patch_resources_hash:
                self.patch_resources_hash = ""

        logger.info("iOS Bootstrap: engine=%s, patch=%s, resources_hash=%s",
                    self.engine_version, self.g79_latest_version or "(empty)",
                    self.patch_resources_hash[:16] + "..." if self.patch_resources_hash else "(empty)")

    def _fetch_ios_patch_metadata(self) -> None:
        """从 iOS 补丁列表获取 patch_version 和 resources_hash。

        对齐 g79client/globals.go:fetchG79PatchMetadata，但用 iOS 补丁列表和 ios_ 路径前缀。
        """
        resp = self._session.get(PATCH_LIST_URL, timeout=15)
        resp.raise_for_status()
        patch_info = resp.json()

        ios_versions = patch_info.get("ios", [])
        if not ios_versions:
            raise ValueError("补丁列表无 iOS 版本")

        # 按 engine 版本前缀匹配（取前 2 段，如 "3.9"）
        base_prefix = ".".join(self.engine_version.split(".")[:2])
        selected_version = None
        for v in reversed(ios_versions):
            if v.startswith(base_prefix):
                selected_version = v
                break
        if not selected_version:
            selected_version = ios_versions[-1]
        self.g79_latest_version = selected_version

        # 从 iOS manifest 计算 resources_hash
        base_url = patch_info.get("urlNew", "").rstrip("/") or patch_info.get("url", "").rstrip("/")
        if not base_url:
            raise ValueError("补丁列表无 URL")

        # iOS 路径: {base}/ios_{version}/{version}/ios/manifest.zip
        pre = f"{base_url}/ios_{selected_version}/{selected_version}/ios"

        hash_base = ""
        try:
            resp = self._session.get(f"{pre}/manifest.zip", timeout=15)
            resp.raise_for_status()
            with zipfile.ZipFile(io.BytesIO(resp.content)) as zf:
                manifest_data = json.loads(zf.read(zf.namelist()[0]))
            assets = manifest_data.get("assets", {})
            if "vanilla.mcp" in assets:
                hash_base = assets["vanilla.mcp"].get("md5", "")
            if "vanilla_patch.mcp" in assets:
                hash_base += assets["vanilla_patch.mcp"].get("md5", "")
        except Exception as e:
            logger.warning("获取 iOS manifest 失败: %s", e)

        # RN bundle MD5
        rn_hash = ""
        try:
            rn_url = f"{pre}/rn/index.bundle.backup"
            resp = self._session.get(rn_url, timeout=15)
            resp.raise_for_status()
            payload = resp.content
            try:
                with zipfile.ZipFile(io.BytesIO(payload)) as zf:
                    payload = zf.read(zf.namelist()[0])
            except Exception:
                pass
            rn_hash = hashlib.md5(payload).hexdigest()
        except Exception as e:
            logger.warning("获取 iOS RN bundle 失败: %s", e)

        self.patch_resources_hash = hashlib.md5(
            (hash_base + rn_hash).encode()
        ).hexdigest()

    def _fetch_engine_version(self) -> None:
        """从 pack list 获取 netease 渠道的引擎版本。

        对齐 g79client/client_core.go:Refetch() — EngineVersion = packList["netease"].Version
        """
        resp = self._session.get(PACK_LIST_URL, timeout=15)
        resp.raise_for_status()
        pack_list = resp.json()
        if "netease" in pack_list and pack_list["netease"].get("version"):
            self.engine_version = pack_list["netease"]["version"]
            logger.info("Engine version 从 pack list 更新: %s", self.engine_version)

    def _fetch_official_download_version(self) -> None:
        """Use the public official Android download, independently of gray pack rollout.

        Do not infer a stable version from min_ver or a different channel.
        This reads the download landing page only, never installs/downloads the APK.
        """
        response = self._session.get(
            "https://adl.netease.com/d/g/mc/c/gwnew?type=android", timeout=15)
        response.raise_for_status()
        match = re.search(r'var android_link = android_type \? "([^"\n]+)"', response.text)
        if not match:
            raise ValueError("official Android download URL missing")
        parsed = urlsplit(match[1])
        if (parsed.scheme != "https" or parsed.hostname != "g79.gdl.netease.com"
                or parsed.username or parsed.password):
            raise ValueError("unexpected Android download origin")
        version = re.search(r'-(\d+\.\d+\.\d+\.\d+)-Publish-', parsed.path)
        if not version:
            raise ValueError("official Android package version missing")
        self.engine_version = version[1]

    def _fetch_patch_metadata(self) -> None:
        """获取补丁版本和资源哈希。"""
        # 1. 获取补丁列表
        resp = self._session.get(PATCH_LIST_URL, timeout=15)
        resp.raise_for_status()
        patch_info = resp.json()

        # 2. 选择匹配引擎版本的 Android 补丁版本
        base_prefix = ".".join(self.engine_version.split(".")[:2])  # e.g. "3.4"
        android_versions = patch_info.get("android", [])
        selected_version = None
        for v in reversed(android_versions):
            if v.startswith(base_prefix + "."):
                selected_version = v
                break
        if not selected_version:
            raise ValueError(f"补丁列表无匹配 Android 引擎 {base_prefix} 的版本")
        self.g79_latest_version = selected_version

        # 3. 获取 manifest.zip → 提取 vanilla.mcp 和 vanilla_patch.mcp 的 MD5
        base_url = patch_info.get("urlNew", "").rstrip("/")
        if not base_url:
            base_url = patch_info.get("url", "").rstrip("/")
        if not base_url:
            raise ValueError("补丁列表无 URL")

        pre = f"{base_url}/android_{selected_version}/{selected_version}/android"
        manifest_url = f"{pre}/manifest.zip"

        hash_base = ""
        try:
            resp = self._session.get(manifest_url, timeout=15)
            resp.raise_for_status()
            with zipfile.ZipFile(io.BytesIO(resp.content)) as zf:
                manifest_data = json.loads(zf.read(zf.namelist()[0]))
            assets = manifest_data.get("assets", {})
            if "vanilla.mcp" in assets:
                hash_base = assets["vanilla.mcp"].get("md5", "")
            if "vanilla_patch.mcp" in assets:
                hash_base += assets["vanilla_patch.mcp"].get("md5", "")
        except Exception:
            raise ValueError("Android manifest unavailable or invalid") from None
        if not hash_base:
            raise ValueError("Android manifest missing vanilla hash")

        # RN is optional in the reference hash algorithm. Only an explicit 404
        # with no bundle declared by the manifest establishes absence; network
        # failures must not be silently converted into an empty hash.
        rn_hash = ""
        resp = self._session.get(f"{pre}/rn/index.bundle.backup", timeout=15)
        bundle_declared = any("index.bundle" in name for name in assets)
        if resp.status_code == 404 and not bundle_declared:
            self.rn_bundle_present = False
        else:
            resp.raise_for_status()
            payload = resp.content
            try:
                with zipfile.ZipFile(io.BytesIO(payload)) as zf:
                    payload = zf.read(zf.namelist()[0])
            except zipfile.BadZipFile:
                pass
            if not payload:
                raise ValueError("Android RN bundle empty")
            rn_hash = hashlib.md5(payload).hexdigest()
            self.rn_bundle_present = True

        # 5. 最终 resources_hash
        self.patch_resources_hash = hashlib.md5(
            (hash_base + rn_hash).encode()
        ).hexdigest()

    # ──────────────────────────────────────────────────────────────
    # PE 认证
    # ──────────────────────────────────────────────────────────────
    def authenticate_with_cookie(self, cookie_str: str) -> None:
        """使用 cookie（sauth_json）执行 PE 认证。

        Args:
            cookie_str: cookie JSON 字符串，格式 {"sauth_json": "...", ...}
        """
        self.user_id = self.user_token = ""
        self.user_detail = None
        # Explicit diagnostic profile, bound to the inspected official APK and
        # independently downloaded/verified patch MCPK. Never apply it to 3.10
        # or silently carry its hashes forward to unexamined artifacts.
        shipped_profile = self.pe_auth_profile == "official-android-3915"
        if shipped_profile and (
            self.platform != "android"
            or self.engine_version != OFFICIAL_ENGINE
            or (self.g79_latest_version,self.patch_resources_hash) not in OFFICIAL_PATCHES
        ):
            raise PEMetadataMismatchError(self.engine_version,self.g79_latest_version)
        self.cookie = cookie_str

        # 解析 cookie
        cookie_data = self._parse_cookie(cookie_str)
        sauth_data = self._parse_sauth(cookie_data.sauth_json)

        # 验证必需字段
        for field_name in ("sdkuid", "sessionid", "udid", "deviceid"):
            if not getattr(sauth_data, field_name).strip():
                raise ValueError(f"sauth_json 缺少字段 {field_name}")

        # 生成 seed 和 client_login_sn
        self.seed = str(uuid.uuid4())
        client_login_sn = secrets.token_hex(16).upper()

        # 构建 sauth payload（Android 格式）
        sauth_payload = self._build_sauth_payload(sauth_data, client_login_sn)
        sa_data_payload = self._build_sa_data_payload(sauth_data, cookie_data)
        sa_data_json = json.dumps(sa_data_payload, ensure_ascii=False, separators=(",", ":"))

        # 构建 version message 和 sign（平台相关 hash）
        if self.platform == "ios":
            library_hash = G79_IOS_LIBRARY_HASH
            signature_hash = G79_IOS_SIGNATURE_HASH
            pay_channel = G79_IOS_MESSAGE_TAG
        else:
            library_hash = G79_LIBRARY_HASH
            signature_hash = G79_SIGNATURE_HASH
            pay_channel = G79_ANDROID_MESSAGE_TAG
        if shipped_profile:
            # libminecraftpe.so SHA256 953d4ebd...88c0c73d; MD5 of its
            # complete APK entry. Certificate MD5 matches the legacy value.
            # Native call 0x95fc1d4..0x95fc1dc selects (3, 6).
            # See docs/rea-mcpk-auth-analysis.md for provenance and limits.
            library_hash = "260a8b0499822c3f4fb4e35e8051afab"

        version_message = (
            self.engine_version
            + library_hash
            + self.g79_latest_version
            + self.patch_resources_hash
            + signature_hash
            + self.seed
        )
        if shipped_profile:
            sign = pe_auth_sign(version_message, 3, 6, pad_full_block=True)
        else:
            sign = pe_auth_sign(version_message, 2, 9)

        # 构建 PeAuthData
        pe_auth_data = {
            "engine_version": self.engine_version,
            "extra_param": "extra",
            "message": version_message,
            "patch_version": self.g79_latest_version,
            "pay_channel": pay_channel,
            "sa_data": sa_data_json,
            "sauth_json": sauth_payload,
            "seed": self.seed,
            "sign": sign,
        }

        # 加密并发送
        payload = json.dumps(pe_auth_data, ensure_ascii=False, separators=(",", ":")).encode()
        encrypted = g79_http_encrypt(payload)

        resp = self._session.post(
            f"{self.core_server_url}/pe-authentication",
            data=encrypted.hex(),
            headers={
                "User-Agent": "libhttpclient/1.0.0.0",
                "Accept-Encoding": "gzip",
                "Content-Type": "application/json",
            },
            timeout=15,
        )
        resp.raise_for_status()

        # 解密响应
        encrypted_resp = bytes.fromhex(resp.text)
        decrypted = g79_http_decrypt(encrypted_resp)
        valid_json = get_valid_json(decrypted)

        login_resp = json.loads(valid_json)
        if login_resp.get("code", -1) != 0:
            raise PEAuthenticationError(login_resp.get('code'))

        entity = login_resp.get("entity", {})
        uid, token = entity.get("entity_id"), entity.get("token")
        if (isinstance(uid, bool) or not str(uid).isdigit() or int(uid) <= 0
                or not isinstance(token, str) or not token.strip()):
            raise RuntimeError("PE 认证响应缺少有效游戏凭据")
        self.user_id = str(uid)
        self.user_token = token
        self.seed = entity.get("seed", self.seed)

        logger.info("PE 认证成功: user_id=%s", self.user_id)

        # 获取用户详情（对齐 g79client G79AuthenticateWithCookie → GetUserDetail）
        # authentication-v2 的 displayName 需要真实昵称
        try:
            self.get_user_detail()
            logger.info("用户详情: name=%s, level=%s",
                        self.user_detail.get("name", ""),
                        self.user_detail.get("level", ""))
        except Exception as e:
            logger.warning("获取用户详情失败: %s", e)

    def _parse_cookie(self, cookie_str: str) -> CookieData:
        data = json.loads(cookie_str)
        return CookieData(
            sauth_json=data.get("sauth_json", ""),
            mac_addr=data.get("mac_addr", ""),
            ram=data.get("ram", ""),
            rom=data.get("rom", ""),
            is_guest=data.get("is_guest"),
            emulator=data.get("emulator"),
        )

    def _parse_sauth(self, sauth_json_str: str) -> SauthData:
        data = json.loads(sauth_json_str)
        # 解析 sdklog 里的设备信息（iOS cookie 包含 sdklog）
        sdklog: dict = {}
        sdklog_str = data.get("sdklog", "")
        if sdklog_str:
            try:
                sdklog = json.loads(sdklog_str)
            except (json.JSONDecodeError, TypeError):
                pass
        guest = data.get("is_unisdk_guest", 0)
        if type(guest) is not int or guest not in (0, 1):
            raise ValueError("invalid is_unisdk_guest")
        return SauthData(
            is_unisdk_guest=guest,
            gameid=data.get("gameid", ""),
            login_channel=data.get("login_channel", ""),
            app_channel=data.get("app_channel", ""),
            platform=data.get("platform", ""),
            sdkuid=data.get("sdkuid", ""),
            sessionid=data.get("sessionid", ""),
            sdk_version=data.get("sdk_version", ""),
            udid=data.get("udid", ""),
            deviceid=data.get("deviceid", ""),
            aim_info=data.get("aim_info", ""),
            client_login_sn=data.get("client_login_sn", ""),
            gas_token=data.get("gas_token", ""),
            source_platform=data.get("source_platform", ""),
            ip=data.get("ip", ""),
            device_model=sdklog.get("device_model", ""),
            os_name=sdklog.get("os_name", ""),
            os_ver=sdklog.get("os_ver", ""),
            access_token=data.get("access_token", ""),
        )

    def _build_sauth_payload(self, s: SauthData, client_login_sn: str) -> dict[str, Any]:
        """构建 sauth payload（使用 cookie 中的真实 platform/app_channel，适配 iOS）。

        对齐 g79client/auth.go:buildAndroidSauthPayload，但用 cookie 真实值替代硬编码 Android 值。
        """
        aim_info = s.aim_info.strip() or '{"aim":"127.0.0.1","country":"CN","tz":"+0800","tzid":"","celluar_ip":"","operator":"","is_vpn_enabled":"false"}'
        sdk_version = s.sdk_version.strip() or "5.9.0"
        ip = s.ip.strip() or "127.0.0.1"
        game_id = s.gameid.strip() or "x19"
        app_channel = s.app_channel.strip() or "netease"
        platform = s.platform.strip() or "ad"
        source_platform = s.source_platform.strip() or platform
        payload = {
            "access_token": s.access_token,
            "aim_info": aim_info,
            "app_channel": app_channel,
            "client_login_sn": client_login_sn,
            "deviceid": s.deviceid,
            "gameid": game_id,
            "gas_token": s.gas_token,
            "get_access_token": "1",
            "ip": ip,
            "is_unisdk_guest": s.is_unisdk_guest,
            "login_channel": "netease",
            "platform": platform,
            "sdk_version": sdk_version,
            "sdkuid": s.sdkuid,
            "sessionid": s.sessionid,
            "source_app_channel": app_channel,
            "source_platform": source_platform,
            "udid": s.udid,
        }

        if self.platform == "android":
            payload.update(step=G79_ANDROID_STEP, step2=G79_ANDROID_STEP2)
        return payload

    def _build_sa_data_payload(self, s: SauthData, c: CookieData) -> dict[str, Any]:
        """构建 sa_data payload（设备信息，使用 cookie 中的真实设备信息，适配 iOS）。

        对齐 g79client/auth.go:buildAndroidSaDataPayload，但用 cookie sdklog 里的真实值。
        """
        sdk_version = s.sdk_version.strip() or "5.9.0"
        udid = s.udid.strip() or "0000000000000000"
        app_channel = s.app_channel.strip() or "netease"
        os_name = s.os_name.strip() or ("ios" if self.platform == "ios" else "android")
        device_model = s.device_model.strip() or ("iPad8,6" if self.platform == "ios" else "SAMSUNG#SM-G977N")
        os_ver = s.os_ver.strip() or ("26.5" if self.platform == "ios" else "5.1.1")
        return {
            "app_channel": app_channel,
            "app_ver": self.g79_latest_version,
            "core_num": "u0004",
            "cpu_digit": "64",
            "cpu_hz": "2465600",
            "cpu_name": "placeholder",
            "device_height": "2732" if self.platform == "ios" else "900",
            "device_model": device_model,
            "device_width": "2048" if self.platform == "ios" else "1600",
            "disk": "",
            "emulator": c.emulator_flag,
            "first_udid": udid,
            "is_guest": c.is_guest_flag if c.is_guest is not None else s.is_unisdk_guest,
            "launcher_type": "PE_C++",
            "mac_addr": c.mac_address,
            "network": "wifi" if self.platform == "ios" else "mm_10086",
            "os_name": os_name,
            "os_ver": os_ver,
            "ram": c.ram_value,
            "rom": c.rom_value,
            "root": False,
            "sdk_ver": sdk_version,
            "start_type": "default",
            "udid": udid,
        }

    # ──────────────────────────────────────────────────────────────
    # HTTP API 请求辅助
    # ──────────────────────────────────────────────────────────────
    def _api_request(self, base_url: str, api_path: str, body: dict[str, Any]) -> dict:
        """发送加密 API 请求并返回解密后的 JSON。

        所有网易 API 请求格式相同：
        1. JSON body → G79HttpEncrypt → hex 编码
        2. Headers: user-id, user-token(动态 token)
        3. Response: hex → G79HttpDecrypt → GetValidJSON → JSON
        """
        body_str = json.dumps(body, ensure_ascii=False, separators=(",", ":"))
        encrypted = g79_http_encrypt(body_str.encode())
        token = calculate_dynamic_token(api_path, body_str, self.user_token)

        resp = self._session.post(
            base_url + api_path,
            data=encrypted.hex(),
            headers={
                "User-Agent": "libhttpclient/1.0.0.0",
                "Accept-Encoding": "gzip",
                "Content-Type": "application/json",
                "user-id": self.user_id,
                "user-token": token,
            },
            timeout=15,
        )
        resp.raise_for_status()

        encrypted_resp = bytes.fromhex(resp.text)
        decrypted = g79_http_decrypt(encrypted_resp)
        valid = get_valid_json(decrypted)
        return json.loads(valid)

    def _api_request_plain(self, base_url: str, api_path: str, body: dict[str, Any]) -> dict:
        """发送明文 JSON API 请求（租赁服等 API 用此格式，不加密）。

        对齐 g79client/rental_search_by_name.go：
        1. 请求体为明文 JSON（不加密、不 hex）
        2. Headers: user-id, user-token(动态 token), Content-Type: application/json; charset=utf-8
        3. 响应为明文 JSON（不 hex 解码、不 G79 解密）
        """
        body_str = json.dumps(body, ensure_ascii=False, separators=(",", ":"))
        token = calculate_dynamic_token(api_path, body_str, self.user_token)

        resp = self._session.post(
            base_url + api_path,
            data=body_str.encode(),
            headers={
                "User-Agent": "WPFLauncher/0.0.0.0",
                "Content-Type": "application/json; charset=utf-8",
                "user-id": self.user_id,
                "user-token": token,
            },
            timeout=15,
        )
        resp.raise_for_status()
        return resp.json()

    # ──────────────────────────────────────────────────────────────
    # 租赁服查询
    # ──────────────────────────────────────────────────────────────
    def search_rental_server(self, server_code: str) -> RentalServerEntity:
        """搜索租赁服，返回服务器实体。

        Args:
            server_code: 服务器编号（如 "12345678"）

        Returns:
            RentalServerEntity（含 entity_id, owner_id, min_level）
        """
        result = self._api_request_plain(
            self.web_server_url,
            "/rental-server/query/search-by-name",
            {"server_name": server_code},
        )
        entities = result.get("entities", [])
        if not entities:
            raise RuntimeError(f"未找到服务器号 {server_code} 对应的租赁服")
        e = entities[0]
        entity = RentalServerEntity(
            entity_id=str(e.get("entity_id", "")),
            owner_id=str(e.get("owner_id", "")),
            min_level=int(e.get("min_level", 0) or 0),
        )
        logger.info("搜索租赁服 %s → serverID=%s, owner=%s, min_level=%d",
                    server_code, entity.entity_id,
                    entity.owner_id or "(空)", entity.min_level)
        return entity

    def enter_rental_server_world(self, server_id: str, password: str) -> tuple[str, int]:
        """进入租赁服世界，返回 (IP, port)。

        Args:
            server_id: 服务器 ID（从 search_rental_server 获取）
            password: 服务器密码（空字符串表示无密码）

        Returns:
            (host, port) 元组
        """
        result = self._api_request_plain(
            self.web_server_url,
            "/rental-server-world-enter/get",
            {"server_id": server_id, "pwd": password},
        )
        status_entity = result.get('entity')
        status_entity = status_entity if isinstance(status_entity,dict) else {}
        self._last_world_enter_status = dict(
            code=result.get('code') if type(result.get('code')) is int else None,
            state=status_entity.get('state') if type(status_entity.get('state')) is int else None,
            progress=status_entity.get('async_progress') if type(status_entity.get('async_progress')) is int else None,
            total=status_entity.get('total_progress') if type(status_entity.get('total_progress')) is int else None,
            host_present=bool(status_entity.get('mcserver_host')),
            port_present=bool(status_entity.get('mcserver_port')),
            port_type=type(status_entity.get('mcserver_port')).__name__)
        if type(result.get('code')) is not int or result['code'] != 0:
            raise RentalWorldEnterError(result.get('code'))
        entity = result.get("entity") or {}
        if not isinstance(entity, dict):
            raise RentalWorldEnterError()
        host = entity.get("mcserver_host", "")
        if host is None:
            host = ''
        raw_port = entity.get("mcserver_port", 0)
        if raw_port is None or raw_port == '':
            raw_port = 0
        try:
            port = int(raw_port) if not isinstance(raw_port, bool) else 0
        except (ValueError, TypeError):
            raise RentalWorldEnterError() from None
        if not isinstance(host, str) or (port and not 1 <= port <= 65535):
            raise RentalWorldEnterError()
        if not host or port == 0:
            if entity.get('state') == 0:
                raise RentalWorldPending(entity.get('state'), entity.get('async_progress'), entity.get('total_progress'))
            raise RentalWorldEnterError()
        logger.info("进入租赁服世界 → %s:%d", host, port)
        return host, port

    # ──────────────────────────────────────────────────────────────
    # Link 连接 + GameStart（Bedrock 连接前置，对齐 fatalder login.go:454-496）
    # ──────────────────────────────────────────────────────────────
    def connect_link_and_send_game_start(
        self,
        server_code: str,
        server_entity: RentalServerEntity,
    ) -> None:
        """建立 Link 连接并发送 GameStart（通知网易后端玩家要进入游戏）。

        对齐 fatalder login.go:454-496 的 Link+GameStart 流程：
        1. 从 Link 服务器列表随机选一个节点
        2. TCP 连接 → RSA 握手 → ChaCha8 → LoginV2
        3. 发送 GameStart（含游戏信息）
        4. 关闭连接

        必须在 send_auth_v2_request 之前调用（fatalder 的顺序是
        Link+GameStart → GenerateRentalGameAuthV2 → SendAuthV2Request）。

        Args:
            server_code: 服务器编号（如 "12345678"）
            server_entity: search_rental_server 返回的服务器实体
        """
        from proloader.net.netease.link import dial_link

        conn = dial_link(self._session, self.user_id, self.user_token)
        try:
            # 构建 game_info（对齐 fatalder login.go:469-477，序列化为嵌套 JSON 字符串）
            owner_id = (server_entity.owner_id or "").strip()
            owner_name = owner_id or server_code  # 简化：不调 GetUserElseDetailMany
            game_info = {
                "min_level": server_entity.min_level,
                "room_name": server_code,
                "gameType": "RentalGame",
                "res_name": server_code,
                "ownerName": owner_name,
                "ownerId": owner_id,
                "id": server_entity.entity_id,
            }
            game_info_json = json.dumps(
                game_info, ensure_ascii=False, separators=(",", ":")
            )

            # GameStart payload（对齐 fatalder login.go:482-489）
            game_start_payload = {
                "game_info": game_info_json,  # 嵌套 JSON 字符串
                "strict_mode": True,
                "game_type": 10,  # GAME_START_TYPE_RENTAL
                "is_free_play": False,
                "game_id": server_entity.entity_id,
                "play_iids": [],
            }
            conn.send_game_start(game_start_payload)
        finally:
            conn.close()

        logger.info("Link+GameStart 完成（serverID=%s）", server_entity.entity_id)

    # ──────────────────────────────────────────────────────────────
    # JWT Chain 获取
    # ──────────────────────────────────────────────────────────────
    def generate_rental_game_auth_v2(self, server_id: str, client_key: str) -> bytes:
        """生成租赁服认证 v2 数据。

        Args:
            server_id: 服务器 ID
            client_key: 客户端 ECDH 公钥（base64 SPKI DER）

        Returns:
            authv2 JSON bytes
        """
        uid_int = int(self.user_id)
        authv2 = {
            "bit": "64",
            "clientKey": client_key,
            "displayName": self._get_display_name(),
            "engineVersion": self.engine_version,
            "netease_sid": f"{server_id}:RentalGame",
            "os_name": "android",
            "patchVersion": self.g79_latest_version,
            "uid": uid_int,
        }
        return json.dumps(authv2, ensure_ascii=False, separators=(",", ":")).encode()

    def send_auth_v2_request(self, authv2_data: bytes) -> str:
        """发送认证 v2 请求，获取 JWT chain。

        Args:
            authv2_data: generate_rental_game_auth_v2 返回的 JSON bytes

        Returns:
            JWT chain 字符串（chainInfo）
        """
        encrypted = g79_http_encrypt(authv2_data)
        token = calculate_dynamic_token("/authentication-v2", authv2_data.decode(), self.user_token)

        resp = self._session.post(
            f"{self.auth_server_url}/authentication-v2",
            data=encrypted.hex(),
            headers={
                "User-Agent": "libhttpclient/1.0.0.0",
                "Accept-Encoding": "gzip",
                "Content-Type": "application/json",
                "user-id": self.user_id,
                "user-token": token.encode().hex(),
            },
            timeout=15,
        )
        if resp.status_code != 200:
            logger.error("authentication-v2 返回 %d", resp.status_code)
            logger.error("响应头: %s", dict(resp.headers))
            logger.error("响应体: %s", resp.text[:1000])
            resp.raise_for_status()

        text = resp.text.strip()
        # 响应可能是加密 hex 或明文 JSON
        try:
            encrypted_resp = bytes.fromhex(text)
            decrypted = g79_http_decrypt(encrypted_resp)
            valid = get_valid_json(decrypted)
            result = json.loads(valid)
        except (ValueError, json.JSONDecodeError):
            # 不是 hex，尝试直接作为明文 JSON
            result = json.loads(text)

        # 响应格式: {"chain": ["jwt1", "jwt2", ...]}（该 API 无 code 字段）
        chain = result.get("chain") or result.get("chainInfo")
        if not chain:
            code = result.get("code")
            if code is not None and code != 0:
                raise RuntimeError(
                    f"获取 JWT chain 失败 (code: {code}): {result.get('message', '')}"
                )
            raise RuntimeError(f"JWT chain 为空: {result}")
        # 返回 JSON 字符串（chain 是数组时序列化，是字符串时直接用）
        if isinstance(chain, list):
            chain_info = json.dumps(chain)
            logger.info("获取 JWT chain 成功 (%d 个 JWT)", len(chain))
        else:
            chain_info = chain
            logger.info("获取 JWT chain 成功 (%d 字符)", len(chain_info))
        return chain_info

    def _get_display_name(self) -> str:
        """获取用户昵称。"""
        if self.user_detail:
            return self.user_detail.get("name", "player")
        return "player"

    def get_user_detail(self) -> dict:
        """获取用户详情（昵称等）。

        对齐 g79client/user_get_detail.go:GetUserDetail
        - URL: CoreServerURL + "/pe-user-detail/get"
        - 格式：明文 JSON（不走 G79 加密）
        """
        result = self._api_request_plain(
            self.core_server_url,
            "/pe-user-detail/get",
            {},
        )
        entity = result.get("entity", {})
        self.user_detail = entity
        return entity
