#!/usr/bin/env python3
"""分析网易租赁服 UDP 抓包，对比 vanilla Bedrock 协议。

用法: python3 -m tools.analyze_netease_pcap <pcap文件>
"""
import struct
import sys
from pathlib import Path

# RakNet 常量
MAGIC = bytes([0x00, 0xff, 0xff, 0x00, 0xfe, 0xfe, 0xfe, 0xfe,
               0xfd, 0xfd, 0xfd, 0xfd, 0x12, 0x34, 0x56, 0x78])

# RakNet 离线消息 ID
ID_UNCONNECTED_PING = 0x01
ID_UNCONNECTED_PONG = 0x1c
ID_OPEN_CONNECTION_REQUEST_1 = 0x05
ID_OPEN_CONNECTION_REPLY_1 = 0x06
ID_OPEN_CONNECTION_REQUEST_2 = 0x07
ID_OPEN_CONNECTION_REPLY_2 = 0x08

# RakNet 在线帧
ID_CONNECTION_REQUEST = 0x09
ID_CONNECTION_REQUEST_ACCEPTED = 0x10
ID_NEW_INCOMING_CONNECTION = 0x13

# FrameSet 包 ID 范围 (0x80-0x8f)
ID_FRAME_SET = range(0x80, 0x90)

# ACK / NACK
ID_ACK = 0xc0
ID_NACK = 0xc1


def read_pcap(path: Path):
    """简单解析 pcap 文件（libpcap 格式），返回 (src_ip, src_port, dst_ip, dst_port, payload) 列表。"""
    data = path.read_bytes()
    # pcap 全局头 24 字节
    magic = struct.unpack('<I', data[:4])[0]
    if magic == 0xa1b2c3d4:
        endian = '<'
    elif magic == 0xd4c3b2a1:
        endian = '>'
    else:
        raise ValueError(f"不是 pcap 格式: magic={magic:#x}")
    link_type = struct.unpack(endian + 'I', data[20:24])[0]
    if link_type != 1:  # LINKTYPE_ETHERNET
        # 可能是 LINKTYPE_RAW (101) 或 LINKTYPE_LOOP (108)
        if link_type not in (101, 108):
            print(f"⚠️ 未知的链路类型: {link_type}，尝试按 RAW 解析")

    packets = []
    offset = 24
    while offset + 16 <= len(data):
        ts_sec, ts_usec, incl_len, orig_len = struct.unpack(endian + 'IIII', data[offset:offset+16])
        offset += 16
        pkt = data[offset:offset+incl_len]
        offset += incl_len

        # 跳过链路层头
        if link_type == 1:  # Ethernet
            if len(pkt) < 14:
                continue
            pkt = pkt[14:]
        # IP 头
        if len(pkt) < 20:
            continue
        ver_ihl = pkt[0]
        if (ver_ihl >> 4) != 4:
            continue
        ihl = (ver_ihl & 0x0f) * 4
        protocol = pkt[9]
        if protocol != 17:  # UDP
            continue
        src_ip = '.'.join(str(b) for b in pkt[12:16])
        dst_ip = '.'.join(str(b) for b in pkt[16:20])
        # UDP 头
        udp = pkt[ihl:]
        if len(udp) < 8:
            continue
        src_port, dst_port, udp_len, _ = struct.unpack('>HHHH', udp[:8])
        payload = udp[8:]
        packets.append((src_ip, src_port, dst_ip, dst_port, payload))
    return packets


def parse_offline_packet(payload: bytes):
    """解析 RakNet 离线消息。"""
    if not payload:
        return None
    pid = payload[0]
    result = {"pid": pid, "name": None, "magic_ok": None, "details": {}}

    if pid == ID_OPEN_CONNECTION_REQUEST_1:
        result["name"] = "OpenConnectionRequest1"
        if len(payload) >= 1 + 16 + 1:
            magic = payload[1:17]
            result["magic_ok"] = (magic == MAGIC)
            result["details"]["protocol_version"] = payload[17]
            result["details"]["mtu_padding_len"] = len(payload) - 18
            result["details"]["total_len"] = len(payload)
    elif pid == ID_OPEN_CONNECTION_REPLY_1:
        result["name"] = "OpenConnectionReply1"
        if len(payload) >= 1 + 16 + 8 + 1:
            magic = payload[1:17]
            result["magic_ok"] = (magic == MAGIC)
            result["details"]["server_guid"] = struct.unpack('>Q', payload[17:25])[0]
            result["details"]["use_security"] = payload[25]
            if len(payload) >= 28:
                result["details"]["mtu"] = struct.unpack('>H', payload[26:28])[0]
    elif pid == ID_OPEN_CONNECTION_REQUEST_2:
        result["name"] = "OpenConnectionRequest2"
        if len(payload) >= 1 + 16 + 8:
            magic = payload[1:17]
            result["magic_ok"] = (magic == MAGIC)
            result["details"]["client_guid"] = struct.unpack('>Q', payload[17:25])[0]
            # server address (7 bytes for IPv4: 1 version + 4 ip + 2 port)
            if len(payload) >= 25 + 7:
                addr_ver = payload[25]
                ip_bytes = payload[26:30]
                ip = '.'.join(str(b) for b in ip_bytes)
                port = struct.unpack('>H', payload[30:32])[0]
                result["details"]["server_addr"] = f"{ip}:{port} (version={addr_ver})"
            if len(payload) >= 34:
                result["details"]["mtu"] = struct.unpack('>H', payload[32:34])[0]
    elif pid == ID_OPEN_CONNECTION_REPLY_2:
        result["name"] = "OpenConnectionReply2"
        if len(payload) >= 1 + 16 + 8:
            magic = payload[1:17]
            result["magic_ok"] = (magic == MAGIC)
            result["details"]["server_guid"] = struct.unpack('>Q', payload[17:25])[0]
            if len(payload) >= 25 + 7:
                addr_ver = payload[25]
                ip = '.'.join(str(b) for b in payload[26:30])
                port = struct.unpack('>H', payload[30:32])[0]
                result["details"]["client_addr"] = f"{ip}:{port} (version={addr_ver})"
            if len(payload) >= 34:
                result["details"]["mtu"] = struct.unpack('>H', payload[32:34])[0]
                result["details"]["use_encryption"] = payload[34] if len(payload) > 34 else None
    return result


def parse_online_packet(payload: bytes):
    """解析 RakNet 在线消息。"""
    if not payload:
        return None
    pid = payload[0]
    result = {"pid": pid, "name": None, "details": {}}

    if pid == ID_CONNECTION_REQUEST:
        result["name"] = "ConnectionRequest"
        if len(payload) >= 1 + 8 + 8 + 1:
            result["details"]["client_guid"] = struct.unpack('>Q', payload[1:9])[0]
            result["details"]["send_ping_time"] = struct.unpack('>Q', payload[9:17])[0]
            result["details"]["use_security"] = payload[17]
    elif pid == ID_CONNECTION_REQUEST_ACCEPTED:
        result["name"] = "ConnectionRequestAccepted"
        # 复杂格式：clientAddress + systemAddress x20 + requestTime + replyTime
        result["details"]["raw_len"] = len(payload)
        result["details"]["raw_hex_first40"] = payload[:40].hex()
    elif pid == ID_NEW_INCOMING_CONNECTION:
        result["name"] = "NewIncomingConnection"
        result["details"]["raw_len"] = len(payload)
        result["details"]["raw_hex_first40"] = payload[:40].hex()
    elif pid in ID_FRAME_SET:
        result["name"] = f"FrameSet(pid=0x{pid:02x})"
        result["details"]["raw_len"] = len(payload)
        # 解析 FrameSet
        if len(payload) >= 4:
            seq = (payload[1] << 16) | (payload[2] << 8) | payload[3]
            result["details"]["sequence"] = seq
            # 解析 frames
            result["details"]["frames"] = parse_frames(payload[4:])
    elif pid == ID_ACK:
        result["name"] = "ACK"
        result["details"]["raw_len"] = len(payload)
        result["details"]["raw_hex"] = payload.hex()
    elif pid == ID_NACK:
        result["name"] = "NACK"
        result["details"]["raw_len"] = len(payload)
        result["details"]["raw_hex"] = payload.hex()
    return result


def parse_frames(data: bytes):
    """解析 FrameSet 里的 frames。"""
    frames = []
    offset = 0
    count = 0
    while offset < len(data) and count < 20:  # 限制最多解析20个
        if offset + 4 > len(data):
            break
        flags = data[offset]
        rel = (data[offset+1] << 16) | (data[offset+2] << 8) | data[offset+3]
        offset += 4
        frame = {"flags": flags, "reliability": (flags >> 5) & 7,
                 "fragment": bool(flags & 0x10), "length": None, "body_hex": None}

        # 可靠帧有 order index
        reliability = frame["reliability"]
        if reliability in (2, 3, 4, 6, 7):  # reliable
            if offset + 3 > len(data):
                break
            order_index = (data[offset] << 16) | (data[offset+1] << 8) | data[offset+2]
            offset += 3
        if offset + 4 > len(data):
            break
        length = struct.unpack('>H', data[offset:offset+2])[0]
        offset += 2
        # order channel
        if offset < len(data):
            offset += 1
        body = data[offset:offset+length]
        offset += length
        frame["length"] = length
        frame["body_hex"] = body[:64].hex()
        frames.append(frame)
        count += 1
    return frames


def main():
    if len(sys.argv) < 2:
        pcap = Path("cache/captures/netease_udp_19132.pcap")
    else:
        pcap = Path(sys.argv[1])

    if not pcap.exists():
        print(f"❌ pcap 文件不存在: {pcap}")
        return

    packets = read_pcap(pcap)
    print(f"=== pcap 分析: {pcap.name} ===")
    print(f"总包数: {len(packets)}")
    print()

    # 统计通信对端
    peers = set()
    for src_ip, src_port, dst_ip, dst_port, _ in packets:
        if src_port == 19132:
            peers.add((dst_ip, dst_port))
        else:
            peers.add((src_ip, src_port))
    print("=== 通信对端 ===")
    for ip, port in peers:
        print(f"  {ip}:{port}")
    print()

    # 逐包分析
    print("=== 逐包分析 ===")
    for i, (src_ip, src_port, dst_ip, dst_port, payload) in enumerate(packets):
        direction = "→" if src_port == 19132 else "←"
        peer = f"{dst_ip}:{dst_port}" if src_port == 19132 else f"{src_ip}:{src_port}"
        print(f"\n[包 {i+1}] {direction} {peer}  长度={len(payload)}")

        if not payload:
            print("  (空)")
            continue

        pid = payload[0]
        print(f"  首字节: 0x{pid:02x}")

        # 判断离线/在线
        if pid in (ID_OPEN_CONNECTION_REQUEST_1, ID_OPEN_CONNECTION_REPLY_1,
                   ID_OPEN_CONNECTION_REQUEST_2, ID_OPEN_CONNECTION_REPLY_2,
                   ID_UNCONNECTED_PING, ID_UNCONNECTED_PONG):
            r = parse_offline_packet(payload)
        else:
            r = parse_online_packet(payload)

        if r:
            print(f"  类型: {r['name']}")
            if r.get("magic_ok") is not None:
                print(f"  MAGIC 一致: {r['magic_ok']} {'✅' if r['magic_ok'] else '❌ 魔改!'}")
            for k, v in r.get("details", {}).items():
                if k == "frames":
                    print(f"  {k}:")
                    for j, f in enumerate(v):
                        print(f"    frame[{j}]: reliability={f['reliability']} len={f['length']} body={f['body_hex']}")
                else:
                    print(f"  {k}: {v}")

    # 总结
    print("\n=== 对比 vanilla Bedrock 的结论 ===")
    offline_count = sum(1 for _, _, _, _, p in packets
                       if p and p[0] in (0x05, 0x06, 0x07, 0x08))
    print(f"离线握手包数: {offline_count}")
    print(f"在线帧/数据包数: {len(packets) - offline_count}")
    print(f"服务器端口: {[p[3] for p in packets if p[3] != 19132][0] if any(p[3] != 19132 for p in packets) else '19132'}")


if __name__ == "__main__":
    main()
