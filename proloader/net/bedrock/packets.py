"""Bedrock 协议包 ID 常量 + GamePacket 头编解码。

包 ID 来源：Gophertunnel minecraft/protocol/packet/id.go（协议 2168 / MC 1.26.40）
⚠️ ID 用 Go iota 连续定义，中间有占位符，不能假设连续，必须查表。

GamePacket 头格式（每个包的第一个字段）：
    varuint32，低 10 位是 PacketID，bit10-11 是 SenderSubClient，bit12-13 是 TargetSubClient
    单客户端时 sub client 都是 0，头就是纯 PacketID。
"""

from __future__ import annotations

from proloader.net.raknet.wire import ByteReader, ByteWriter


# Batch 包头（多个 GamePacket 打包发送的标志字节）
ID_BATCH: int = 0xFE


class PacketID:
    """Bedrock 包 ID（协议 2168）。"""

    LOGIN = 1
    PLAY_STATUS = 2
    SERVER_TO_CLIENT_HANDSHAKE = 3
    CLIENT_TO_SERVER_HANDSHAKE = 4
    DISCONNECT = 5
    RESOURCE_PACKS_INFO = 6
    RESOURCE_PACK_STACK = 7
    RESOURCE_PACK_CLIENT_RESPONSE = 8
    TEXT = 9
    SET_TIME = 10
    START_GAME = 11
    ADD_PLAYER = 12
    ADD_ACTOR = 13
    REMOVE_ACTOR = 14
    ADD_ITEM_ACTOR = 15
    TAKE_OUT_ACTOR = 16
    MOVE_ACTOR_ABSOLUTE = 18
    MOVE_PLAYER = 19
    RIDER_JUMP = 20
    UPDATE_BLOCK = 21
    ADD_PAINTING = 22
    TICK_SYNC = 23
    LEVEL_SOUND_EVENT_V1 = 24
    LEVEL_EVENT = 25
    BLOCK_EVENT = 26
    ACTOR_EVENT = 27
    MOB_EFFECT = 28
    UPDATE_ATTRIBUTES = 29
    INVENTORY_TRANSACTION = 30
    MOB_ARMOR_EQUIPMENT = 31
    MOB_EQUIPMENT = 32
    INTERACT = 33
    BLOCK_PICK_REQUEST = 34
    ACTOR_PICK_REQUEST = 35
    PLAYER_ACTION = 36
    HURT_ARMOR = 38
    SET_ACTOR_DATA = 39
    SET_ACTOR_MOTION = 40
    SET_ACTOR_LINK = 41
    SET_HEALTH = 42
    SET_SPAWN_POSITION = 43
    ANIMATE = 44
    RESPAWN = 45
    CONTAINER_OPEN = 46
    CONTAINER_CLOSE = 47
    PLAYER_HOTBAR = 48
    INVENTORY_CONTENT = 49
    INVENTORY_SLOT = 50
    CONTAINER_SET_DATA = 51
    CRAFTING_DATA = 52
    CRAFTING_EVENT = 53
    GUI_DATA_PICK_ITEM = 54
    ADVENTURE_SETTINGS = 55
    BLOCK_ENTITY_DATA = 56
    PLAYER_INPUT = 57
    LEVEL_CHUNK = 58
    SET_COMMANDS_ENABLED = 59
    SET_DIFFICULTY = 60
    CHANGE_DIMENSION = 61
    SET_PLAYER_GAME_TYPE = 62
    PLAYER_LIST = 63
    SIMPLE_EVENT = 64
    EVENT = 65
    SPAWN_EXPERIENCE_ORB = 66
    CLIENTBOUND_MAP_ITEM_DATA = 67
    MAP_INFO_REQUEST = 68
    REQUEST_CHUNK_RADIUS = 69
    CHUNK_RADIUS_UPDATED = 70
    ITEM_FRAME_SET_ITEM = 71
    GAME_RULES_CHANGED = 72
    CAMERA = 73
    BOSS_EVENT = 74
    SHOW_CREDITS = 75
    AVAILABLE_COMMANDS = 76
    COMMAND_REQUEST = 77
    COMMAND_BLOCK_UPDATE = 78
    SUB_CHUNK = 174
    SUB_CHUNK_REQUEST = 175
    COMMAND_OUTPUT = 79
    UPDATE_TRADE = 80
    UPDATE_EQUIP = 81
    RESOURCE_PACK_DATA_INFO = 82
    RESOURCE_PACK_CHUNK_DATA = 83
    RESOURCE_PACK_CHUNK_REQUEST = 84
    TRANSFER = 85
    PLAY_SOUND = 86
    STOP_SOUND = 87
    SET_TITLE = 88
    ADD_VOLUME = 89
    REMOVE_VOLUME = 91
    SCRIPT_CUSTOM_EVENT = 93
    SPAWN_PARTICLE_EFFECT = 94
    AVAILABLE_ACTOR_IDENTIFIERS = 95
    LEVEL_SOUND_EVENT_V2 = 96
    NETWORK_CHUNK_PUBLISHER_UPDATE = 97
    BIOME_DEFINITION_LIST = 98
    SOUND = 99
    SET_LAST_HURT_BY = 100
    STRUCTURE_TEMPLATE_DATA_EXPORT_REQUEST = 101
    STRUCTURE_TEMPLATE_DATA_EXPORT_RESPONSE = 102
    UPDATE_BLOCK_SYNCED = 103
    ENTITY_FALL = 104
    AVAILABLE_BIOMES = 110
    NETWORK_SETTINGS = 143
    REQUEST_NETWORK_SETTINGS = 193
    SET_LOCAL_PLAYER_AS_INITIALISED = 113
    ADD_VOLUME = 112

    # ── 网易专有包 ID（fatalder id.go 确认，686 协议）──
    CLIENT_CACHE_STATUS = 129   # vanilla 也有但 ID 相同
    PYRPC = 200                 # fatalder IDPyRpc
    NETEASE_JSON = 203          # fatalder IDNeteaseJson


# PlayStatus 状态码
class PlayStatus:
    LOGIN_SUCCESS = 0
    LOGIN_FAILED_CLIENT = 1
    LOGIN_FAILED_SERVER = 2
    PLAYER_SPAWN = 3
    LOGIN_FAILED_INVALID_TENANT = 4


# 压缩算法（Batch 包 compression 字段，u8；NetworkSettings 协商用 0/1/2）
# fatalder network_settings.go: Flate=0, Snappy=1, NetEase=2, None=0xffff
class CompressionAlgorithm:
    FLATE = 0    # zlib/deflate（BDS 默认）
    SNAPPY = 1   # snappy
    NETEASE = 2  # 网易定制 Flate（无 compression byte + 无 final block + 滑动窗口）
    NONE = 0xFF  # 不压缩（Batch 包专用，单字节 255）


# CommandOrigin 类型（编码为字符串，不是数字）
COMMAND_ORIGIN_STR = {
    0: "player",
    1: "commandblock",
    3: "devconsole",
    7: "dedicatedserver",
    11: "entityserver",
}


# ═══════════════════════════════════════════════════════════════════════════════
# GamePacket 头编解码
# ═══════════════════════════════════════════════════════════════════════════════
def write_packet_header(w: ByteWriter, packet_id: int, sender_sub: int = 0, target_sub: int = 0) -> None:
    """写包头：varuint32，低10位=PacketID，bit10-11=SenderSub，bit12-13=TargetSub。"""
    value = (packet_id & 0x3FF) | ((sender_sub & 0x3) << 10) | ((target_sub & 0x3) << 12)
    w.write_unsigned_varint(value)


def read_packet_header(r: ByteReader) -> tuple:
    """读包头，返回 (packet_id, sender_sub, target_sub)。"""
    value = r.read_unsigned_varint()
    packet_id = value & 0x3FF
    sender_sub = (value >> 10) & 0x3
    target_sub = (value >> 12) & 0x3
    return packet_id, sender_sub, target_sub


def wrap_packet(packet_id: int, payload: bytes) -> bytes:
    """把单个包的 payload 包装成带头的完整包（用于放进 batch）。"""
    w = ByteWriter()
    write_packet_header(w, packet_id)
    w.write(payload)
    return w.to_bytes()


def unwrap_packet(data: bytes) -> tuple:
    """从带头的包数据解析出 (packet_id, payload)。"""
    r = ByteReader(data)
    packet_id, _, _ = read_packet_header(r)
    return packet_id, r.read(r.remaining)
