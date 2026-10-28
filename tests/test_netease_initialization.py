"""初始化序列对齐 Fatalder；故意让 unique ID 与 runtime ID 不同。"""
import struct
from unittest.mock import AsyncMock

import pytest

from proloader.net.bedrock.login import (
    StartGameData, build_request_chunk_radius, parse_network_settings, parse_start_game,
)
from proloader.net.bedrock.netease_packets import build_all_netease_login_packets, parse_py_rpc
from proloader.net.bedrock.session import BedrockSession, BedrockLoginError
from proloader.net.raknet.wire import ByteReader, ByteWriter


def start_game_prefix(unique=-12345, runtime=98765):
    w = ByteWriter()
    w.write_unsigned_varint(11)
    w.write_varlong(unique)
    w.write_unsigned_varlong(runtime)
    w.write_varint(1)
    return w.to_bytes()


def test_686_network_settings_and_start_game_prefix():
    # network_settings.go 的 LE u16/u16/bool/u8/f32 布局。
    assert parse_network_settings(b'\x8f\x01' + struct.pack('<HHBBf', 256, 2, 1, 5, 0.5)) == (256, 2)
    game = parse_start_game(start_game_prefix())
    assert (game.entity_unique_id, game.entity_runtime_id, game.player_game_type) == (-12345, 98765, 1)


@pytest.mark.parametrize('protocol,expected', [(686, b'\x45\x08\x80\x01'), (860, b'\x45\x08\x40')])
def test_chunk_radius_layout(protocol, expected):
    assert build_request_chunk_radius(4, 64, True, protocol=protocol) == expected


def test_components_follow_nullable_pointer_semantics():
    packets = build_all_netease_login_packets(42, -12345, 'skin', {'mod-a': 0, 'mod-b': None, 'mod-c': 7})
    assert len(packets) == 9
    assert packets[0] == b'\x81\x01\x00'
    r = ByteReader(packets[1]); assert r.read_unsigned_varint() == 203
    assert r.read_string() == '{"eventName":"LOGIN_UID","resid":"","uid":"42"}'
    rpc = [parse_py_rpc(p) for p in packets[2:]]
    assert [p['name'] for p in rpc] == [
        'e', 'SyncUsingMod', 'SyncVipSkinUuid', 'ClientLoadAddonsFinishedFromGac',
        'ModEventC2S', 'arenaGamePlayerFinishLoad', 'ModEventC2S',
    ]
    assert all(p['operation_type'] == 0x05DB23AE for p in rpc)
    assert rpc[1]['payload'] == [['mod-a', 'mod-b', 'mod-c'], 'skin', '', True, {'mod-a': 0, 'mod-c': 7}]
    assert rpc[4]['payload'] == ['Minecraft', 'Preset', 'GetLoadedInstances', {'playerId': '-12345'}]
    assert rpc[6]['payload'] == ['Minecraft', 'VIPEventSystem', 'PlayerUiInit', '-12345']


@pytest.mark.parametrize('components', [{'x': True}, {'x': '7'}, {'': 1}, {'x': 2**63}])
def test_reject_invalid_components(components):
    with pytest.raises(ValueError, match='BotComponent'):
        build_all_netease_login_packets(1, 2, bot_components=components)


@pytest.mark.parametrize('protocol', [686, 860])
async def test_initialization_order_and_distinct_entity_ids(protocol):
    # 登录已由 test_netease_login 验证；此处从资源包协商起模拟服务端。
    session = BedrockSession(AsyncMock())
    session._netease_mode = True
    session._netease_uid = 42
    session._netease_protocol = protocol
    session._netease_bot_components = {'mod-a': 0, 'mod-b': None}
    sent = []
    async def send(packet):
        sent.append(packet)
    session._send_packet = send
    received = []
    async def wait(pid, timeout, name):
        received.append(pid)
        if pid == 11:
            return start_game_prefix()
        if pid == 70:
            return b'\x46\x0a'  # Server accepted radius 5 (signed VarInt).
        if pid == 2:
            # 直到 PLAYER_SPAWN 到达之前，不能发生成确认或附加事件。
            assert [session._read_packet_id(p)[0] for p in sent] == [8, 8, 69]
            return b'\x02' + struct.pack('>i', 3)
        return b''
    session._wait_packet = wait
    game = await session.initialize_game()
    assert received == [6, 7, 11, 70, 2]
    assert game == StartGameData(-12345, 98765, 1, '')
    assert session._chunk_radius == 5
    assert [session._read_packet_id(p)[0] for p in sent] == [8, 8, 69, 113, 129, 203] + [200] * 7
    assert sent[:2] == [b'\x08\x03\x00\x00', b'\x08\x04\x00\x00']
    assert sent[2] == (b'\x45\x20\x00' if protocol == 686 else b'\x45\x08\x40')
    r = ByteReader(sent[3]); assert r.read_unsigned_varint() == 113
    assert r.read_unsigned_varlong() == 98765
    rpc = [parse_py_rpc(p) for p in sent[6:]]
    assert rpc[4]['payload'][3] == {'playerId': '-12345'}
    assert rpc[6]['payload'][3] == '-12345'
    assert rpc[1]['payload'][4] == {'mod-a': 0}


async def test_refuse_additional_packets_before_start_game():
    session = BedrockSession(AsyncMock())
    session._send_packet = AsyncMock()
    with pytest.raises(BedrockLoginError, match='StartGame'):
        await session._send_netease_login_packets()
    session._send_packet.assert_not_called()
