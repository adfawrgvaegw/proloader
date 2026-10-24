"""网易 Bedrock 专有包构建（PyRpc / NeteaseJson / ClientCacheStatus）。

对齐 fatalder `modules/FunShuttler/core/minecraft/protocol/packet/` 中的包定义。

包格式（fatalder writer.go 确认，全小端）：
    PyRpc:           varuint(200) | varuint(msgpack_len) | msgpack_bytes | uint32_LE(op_type)
    NeteaseJson:     varuint(203) | varuint(data_len) | data_bytes
    ClientCacheStatus: varuint(129) | bool(1 byte)

PyRpc 的 Value 用 msgpack 编码（对齐 fatalder ugorji/codec MsgpackHandle）。
fatalder py_rpc.Marshal 输出 [name, payload, nil] 三元数组。
"""

from __future__ import annotations

import json
import struct
from typing import Any, List, Optional

import msgpack

from proloader.net.bedrock.packets import PacketID
from proloader.net.raknet.wire import ByteWriter

# PyRpc 操作类型（fatalder py_rpc.go 确认）
PYRPC_OP_SEND: int = 0x05DB23AE  # 客户端→服务端
PYRPC_OP_RECV: int = 0x0094D408  # 服务端→客户端


def build_client_cache_status(enabled: bool = False) -> bytes:
    """构建 ClientCacheStatus 包。

    格式：varuint(129) | bool(1 byte)
    """
    w = ByteWriter()
    w.write_unsigned_varint(PacketID.CLIENT_CACHE_STATUS)
    w.write_bool(enabled)
    return w.to_bytes()


def build_netease_json(data: str) -> bytes:
    """构建 NeteaseJson 包。

    格式：varuint(203) | varuint(data_len) | data_bytes

    Args:
        data: JSON 字符串（如 '{"eventName":"LOGIN_UID","resid":"","uid":"123"}'）
    """
    data_bytes = data.encode("utf-8")
    w = ByteWriter()
    w.write_unsigned_varint(PacketID.NETEASE_JSON)
    w.write_unsigned_varint(len(data_bytes))
    w.write(data_bytes)
    return w.to_bytes()


def build_netease_login_uid(uid: int) -> bytes:
    """构建 LOGIN_UID 的 NeteaseJson 包（对齐 fatalder open_connection.go:74）。

    内容：{"eventName":"LOGIN_UID","resid":"","uid":"<uid>"}
    """
    data = json.dumps(
        {"eventName": "LOGIN_UID", "resid": "", "uid": str(uid)},
        separators=(",", ":"),
    )
    return build_netease_json(data)


def build_py_rpc(value: List[Any], operation_type: int = PYRPC_OP_SEND) -> bytes:
    """构建 PyRpc 包。

    格式：varuint(200) | varuint(msgpack_len) | msgpack_bytes | uint32_LE(op_type)

    Args:
        value: PyRpc 值列表，对齐 fatalder py_rpc.Marshal 的 [name, payload, nil] 三元数组
        operation_type: 操作类型，默认 PYRPC_OP_SEND (0x05db23ae)
    """
    msgpack_bytes = msgpack.packb(value, use_bin_type=True)
    w = ByteWriter()
    w.write_unsigned_varint(PacketID.PYRPC)
    w.write_unsigned_varint(len(msgpack_bytes))
    w.write(msgpack_bytes)
    w.write(struct.pack("<I", operation_type))  # uint32 小端
    return w.to_bytes()


# ═══════════════════════════════════════════════════════════════════════════════
# PyRpc 包构建辅助（对齐 fatalder open_connection.go 中的 7 个 PyRpc 包）
# ═══════════════════════════════════════════════════════════════════════════════
def build_py_rpc_e() -> bytes:
    """PyRpc "e" 包（DomainGame need，fatalder open_connection.go:67-72）。

    内容：["e", [], nil]
    """
    return build_py_rpc(["e", [], None])


def build_py_rpc_sync_using_mod(
    mod_uuids: Optional[List[str]] = None,
    skin_id: str = "",
    bot_component: Optional[dict] = None,
) -> bytes:
    """PyRpc SyncUsingMod 包（fatalder open_connection.go:75-88）。

    内容：["SyncUsingMod", [modUUIDs, skinID, "", true, botComponent], nil]
    """
    return build_py_rpc([
        "SyncUsingMod",
        [mod_uuids or [], skin_id, "", True, bot_component or {}],
        None,
    ])


def build_py_rpc_sync_vip_skin_uuid() -> bytes:
    """PyRpc SyncVipSkinUuid 包（fatalder open_connection.go:90-93）。

    内容：["SyncVipSkinUuid", [nil], nil]
    """
    return build_py_rpc(["SyncVipSkinUuid", [None], None])


def build_py_rpc_client_load_addons() -> bytes:
    """PyRpc ClientLoadAddonsFinishedFromGac 包（fatalder open_connection.go:95-98）。

    内容：["ClientLoadAddonsFinishedFromGac", [], nil]
    """
    return build_py_rpc(["ClientLoadAddonsFinishedFromGac", [], None])


def build_py_rpc_get_loaded_instances(runtime_id: str) -> bytes:
    """PyRpc ModEventC2S GetLoadedInstances（fatalder open_connection.go:100-108）。

    内容：["ModEventC2S", ["Minecraft", "Preset", "GetLoadedInstances", {"playerId": runtime_id}], nil]
    """
    return build_py_rpc([
        "ModEventC2S",
        ["Minecraft", "Preset", "GetLoadedInstances", {"playerId": runtime_id}],
        None,
    ])


def build_py_rpc_arena_finish_load() -> bytes:
    """PyRpc arenaGamePlayerFinishLoad（fatalder open_connection.go:110-113）。

    内容：["arenaGamePlayerFinishLoad", [], nil]
    """
    return build_py_rpc(["arenaGamePlayerFinishLoad", [], None])


def build_py_rpc_player_ui_init(runtime_id: str) -> bytes:
    """PyRpc ModEventC2S PlayerUiInit（fatalder open_connection.go:115-123）。

    内容：["ModEventC2S", ["Minecraft", "VIPEventSystem", "PlayerUiInit", runtime_id], nil]

    注意：PlayerUiInit 的 MakeGo() 返回的是字符串（runtime_id），不是 map。
    """
    return build_py_rpc([
        "ModEventC2S",
        ["Minecraft", "VIPEventSystem", "PlayerUiInit", runtime_id],
        None,
    ])


def build_py_rpc_ai_execute_command(command_line: str, request_uuid: str) -> bytes:
    """PyRpc ModEventC2S AICommand ExecuteCommandEvent（★ 绕过 MCP 的魔法指令通道）。

    对齐 fatalder commands.go:SendSettingsCommand + ai_command/execute_command_event.go。

    这是网易的「魔法指令」通道：命令不走 CommandRequest 包，而是包在 PyRpc 的
    aiCommand.ExecuteCommandEvent 里发出去，因此绕过 MCP 反作弊校验（GetMCPCheckNum）。
    execute_command_event.go 注释明确：此操作不消耗魔法指令使用次数，无限制。

    响应通过 PyRpc ModEventS2C ExecuteCommandOutputEvent 回来（含 msg + uuid）。

    Args:
        command_line: 命令行（fatalder 会前缀 "execute run "，见 session.run_command_ai）
        request_uuid: 请求 UUID 字符串（用于匹配响应）
    """
    return build_py_rpc([
        "ModEventC2S",
        ["Minecraft", "aiCommand", "ExecuteCommandEvent",
         {"cmd": command_line, "uuid": request_uuid}],
        None,
    ])


def build_all_netease_login_packets(
    uid: int,
    entity_unique_id: int,
    skin_id: str = "",
    bot_components: Optional[dict[str, Optional[int]]] = None,
) -> List[bytes]:
    """构建登录后需要发送的全部网易特有包（对齐 fatalder open_connection.go:67-123）。

    顺序：
        1. ClientCacheStatus{false}
        2. NeteaseJson{LOGIN_UID, uid}
        3. PyRpc "e"
        4. PyRpc SyncUsingMod
        5. PyRpc SyncVipSkinUuid
        6. PyRpc ClientLoadAddonsFinishedFromGac
        7. PyRpc GetLoadedInstances
        8. PyRpc arenaGamePlayerFinishLoad
        9. PyRpc PlayerUiInit

    Args:
        uid: 玩家 UID（IdentityData.Uid）
        entity_unique_id: StartGame.EntityUniqueID（Fatalder 虽命名 runtimeid，实际用 unique ID）
        skin_id: 皮肤 ID（ClientData.SkinId）
    """
    # Fatalder 的 map[string]*int：null 也加入 modUUIDs，但不进入 outfit 映射。
    components = dict(bot_components) if bot_components is not None else {}
    if any(not isinstance(k, str) or not k or
           (v is not None and (type(v) is not int or not -(2**63) <= v < 2**63))
           for k, v in components.items()):
        raise ValueError("BotComponent 需要非空字符串键和 int64/null 值")
    runtime_id_str = str(entity_unique_id)
    return [
        build_client_cache_status(enabled=False),
        build_netease_login_uid(uid),
        build_py_rpc_e(),
        build_py_rpc_sync_using_mod(
            mod_uuids=list(components), skin_id=skin_id,
            bot_component={k: v for k, v in components.items() if v is not None},
        ),
        build_py_rpc_sync_vip_skin_uuid(),
        build_py_rpc_client_load_addons(),
        build_py_rpc_get_loaded_instances(runtime_id_str),
        build_py_rpc_arena_finish_load(),
        build_py_rpc_player_ui_init(runtime_id_str),
    ]


# ═══════════════════════════════════════════════════════════════════════════════
# PyRpc 包解析（用于接收服务端 PyRpc 包，MCP 反作弊等）
# ═══════════════════════════════════════════════════════════════════════════════
def parse_py_rpc(data: bytes) -> dict:
    """解析 PyRpc 包，返回 {name, payload, operation_type}。

    格式：varuint(200) | varuint(msgpack_len) | msgpack_bytes | uint32_LE(op_type)
    """
    from proloader.net.raknet.wire import ByteReader

    r = ByteReader(data)
    packet_id = r.read_unsigned_varint()
    if packet_id != PacketID.PYRPC:
        raise ValueError(f"不是 PyRpc 包: ID={packet_id}")
    msgpack_len = r.read_unsigned_varint()
    msgpack_bytes = r.read(msgpack_len)
    op_type = struct.unpack("<I", r.read(4))[0]

    value = msgpack.unpackb(msgpack_bytes, raw=False)
    if isinstance(value, list) and len(value) >= 2:
        return {
            "name": value[0],
            "payload": value[1],
            "operation_type": op_type,
            "raw_value": value,
        }
    return {"name": None, "payload": value, "operation_type": op_type, "raw_value": value}
