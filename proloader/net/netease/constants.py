"""网易 G79 客户端常量。

来源：g79client/auth.go + crypto.go + globals.go + const.go
"""

# ── 游戏版本 ──────────────────────────────────────────────
GAME_VERSION = "1.21.2"
ENGINE_VERSION = "3.4.5.273121"

# ── G79 哈希常量（PE 认证 message 组成部分）─────────────────
G79_LIBRARY_HASH = "815873bdd82cb7a4ce422da361f7d60d"
G79_SIGNATURE_HASH = "2b3e7ca013bb30a74d822579860c042b"

# iOS 平台常量（来自 g79client/engine_consts.go 注释中的第二组值 + 二进制分析）
# 注释显示同版本有两个不同 library_hash，815873... 是 Android，59cc563... 疑似 iOS
G79_IOS_LIBRARY_HASH = "59cc563ed3c6430aff44500cd3f9e525"
G79_IOS_SIGNATURE_HASH = "2b3e7ca013bb30a74d822579860c042b"  # signature_hash 跨平台不变
G79_IOS_MESSAGE_TAG = "ds_cloudgame"  # iOS pay_channel（Android 是 dashen_cloudgame）

# ── Android sauth payload 常量 ────────────────────────────
G79_ANDROID_MESSAGE_TAG = "dashen_cloudgame"
G79_ANDROID_STEP = "695616851"
G79_ANDROID_STEP2 = "2146985406"

# ── 动态 token 盐 ────────────────────────────────────────
DYNAMIC_TOKEN_SALT = "0eGsBkhl"

# ── MPay 游戏 ID ──────────────────────────────────────────
MPAY_GAME_ID = "aecfrxodyqaaaajp-g-x19"

# ── 服务器 URL（来自 release JSON）────────────────────────
AUTH_SERVER_URL = "https://g79authobt.minecraft.cn"        # JWT chain (/authentication-v2)
CORE_SERVER_URL = "https://g79obtapigtcoregray.minecraft.cn"  # PE 认证 (/pe-authentication)
WEB_SERVER_URL = "https://g79mclobt.minecraft.cn"           # 租赁服操作
API_GATEWAY_URL = "https://g79apigatewayobt.minecraft.cn"   # 游戏/大厅
LINK_SERVER_LIST_URL = "https://g79.update.netease.com/linkserver_obt.list"
RELEASE_JSON_URL = "https://g79.update.netease.com/serverlist/adr_release.0.17.json"
PACK_LIST_URL = "https://g79.update.netease.com/pack_list/production/g79_packlist_2"
PATCH_LIST_URL = "https://g79.update.netease.com/patch_list/production/g79_rn_patchlist"

# ── AES 密钥列表（G79HttpEncrypt/Decrypt 用）──────────────
# 16 个 hex 编码的 AES-128 密钥，由 flag 高 4 位索引
G79_HTTP_KEYS = [
    "60F1E0D1FD635362430747215CF1C2FF",
    "EA5B62D27D0338374852C4B9469D7AC6",
    "17238D55501C5F020B155FB3303591E6",
    "8C5CEAE0F443E006A050266F73ADD5B0",
    "1C02CE22FB22F0E72060217418F351F3",
    "9A01773FEBB0CFE0EBDBF37F4D23C27F",
    "43F32300BF252CC320E2572ACE766367",
    "07F161011B3101F1ED0301735631E734",
    "0454E7707A5F37565601E100406060AF",
    "647554BAD3100C43C16660F002CC10F3",
    "E157213170F842382032564265B0B043",
    "914FC59311B04151393EF6896A847636",
    "0710C0205D224237025323265C145FA1",
    "054E6F01165267025C3111F562A921E9",
    "722D1789E792E2CA0D5322211FD0F5AE",
    "91F7C751FCF671F34943430772341799",
]

# ── Link 连接硬编码 RSA 密钥 ──────────────────────────────
# 服务端 RSA-2048 公钥（PEM base64）
LINK_SERVER_PUBLIC_KEY_B64 = (
    "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEArht9ioo6tc3Z7On/"
    "0xZ5Q2q3lN2q5vR6wV7oV1TQzM5KQR0G3WZQ25U7Z6f8j4xQ1x/WKB2K5e1o"
    "8N5Vo9p2N3q5vR6wV7oV1TQzM5KQR0G3WZQ25U7Z6f8j4xQ1x/WKB2K5e1o"
    "placeholder_real_key_in_source"
)

# 客户端 RSA-2048 私钥（PEM base64）— 从 connection.go 提取
# 实际值需要从 g79client/service/link_connection/connection.go 读取

# ── ChaCha8 常量 ──────────────────────────────────────────
CHACHA8_NONCE = b"163 NetEase\n"

# ── Link 客户端公钥 MD5 ───────────────────────────────────
LINK_CLIENT_PUBKEY_MD5 = "3f17d36cec77e47f1d301c0cc41cd73f"

# ── GameStart 常量 ────────────────────────────────────────
GAME_START_TYPE_RENTAL = 10  # 租赁服 game_type

# ── netease_sid 后缀 ──────────────────────────────────────
NETEASE_SID_RENTAL = "RentalGame"
NETEASE_SID_DOMAIN = "DomainGame"
NETEASE_SID_LOBBY = "LobbyGame"
NETEASE_SID_NETWORK = "NetworkGame"

# ── HTTP 请求默认头 ───────────────────────────────────────
DEFAULT_USER_AGENT = "libhttpclient/1.0.0.0"
