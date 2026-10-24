"""终止事件必须中断等待，不能伪装成超时或继续消费缓存成功包。"""
import asyncio
from unittest.mock import AsyncMock

import pytest

from proloader.net.bedrock.batch import encode_batch
from proloader.net.bedrock.login import parse_disconnect
from proloader.net.bedrock.session import BedrockSession, BedrockLoginError, BedrockDisconnectedError
from proloader.net.raknet.wire import ByteWriter


def disconnect(reason=7, hide=False, message='server refusal'):
    w = ByteWriter()
    w.write_unsigned_varint(5)
    w.write_varint(reason)
    w.write_bool(hide)
    if not hide:
        w.write_string(message)
    return w.to_bytes()


@pytest.mark.parametrize('hide', [True, False])
def test_disconnect_prefix(hide):
    d = parse_disconnect(disconnect(-3, hide, '拒绝连接'))
    assert (d.reason, d.hide_screen, d.message) == (-3, hide, '' if hide else '拒绝连接')


async def test_disconnect_before_wait_discards_stale_success():
    session = BedrockSession(AsyncMock())
    session._dispatch_packet(b'\x02\x00\x00\x00\x00')
    session._dispatch_packet(disconnect(message='PRIVATE_SERVER_TEXT'))
    with pytest.raises(BedrockDisconnectedError) as exc:
        await session._wait_packet(2, 10, 'PlayStatus')
    assert exc.value.reason == 7
    assert exc.value.server_message == 'PRIVATE_SERVER_TEXT'
    assert 'PRIVATE' not in str(exc.value)
    with pytest.raises(BedrockDisconnectedError):
        await session.send_packet(b'\x04')
    session._raknet.send_reliable.assert_not_called()


async def test_disconnect_wakes_all_waiters():
    session = BedrockSession(AsyncMock())
    packet_wait = asyncio.create_task(session._wait_packet(11, 30, 'StartGame'))
    public_waits = [asyncio.create_task(session.recv_packet()) for _ in range(2)]
    command = asyncio.get_running_loop().create_future()
    session._ai_command_futures['pending-command'] = command
    await asyncio.sleep(0)
    session._dispatch_packet(disconnect())
    results = await asyncio.wait_for(asyncio.gather(packet_wait, *public_waits, command,
                                                   return_exceptions=True), 0.5)
    assert all(isinstance(result, BedrockDisconnectedError) for result in results)
    assert not session._waiting
    await session.close()
    assert isinstance(session._terminal_error, BedrockDisconnectedError)


@pytest.mark.parametrize('packet', [b'\x05', b'\x05\x00\x02', b'\x05\x00\x00\x10a'])
async def test_malformed_disconnect_is_terminal(packet):
    session = BedrockSession(AsyncMock())
    session._dispatch_packet(packet)
    with pytest.raises(BedrockLoginError, match='Disconnect 包格式无效'):
        await session._wait_packet(2, 30, 'PlayStatus')


async def test_receive_exception_wakes_waiter_without_leaking_details():
    raknet = AsyncMock()
    raknet.recv.side_effect = OSError('PRIVATE_EXCEPTION')
    session = BedrockSession(raknet)
    waiting = asyncio.create_task(session._wait_packet(143, 30, 'NetworkSettings'))
    await asyncio.sleep(0)
    await session._recv_loop()
    with pytest.raises(BedrockLoginError, match='RakNet 收包失败') as exc:
        await asyncio.wait_for(waiting, 0.5)
    assert 'PRIVATE' not in str(exc.value)


async def test_corrupt_encrypted_batch_is_terminal():
    session = BedrockSession(AsyncMock())
    crypto = AsyncMock()
    crypto.decrypt = lambda data: (_ for _ in ()).throw(ValueError('PRIVATE_CHECKSUM'))
    session._crypto = crypto
    session._raknet.recv.return_value = b'\xfeanything'
    await session._recv_loop()
    with pytest.raises(BedrockLoginError, match='Batch 解密/解压失败'):
        await session.recv_packet()
    session._raknet.recv.assert_awaited_once()


async def test_disconnect_stops_dispatch_of_rest_of_batch():
    raknet = AsyncMock()
    raknet.recv.return_value = encode_batch([disconnect(), b'\x02\x00\x00\x00\x00'], compression_enabled=False)
    session = BedrockSession(raknet)
    await session._recv_loop()
    assert session._inbox.qsize() == 1  # 只有终止哨兵，不含 Disconnect 后的成功包。
    raknet.recv.assert_awaited_once()


async def test_explicit_close_wakes_waiter_and_joins_receive_task():
    raknet = AsyncMock()
    pending = asyncio.get_running_loop().create_future()
    async def recv():
        return await pending
    raknet.recv.side_effect = recv
    session = BedrockSession(raknet)
    session._recv_task = asyncio.create_task(session._recv_loop())
    waiting = asyncio.create_task(session._wait_packet(2, 30, 'PlayStatus'))
    await asyncio.sleep(0)
    await session.close()
    with pytest.raises(BedrockLoginError, match='会话已关闭'):
        await asyncio.wait_for(waiting, 0.5)
    assert session._recv_task.done()
    await session.close()
    raknet.stop.assert_awaited_once()


async def test_disconnect_propagates_through_ai_command_and_cleans_future():
    session = BedrockSession(AsyncMock())
    async def send(packet):
        session._dispatch_packet(disconnect())
    session._send_packet = send
    with pytest.raises(BedrockDisconnectedError):
        await session.run_command_ai('list')
    assert session._ai_command_futures == {}


async def test_send_failure_cleans_ai_command_future():
    session = BedrockSession(AsyncMock())
    session._send_packet = AsyncMock(side_effect=OSError('send failed'))
    with pytest.raises(OSError):
        await session.run_command_ai('list')
    assert session._ai_command_futures == {}
