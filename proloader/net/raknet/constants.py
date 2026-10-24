"""RakNet 协议常量。

数据来源：https://wiki.bedrock.dev/servers/raknet
这些常量是协议级的、不会变的，跟 Bedrock 版本无关。

学习要点：
    - MAGIC 是 RakNet 离线消息的"魔数"，所有 Unconnected 包都带它
      值固定为 0x00ffff00fefefefefdfdfdfd12345678（16 字节）
    - 包 ID（第一个字节）区分消息类型：离线握手 / 在线帧 / ACK / NACK
    - 可靠性等级决定传输保证（Reliable Ordered 最高，用于游戏包）
"""

from __future__ import annotations

# ── 离线消息魔数 ──────────────────────────────────────────────────────────────
# 所有 Unconnected 消息（Ping/Pong/OCR1/2）都带这 16 字节，用于识别 RakNet 流量
MAGIC: bytes = bytes.fromhex("00ffff00fefefefefdfdfdfd12345678")

# 默认端口
DEFAULT_PORT: int = 19132


# ── 离线包 ID（Unconnected，握手前）───────────────────────────────────────────
class OfflinePacketID:
    """离线阶段包 ID（握手前，不加密，带 MAGIC）。"""

    UNCONNECTED_PING = 0x01
    UNCONNECTED_PING_OPEN_CONNECTIONS = 0x02
    UNCONNECTED_PONG = 0x1C
    OPEN_CONNECTION_REQUEST_1 = 0x05
    OPEN_CONNECTION_REPLY_1 = 0x06
    OPEN_CONNECTION_REQUEST_2 = 0x07
    OPEN_CONNECTION_REPLY_2 = 0x08


# ── 在线帧包 ID（Connected，握手后）─────────────────────────────────────────
class FramePacketID:
    """在线阶段帧包 ID（握手后，封装游戏包）。

    0x80-0x8D 是 Frame Set Packet（带不同 flag 的帧集合）。
    """

    FRAME_SET = 0x80  # 普通 Frame Set（最常见）
    FRAME_SET_SPLIT = 0x8A  # 带分片 flag
    ACK = 0xC0  # 确认（告知对方哪些帧收到了）
    NACK = 0xA0  # 否认（告知对方哪些帧没收到，要求重传）
    CONNECTION_REQUEST = 0x09
    CONNECTION_REQUEST_ACCEPTED = 0x10
    NEW_INCOMING_CONNECTION = 0x13
    CONNECTED_PING = 0x00
    CONNECTED_PONG = 0x03
    DISCONNECTION_NOTIFICATION = 0x15


# ── 可靠性等级 ────────────────────────────────────────────────────────────────
class Reliability:
    """帧可靠性等级（影响是否需要 ACK / 是否保序）。

    - UNRELIABLE: 不保证送达，不保序（如位置同步，丢一帧无所谓）
    - UNRELIABLE_SEQUENCED: 不保证送达，但保序（旧帧会被丢弃）
    - RELIABLE: 保证送达，不保序
    - RELIABLE_ORDERED: 保证送达且保序（游戏包常用）
    - RELIABLE_SEQUENCED: 保证送达，保序，旧帧丢弃
    """

    UNRELIABLE = 0
    UNRELIABLE_SEQUENCED = 1
    RELIABLE = 2
    RELIABLE_ORDERED = 3
    RELIABLE_SEQUENCED = 4
    UNRELIABLE_WITH_ACK_RECEIPT = 5
    RELIABLE_WITH_ACK_RECEIPT = 6
    RELIABLE_ORDERED_WITH_ACK_RECEIPT = 7


# 可靠性判断函数（对应 RakNet 帧字段是否存在）
# 帧字段规则（Gophertunnel/go-raknet）：
#   - reliable（有 frame_index）: 2,3,4,6,7  即除 0,1,5 外都可靠
#   - sequenced（有 sequence_index）: 1,4    丢弃旧帧
#   - order_index + order_channel 线格式字段: 1,3,4,7（sequenced也携带）
#   - ordered消息排队语义: 3,7
# 注意：4=RELIABLE_SEQUENCED 同时是 reliable + sequenced（有 frame_index + sequence_index）
#       1=UNRELIABLE_SEQUENCED 无 frame_index，但有 sequence_index 和 order字段
def is_reliable(reliability: int) -> bool:
    """该可靠性等级是否可靠（有 frame_index，需要 ACK）。"""
    return reliability in (
        Reliability.RELIABLE,
        Reliability.RELIABLE_ORDERED,
        Reliability.RELIABLE_SEQUENCED,
        Reliability.RELIABLE_WITH_ACK_RECEIPT,
        Reliability.RELIABLE_ORDERED_WITH_ACK_RECEIPT,
    )


def is_ordered(reliability: int) -> bool:
    """该可靠性等级是否为ordered队列消息（sequenced的线格式也含order字段）。"""
    return reliability in (
        Reliability.RELIABLE_ORDERED,
        Reliability.RELIABLE_ORDERED_WITH_ACK_RECEIPT,
    )


def is_sequenced(reliability: int) -> bool:
    """该可靠性等级是否 sequenced（有 sequence_index，旧帧丢弃）。"""
    return reliability in (
        Reliability.UNRELIABLE_SEQUENCED,
        Reliability.RELIABLE_SEQUENCED,
    )
