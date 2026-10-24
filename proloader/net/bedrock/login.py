"""Bedrock Login 流程：JWT chain 构建 + Login 包 + 加密握手解析。

⚠️ 关键格式（Gophertunnel minecraft/protocol/login/ 确认）：

    Login 包：varuint(1) | ClientProtocol(i32 BE) | ConnectionRequest(varuint len + bytes)

    ConnectionRequest（二进制长度前缀 + JSON）：
        chainLength(i32 LE) | chainJSON | rawTokenLength(i32 LE) | rawToken(JWT)

    chainJSON（离线 legacy 模式）：
        {"chain":["<自签名JWT>"]}  （单 JWT，含 extraData + identityPublicKey）

    JWT（ES384 签名，手工构建）：
        base64url(header).base64url(payload).base64url(signature)
        header: {"alg":"ES384","x5u":"<base64 SPKI 公钥>"}
        payload: {"exp","nbf","identityPublicKey","extraData":{XUID,Identity,DisplayName}}

    ServerToClientHandshake：varuint(3) | JWT(varuint len + bytes)
        JWT payload: {"salt":"<base64 16字节盐>"}
        JWT header x5u: 服务端 P-384 公钥

    ClientToServerHandshake：varuint(4)，空包（第一个加密包）

离线模式连原版 BDS：online-mode=false，用 EncodeOffline legacy=true 自签名。
"""

from __future__ import annotations

import base64
import json
import random
import time
import uuid
from dataclasses import dataclass, field
from pathlib import Path

from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.asymmetric import ec

from proloader.net.bedrock.crypto import (
    load_public_key_spki,
    serialize_public_key_b64,
)
from proloader.net.bedrock.packets import PacketID
from proloader.net.raknet.wire import ByteReader, ByteWriter


# ═══════════════════════════════════════════════════════════════════════════════
# JWT 工具（手工构建，ES384 签名）
# ═══════════════════════════════════════════════════════════════════════════════
def _b64url(data: bytes) -> str:
    """base64url 编码（无填充）。"""
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode("ascii")


def _b64url_decode(s: str) -> bytes:
    """base64url 解码（补齐填充）。"""
    padding = 4 - len(s) % 4
    if padding != 4:
        s += "=" * padding
    return base64.urlsafe_b64decode(s)


def _der_to_jose_ecdsa(signature: bytes, curve_key_size_bits: int = 384) -> bytes:
    """把 DER 编码的 ECDSA 签名转成 JOSE 原始编码 (r || s)。

    ⚠️ Python cryptography 库的 sign() 输出 DER 编码（ASN.1 SEQUENCE），
    但 JWT/JOSE (ES384) 标准要求原始编码：r 和 s 各 zero-padded 到曲线字节长度，
    拼接在一起。P-384 → r(48B) + s(48B) = 96B。

    Gophertunnel 用的 go-jose 库自动输出 JOSE 编码，所以 BDS 期望 JOSE 格式。
    不转换的话签名验证必失败，BDS 会静默丢弃 Login 包。
    """
    from cryptography.hazmat.primitives.asymmetric.utils import decode_dss_signature
    r, s = decode_dss_signature(signature)
    coord_len = curve_key_size_bits // 8  # P-384 → 48 bytes
    return r.to_bytes(coord_len, "big") + s.to_bytes(coord_len, "big")


def sign_jwt(priv: ec.EllipticCurvePrivateKey, header: dict, payload: dict) -> str:
    """构建并签名 JWT（ES384）。

    JWT = base64url(header).base64url(payload).base64url(signature)
    签名用 ECDSA P-384 + SHA-384，签名用 JOSE 原始编码（非 DER）。
    """
    header_b64 = _b64url(json.dumps(header, separators=(",", ":")).encode())
    payload_b64 = _b64url(json.dumps(payload, separators=(",", ":")).encode())
    signing_input = f"{header_b64}.{payload_b64}".encode("ascii")
    der_sig = priv.sign(signing_input, ec.ECDSA(hashes.SHA384()))
    signature = _der_to_jose_ecdsa(der_sig, 384)  # DER → JOSE 原始编码
    sig_b64 = _b64url(signature)
    return f"{header_b64}.{payload_b64}.{sig_b64}"


def parse_jwt(jwt_str: str) -> tuple:
    """解析 JWT（不验证签名），返回 (header_dict, payload_dict, signature_bytes)。"""
    parts = jwt_str.split(".")
    if len(parts) != 3:
        raise ValueError(f"JWT 应有 3 段，实际 {len(parts)}")
    header = json.loads(_b64url_decode(parts[0]))
    payload = json.loads(_b64url_decode(parts[1]))
    signature = _b64url_decode(parts[2])
    return header, payload, signature


# ═══════════════════════════════════════════════════════════════════════════════
# 登录数据结构
# ═══════════════════════════════════════════════════════════════════════════════
@dataclass
class IdentityData:
    """玩家身份数据（JWT chain 的 extraData）。

    离线模式：XUID 为空（未登录 Xbox Live），Identity 是随机 UUID。
    """

    XUID: str = ""
    Identity: str = field(default_factory=lambda: str(uuid.uuid4()))
    DisplayName: str = "proloader"
    TitleID: str = ""


# 默认皮肤资源补丁（fatalder skin_resource_patch.json）
# 服务端要求 SkinResourcePatch 是 base64 编码的合法 JSON
# ⚠️ 对齐 fatalder defaultClientData: 从 embed 的 skin_resource_patch.json 读取
_DEFAULT_SKIN_RESOURCE_PATCH = (
    Path(__file__).resolve().parent / "skin_resource_patch.json"
).read_bytes()

# 默认皮肤几何数据（fatalder skin_geometry.json）
# ⚠️ fatalder defaultClientData 用 embed 的 skin_geometry.json（非空），网易服务端可能要求非空
_DEFAULT_SKIN_GEOMETRY = (
    Path(__file__).resolve().parent / "skin_geometry.json"
).read_bytes()


def _make_default_skin_data(width: int = 256, height: int = 256) -> str:
    """生成默认透明皮肤数据（base64 编码的 RGBA 像素）。

    抓包确认网易客户端发 256x256 全零像素（extract_game_version.py）。
    每像素 4 字节 (R=0, G=0, B=0, A=0)，共 width*height 像素。
    """
    raw = b"\x00\x00\x00\x00" * (width * height)
    return base64.b64encode(raw).decode("ascii")


def _make_random_playfab_id() -> str:
    """生成随机 PlayFabId（8 字节 hex，对齐 fatalder defaultClientData）。"""
    return uuid.uuid4().hex[:16]


@dataclass
class ClientData:
    """客户端数据（JWT rawToken 的 payload）。

    字段对齐 fatalder protocol/login/data.go 的 ClientData 结构。
    所有 JSON 键名严格按 fatalder 的 json tag（驼峰，注意大小写差异）。

    ⚠️ 默认值严格对齐 fatalder defaultClientData (dial.go:468-519)：
       fatalder 未显式设置的字段用 Go 零值（""/false/0）。
       网易服务端会校验 ClientData，非零值可能导致 "Connection Request invalid"。
    """

    # ── 核心字段 ──
    ClientRandomId: int = field(default_factory=lambda: random.randint(1, 2**63 - 1))
    ServerAddress: str = ""
    GameVersion: str = "1.21.2"  # fatalder protocol.CurrentVersion
    DeviceOS: int = 1  # protocol.DeviceAndroid（fatalder setAndroidData 强制 1）
    DeviceID: str = field(default_factory=lambda: str(uuid.uuid4()))
    DeviceModel: str = ""  # fatalder 零值
    ThirdPartyName: str = ""  # fatalder 零值（非 "proloader"）
    LanguageCode: str = "zh_CN"  # fatalder defaultClientData

    # ── 皮肤字段 ──
    SkinId: str = field(default_factory=lambda: str(uuid.uuid4()))  # json:"SkinId"
    SkinData: str = field(default_factory=_make_default_skin_data)  # base64 RGBA 256x256
    SkinImageWidth: int = 256  # 抓包确认（网易客户端发 256x256）
    SkinImageHeight: int = 256
    PlayFabId: str = field(default_factory=_make_random_playfab_id)
    SelfSignedId: str = field(default_factory=lambda: str(uuid.uuid4()))
    AnimatedImageData: list = field(default_factory=list)
    PersonaSkin: bool = False  # fatalder 零值（非 True）
    CapeId: str = ""
    CapeData: str = ""
    CapeImageWidth: int = 0
    CapeImageHeight: int = 0
    CapeOnClassicSkin: bool = False
    PremiumSkin: bool = False
    SkinResourcePatch: str = field(
        default_factory=lambda: base64.b64encode(_DEFAULT_SKIN_RESOURCE_PATCH).decode("ascii")
    )
    SkinGeometryData: str = field(
        default_factory=lambda: base64.b64encode(_DEFAULT_SKIN_GEOMETRY).decode("ascii")
    )  # fatalder 用 embed 的 skin_geometry.json（非空）
    # fatalder defaultClientData: base64.StdEncoding.EncodeToString([]byte("0.0.0")) = "MC4wLjA="
    SkinGeometryDataEngineVersion: str = field(
        default_factory=lambda: base64.b64encode(b"0.0.0").decode("ascii")
    )
    SkinAnimationData: str = ""
    ArmSize: str = ""  # fatalder 零值（非 "wide"）
    SkinColor: str = ""  # fatalder 零值（非 "#ffffffff"）
    PersonaPieces: list = field(default_factory=list)
    PieceTintColors: list = field(default_factory=list)  # json:"PieceTintColors"

    # ── 输入/UI 字段 ──
    CurrentInputMode: int = 0  # fatalder 零值（非 1）
    DefaultInputMode: int = 0  # fatalder 零值（非 1）
    GuiScale: int = 0  # json:"GuiScale"
    IsEditorMode: bool = False
    UIProfile: int = 0  # 0=Classic
    ThirdPartyNameOnly: bool = False
    TrustedSkin: bool = False
    OverrideSkin: bool = False
    CompatibleWithClientSideChunkGen: bool = False

    # ── 平台字段 ──
    PlatformOfflineId: str = ""  # json:"PlatformOfflineId"
    PlatformOnlineId: str = ""  # json:"PlatformOnlineId"
    PlatformUserId: str = ""  # omitempty

    # ── 网易专有字段 ──
    # ⚠️ 抓包确认 JSON 键名是 "SkinIID"（两个大写 I），不是 "SkinID"
    SkinItemID: str = ""  # json:"SkinIID" — 网易皮肤道具 ID
    GrowthLevel: int = -1  # 网易成长等级，-1=未知（fatalder defaultClientData）
    # ⚠️ 以下字段抓包确认网易 860 客户端发送（extract_game_version.py 提取）
    BloomData: str = ""  # json:"BloomData"
    MaxViewDistance: int = 0  # json:"MaxViewDistance"
    MemoryTier: int = 0  # json:"MemoryTier"
    PlatformType: int = 0  # json:"PlatformType"
    GraphicsMode: int = 0  # json:"GraphicsMode"
    IsReconnect: bool = False  # json:"IsReconnect"

    def to_jwt_claims(self) -> dict:
        """转为 JWT payload claims。

        ⚠️ 键名严格对齐 fatalder data.go 的 json tag。
        注意大小写差异：SkinId(小d) vs SkinID(大ID) vs DeviceId(小d)。
        """
        return {
            "AnimatedImageData": self.AnimatedImageData,
            "CapeData": self.CapeData,
            "CapeId": self.CapeId,
            "CapeImageHeight": self.CapeImageHeight,
            "CapeImageWidth": self.CapeImageWidth,
            "CapeOnClassicSkin": self.CapeOnClassicSkin,
            "ClientRandomId": self.ClientRandomId,
            "CurrentInputMode": self.CurrentInputMode,
            "DefaultInputMode": self.DefaultInputMode,
            "DeviceModel": self.DeviceModel,
            "DeviceOS": self.DeviceOS,
            "DeviceId": self.DeviceID,
            "GameVersion": self.GameVersion,
            "GuiScale": self.GuiScale,
            "IsEditorMode": self.IsEditorMode,
            "LanguageCode": self.LanguageCode,
            "PersonaSkin": self.PersonaSkin,
            "PlatformOfflineId": self.PlatformOfflineId,
            "PlatformOnlineId": self.PlatformOnlineId,
            "PremiumSkin": self.PremiumSkin,
            "SelfSignedId": self.SelfSignedId,
            "ServerAddress": self.ServerAddress,
            "SkinAnimationData": self.SkinAnimationData,
            "SkinData": self.SkinData,
            "SkinGeometryData": self.SkinGeometryData,
            "SkinGeometryDataEngineVersion": self.SkinGeometryDataEngineVersion,
            "SkinId": self.SkinId,
            "PlayFabId": self.PlayFabId,
            "SkinImageHeight": self.SkinImageHeight,
            "SkinImageWidth": self.SkinImageWidth,
            "SkinResourcePatch": self.SkinResourcePatch,
            "SkinColor": self.SkinColor,
            "ArmSize": self.ArmSize,
            "PersonaPieces": self.PersonaPieces,
            "PieceTintColors": self.PieceTintColors,
            "ThirdPartyName": self.ThirdPartyName,
            "ThirdPartyNameOnly": self.ThirdPartyNameOnly,
            "UIProfile": self.UIProfile,
            "TrustedSkin": self.TrustedSkin,
            "OverrideSkin": self.OverrideSkin,
            "CompatibleWithClientSideChunkGen": self.CompatibleWithClientSideChunkGen,
            # ── 网易专有字段（抓包确认键名）──
            "SkinIID": self.SkinItemID,  # ⚠️ 两个大写 I（抓包确认），不是 SkinID
            "GrowthLevel": self.GrowthLevel,
            "BloomData": self.BloomData,
            "MaxViewDistance": self.MaxViewDistance,
            "MemoryTier": self.MemoryTier,
            "PlatformType": self.PlatformType,
            "GraphicsMode": self.GraphicsMode,
            "IsReconnect": self.IsReconnect,
        }


# ═══════════════════════════════════════════════════════════════════════════════
# ConnectionRequest 编码
# ═══════════════════════════════════════════════════════════════════════════════
def build_offline_login_chain(
    priv: ec.EllipticCurvePrivateKey,
    identity: IdentityData,
    client_data: ClientData,
    legacy: bool = False,
) -> bytes:
    """构建离线模式 ConnectionRequest（自签名 JWT）。

    格式：chainLength(i32 LE) + chainJSON + rawTokenLength(i32 LE) + rawToken

    ⚠️ BDS 1.26.10+ 用 OIDC multiplayer token 格式（legacy=False）：
        chainJSON = {"Certificate":"{\"chain\":[\"\"]}","AuthenticationType":2,"Token":"<OIDC_JWT>"}
        OIDC JWT claims 用短键名（cpk/xid/xname/leguuid），aud=api://auth-minecraft-services/multiplayer
        RawToken 仍是 ClientData JWT（同一私钥签名）

    legacy=True 用于 pre-1.26.10 服务器：
        chainJSON = {"chain":["<JWT>"]}  （JWT 含 extraData + identityPublicKey）
    """
    import struct

    pub_key_b64 = serialize_public_key_b64(priv)
    now = int(time.time())
    jwt_header = {"alg": "ES384", "x5u": pub_key_b64}
    claims_base = {
        "exp": now + 6 * 3600,
        "nbf": now - 6 * 3600,
    }

    if legacy:
        # ── legacy 格式（pre-1.26.10）──
        chain_jwt_payload = {
            **claims_base,
            "identityPublicKey": pub_key_b64,
            "extraData": {
                "XUID": identity.XUID,
                "identity": identity.Identity,
                "displayName": identity.DisplayName,
                "titleId": identity.TitleID,
            },
            "certificateAuthority": True,
        }
        chain_jwt = sign_jwt(priv, header=jwt_header, payload=chain_jwt_payload)
        chain_bytes = json.dumps({"chain": [chain_jwt]}, separators=(",", ":")).encode()
    else:
        # ── 新格式（1.26.10+，OIDC multiplayer token）──
        # OIDC token JWT：短键名 claims（Gophertunnel tokenClaims 确认）
        oidc_payload = {
            **claims_base,
            "aud": ["api://auth-minecraft-services/multiplayer"],
            "ipt": "",            # IdentityProviderType
            "mid": "",            # PlayFabID
            "tid": "",            # PlayFabTitleID
            "cpk": pub_key_b64,   # ClientPublicKey
            "xid": identity.XUID,  # XUID（离线为空）
            "xname": identity.DisplayName,  # DisplayName
            "leguuid": identity.Identity,   # Identity UUID（离线模式必填）
        }
        oidc_jwt = sign_jwt(priv, header=jwt_header, payload=oidc_payload)

        # chainJSON：Certificate 是嵌套 JSON 字符串，AuthenticationType=2
        cert_json = json.dumps({"chain": [""]}, separators=(",", ":"))
        request_obj = {
            "Certificate": cert_json,
            "AuthenticationType": 2,
            "Token": oidc_jwt,
        }
        chain_bytes = json.dumps(request_obj, separators=(",", ":")).encode()

    # RawToken JWT：含 ClientData（两种格式共用）
    raw_token_payload = {**claims_base, **client_data.to_jwt_claims()}
    raw_token = sign_jwt(priv, header=jwt_header, payload=raw_token_payload)

    # 编码 ConnectionRequest
    w = ByteWriter()
    w.write(struct.pack("<i", len(chain_bytes)))  # chainLength LE i32
    w.write(chain_bytes)
    w.write(struct.pack("<i", len(raw_token)))  # rawTokenLength LE i32
    w.write(raw_token.encode())
    return w.to_bytes()


def build_login_from_jwt_chain(
    jwt_chain: str,
    client_data: ClientData,
    priv: ec.EllipticCurvePrivateKey,
    prepend_client_jwt: bool | None = None,
    *,
    protocol: int = 860,
) -> bytes:
    """将认证 API 的 chain 编为 ConnectionRequest，RawToken 使用认证时的私钥。

    protocol=686 对齐 Fatalder protocol/login/request.go:Encode：前插客户端
    ES384 JWT，chainJSON 为 {"chain":[...]}。860 保留现有 Certificate 封装。
    API 的 JWT 原文和顺序保留；这里只检查结构与末端公钥绑定，不验证 CA 信任。
    prepend_client_jwt=None 按协议选择；显式 bool 保留兼容旧调用方。
    """
    import struct

    if protocol not in (686, 860):
        raise ValueError("网易 Login 仅支持协议 686 或 860")
    if not isinstance(priv.curve, ec.SECP384R1):
        raise ValueError("网易 Login 需要 P-384 私钥")
    if prepend_client_jwt is None:
        prepend_client_jwt = protocol == 686

    # 1. 解析 jwt_chain，提取 chain 数组
    parsed = json.loads(jwt_chain)
    if isinstance(parsed, list):
        chain_list = parsed
    elif isinstance(parsed, dict) and "chain" in parsed:
        chain_list = parsed["chain"]
    else:
        raise ValueError(f"无法识别的 JWT chain 格式: {type(parsed)}")

    if not isinstance(chain_list, list) or not chain_list:
        raise ValueError("JWT chain 必须是非空数组")
    if any(not isinstance(token, str) or not token for token in chain_list):
        raise ValueError("JWT chain 元素必须是非空 JWT 字符串")
    try:
        decoded = [parse_jwt(token) for token in chain_list]
        if any(not isinstance(h, dict) or not isinstance(p, dict) or
               h.get("alg") != "ES384" or len(sig) != 96
               for h, p, sig in decoded):
            raise ValueError
        terminal_key = load_public_key_spki(
            base64.b64decode(decoded[-1][1]["identityPublicKey"], validate=True)
        )
        if terminal_key.public_numbers() != priv.public_key().public_numbers():
            raise ValueError("JWT chain 末端公钥与认证私钥不匹配")
    except (KeyError, TypeError, ValueError) as exc:
        raise ValueError("JWT chain 格式或末端公钥无效；请使用获取 chain 时的 P-384 私钥") from exc

    pub_key_b64 = serialize_public_key_b64(priv)
    jwt_header = {"alg": "ES384", "x5u": pub_key_b64}

    if prepend_client_jwt:
        # 前插客户端 JWT（fatalder Encode 逻辑，686 协议用）
        first_header, _, _ = parse_jwt(chain_list[0])
        nease_x5u = first_header.get("x5u")
        if not nease_x5u:
            raise ValueError("JWT chain[0] 缺少 x5u header")
        now = int(time.time())
        claims_base = {"exp": now + 6 * 3600, "nbf": now - 6 * 3600}
        client_chain_jwt_payload = {
            **claims_base,
            "identityPublicKey": nease_x5u,
            "certificateAuthority": True,
        }
        client_chain_jwt = sign_jwt(priv, header=jwt_header, payload=client_chain_jwt_payload)
        new_chain = [client_chain_jwt] + chain_list
    else:
        # 不前插：直接用网易返回的原始 chain（860 协议，抓包确认）
        new_chain = list(chain_list)

    # 686 使用 legacy chain；860 使用嵌套 Certificate。不可只改 Login 版本号。
    chain_obj = {"chain": new_chain}
    if protocol == 860:
        cert_json = json.dumps(chain_obj, separators=(",", ":"))
        chain_obj = {"AuthenticationType": 0, "Certificate": cert_json, "Token": ""}
    chain_bytes = json.dumps(chain_obj, separators=(",", ":")).encode()

    # 3. 创建 RawToken（ClientData JWT，用客户端私钥签名）
    #    ⚠️ RawToken 只含 ClientData 字段，不含 exp/nbf
    raw_token_payload = client_data.to_jwt_claims()
    raw_token = sign_jwt(priv, header=jwt_header, payload=raw_token_payload)

    # 4. 编码 ConnectionRequest
    #    格式：chainLength(i32 LE) + chainJSON + rawTokenLength(i32 LE) + rawToken
    w = ByteWriter()
    w.write(struct.pack("<i", len(chain_bytes)))  # chainLength LE i32
    w.write(chain_bytes)
    w.write(struct.pack("<i", len(raw_token)))  # rawTokenLength LE i32
    w.write(raw_token.encode())
    return w.to_bytes()


# ═══════════════════════════════════════════════════════════════════════════════
# Login 包编码
# ═══════════════════════════════════════════════════════════════════════════════
PROTOCOL_VERSION: int = 2168  # MC 1.26.40
GAME_VERSION: str = "1.26.40"


def build_login_packet(connection_request: bytes, protocol: int = PROTOCOL_VERSION) -> bytes:
    """构建 Login 包。

    格式：varuint(1) | ClientProtocol(i32 BE) | ConnectionRequest(varuint len + bytes)
    """
    w = ByteWriter()
    w.write_unsigned_varint(PacketID.LOGIN)  # 包头
    import struct
    w.write(struct.pack(">i", protocol))  # ClientProtocol 大端 i32
    w.write_unsigned_varint(len(connection_request))  # ConnectionRequest 长度
    w.write(connection_request)
    return w.to_bytes()


def build_request_network_settings(protocol: int = PROTOCOL_VERSION) -> bytes:
    """构建 RequestNetworkSettings 包（Login 前发，协商压缩）。"""
    import struct
    w = ByteWriter()
    w.write_unsigned_varint(PacketID.REQUEST_NETWORK_SETTINGS)
    w.write(struct.pack(">i", protocol))  # 大端 i32
    return w.to_bytes()


def build_client_to_server_handshake() -> bytes:
    """构建 ClientToServerHandshake 包（空包，第一个加密包）。"""
    w = ByteWriter()
    w.write_unsigned_varint(PacketID.CLIENT_TO_SERVER_HANDSHAKE)
    return w.to_bytes()


# ═══════════════════════════════════════════════════════════════════════════════
# ServerToClientHandshake 解析（提取服务端公钥 + salt）
# ═══════════════════════════════════════════════════════════════════════════════
@dataclass
class ServerHandshake:
    """ServerToClientHandshake 解析结果。"""

    server_public_key: ec.EllipticCurvePublicKey
    salt: bytes  # 16 字节
    raw_jwt: str


def parse_server_to_client_handshake(data: bytes) -> ServerHandshake:
    """解析 ServerToClientHandshake 包。

    格式：varuint(3) | JWT(varuint len + bytes)
    JWT header x5u: 服务端 P-384 公钥（base64 SPKI DER）
    JWT payload salt: base64 编码的 16 字节盐
    """
    r = ByteReader(data)
    packet_id = r.read_unsigned_varint()
    if packet_id != PacketID.SERVER_TO_CLIENT_HANDSHAKE:
        raise ValueError(f"不是 ServerToClientHandshake: ID={packet_id}")
    jwt_len = r.read_unsigned_varint()
    jwt_str = r.read(jwt_len).decode("ascii")

    header, payload, _ = parse_jwt(jwt_str)

    # x5u = 服务端公钥（base64 SPKI DER）
    x5u = header.get("x5u")
    if not x5u:
        raise ValueError("ServerToClientHandshake JWT 缺少 x5u")
    server_pub_der = base64.b64decode(x5u)  # 注意：x5u 用标准 base64，不是 base64url
    server_pub = load_public_key_spki(server_pub_der)

    # salt = base64(RawStdEncoding) 编码的 16 字节
    salt_b64 = payload.get("salt")
    if not salt_b64:
        raise ValueError("ServerToClientHandshake JWT 缺少 salt")
    # Gophertunnel 用 base64.RawStdEncoding（无填充标准 base64）
    salt = base64.b64decode(salt_b64 + "==")  # 补齐可能的填充

    return ServerHandshake(
        server_public_key=server_pub,
        salt=salt,
        raw_jwt=jwt_str,
    )


# ═══════════════════════════════════════════════════════════════════════════════
# PlayStatus 解析
# ═══════════════════════════════════════════════════════════════════════════════
@dataclass(frozen=True)
class DisconnectData:
    """Disconnect 的共同前缀；不推断较新版本尾部字段。"""

    reason: int
    hide_screen: bool
    message: str


def parse_disconnect(data: bytes) -> DisconnectData:
    """按本地 Fatalder disconnect.go 读取 reason/HideScreen/可选 Message。

    注释中的消息条件写反了，以 Marshal 的 !HideDisconnectionScreen 为准。
    860/2168 可能有额外尾部字段，此处仅提取共同前缀，不宣称完整解码。
    """
    r = ByteReader(data)
    if r.read_unsigned_varint() & 0x3FF != PacketID.DISCONNECT:
        raise ValueError("不是 Disconnect 包")
    reason = r.read_varint()
    hide = r.read_byte()
    if hide not in (0, 1):
        raise ValueError("Disconnect HideScreen 不是 bool")
    return DisconnectData(reason, bool(hide), "" if hide else r.read_string())


def parse_play_status(data: bytes) -> int:
    """解析 PlayStatus 包，返回状态码。"""
    import struct
    r = ByteReader(data)
    r.read_unsigned_varint()  # 包头
    return struct.unpack(">i", r.read(4))[0]  # 大端 i32


def parse_network_settings(data: bytes) -> tuple:
    """解析 NetworkSettings 包，返回 (compression_threshold, compression_algorithm)。

    格式（Gophertunnel network_settings.go 确认，全 LE）：
        varuint(packet_id) | CompressionThreshold(u16 LE) | CompressionAlgorithm(u16 LE)
        | ClientThrottle(bool) | ClientThrottleThreshold(u8) | ClientThrottleScalar(f32 LE)

    algorithm: 0=flate, 1=snappy, 0xffff=none（协商用）
    """
    import struct
    r = ByteReader(data)
    r.read_unsigned_varint()  # 包头
    threshold = struct.unpack("<H", r.read(2))[0]  # u16 LE
    algorithm = struct.unpack("<H", r.read(2))[0]  # u16 LE
    # 跳过 ClientThrottle(bool) + ClientThrottleThreshold(u8) + ClientThrottleScalar(f32) = 6 字节
    r.read(6)
    return threshold, algorithm


# ═══════════════════════════════════════════════════════════════════════════════
# 游戏初始化包（ResourcePack 响应 + StartGame 解析 + ChunkRadius + Init）
# ═══════════════════════════════════════════════════════════════════════════════
def build_resource_pack_client_response(status: int = 2, netease_mode: bool = False) -> bytes:
    """ResourcePackClientResponse。

    格式取决于协议版本：

    **protocol 2168（vanilla BDS 1.26+）**（Gophertunnel 源码确认）：
        varuint(8) | varuint(response) | string(response_name) | [if response==1: pack list]
        枚举从 0 开始：0=Refused, 1=SendPacks, 2=AllPacksDownloaded, 3=Completed

    **protocol 860（网易租赁服）**（fatalder resource_pack_client_response.go 确认）：
        varuint(8) | u8(response) | uint16_LE(pack_count) | [pack entries...]
        枚举从 1 开始：1=Refused, 2=SendPacks, 3=AllPacksDownloaded, 4=Completed
        ⚠️ response 是 u8（不是 varuint！），pack_count 是 uint16 LE
        ⚠️ 只有 response==2(SendPacks) 才写 pack 列表

    离线/网易 BDS 无资源包的标准流程：
        1. 收到 ResourcePacksInfo  → 发 AllPacksDownloaded
        2. 收到 ResourcePackStack  → 发 Completed
    """
    import struct

    w = ByteWriter()
    w.write_unsigned_varint(PacketID.RESOURCE_PACK_CLIENT_RESPONSE)

    if netease_mode:
        # ── 网易 860 格式（fatalder 确认）──
        # 枚举从 1 开始：1=Refused, 2=SendPacks, 3=AllPacksDownloaded, 4=Completed
        NETEASE_STATUS = {0: 1, 1: 2, 2: 3, 3: 4}  # 我们的 status → 网易 status
        ne_status = NETEASE_STATUS.get(status)
        if ne_status is None:
            raise ValueError(f"未知 ResourcePackResponse 状态: {status}")
        w.write_byte(ne_status)  # u8 response（不是 varuint！）
        # pack_count: uint16 LE（只有 SendPacks 才有 pack 列表，这里总是 0）
        w.write(struct.pack("<H", 0))
    else:
        # ── vanilla 2168 格式 ──
        RESPONSE_NAMES = {
            0: "cancel",
            1: "downloading",
            2: "downloadingfinished",
            3: "resourcepackstackfinished",
        }
        name = RESPONSE_NAMES.get(status, "")
        if not name:
            raise ValueError(f"未知 ResourcePackResponse 状态: {status}")
        w.write_unsigned_varint(status)  # varuint32
        w.write_string(name)  # string response_name
        # 只有 SendPacks(1) 才写 pack 列表；其它值不写任何额外字段

    return w.to_bytes()


def build_request_chunk_radius(
    radius: int = 4, max_radius: int = 64, netease_mode: bool = False,
    *, protocol: int = 860,
) -> bytes:
    """RequestChunkRadius。

    网易 686 对齐本地 Fatalder request_chunk_radius.go：radius/max_radius 均为 VarInt32。
    860 和 vanilla 2168：
        varuint(69) | varint32(radius) | u8(max_radius)

    ⚠️ MaxChunkRadius 是 u8（无符号字节），不是 varint32！
    CloudburstMC/AllayMC Protocol v582 serializer 确认：
        buffer.writeByte(packet.getMaxRadius())  // u8
    网易 v860_NetEase codec 继承 vanilla v860，未覆盖此 serializer。

    之前误用 fatalder 的 varint32 格式（fatalder request_chunk_radius.go 的
    MaxChunkRadius 是 int32），但 fatalder 实发 maxRadius=0（varint32 和 u8
    编码都是 0x00），所以没暴露。我们发 maxRadius=64 时 varint32=0x80 0x01
    （2 字节），服务端按 u8 读 0x80=128 后多出 1 字节 → PacketViolationWarning
    "BinaryStream read() incomplete"。
    """
    w = ByteWriter()
    w.write_unsigned_varint(PacketID.REQUEST_CHUNK_RADIUS)
    w.write_varint(radius)        # varint32（有符号，ZigZag LEB128）
    if netease_mode and protocol == 686:
        w.write_varint(max_radius)  # Fatalder 的协议 686 分支
    else:
        w.write_byte(max_radius)    # 保留 860/vanilla 的 u8 编码
    return w.to_bytes()


def build_set_local_player_as_initialised(runtime_id: int) -> bytes:
    """SetLocalPlayerAsInitialised: varuint(113) | varuint64(runtime_id)。"""
    w = ByteWriter()
    w.write_unsigned_varint(PacketID.SET_LOCAL_PLAYER_AS_INITIALISED)
    w.write_unsigned_varlong(runtime_id)  # varuint64（ActorRuntimeID，用 varlong 写）
    return w.to_bytes()


@dataclass
class StartGameData:
    """StartGame 包解析结果（只提取关键字段）。"""
    entity_unique_id: int
    entity_runtime_id: int  # 放方块/命令需要
    player_game_type: int
    level_name: str


def parse_start_game(data: bytes) -> StartGameData:
    """解析 StartGame 包，提取 EntityRuntimeID 等关键字段。

    StartGame 包前几个字段（Gophertunnel start_game.go Marshal 确认）：
        varuint(11) | ActorUniqueID(varint64) | ActorRuntimeID(varuint64)
        | varint32(PlayerGameMode) | Vec3(PlayerPosition) | ...

    ⚠️ 关键类型（Gophertunnel writer.go）：
        - ActorUniqueID = Varint64（ZigZag 有符号 varint）
        - ActorRuntimeID = Varuint64（无符号 varint）← 之前误用 read_varlong 导致 -1
        - PlayerGameMode = Varint32（ZigZag 有符号）
    """
    r = ByteReader(data)
    r.read_unsigned_varint()  # 包头
    entity_unique_id = r.read_varlong()            # varint64（有符号）
    entity_runtime_id = r.read_unsigned_varlong()  # varuint64（无符号！）
    player_game_type = r.read_varint()             # varint32（有符号）
    # 后面还有 Vec3(PlayerPosition) 等几十个字段，不解析
    return StartGameData(
        entity_unique_id=entity_unique_id,
        entity_runtime_id=entity_runtime_id,
        player_game_type=player_game_type,
        level_name="",  # 深度解析时再提取
    )


# ═══════════════════════════════════════════════════════════════════════════════
# CommandRequest：发命令（如 /setblock）
# ═══════════════════════════════════════════════════════════════════════════════
def build_command_request(
    command: str,
    request_id: str,
    player_runtime_id: int,
    entity_unique_id: int = 0,
    origin: int = 0,  # 0=Player
    netease_mode: bool = False,
) -> bytes:
    """构建 CommandRequest 包。

    格式取决于协议版本：

    **vanilla BDS 1.26.40（protocol 2168，Gophertunnel master 格式）**：
        varuint(77) | string(command)
        | string(origin_name) | UUID(16) | string(request_id) | i64_LE(player_unique_id)
        | bool(internal) | string(version)

        - Origin 是 string（"player"/"commandblock"/...）
        - PlayerUniqueID 是 i64 LE，**总是写**
        - Version 是 string

    **网易 860（fatalder command_request.go + command.go CommandOriginData）**：
        varuint(77) | string(command)
        | varuint(origin) | UUID(16) | string(request_id)
        | [varint64(player_unique_id)  仅 origin∈{3,4} 时写]
        | bool(internal) | varint32(version) | bool(unlimited)

        - Origin 是 varuint（枚举值，0=Player），**不是** string
        - PlayerUniqueID 是 varint64，且**条件写**（仅 DevConsole=3 / Test=4 才写）
        - Version 是 varint32（fatalder 实发 39），**不是** string
        - 多一个 UnLimited bool 字段（网易专有）

    fatalder packCommandRequest 实发值（game_interface/commands.go:49-61）：
        Origin=0(Player), RequestID="96045347-a6a3-4114-94c0-1bc4cc561694"（固定字符串）,
        Internal=false, Version=39, UnLimited=false

    ⚠️ 之前网易模式下错用 vanilla 格式 → 网易服务器按 fatalder 格式解析，
       字段全部错位 → 静默丢弃命令（无 CommandOutput 回包）。

    注意：命令不带 /，服务器自动识别（setblock 而非 /setblock）。
    """
    import uuid as _uuid
    w = ByteWriter()
    w.write_unsigned_varint(PacketID.COMMAND_REQUEST)  # 77
    w.write_string(command)                              # string: command line

    if netease_mode:
        # ── 网易 860 格式（fatalder command_request.go 确认）──
        w.write_unsigned_varint(origin)                  # varuint: origin 枚举值（0=Player）
        w.write_uuid(_uuid.uuid4().bytes)                # UUID: 16 bytes（Bedrock 格式，匹配 CommandOutput 用）
        # RequestID 是固定字符串（fatalder packCommandRequest 硬编码）
        w.write_string("96045347-a6a3-4114-94c0-1bc4cc561694")
        # PlayerUniqueID 仅 DevConsole(3)/Test(4) 才写（CommandOriginData 条件写）
        if origin in (3, 4):
            w.write_varint(entity_unique_id)            # varint64: player_unique_id
        w.write_bool(False)                              # internal
        w.write_varint(39)                               # varint32: version（fatalder 实发 39）
        w.write_bool(False)                              # UnLimited（网易专有字段）
    else:
        # ── vanilla 2168 格式 ──
        ORIGIN_NAMES = {
            0: "player",
            1: "commandblock",
            2: "minecartcommandblock",
            3: "devconsole",
            4: "test",
            5: "automationplayer",
            6: "clientautomation",
            7: "dedicatedserver",
            8: "entity",
            9: "virtual",
            10: "gameargument",
            11: "entityserver",
            12: "precompiled",
            13: "gamedirectorentityserver",
            14: "scripting",
            15: "executecontext",
        }
        origin_name = ORIGIN_NAMES.get(origin, "unknown")
        w.write_string(origin_name)                      # string: origin name
        w.write_uuid(_uuid.uuid4().bytes)                # UUID: 16 bytes
        w.write_string(request_id)                       # string: request_id
        w.write_i64(entity_unique_id)                    # i64 LE: player_unique_id（总是写）
        w.write_bool(False)                              # internal
        w.write_string("")                               # version: string
    return w.to_bytes()
