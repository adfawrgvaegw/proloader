"""常量定义：空气方块、RakNet Magic、方块名前缀等。

学习要点：
    - 基岩版方块用 "name" + "states" 唯一标识，不用数值 ID（数值 ID 每版本变）
    - 网易方块用 "netease:" 前缀（部分自定义方块），标准方块仍是 "minecraft:"
    - RakNet Magic 是基岩版 UDP 握手的固定字节序列
"""

from __future__ import annotations

# ── 方块名前缀 ────────────────────────────────────────────────────────────────
MINECRAFT_PREFIX = "minecraft:"
NETEASE_PREFIX = "netease:"

# ── RakNet 协议常量（参考 wiki.bedrock.dev/servers/raknet）───────────────────
# 离线消息 Magic：所有 Unconnected Ping/Pong 等握手包都带这 16 字节
RAKNET_MAGIC = bytes(
    [0x00, 0xFF, 0xFF, 0x00, 0xFE, 0xFE, 0xFE, 0xFE, 0xFD, 0xFD, 0xFD, 0xFD, 0x12, 0x34, 0x56, 0x78]
)
RAKNET_DEFAULT_PORT = 19132  # 基岩版默认 UDP 端口

# ── 协议版本（网易 NEMC v1.21.90 对应的基岩版协议版本，参考 bedrock-world-operator）──
# 真实对接时需要跟进版本号，MVP 阶段仅作占位
NETEASE_GAME_VERSION = "1.21.90"
BEDROCK_PROTOCOL_VERSION = 712  # 基岩版 1.21.90 协议版本号

# ── 基岩版世界尺寸常量 ────────────────────────────────────────────────────────
CHUNK_SIZE = 16  # 一个 chunk 是 16×16 方块
SUBCHUNK_SIZE = 16  # Y 方向每 16 格一个 sub-chunk
WORLD_MIN_Y = -64  # 1.18+ 世界最低 Y
WORLD_MAX_Y = 319  # 1.18+ 世界最高 Y

# ── 放置相关限制（真实后端用）──────────────────────────────────────────────────
# 单个 GamePacket 帧最多打包的 UpdateBlock 数量（协议实践值）
MAX_BLOCKS_PER_PACKET = 256
# 单假人加载范围（基岩版约 4 chunk = 64 格半径）
BOT_LOAD_RADIUS = 64

# ── 常用方块名 ────────────────────────────────────────────────────────────────
AIR_NAME = "minecraft:air"
STONE_NAME = "minecraft:stone"
