#!/usr/bin/env python3
"""分析网易 batch 包格式，对比 vanilla。

vanilla batch: 0xfe + raw DEFLATE
网易可能: 0xfe + varuint(length) + raw packet（不压缩）
"""
import struct
import sys
import zlib
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from proloader.net.raknet.frames import FrameSet

BEDROCK_NAMES = {
    0x01: "Login", 0x02: "PlayStatus", 0x03: "ServerToClientHandshake",
    0x04: "ClientToServerHandshake", 0x06: "ResourcePacksInfo",
    0x07: "ResourcePackStack", 0x08: "ResourcePackClientResponse",
    0x0b: "StartGame", 0x8f: "NetworkSettings", 0x9c: "PacketViolationWarning",
    0xc1: "RequestNetworkSettings",
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


def read_varuint(data, offset):
    """读 varuint，返回 (value, new_offset)。"""
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


def try_parse_uncompressed_batch(body):
    """尝试按不压缩格式解析: fe + varuint(len) + packet_data + ..."""
    if not body or body[0] != 0xfe:
        return None
    offset = 1
    packets = []
    total = len(body)
    while offset < total:
        pkt_len, offset = read_varuint(body, offset)
        if pkt_len == 0 or offset + pkt_len > total:
            return None  # 格式不对
        pkt_data = body[offset:offset+pkt_len]
        offset += pkt_len
        packets.append(pkt_data)
    return packets


def main():
    pcap = Path("cache/captures/netease_game.pcap")
    packets = read_pcap(pcap)

    print("=== 网易 batch 包格式分析 ===\n")

    # 找前 10 个 batch 包，分析格式
    batch_count = 0
    for i, (src_port, dst_port, payload) in enumerate(packets):
        if not payload or (payload[0] & 0xf0) != 0x80:
            continue
        try:
            fs = FrameSet.decode(payload)
        except Exception:
            continue

        for frame in fs.frames:
            body = frame.body
            if not body or body[0] != 0xfe:
                continue

            batch_count += 1
            if batch_count > 15:
                break

            direction = "C→S" if dst_port == 12895 else "S→C"
            print(f"[包{i+1}] {direction} batch body len={len(body)}")
            print(f"  body[:30]={body[:30].hex()}")

            # 尝试 DEFLATE 解压
            deflate_ok = False
            for skip in [1, 2]:
                try:
                    d = zlib.decompressobj(-15).decompress(body[skip:])
                    if d:
                        print(f"  ✅ DEFLATE 解压成功 (skip={skip}, {len(d)} bytes)")
                        print(f"  解压内容[:40]={d[:40].hex()}")
                        deflate_ok = True
                        break
                except Exception:
                    continue

            # 尝试不压缩格式
            if not deflate_ok:
                uncompressed = try_parse_uncompressed_batch(body)
                if uncompressed:
                    print(f"  ✅ 不压缩格式解析成功 ({len(uncompressed)} 个包)")
                    for j, pkt in enumerate(uncompressed):
                        pid = pkt[0]
                        name = BEDROCK_NAMES.get(pid, f"Unknown(0x{pid:02x})")
                        print(f"    packet[{j}]: {name} (0x{pid:02x}) len={len(pkt)} payload={pkt[1:].hex()}")
                else:
                    print(f"  ❌ 两种格式都失败")

            print()
            break

        if batch_count > 15:
            break


if __name__ == "__main__":
    main()
