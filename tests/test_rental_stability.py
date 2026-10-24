"""Acceptance cannot count reconnects, failed probes, or incomplete cleanup."""
from contextlib import asynccontextmanager
from types import SimpleNamespace
from unittest.mock import AsyncMock
import asyncio
import socket

import pytest

from proloader.net.raknet.session import RakNetSession
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
