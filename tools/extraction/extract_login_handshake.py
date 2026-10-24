#!/usr/bin/env python3
"""提取网易 Login 和 ServerToClientHandshake 的完整内容，对比 vanilla。

用法: python3 -m tools.extract_login_handshake
"""
import base64
import json
import struct
import sys
import zlib
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from proloader.net.raknet.frames import FrameSet


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


def parse_batch(body):
    """解析 batch 包，支持压缩和不压缩两种格式。返回 packet_data 列表。"""
    if not body or body[0] != 0xfe:
        return []
    payload = body[1:]

    # 尝试 DEFLATE 解压
    try:
        decompressed = zlib.decompressobj(-15).decompress(payload)
        if decompressed and len(decompressed) > 2:
            return _parse_batch_inner(decompressed)
    except Exception:
        pass

    # 尝试不压缩格式: varuint(len) + packet
    try:
        return _parse_batch_inner(payload)
    except Exception:
        return []


def _parse_batch_inner(data):
    packets = []
    offset = 0
    while offset < len(data):
        pkt_len, offset = read_varuint(data, offset)
        if pkt_len == 0 or offset + pkt_len > len(data):
            break
        packets.append(data[offset:offset+pkt_len])
        offset += pkt_len
    return packets


def try_decode_jwt(jwt_str):
    """尝试解码 JWT（3段 base64url，用 . 分隔）。"""
    parts = jwt_str.split('.')
    result = {"raw": jwt_str[:200] + "..." if len(jwt_str) > 200 else jwt_str,
              "parts_count": len(parts)}
    for i, part in enumerate(parts[:2]):  # 只解码 header 和 payload
        try:
            # base64url 解码，补 padding
            padded = part + '=' * (4 - len(part) % 4)
            decoded = base64.urlsafe_b64decode(padded)
            result[f"part_{i}"] = json.loads(decoded)
        except Exception as e:
            result[f"part_{i}_raw"] = part[:100]
            result[f"part_{i}_error"] = str(e)
    return result


def main():
    pcap = Path("cache/captures/netease_game.pcap")
    packets = read_pcap(pcap)

    print("=" * 70)
    print("网易 Bedrock Login + ServerToClientHandshake 完整分析")
    print("=" * 70)

    found = {"RequestNetworkSettings": False, "NetworkSettings": False,
             "Login": False, "ServerToClientHandshake": False,
             "ClientToServerHandshake": False, "PlayStatus": False}

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

            batch_packets = parse_batch(body)
            for pkt_data in batch_packets:
                if not pkt_data:
                    continue
                pid = pkt_data[0]
                direction = "C→S" if dst_port == 12895 else "S→C"
                pkt_payload = pkt_data[1:]

                if pid == 0xc1 and not found["RequestNetworkSettings"]:
                    found["RequestNetworkSettings"] = True
                    print(f"\n{'='*70}")
                    print(f"[包{i+1}] {direction} RequestNetworkSettings (0xc1)")
                    print(f"{'='*70}")
                    print(f"payload ({len(pkt_payload)} bytes): {pkt_payload.hex()}")
                    # vanilla: protocol version (varint)
                    proto_ver, _ = read_varuint(pkt_payload, 0)
                    print(f"protocol version (varuint): {proto_ver}")

                elif pid == 0x8f and not found["NetworkSettings"]:
                    found["NetworkSettings"] = True
                    print(f"\n{'='*70}")
                    print(f"[包{i+1}] {direction} NetworkSettings (0x8f)")
                    print(f"{'='*70}")
                    print(f"payload ({len(pkt_payload)} bytes): {pkt_payload.hex()}")
                    # vanilla: compression_threshold(varuint) + encryption_enabled(bool) + ...
                    comp_thresh, off = read_varuint(pkt_payload, 0)
                    print(f"compression_threshold (varuint): {comp_thresh}")
                    if off < len(pkt_payload):
                        print(f"remaining: {pkt_payload[off:].hex()}")

                elif pid == 0x01 and not found["Login"]:
                    found["Login"] = True
                    print(f"\n{'='*70}")
                    print(f"[包{i+1}] {direction} Login (0x01)")
                    print(f"{'='*70}")
                    print(f"payload ({len(pkt_payload)} bytes)")
                    # vanilla Login: protocol_version(varuint) + JWT chain(string)
                    proto_ver, off = read_varuint(pkt_payload, 0)
                    print(f"protocol version (varuint): {proto_ver}")
                    # JWT chain: varuint(length) + UTF-8 string (JSON array of JWTs)
                    chain_len, off2 = read_varuint(pkt_payload, off)
                    print(f"JWT chain length: {chain_len}")
                    chain_str = pkt_payload[off2:off2+chain_len].decode('utf-8', errors='replace')
                    try:
                        chain_json = json.loads(chain_str)
                        print(f"JWT chain: {len(chain_json)} JWTs")
                        for j, jwt in enumerate(chain_json):
                            print(f"\n  JWT[{j}]:")
                            decoded = try_decode_jwt(jwt)
                            for k, v in decoded.items():
                                print(f"    {k}: {v}")
                    except json.JSONDecodeError:
                        print(f"JWT chain (raw): {chain_str[:500]}")
                    # 剩余数据（客户端随机值等）
                    remaining = pkt_payload[off2+chain_len:]
                    if remaining:
                        print(f"\n剩余数据 ({len(remaining)} bytes): {remaining.hex()}")

                elif pid == 0x03 and not found["ServerToClientHandshake"]:
                    found["ServerToClientHandshake"] = True
                    print(f"\n{'='*70}")
                    print(f"[包{i+1}] {direction} ServerToClientHandshake (0x03)")
                    print(f"{'='*70}")
                    print(f"payload ({len(pkt_payload)} bytes)")
                    # vanilla: JWT string (varuint length + UTF-8)
                    jwt_len, off = read_varuint(pkt_payload, 0)
                    print(f"JWT length: {jwt_len}")
                    jwt_str = pkt_payload[off:off+jwt_len].decode('utf-8', errors='replace')
                    print(f"\nJWT 内容:")
                    decoded = try_decode_jwt(jwt_str)
                    for k, v in decoded.items():
                        print(f"  {k}: {v}")
                    remaining = pkt_payload[off+jwt_len:]
                    if remaining:
                        print(f"\n剩余数据 ({len(remaining)} bytes): {remaining.hex()}")

                elif pid == 0x02 and not found["PlayStatus"]:
                    found["PlayStatus"] = True
                    print(f"\n{'='*70}")
                    print(f"[包{i+1}] {direction} PlayStatus (0x02)")
                    print(f"{'='*70}")
                    print(f"payload ({len(pkt_payload)} bytes): {pkt_payload[:20].hex()}")
                    status, _ = read_varuint(pkt_payload, 0)
                    print(f"status (varuint): {status}")
                    print(f"  (0=LoginSuccess, 1=FailedClient, 2=FailedSpawn, 3=FailedEditor, 4=PlayerSpawn)")

    print(f"\n{'='*70}")
    print("找到的关键包:")
    for k, v in found.items():
        print(f"  {k}: {'✅' if v else '❌'}")


if __name__ == "__main__":
    main()
