"""Receive disorder must not lose acknowledged handshake/encrypted messages."""
from types import SimpleNamespace
from unittest.mock import Mock

import pytest

from proloader.net.raknet.constants import Reliability
from proloader.net.raknet.frames import Frame
from proloader.net.raknet.session import RakNetSession


def make_session():
    return RakNetSession(SimpleNamespace(mtu=1200, sock=Mock(), server_addr=('127.0.0.1', 19132)))


def ordered(body, reliable, order=0, channel=0, **kwargs):
    return Frame(body=body, reliability=Reliability.RELIABLE_ORDERED,
                 frame_index=reliable, order_index=order, order_channel=channel, **kwargs)


async def test_late_reliable_unordered_packet_is_not_lost():
    session = make_session()
    for i in (2, 0, 1, 2, 0):
        session._handle_frame(Frame(body=b'\xfe' + bytes([i]), reliability=Reliability.RELIABLE,
                                    frame_index=i))
    assert [session._inbox.get_nowait() for _ in range(3)] == [b'\xfe\x02', b'\xfe\x00', b'\xfe\x01']
    assert session._inbox.empty()


async def test_ordered_packets_wait_and_replays_are_ignored():
    session = make_session()
    session._handle_frame(ordered(b'\xfesecond', 1, 1))
    assert session._inbox.empty()
    session._handle_frame(ordered(b'\xfefirst', 0, 0))
    assert session._inbox.get_nowait() == b'\xfefirst'
    assert session._inbox.get_nowait() == b'\xfesecond'
    session._handle_frame(ordered(b'\xfereplay', 2, 0))
    assert session._inbox.empty()


async def test_online_control_consumes_order_before_delivering_next_message():
    session = make_session()
    session._handle_online_packet = Mock()
    session._handle_frame(ordered(b'\xfelogin-handshake', 1, 1))
    session._handle_frame(ordered(b'\x10accepted', 0, 0))
    session._handle_online_packet.assert_called_once_with(b'\x10accepted')
    assert session._inbox.get_nowait() == b'\xfelogin-handshake'


async def test_reversed_split_survives_interleaved_newer_reliable_message():
    session = make_session()
    parts = [ordered(body, i, is_split=True, split_count=2, split_id=7, split_index=i)
             for i, body in enumerate((b'\xfehand', b'shake'))]
    session._handle_frame(parts[1])
    session._handle_frame(Frame(body=b'\xfelater', reliability=Reliability.RELIABLE, frame_index=2))
    session._handle_frame(parts[0])
    assert session._inbox.get_nowait() == b'\xfelater'
    assert session._inbox.get_nowait() == b'\xfehandshake'
    for part in parts:
        session._handle_frame(part)
    assert not session._split_buffer
    assert session._inbox.empty()


async def test_channels_and_u24_wrap():
    session = make_session()
    session._order_recv[0] = 0xFFFFFF
    session._handle_frame(ordered(b'\xfeafter-wrap', 3, 0))
    session._handle_frame(ordered(b'\xfeother-channel', 1, 0, channel=1))
    session._handle_frame(ordered(b'\xfebefore-wrap', 2, 0xFFFFFF))
    assert [session._inbox.get_nowait() for _ in range(3)] == [
        b'\xfeother-channel', b'\xfebefore-wrap', b'\xfeafter-wrap']
    assert session._order_recv == {0: 1, 1: 1}


async def test_reliable_dedup_history_is_bounded():
    session = make_session()
    session._deliver_body = Mock()
    for i in range(8200):
        session._handle_frame(Frame(body=b'\xfe', reliability=Reliability.RELIABLE, frame_index=i))
    assert len(session._reliable_seen) == len(session._reliable_history) == 8192
