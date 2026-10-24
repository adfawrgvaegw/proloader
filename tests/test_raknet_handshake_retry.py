import asyncio
import socket
import struct
import pytest
from types import SimpleNamespace
from unittest.mock import Mock

from proloader.net.raknet.connection import handshake, SystemAddress
from proloader.net.raknet.constants import MAGIC
from proloader.net.raknet.wire import ByteWriter
from proloader.net.raknet.session import RakNetSession
from proloader.net.raknet.frames import FrameSet


@pytest.mark.parametrize('security', [False, True])
async def test_lost_second_request_recovers_and_disconnect_is_sent(security):
    """真实本机UDP：丢弃首个OCR2，随后接通；关闭时必须实际收到断开帧。"""
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.bind(('127.0.0.1', 0))
    sock.settimeout(3)
    loop = asyncio.get_running_loop()
    second_requests = 0
    async def server():
        nonlocal second_requests
        while True:
            data, peer = await loop.run_in_executor(None, sock.recvfrom, 4096)
            if data[0] == 5:
                reply = b'\x06' + MAGIC + struct.pack('>qB', 42, int(security))
                if security:
                    reply += bytes.fromhex('01020304')
                sock.sendto(reply + struct.pack('>H', 1200), peer)
            elif data[0] == 7:
                if security:
                    assert len(data) == 39
                    assert data[17:23] == bytes.fromhex('010203040004')
                else:
                    assert len(data) == 34
                    assert data[17] == 4
                assert struct.unpack('>H', data[-10:-8])[0] == 1200
                second_requests += 1
                if second_requests == 1:
                    continue
                w = ByteWriter()
                SystemAddress(peer[0], peer[1]).encode(w)
                reply = b'\x08' + MAGIC + struct.pack('>q', 42) + w.to_bytes() + struct.pack('>HB', 1200, 0)
                sock.sendto(reply, peer)
            elif data[0] & 0xF0 == 0x80:
                # Reliable unordered disconnection: frame-set(4), flags(1),
                # bit length(2), reliable index(3), body(1).
                if data[-1:] == b'\x15':
                    return
    task = asyncio.create_task(server())
    session = None
    try:
        conn = await handshake('127.0.0.1', sock.getsockname()[1], timeout=2, max_retries=0)
        assert second_requests == 2
        session = RakNetSession(conn)
        await session.start()
        await session.stop()
        await asyncio.wait_for(task, 1)
    finally:
        if session and not session._closed:
            await session.stop()
        task.cancel()
        sock.close()


@pytest.mark.parametrize('advertised', [576, 40982])
async def test_split_respects_ip_mtu_and_retry_preserves_identity(advertised):
    conn = SimpleNamespace(mtu=advertised, sock=Mock(), server_addr=('127.0.0.1', 19132))
    session = RakNetSession(conn)
    payload = bytes(range(256)) * 10
    await session.send_reliable(payload)
    fragments = []
    while not session._send_queue.empty():
        frame = session._send_queue.get_nowait()
        assert len(FrameSet(sequence_number=0, frames=[frame]).encode()) + 28 <= min(advertised, 1492)
        fragments.append(frame)
    assert b''.join(frame.body for frame in fragments) == payload
    assert len({frame.order_index for frame in fragments}) == 1
    original_index = fragments[0].frame_index
    await session._resend_frame(fragments[0])
    retry = session._send_queue.get_nowait()
    assert retry.frame_index == original_index


async def test_stop_cleans_socket_after_remote_disconnect():
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    conn = SimpleNamespace(mtu=1200, sock=sock, server_addr=('127.0.0.1', 19132))
    session = RakNetSession(conn)
    session._closed = True
    await session.stop()
    assert sock.fileno() == -1
    await session.stop()


async def test_online_loss_recovers_before_login_deadline():
    """Drop the first reliable datagram; recovery must be RTT-based, not 4 seconds."""
    from proloader.net.raknet.frames import build_ack, AckRange
    server = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    server.bind(('127.0.0.1', 0))
    server.setblocking(False)
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    session = RakNetSession(SimpleNamespace(mtu=1200, sock=sock, server_addr=server.getsockname()))
    loop = asyncio.get_running_loop()
    await session.start()
    try:
        await session.send_reliable(b'client-encryption-handshake')
        first, peer = await asyncio.wait_for(loop.sock_recvfrom(server, 4096), 1) if hasattr(loop, 'sock_recvfrom') else await asyncio.wait_for(asyncio.to_thread(_receive_timeout, server), 1)
        # Python 3.9 test server uses its own short bounded blocking receiver.
        server.settimeout(1)
        retry, peer = await asyncio.wait_for(asyncio.to_thread(server.recvfrom, 4096), 1)
        a, b = FrameSet.decode(first), FrameSet.decode(retry)
        assert a.frames[0].frame_index == b.frames[0].frame_index
        assert a.frames[0].body == b.frames[0].body
        server.sendto(build_ack([AckRange.single(b.sequence_number)]), peer)
        await asyncio.sleep(0.02)
        assert not session._pending_acks
    finally:
        await session.stop()
        server.close()


def _receive_timeout(sock):
    sock.settimeout(1)
    return sock.recvfrom(4096)


async def test_receive_gap_nacks_after_grace_and_late_packet_cancels_gap():
    import time
    from proloader.net.raknet.frames import Frame, parse_nack
    from proloader.net.raknet.constants import Reliability
    server = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    server.bind(('127.0.0.1', 0))
    server.settimeout(1)
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.connect(server.getsockname())
    sock.setblocking(False)
    session = RakNetSession(SimpleNamespace(mtu=1200, sock=sock, server_addr=server.getsockname()))
    try:
        session._handle_frame_set(FrameSet(sequence_number=2, frames=[Frame(body=b'a', reliability=Reliability.UNRELIABLE)]).encode())
        assert set(session._missing_sequences) == {0,1}
        session._handle_frame_set(FrameSet(sequence_number=0, frames=[]).encode())
        assert set(session._missing_sequences) == {1}
        session._missing_sequences[1] = time.monotonic()-1
        await session._flush_nacks()
        data,_ = await asyncio.to_thread(server.recvfrom,4096)
        assert [r.start for r in parse_nack(data)] == [1]
        assert not session._missing_sequences  # retransmissions use a new datagram sequence
    finally:
        sock.close()
        server.close()
