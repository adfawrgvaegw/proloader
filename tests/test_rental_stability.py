"""Acceptance cannot count reconnects, failed probes, or incomplete cleanup."""
from contextlib import asynccontextmanager
from types import SimpleNamespace
from unittest.mock import AsyncMock
import asyncio
import socket

import pytest

from proloader.net.raknet.session import RakNetSession
from proloader.net.raknet.frames import Frame, Reliability
from tools.diagnostics import verify_rental_stability as acceptance


async def test_idle_udp_receiver_cancels_without_executor_thread():
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    server = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    server.bind(('127.0.0.1', 0))
    session = RakNetSession(SimpleNamespace(mtu=1200, sock=sock, server_addr=server.getsockname()))
    await session.start()
    await asyncio.sleep(0.01)
    await asyncio.wait_for(session.stop(), 1)
    assert all(t.done() for t in session._tasks)
    assert sock.fileno() == -1
    server.close()


async def test_peer_disconnect_wakes_receive_and_rejects_new_sends():
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    session = RakNetSession(SimpleNamespace(mtu=1200, sock=sock, server_addr=('127.0.0.1',19132)))
    waiter = asyncio.create_task(session.recv())
    await asyncio.sleep(0)
    session._handle_frame(Frame(body=b'\x15', reliability=Reliability.RELIABLE))
    with pytest.raises(ConnectionError):
        await asyncio.wait_for(waiter, .5)
    assert session.close_reason == 'peer_disconnect'
    with pytest.raises(ConnectionError):await session.send_reliable(b'no replay')
    await session.stop()
    assert session.close_reason == 'peer_disconnect'
    assert sock.fileno() == -1


async def test_udp_disconnect_and_socket_termination_preserve_first_reason():
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    session = RakNetSession(SimpleNamespace(mtu=1200, sock=sock, server_addr=('127.0.0.1',19132)))
    session._dispatch(b'\x15')
    session._terminate('socket_error')
    with pytest.raises(ConnectionError):await session.recv()
    assert session.close_reason == 'peer_disconnect'
    await session.stop()


@pytest.mark.parametrize('probe,clean,passes', [('', True, True), (None, True, False), ('ok', False, False)])
async def test_first_failure_stops_and_empty_success_is_accepted(tmp_path, monkeypatch, probe, clean, passes):
    calls = []
    monkeypatch.setattr(acceptance, 'BotStore', lambda: SimpleNamespace(list=lambda: [SimpleNamespace(id='bot')]))
    @asynccontextmanager
    async def lifecycle(store, bot, code, password, on_event):
        calls.append(1)
        try:
            yield SimpleNamespace(run_command_ai=AsyncMock(return_value=probe)), 'player'
        finally:
            on_event(dict(event='cleanup', socket_closed=clean, tasks_finished=clean))
    monkeypatch.setattr(acceptance, 'rental_session', lifecycle)
    args=SimpleNamespace(bot='bot', server='server', hold=0, interval=0, rounds=2, output=tmp_path/'report.json')
    assert await acceptance.run(args, 'private') == (0 if passes else 1)
    assert len(calls) == (2 if passes else 1)
    assert 'private' not in args.output.read_text()
