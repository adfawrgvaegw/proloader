#!/usr/bin/env python3
"""分析网易游戏流量连接开头的明文阶段（加密握手前）。

重点找 RequestNetworkSettings / Login / ServerToClientHandshake 的明文格式，
对比 vanilla Bedrock 找魔改差异。

用法: python3 -m tools.analyze_netease_login <pcap文件> [前N个包]
"""
import struct
import sys
import zlib
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from proloader.net.raknet.frames import FrameSet
from proloader.net.raknet.constants import Reliability

REL_NAMES = {
    0: "Unreliable", 1: "UnreliableSeq", 2: "Reliable",
    3: "ReliableOrdered", 4: "ReliableSeq",
    5: "UnreliableAck", 6: "ReliableAck", 7: "ReliableOrderedAck",
}

# RakNet 在线层包（不是 Bedrock 包！）
RAKNET_ONLINE = {
    0x00: "ConnectedPing",
    0x03: "ConnectedPong",
    0x09: "ConnectionRequest",
    0x10: "ConnectionRequestAccepted",
    0x13: "NewIncomingConnection",
    0x15: "DisconnectionNotification",
}

# Bedrock 包 ID
BEDROCK_NAMES = {
    0x01: "Login", 0x02: "PlayStatus", 0x03: "ServerToClientHandshake",
    0x04: "ClientToServerHandshake", 0x06: "ResourcePacksInfo",
    0x07: "ResourcePackStack", 0x08: "ResourcePackClientResponse",
    0x0b: "StartGame", 0x45: "RequestChunkRadius", 0x46: "ChunkRadiusUpdated",
    0x4d: "CommandRequest", 0x4f: "CommandOutput",
    0x71: "SetLocalPlayerAsInitialised", 0x8f: "NetworkSettings",
    0x9c: "PacketViolationWarning", 0xc1: "RequestNetworkSettings",
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
        udp = pkt[ihl:]
        if len(udp) < 8:
            continue
        src_port, dst_port, _, _ = struct.unpack('>HHHH', udp[:8])
        packets.append((src_port, dst_port, udp[8:]))
    return packets


def try_inflate_batch(body):
    """解压 batch 包。返回 (method, decompressed) 或 None。"""
    if not body or body[0] != 0xfe:
        return None
    payload = body[1:]
    for name, data in [("raw-deflate", payload), ("skip1", payload[1:])]:
        try:
            d = zlib.decompressobj(-15).decompress(data)
            if d:
                return name, d
        except Exception:
            continue
    return None


def parse_batch_packets(decompressed):
    """解析 batch 内的 Bedrock 包。返回 [(pid, name, payload), ...]"""
    packets = []
    offset = 0
    while offset < len(decompressed):
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
            pid = pkt_data[0]
            name = BEDROCK_NAMES.get(pid, f"Unknown(0x{pid:02x})")
            packets.append((pid, name, pkt_data[1:]))
    return packets


def main():
    pcap = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(
        "cache/captures/netease_game.pcap")
    limit = int(sys.argv[2]) if len(sys.argv) > 2 else 60

    packets = read_pcap(pcap)
    print(f"=== 网易游戏流量：前 {limit} 个包（连接开头明文阶段）===")
    print(f"pcap: {pcap.name}  总包数: {len(packets)}")
    print()

    batch_success = 0
    batch_fail = 0
    login_found = False

    for i, (src_port, dst_port, payload) in enumerate(packets[:limit]):
        if not payload:
            continue
        direction = "C→S" if dst_port == 12895 else "S→C"
        pid = payload[0]

        # 离线包
        if pid in (0x05, 0x06, 0x07, 0x08):
            names = {0x05: "OpenConnReq1", 0x06: "OpenConnReply1",
                     0x07: "OpenConnReq2", 0x08: "OpenConnReply2"}
            print(f"[{i+1:3d}] {direction} {names[pid]:25s} len={len(payload)}")
            continue

        # ACK
        if pid == 0xc0:
            print(f"[{i+1:3d}] {direction} {'ACK':25s} len={len(payload)}")
            continue
        if pid == 0xa0:
            print(f"[{i+1:3d}] {direction} {'NACK':25s} len={len(payload)}")
            continue

        # FrameSet
        if (pid & 0xf0) == 0x80:
            try:
                fs = FrameSet.decode(payload)
                for j, frame in enumerate(fs.frames):
                    body = frame.body
                    if not body:
                        print(f"[{i+1:3d}] {direction} FrameSet seq={fs.sequence_number} frame[{j}] (空body)")
                        continue

                    bpid = body[0]

                    # RakNet 在线层包
                    if bpid in RAKNET_ONLINE:
                        rname = RAKNET_ONLINE[bpid]
                        print(f"[{i+1:3d}] {direction} {rname:25s} (RakNet) len={len(body)} body={body.hex()}")
                        continue

                    # Bedrock batch 包
                    if bpid == 0xfe:
                        result = try_inflate_batch(body)
                        if result:
                            batch_success += 1
                            method, decompressed = result
                            bedrock_pkts = parse_batch_packets(decompressed)
                            for bpid2, bname2, bpayload2 in bedrock_pkts:
                                is_key = bpid2 in (0x01, 0x02, 0x03, 0x04, 0x06, 0x07, 0x08, 0x0b, 0x8f, 0xc1, 0x9c)
                                marker = " ★" if is_key else ""
                                print(f"[{i+1:3d}] {direction} Batch→{bname2:23s} len={len(bpayload2)}{marker}")
                                if is_key:
                                    print(f"      payload[:120]={bpayload2[:120].hex()}")
                                    if bpid2 == 0x01:
                                        login_found = True
                                        print(f"      *** LOGIN 包找到！分析 JWT chain ***")
                        else:
                            batch_fail += 1
                            print(f"[{i+1:3d}] {direction} Batch(解压失败/加密)     len={len(body)} body[:20]={body[:20].hex()}")
                        continue

                    # 其他裸包
                    bname = BEDROCK_NAMES.get(bpid, f"Unknown(0x{bpid:02x})")
                    print(f"[{i+1:3d}] {direction} {bname:25s} len={len(body)} body={body.hex()}")
            except Exception as e:
                print(f"[{i+1:3d}] {direction} FrameSet(解析失败: {e})")
            continue

        print(f"[{i+1:3d}] {direction} Unknown(0x{pid:02x}) len={len(payload)}")

    print(f"\n=== 统计（前{limit}包）===")
    print(f"  batch 解压成功: {batch_success}")
    print(f"  batch 解压失败(加密): {batch_fail}")
    print(f"  Login 包找到: {login_found}")


if __name__ == "__main__":
    main()
