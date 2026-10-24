#!/usr/bin/env python3
"""统计网易游戏流量的包类型分布，提取关键 Bedrock 包。

用法: python3 -m tools.analyze_netease_summary <pcap文件>
"""
import struct
import sys
import zlib
from collections import Counter
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from proloader.net.raknet.frames import FrameSet
from proloader.net.raknet.constants import Reliability

REL_NAMES = {
    0: "Unreliable", 1: "UnreliableSequenced", 2: "Reliable",
    3: "ReliableOrdered", 4: "ReliableSequenced",
    5: "UnreliableWithAck", 6: "ReliableWithAck", 7: "ReliableOrderedWithAck",
}

OFFLINE_IDS = {
    0x01: "UnconnectedPing", 0x1c: "UnconnectedPong",
    0x05: "OpenConnectionRequest1", 0x06: "OpenConnectionReply1",
    0x07: "OpenConnectionRequest2", 0x08: "OpenConnectionReply2",
}
ONLINE_IDS = {
    0x09: "ConnectionRequest", 0x10: "ConnectionRequestAccepted",
    0x13: "NewIncomingConnection", 0x00: "ConnectedPing", 0x03: "ConnectedPong",
    0x15: "DisconnectionNotification",
}
BEDROCK_NAMES = {
    0x01: "Login", 0x02: "PlayStatus", 0x03: "ServerToClientHandshake",
    0x04: "ClientToServerHandshake", 0x06: "ResourcePacksInfo",
    0x07: "ResourcePackStack", 0x08: "ResourcePackClientResponse",
    0x0b: "StartGame", 0x45: "RequestChunkRadius", 0x46: "ChunkRadiusUpdated",
    0x4d: "CommandRequest", 0x4f: "CommandOutput",
    0x71: "SetLocalPlayerAsInitialised", 0x8f: "NetworkSettings",
    0x9c: "PacketViolationWarning", 0xc1: "RequestNetworkSettings",
    0xfe: "Batch",
}


def read_pcap(path):
    data = path.read_bytes()
    magic = struct.unpack('<I', data[:4])[0]
    endian = '<' if magic == 0xa1b2c3d4 else '>'
    link_type = struct.unpack(endian + 'I', data[20:24])[0]
    packets = []
    offset = 24
    while offset + 16 <= len(data):
        _, _, incl_len, _ = struct.unpack(endian + 'IIII', data[offset:offset+16])
        offset += 16
        pkt = data[offset:offset+incl_len]
        offset += incl_len
        if link_type == 1 and len(pkt) >= 14:
            pkt = pkt[14:]
        if len(pkt) < 20 or (pkt[0] >> 4) != 4 or pkt[9] != 17:
            continue
        ihl = (pkt[0] & 0x0f) * 4
        src_ip = '.'.join(str(b) for b in pkt[12:16])
        dst_ip = '.'.join(str(b) for b in pkt[16:20])
        udp = pkt[ihl:]
        if len(udp) < 8:
            continue
        src_port, dst_port, _, _ = struct.unpack('>HHHH', udp[:8])
        packets.append((src_ip, src_port, dst_ip, dst_port, udp[8:]))
    return packets


def try_inflate_batch(body):
    """尝试解压 batch 包。"""
    if not body or body[0] != 0xfe:
        return None
    payload = body[1:]
    for data in [payload, payload[1:]]:
        try:
            return zlib.decompressobj(-15).decompress(data)
        except Exception:
            continue
    return None


def parse_bedrock_in_batch(decompressed):
    """从 batch 解压数据解析 Bedrock 包。返回 [(packet_id, payload), ...]"""
    packets = []
    offset = 0
    while offset < len(decompressed):
        # varuint length
        shift = 0
        pkt_len = 0
        while offset < len(decompressed):
            b = decompressed[offset]
            offset += 1
            pkt_len |= (b & 0x7f) << shift
            shift += 7
            if not (b & 0x80):
                break
        if pkt_len == 0 or offset + pkt_len > len(decompressed):
            break
        pkt_data = decompressed[offset:offset+pkt_len]
        offset += pkt_len
        if pkt_data:
            packets.append((pkt_data[0], pkt_data[1:]))
    return packets


def main():
    pcap = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(
        "cache/captures/netease_game.pcap")
    packets = read_pcap(pcap)
    print(f"=== 网易游戏流量统计 ===")
    print(f"pcap: {pcap.name}")
    print(f"总包数: {len(packets)}")
    print()

    # 顶层包类型统计
    top_counter = Counter()
    bedrock_counter = Counter()
    key_bedrock_packets = {}  # name -> (direction, pid, payload, raw_frame_body)
    batch_count = 0
    batch_inflate_fail = 0

    for i, (src_ip, src_port, dst_ip, dst_port, payload) in enumerate(packets):
        if not payload:
            continue
        pid = payload[0]
        direction = "C→S" if src_port != 12895 else "S→C"

        if pid in OFFLINE_IDS:
            top_counter[f"Offline:{OFFLINE_IDS[pid]}"] += 1
        elif pid == 0xc0:
            top_counter["ACK"] += 1
        elif pid == 0xa0:
            top_counter["NACK"] += 1
        elif (pid & 0xf0) == 0x80:
            top_counter["FrameSet"] += 1
            try:
                fs = FrameSet.decode(payload)
                for frame in fs.frames:
                    body = frame.body
                    if not body:
                        continue
                    bedrock_pid = body[0]
                    name = BEDROCK_NAMES.get(bedrock_pid, f"Unknown(0x{bedrock_pid:02x})")

                    # batch 包
                    if bedrock_pid == 0xfe:
                        batch_count += 1
                        decompressed = try_inflate_batch(body)
                        if decompressed:
                            for inner_pid, inner_payload in parse_bedrock_in_batch(decompressed):
                                inner_name = BEDROCK_NAMES.get(inner_pid, f"Unknown(0x{inner_pid:02x})")
                                bedrock_counter[f"{inner_name}"] += 1
                                # 保存关键包的第一个样本
                                if inner_name not in key_bedrock_packets and inner_pid in (
                                    0x01, 0x02, 0x03, 0x04, 0x06, 0x07, 0x08, 0x0b,
                                    0x8f, 0xc1, 0x9c, 0x45, 0x46, 0x4d, 0x71
                                ):
                                    key_bedrock_packets[inner_name] = (direction, inner_pid, inner_payload, i+1)
                        else:
                            batch_inflate_fail += 1
                            bedrock_counter["Batch(解压失败)"] += 1
                    else:
                        bedrock_counter[f"Raw:{name}"] += 1
                        if name not in key_bedrock_packets and bedrock_pid in (
                            0x09, 0x10, 0x13, 0x00, 0x03, 0x15
                        ):
                            key_bedrock_packets[name] = (direction, bedrock_pid, body[1:], i+1)
            except Exception as e:
                top_counter["FrameSet(解析失败)"] += 1
        else:
            top_counter[f"Unknown(0x{pid:02x})"] += 1

    print("=== 顶层包类型 ===")
    for name, count in top_counter.most_common():
        print(f"  {name:30s} {count:6d}")
    print()

    print("=== Bedrock 包类型（含 batch 内部）===")
    for name, count in bedrock_counter.most_common():
        print(f"  {name:30s} {count:6d}")
    print()

    print(f"=== Batch 包统计 ===")
    print(f"  batch 总数: {batch_count}")
    print(f"  解压失败: {batch_inflate_fail}")
    print()

    print("=== 关键 Bedrock 包样本（用于对比 vanilla）===")
    for name, (direction, pid, payload, pkt_idx) in key_bedrock_packets.items():
        print(f"\n[{name}] 方向={direction} pid=0x{pid:02x} 包序号={pkt_idx} payload_len={len(payload)}")
        print(f"  payload[:80]={payload[:80].hex()}")
        if len(payload) > 80:
            print(f"  payload[80:160]={payload[80:160].hex()}")


if __name__ == "__main__":
    main()
