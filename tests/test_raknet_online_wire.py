import asyncio
import struct
from types import SimpleNamespace
from unittest.mock import Mock, AsyncMock

import pytest

from proloader.net.raknet.connection import SystemAddress, build_open_connection_request_2
from proloader.net.raknet.frames import Frame, FrameSet
from proloader.net.raknet.constants import Reliability
from proloader.net.raknet.session import (
    RakNetSession, OnlineAddress, build_connection_request, build_connected_ping,
    parse_connected_pong, parse_connection_request_accepted,
)
from proloader.net.raknet.wire import ByteWriter


def test_online_guid_matches_offline_and_timestamps_are_network_order():
    guid, at = -0x1020304050607, 0x0102030405060708
    offline = build_open_connection_request_2(SystemAddress('127.0.0.1', 19132), 1200, guid)
    online = build_connection_request(guid, at)
    assert online[1:9] == offline[-8:]
    assert online[9:17] == bytes.fromhex('0102030405060708')
    assert build_connected_ping(at) == b'\0' + bytes.fromhex('0102030405060708')
    pong = parse_connected_pong(b'\x03' + struct.pack('>QQ', at, at + 100))
    assert (pong.ping_time, pong.pong_time) == (at, at + 100)


def test_secure_open_request_has_five_byte_cookie_section():
    data = build_open_connection_request_2(SystemAddress('127.0.0.1', 19132), 1200, -42,
                                           cookie=0x01020304, has_security=True)
    assert len(data) == 39
    assert data[17:22] == b'\x01\x02\x03\x04\x00'
    assert data[22] == 4
    assert struct.unpack('>Hq', data[-10:]) == (1200, -42)


@pytest.mark.parametrize('count', [10, 20])
async def test_accepted_and_new_incoming_timestamp_semantics(count):
    writer = ByteWriter()
    writer.write_byte(0x10)
    OnlineAddress('127.0.0.1', 19132).encode(writer)
    writer.write(struct.pack('>H', 0x0102))
    for _ in range(count):
        OnlineAddress().encode(writer)
    writer.write(struct.pack('>QQ', 12345, 67890))
    accepted = parse_connection_request_accepted(writer.to_bytes())
    assert (accepted.system_index, accepted.request_time, accepted.reply_time) == (258, 12345, 67890)
    session = RakNetSession(SimpleNamespace(mtu=1200, sock=Mock(), server_addr=('127.0.0.1', 19132), client_guid=-42, server_guid=42))
    session._accepted = accepted
    session._handshake_done.set()
    session.send_reliable = AsyncMock()
    await session.connect()
    calls = session.send_reliable.call_args_list
    assert calls[0].kwargs == {'ordered': False}
    assert calls[1].kwargs == {'ordered': True}
    assert struct.unpack('>QQ', calls[1].args[0][-16:])[0] == 67890


@pytest.mark.parametrize('reliability', [Reliability.UNRELIABLE_SEQUENCED, Reliability.RELIABLE_SEQUENCED])
def test_sequenced_header_has_order_index_and_channel(reliability):
    # Independent wire bytes: flags, bit-length, reliable?, sequence, order, channel, body.
    raw = bytes([reliability << 5]) + b'\x00\x10'
    if reliability == Reliability.RELIABLE_SEQUENCED:
        raw += b'\x01\x00\x00'
    raw += b'\x02\x00\x00\x03\x00\x00\x04\xfe\x55'
    decoded = FrameSet.decode(b'\x80\0\0\0' + raw)
    frame = decoded.frames[0]
    assert (frame.sequence_index, frame.order_index, frame.order_channel, frame.body) == (2, 3, 4, b'\xfe\x55')
    assert decoded.encode() == b'\x80\0\0\0' + raw
