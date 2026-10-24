"""诊断 RequestNetworkSettings 无响应 / Disconnect：抓 BDS 在线握手后回的所有包。

用法：
    python3 -m tools.diag_network_settings <host> <port> [protocol] [mode] [compression]

    protocol:     RequestNetworkSettings 的 ClientProtocol（默认 2168）
    mode:         handshake_only | rns | rns_compressed | rns_flate | multi （默认 rns）
                  handshake_only: 只 RakNet 握手 + NewIncoming，不发 RNS，看 BDS 反应
                  rns:            发 RequestNetworkSettings（无 compression byte）
                  rns_compressed: 发 RequestNetworkSettings（带 0xff NopCompression byte）
                  multi:          依次试 protocol-10..protocol+10 找哪个不被踢
    compression:  batch 是否带 compression byte（rns_compressed 用，1=带 0=不带）
"""
from __future__ import annotations

import asyncio
import logging
import sys

from proloader.net.bedrock.batch import decode_batch, encode_batch
from proloader.net.bedrock.login import build_request_network_settings
from proloader.net.raknet.connection import handshake
from proloader.net.raknet.session import RakNetSession

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s | %(message)s", datefmt="%H:%M:%S")


async def _drain(raknet: RakNetSession, seconds: float, label: str) -> int:
    """收 seconds 秒所有帧 body，打印。返回收到的数量。"""
    count = 0
    deadline = asyncio.get_event_loop().time() + seconds
    while asyncio.get_event_loop().time() < deadline:
        try:
            body = await asyncio.wait_for(raknet.recv(), timeout=0.5)
        except asyncio.TimeoutError:
            continue
        count += 1
        head = f"0x{body[0]:02x}" if body else "empty"
        preview = body[:48].hex()
        suffix = "..." if len(body) > 48 else ""
        print(f"  ← [{label}] #{count} [{len(body)}B] head={head}: {preview}{suffix}")
        if body and body[0] == 0xFE:
            try:
                pkts = decode_batch(body)
                for i, p in enumerate(pkts):
                    from proloader.net.raknet.wire import ByteReader
                    r = ByteReader(p)
                    val = r.read_unsigned_varint()
                    pid = val & 0x3FF
                    print(f"    [{i}] packet_id={pid} [{len(p)}B]: {p[:40].hex()}")
            except Exception as e:
                print(f"    解 batch 失败: {e}")
    return count


async def run(host: str, port: int, protocol: int, mode: str, compression: int) -> None:
    conn = await handshake(host, port, timeout=10.0)
    print(f"✓ 离线握手: server_guid={conn.server_guid} mtu={conn.mtu}")
    raknet = RakNetSession(conn)
    await raknet.start()
    await raknet.connect(timeout=10.0)
    print("✓ 在线握手完成（NewIncoming 已发）")

    if mode == "handshake_only":
        print("\n=== handshake_only：不发 RNS，等 6 秒看 BDS 反应 ===")
        n = await _drain(raknet, 6.0, "wait")
        print(f"\n=== handshake_only 完成，共收到 {n} 个 frame body ===")
        await raknet.stop()
        return

    if mode == "multi":
        print(f"\n=== multi：试 protocol {protocol-10}..{protocol+10}，每次新连接（带 compression byte）===")
        await raknet.stop()
        for proto in range(protocol - 10, protocol + 11):
            try:
                conn2 = await handshake(host, port, timeout=6.0)
                rk2 = RakNetSession(conn2)
                await rk2.start()
                await rk2.connect(timeout=6.0)
                pkt = build_request_network_settings(protocol=proto)
                batch = encode_batch([pkt], compression_enabled=True)
                await rk2.send_reliable(batch)
                # 等 1.5 秒看回包
                got = []
                deadline = asyncio.get_event_loop().time() + 1.5
                while asyncio.get_event_loop().time() < deadline:
                    try:
                        body = await asyncio.wait_for(rk2.recv(), timeout=0.3)
                        got.append(body)
                    except asyncio.TimeoutError:
                        break
                tag = "?"
                for body in got:
                    if body and body[0] == 0xFE:
                        try:
                            pkts = decode_batch(body)
                            for p in pkts:
                                from proloader.net.raknet.wire import ByteReader
                                r = ByteReader(p)
                                pid = r.read_unsigned_varint() & 0x3FF
                                tag = f"pid={pid}"
                        except Exception:
                            pass
                    elif body:
                        tag = f"head=0x{body[0]:02x}"
                print(f"  proto={proto}: 收 {len(got)} 包, 首包 {tag}")
                await rk2.stop()
            except Exception as e:
                print(f"  proto={proto}: 异常 {e}")
        return

    # rns / rns_compressed / rns_flate
    pkt = build_request_network_settings(protocol=protocol)
    print(f"\nRequestNetworkSettings 包 protocol={protocol} [{len(pkt)}B]: {pkt.hex()}")
    if mode == "rns_flate":
        # 带 0x00 (flate) compression byte + 实际 zlib 压缩 payload
        from proloader.net.bedrock.packets import CompressionAlgorithm
        batch = encode_batch([pkt], algorithm=CompressionAlgorithm.FLATE, threshold=0, compression_enabled=True)
        print(f"Batch [{len(batch)}B] (flate compressed): {batch.hex()}")
    else:
        use_comp = mode == "rns_compressed" or compression == 1
        batch = encode_batch([pkt], compression_enabled=use_comp)
        print(f"Batch [{len(batch)}B] (compression_byte={'yes' if use_comp else 'no'}): {batch.hex()}")
    await raknet.send_reliable(batch)
    print("已发送，监听 10 秒所有回包...\n")

    n = await _drain(raknet, 10.0, "rns")
    print(f"\n=== 共收到 {n} 个 frame body ===")
    await raknet.stop()


if __name__ == "__main__":
    proto = int(sys.argv[3]) if len(sys.argv) > 3 else 2168
    mode = sys.argv[4] if len(sys.argv) > 4 else "rns"
    comp = int(sys.argv[5]) if len(sys.argv) > 5 else 0
    asyncio.run(run(sys.argv[1], int(sys.argv[2]), proto, mode, comp))
