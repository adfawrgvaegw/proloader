#!/usr/bin/env python3
"""从抓包中提取网易客户端 Login 包的 ClientData.GameVersion。

解决 "disconnectionScreen.gameVersionMismatch" 的关键。

Login 包 ~37KB，分片成多个 RakNet 帧（is_split=True），需要按 split_id 重组。
重组后是完整 batch（0xfe + 网易定制压缩数据），解压后得到 Login 包。
Login 的 RawToken JWT payload 含 ClientData.GameVersion。
"""
import base64
import json
import struct
import sys
import zlib
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


def b64url_decode(s):
    padding = 4 - len(s) % 4
    if padding != 4:
        s += "=" * padding
    return base64.urlsafe_b64decode(s)


def decode_jwt_payload(jwt_str):
    parts = jwt_str.split(".")
    if len(parts) != 3:
        return None
    try:
        return json.loads(b64url_decode(parts[1]))
    except Exception:
        return None


def parse_login_packet(pkt_data):
    """Login = varuint(1) + i32_BE(protocol) + varuint(conn_req_len) + conn_req
    conn_req = i32_LE(chain_len) + chain + i32_LE(raw_token_len) + raw_token
    """
    pid, off = read_varuint(pkt_data, 0)
    if pid != 0x01:
        return None
    if off + 4 > len(pkt_data):
        return None
    protocol = struct.unpack(">i", pkt_data[off:off + 4])[0]
    off += 4
    conn_req_len, off = read_varuint(pkt_data, off)
    conn_req = pkt_data[off:off + conn_req_len]
    if len(conn_req) < 8:
        return None
    chain_len = struct.unpack("<i", conn_req[:4])[0]
    if chain_len < 0 or chain_len > len(conn_req) - 8:
        return None
    chain_bytes = conn_req[4:4 + chain_len]
    rt_off = 4 + chain_len
    raw_token_len = struct.unpack("<i", conn_req[rt_off:rt_off + 4])[0]
    raw_token = conn_req[rt_off + 4:rt_off + 4 + raw_token_len]
    return {
        "protocol": protocol,
        "chain": chain_bytes,
        "raw_token": raw_token,
    }


def process_batch_body(body, direction, netease_decomp):
    """处理一个完整的 batch body（0xfe + data），返回内部 packet 列表。"""
    if not body or body[0] != 0xfe:
        return []
    payload = body[1:]
    if not payload:
        return []

    # 尝试解压
    decompressed = None
    method = "?"

    # 0. 网易定制压缩（补 final block + 滑动窗口字典）
    if netease_decomp is not None:
        try:
            out = netease_decomp.decompress(payload)
            if out and len(out) > 2:
                decompressed = out
                method = "netease"
        except Exception:
            pass

    # 1. 补 final block 再试标准 DEFLATE（无字典版）
    if decompressed is None:
        try:
            suffix = b"\x01\x00\x00\xff\xff"
            d = zlib.decompressobj(-15)
            out = d.decompress(payload + suffix)
            out += d.flush()
            if out and len(out) > 2:
                decompressed = out
                method = "deflate+finalblock"
        except Exception:
            pass

    # 2. 标准 raw DEFLATE
    if decompressed is None:
        try:
            d = zlib.decompressobj(-15)
            out = d.decompress(payload)
            out += d.flush()
            if out and len(out) > 2:
                decompressed = out
                method = "raw-deflate"
        except Exception:
            pass

    # 3. 不压缩
    if decompressed is None:
        try:
            pkt_len, off = read_varuint(payload, 0)
            if 0 < pkt_len <= len(payload) - off:
                decompressed = payload
                method = "uncompressed"
        except Exception:
            pass

    if decompressed is None:
        return []

    # 解析 batch 内的包
    packets = []
    off = 0
    while off < len(decompressed):
        pkt_len, off2 = read_varuint(decompressed, off)
        if pkt_len == 0 or off2 + pkt_len > len(decompressed):
            break
        pkt_data = decompressed[off2:off2 + pkt_len]
        off = off2 + pkt_len
        if pkt_data:
            packets.append((method, pkt_data))
    return packets


class PcapAnalyzer:
    def __init__(self):
        self.netease_active = False
        self.login_found = False
        self.game_version_found = None
        self.netease_decomp_cs = NeteaseDecompressor()
        self.netease_decomp_sc = NeteaseDecompressor()
        self.split_groups = defaultdict(dict)
        self.split_counts = {}

    def handle_packet(self, pid, pkt_data, direction, pkt_idx, method):
        """处理一个解压后的 Bedrock 包。"""
        # NetworkSettings (0x8f=143) → 启用网易解压
        if pid == 0x8f:
            if not self.netease_active:
                self.netease_active = True
                print(f"\n[包{pkt_idx + 1}] {direction} NetworkSettings → 启用网易解压 (method={method})")

        # Login (0x01)
        if pid == 0x01 and direction == "C→S" and not self.login_found:
            self.login_found = True
            print(f"\n{'=' * 70}")
            print(f"[包{pkt_idx + 1}] {direction} Login (0x01)  解压方式={method}")
            print(f"{'=' * 70}")
            info = parse_login_packet(pkt_data)
            if info:
                print(f"protocol version: {info['protocol']}")
                print(f"chain ({len(info['chain'])} bytes):")
                try:
                    chain_str = info["chain"].decode("utf-8")
                    chain_obj = json.loads(chain_str)
                    print(f"  chainJSON keys: {list(chain_obj.keys())}")
                    print(f"  AuthenticationType: {chain_obj.get('AuthenticationType')}")
                    if "Token" in chain_obj:
                        token_jwt = chain_obj["Token"]
                        token_pl = decode_jwt_payload(token_jwt)
                        if token_pl:
                            print(f"  Token JWT claims: {list(token_pl.keys())}")
                            for k, v in token_pl.items():
                                vs = str(v)
                                if len(vs) > 80:
                                    vs = vs[:80] + "..."
                                print(f"    {k}: {vs}")
                        else:
                            print(f"  Token (raw): {token_jwt[:200]}...")
                    if "Certificate" in chain_obj:
                        cert = json.loads(chain_obj["Certificate"])
                        print(f"  Certificate.chain: {len(cert['chain'])} JWTs")
                        for j, jwt in enumerate(cert["chain"]):
                            pl = decode_jwt_payload(jwt)
                            if pl:
                                keys = list(pl.keys())
                                print(f"    JWT[{j}] claims: {keys}")
                                if "identityPublicKey" in pl:
                                    print(f"      identityPublicKey: {pl['identityPublicKey'][:60]}...")
                                if "extraData" in pl:
                                    print(f"      extraData: {pl['extraData']}")
                    elif "chain" in chain_obj:
                        print(f"  chain: {len(chain_obj['chain'])} JWTs")
                except Exception as e:
                    print(f"  (解析失败: {e})")
                    print(f"  raw[:200]: {info['chain'][:200]}")

                print(f"\nraw_token ({len(info['raw_token'])} bytes):")
                try:
                    rt_str = info["raw_token"].decode("utf-8")
                    rt_payload = decode_jwt_payload(rt_str)
                    if rt_payload:
                        print(f"  RawToken claims ({len(rt_payload)} 个):")
                        for k in sorted(rt_payload.keys()):
                            v = rt_payload[k]
                            if isinstance(v, str) and len(v) > 80:
                                v = v[:80] + "..."
                            print(f"    {k}: {v}")
                        if "GameVersion" in rt_payload:
                            gv = rt_payload["GameVersion"]
                            print(f'\n  ★★★ RawToken.GameVersion = "{gv}" ★★★')
                            self.game_version_found = gv
                    else:
                        print(f"  (JWT 解码失败)")
                        print(f"  raw[:200]: {rt_str[:200]}")
                except Exception as e:
                    print(f"  (解析失败: {e})")
                    print(f"  raw[:200]: {info['raw_token'][:200]}")
            else:
                print(f"  (Login 解析失败)")
                print(f"  pkt_data[:60]: {pkt_data[:60].hex()}")

    def process_frame(self, body, direction, pkt_idx):
        """处理一个帧 body（可能是分片或完整 batch）。"""
        if not body:
            return

        # 处理分片帧的重组在外层做，这里只处理完整 body
        if body[0] != 0xfe:
            return

        nd = None
        if self.netease_active:
            nd = self.netease_decomp_cs if direction == "C→S" else self.netease_decomp_sc
        batch_pkts = process_batch_body(body, direction, nd)
        for method, pkt_data in batch_pkts:
            pid, _ = read_varuint(pkt_data, 0)
            self.handle_packet(pid, pkt_data, direction, pkt_idx, method)


def main():
    pcap = Path("cache/captures/netease_game.pcap")
    if not pcap.exists():
        print(f"❌ pcap 不存在: {pcap}")
        return
    packets = read_pcap(pcap)
    print(f"pcap: {pcap.name}  总包数: {len(packets)}")
    print("=" * 70)

    analyzer = PcapAnalyzer()

    for i, (src_port, dst_port, payload) in enumerate(packets):
        if not payload or (payload[0] & 0xf0) != 0x80:
            continue
        direction = "C→S" if dst_port == 12895 else "S→C"
        try:
            fs = FrameSet.decode(payload)
        except Exception:
            continue

        for frame in fs.frames:
            body = frame.body
            if not body:
                continue

            # 处理分片帧
            if frame.is_split:
                key = (direction, frame.split_id)
                analyzer.split_groups[key][frame.split_index] = body
                analyzer.split_counts[key] = frame.split_count

                # 检查是否所有分片都到齐
                if len(analyzer.split_groups[key]) == frame.split_count:
                    assembled = b"".join(
                        analyzer.split_groups[key][idx]
                        for idx in range(frame.split_count)
                    )
                    del analyzer.split_groups[key]
                    del analyzer.split_counts[key]
                    analyzer.process_frame(assembled, direction, i)
                continue

            # 非分片帧：直接处理
            analyzer.process_frame(body, direction, i)

    print(f"\n{'=' * 70}")
    print("结论:")
    if analyzer.game_version_found:
        print(f'  ★ 网易客户端实际发送的 GameVersion = "{analyzer.game_version_found}"')
        print(f'  当前 proloader 发送的 GameVersion = "1.21.70"')
        if analyzer.game_version_found != "1.21.70":
            print(f'  ⚠️ 不匹配！需要改成 "{analyzer.game_version_found}"')
        else:
            print(f"  ✅ 匹配，GameVersion 不是问题根因")
    else:
        print(f"  ❌ 未找到 GameVersion")
    print(f"  Login 包找到: {analyzer.login_found}")


if __name__ == "__main__":
    main()
