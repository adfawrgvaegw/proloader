import asyncio
from unittest.mock import Mock, AsyncMock

import pytest
import requests

from proloader.net.netease.client import NeteaseClient, RentalWorldPending, RentalWorldEnterError
from proloader.net.netease.runtime import _enter_world, RentalError


@pytest.mark.parametrize('port', [0, None, ''])
@pytest.mark.parametrize('host', ['', None])
def test_starting_world_is_not_a_password_error(port,host):
    client = NeteaseClient()
    client._api_request_plain = Mock(return_value=dict(code=0, entity=dict(
        state=0, async_progress=1, total_progress=4, mcserver_host=host, mcserver_port=port)))
    try:
        with pytest.raises(RentalWorldPending) as error:
            client.enter_rental_server_world('id', 'PRIVATE')
        assert (error.value.progress, error.value.total) == (1, 4)
        assert 'PRIVATE' not in str(error.value)
    finally:
        client._session.close()


@pytest.mark.parametrize('response', [
    dict(code=9, message='PRIVATE', entity=dict(mcserver_host='old', mcserver_port=19132)),
    dict(code=0, entity=dict(state=1, mcserver_host='')),
    dict(code=0, entity=dict(mcserver_host='host', mcserver_port=65536)),
    dict(code=0, entity=dict(mcserver_host='host', mcserver_port=True)),
    dict(code=0, entity=dict(mcserver_host='host', mcserver_port='PRIVATE')),
    dict(code=0, entity='PRIVATE'),
])
def test_rejected_or_invalid_endpoint_does_not_leak_response(response):
    client = NeteaseClient()
    client._api_request_plain = Mock(return_value=response)
    try:
        with pytest.raises(RentalWorldEnterError) as error:
            client.enter_rental_server_world('id', 'PRIVATE')
        assert 'PRIVATE' not in str(error.value)
    finally:
        client._session.close()


async def test_pending_then_ready_polls_without_refresh_or_game_connection(monkeypatch):
    client = Mock()
    client.enter_rental_server_world.side_effect = [RentalWorldPending(0,1,4), ('host',19132)]
    pause = AsyncMock()
    monkeypatch.setattr('proloader.net.netease.runtime.asyncio.sleep', pause)
    event, stage = Mock(), Mock()
    assert await _enter_world(client, 'id', 'PRIVATE', stage, event) == ('host',19132)
    assert client.enter_rental_server_world.call_count == 2
    client.authenticate_with_cookie.assert_not_called()
    pause.assert_awaited_once()
    event.assert_called_once_with('world_starting', state=0, progress=1, total=4)


async def test_pending_has_a_deadline_and_safe_error():
    client = Mock()
    client.enter_rental_server_world.side_effect = RentalWorldPending(0,1,4)
    with pytest.raises(RentalError, match='启动') as error:
        await _enter_world(client, 'id', 'PRIVATE', Mock(), Mock(), wait_seconds=0)
    assert client.enter_rental_server_world.call_count == 1
    assert 'PRIVATE' not in str(error.value)


@pytest.mark.parametrize('error', [RentalWorldEnterError(7), requests.Timeout('PRIVATE'),
                                 requests.ConnectionError('PRIVATE')])
async def test_rejection_and_network_errors_are_not_retried(error):
    client = Mock()
    client.enter_rental_server_world.side_effect = error
    with pytest.raises(RentalError) as caught:
        await _enter_world(client, 'id', 'PRIVATE', Mock(), Mock())
    assert 'PRIVATE' not in str(caught.value)
    assert client.enter_rental_server_world.call_count == 1


async def test_startup_wait_can_be_cancelled(monkeypatch):
    client = Mock()
    client.enter_rental_server_world.side_effect = RentalWorldPending(0,1,4)
    monkeypatch.setattr('proloader.net.netease.runtime.asyncio.sleep', AsyncMock(side_effect=asyncio.CancelledError))
    with pytest.raises(asyncio.CancelledError):
        await _enter_world(client, 'id', 'PRIVATE', Mock(), Mock())
    assert client.enter_rental_server_world.call_count == 1
