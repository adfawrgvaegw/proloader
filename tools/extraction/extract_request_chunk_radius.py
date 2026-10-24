#!/usr/bin/env python3
"""从抓包中提取所有 C→S 方向的 Bedrock 包，重点对比 RequestChunkRadius 格式。

复用 extract_game_version.py 的解析逻辑，但打印所有包的 packet_id + hex，
用于对比 proloader 发的包和真实客户端发的包。
"""
import struct
import sys
from collections import defaultdict
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from proloader.net.bedrock.batch import NeteaseDecompressor
from proloader.net.raknet.frames import FrameSet


def read_pcap(path):
    data = path.read_bytes()
    magic = struct.unpack("<I", data[:4])[0]
    endian = "<" if magic == 0xa1b2c3d4 else ">"
    link_type = struct.unpack(endian + "I", data[20:24])[0]
    packets = []
    offset = 24
    while offset + 16 <= len(data):
        _, _, incl_len, _ = struct.unpack(endian + "IIII", data[offset:offset + 16])
        offset += 16
        pkt = data[offset:offset + incl_len]
        offset += incl_len
        if link_type == 1 and len(pkt) >= 14:
            pkt = pkt[14:]
        if len(pkt) < 20 or (pkt[0] >> 4) != 4 or pkt[9] != 17:
            continue
        ihl = (pkt[0] & 0x0f) * 4
        udp = pkt[ihl:]
        if len(udp) < 8:
            continue
        src_port, dst_port, _, _ = struct.unpack(">HHHH", udp[:8])
        packets.append((src_port, dst_port, udp[8:]))
    return packets


def read_varuint(data, offset):
    result = 0
    shift = 0
    while offset < len(data):
        b = data[offset]
        offset += 1
        result |= (b & 0x7f) << shift
        shift += 7
        if not (b & 0x80):
            break
    return result, offset


def process_batch_body(body, netease_decomp):
    """处理一个完整的 batch body（0xfe + data），返回内部 packet 列表。"""
    if not body or body[0] != 0xfe:
        return []
    payload = body[1:]
    if not payload:
        return []
    decompressed = None
    if netease_decomp is not None:
        try:
            out = netease_decomp.decompress(payload)
            if out and len(out) > 2:
                decompressed = out
        except Exception:
            pass
    if decompressed is None:
        # 无压缩回退：尝试当原始 payload 解析（RequestNetworkSettings 阶段）
        try:
            pkt_len, off = read_varuint(payload, 0)
            if 0 < pkt_len <= len(payload) - off:
                decompressed = payload
        except Exception:
            pass
    if decompressed is None:
        return []
    packets = []
    off = 0
    while off < len(decompressed):
        pkt_len, off2 = read_varuint(decompressed, off)
        if pkt_len == 0 or off2 + pkt_len > len(decompressed):
            break
        pkt_data = decompressed[off2:off2 + pkt_len]
        off = off2 + pkt_len
        if pkt_data:
            packets.append(pkt_data)
    return packets


# 网易 860 包 ID 名称（基于 fatalder id.go + 抓包验证）
PID_NAMES = {
    1: "Login", 2: "PlayStatus", 3: "ServerToClientHandshake", 4: "ClientToServerHandshake",
    5: "Disconnect", 6: "ResourcePacksInfo", 7: "ResourcePackStack", 8: "ResourcePackClientResponse",
    9: "Text", 10: "SetTime", 11: "StartGame", 39: "SetActorData", 43: "SetSpawnPosition",
    45: "Respawn", 49: "InventoryContent", 50: "InventorySlot", 52: "CraftingData",
    58: "LevelChunk", 63: "PlayerList", 72: "GameRulesChanged", 76: "AvailableCommands",
    77: "CommandRequest", 79: "CommandOutput", 113: "SetLocalPlayerAsInitialised",
    119: "PlayerAuthInput", 121: "NetworkChunkPublisherUpdate", 124: "StructureTemplateDataRequest",
    129: "ClientCacheStatus", 143: "NetworkSettings", 156: "PacketViolationWarning",
    160: "ClientCacheBlobStatus", 162: "ItemRegistry", 165: "BiomeDefinitionList",
    187: "UpdateAttributes", 188: "InventoryTransaction", 193: "RequestNetworkSettings",
    198: "LevelEvent", 200: "PyRpc", 203: "NeteaseJson", 208: "RequestAbility",
    210: "RequestPermissions", 230: "UpdatePlayerGameType", 236: "AddVolumeEntity",
    302: "PlayerEnchantOptions", 307: "ChunkRadiusUpdated", 313: "BiomeDefinitionList",
    314: "BiomeDefinitionList", 320: "ItemRegistry",
}


def pid_name(pid):
    return PID_NAMES.get(pid, f"Unknown({pid})")


def main():
    pcap = Path("cache/captures/netease_game.pcap")
    if not pcap.exists():
        print(f"❌ pcap 不存在: {pcap}")
        return
    packets = read_pcap(pcap)
    print(f"pcap: {pcap.name}  总包数: {len(packets)}")
    print("=" * 70)

    netease_active = False
    decomp_cs = NeteaseDecompressor()
    decomp_sc = NeteaseDecompressor()
    split_groups = defaultdict(dict)
    split_counts = {}

    cs_packets = []  # (pkt_idx, pid, pkt_data)

    for i, (src_port, dst_port, payload) in enumerate(packets):
        if not payload or (payload[0] & 0xf0) != 0x80:
            continue
        direction = "C→S" if dst_port == 12895 or dst_port == 19132 else "S→C"
        # 严格按端口判断：客户端端口是高频随机端口，服务器端口固定
        # 从抓包看，C→S 的 dst_port 是 12895
        try:
            fs = FrameSet.decode(payload)
        except Exception:
            continue

        for frame in fs.frames:
            body = frame.body
            if not body:
                continue

            # 分片重组（必须在 0xfe 检查之前！分片帧的 body[0] 不是 0xfe）
            if frame.is_split:
                key = (direction, frame.split_id)
                split_groups[key][frame.split_index] = body
                split_counts[key] = frame.split_count
                if len(split_groups[key]) == frame.split_count:
                    assembled = b"".join(
                        split_groups[key][idx] for idx in range(frame.split_count)
                    )
                    del split_groups[key]
                    del split_counts[key]
                    body = assembled
                else:
                    continue

            # 重组后或非分片帧：检查是否是 batch
            if body[0] != 0xfe:
                continue

            # NetworkSettings 后启用网易解压
            nd = (decomp_cs if direction == "C→S" else decomp_sc) if netease_active else None
            batch_pkts = process_batch_body(body, nd)
            for pkt_data in batch_pkts:
                pid, _ = read_varuint(pkt_data, 0)
                if direction == "C→S":
                    cs_packets.append((i, pid, pkt_data))
                # 检测 NetworkSettings 启用压缩
                if pid == 143:  # NetworkSettings
                    netease_active = True

    # 打印所有 C→S 包
    print(f"\n{'=' * 70}")
    print(f"C→S 方向共 {len(cs_packets)} 个 Bedrock 包")
    print(f"{'=' * 70}")
    for idx, (pkt_idx, pid, pkt_data) in enumerate(cs_packets):
        name = pid_name(pid)
        hex_str = pkt_data.hex()
        if len(hex_str) > 120:
            hex_str = hex_str[:120] + "..."
        print(f"[{idx+1:3d}] 包#{pkt_idx:4d} id={pid:3d} {name:30s} [{len(pkt_data):5d}B] {hex_str}")

    # 重点找 RequestChunkRadius（网易 860 可能不是 69）
    print(f"\n{'=' * 70}")
    print("重点分析：RequestChunkRadius / ChunkRadiusUpdated 相关")
    print(f"{'=' * 70}")
    # C→S 方向，找可能的 RequestChunkRadius（通常在 ResourcePackClientResponse 之后，SetLocalPlayerAsInitialised 附近）
    for idx, (pkt_idx, pid, pkt_data) in enumerate(cs_packets):
        # 打印所有 pid 在 60-80 之间的包（RequestChunkRadius 可能的 ID 范围）
        if 60 <= pid <= 80 or pid == 69:
            print(f"\n[C→S #{idx+1}] id={pid} {pid_name(pid)} [{len(pkt_data)}B]")
            print(f"  hex: {pkt_data.hex()}")
            # 尝试按 RequestChunkRadius 格式解析：varuint(pid) + varint(radius) + varint(max_radius)
            try:
                off = 0
                p, off = read_varuint(pkt_data, off)
                # varint ZigZag 解码
                def read_varint(data, off):
                    raw, off = read_varuint(data, off)
                    return (raw >> 1) ^ -(raw & 1), off
                r1, off = read_varint(pkt_data, off)
                r2, off = read_varint(pkt_data, off)
                print(f"  按 RequestChunkRadius 解析: pid={p} radius={r1} max_radius={r2} (剩余 {len(pkt_data)-off}B)")
            except Exception as e:
                print(f"  解析失败: {e}")


if __name__ == "__main__":
    main()
