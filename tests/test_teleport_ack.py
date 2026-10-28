import asyncio,json,struct,subprocess
from unittest.mock import AsyncMock
import pytest
from proloader.net.bedrock.movement import build_teleport_ack,read_player_teleport
from proloader.net.bedrock.packets import wrap_packet,unwrap_packet,PacketID
from proloader.net.raknet.wire import ByteWriter
from proloader.net.bedrock.session import BedrockSession
from proloader.block_entities import codec_binary


def move(runtime=7,mode=2,position=(12027.5,88.62,12228.5)):
    w=ByteWriter();w.write_unsigned_varlong(runtime);w.write(struct.pack('<fff',*position));w.write(bytes(12))
    w.write_byte(mode);w.write_bool(False);w.write_unsigned_varlong(0)
    if mode==2:w.write(bytes(8))
    w.write_unsigned_varlong(0)
    return wrap_packet(19,w.to_bytes())


@pytest.mark.parametrize('protocol',[686,860])
@pytest.mark.parametrize('position',[(12027.5,88.62,12228.5),(-17.5,-60.25,-33.5)])
def test_teleport_ack_matches_pinned_profile_codecs(position,protocol):
    out=subprocess.run([str(codec_binary())],input=json.dumps(dict(action='teleport_ack',protocol=protocol,player_position=position)),
                       capture_output=True,text=True,check=True)
    expected=json.loads(out.stdout)[0]
    pid,body=unwrap_packet(build_teleport_ack(position,protocol))
    assert pid==expected['id']==144 and body.hex()==expected['hex']
    assert PacketID.MOVE_PLAYER==19


async def test_only_own_teleport_is_acknowledged_and_tasks_finish():
    s=BedrockSession(None);s._netease_mode=True;s._netease_protocol=860;s._entity_runtime_id=7;s._teleport_ack_supported=True
    s.send_packet=AsyncMock()
    for packet in [move(runtime=8),move(mode=0),move(),move()[:-2]]:s._dispatch_packet(packet)
    await asyncio.sleep(0);await asyncio.sleep(0)
    assert s.send_packet.call_count==1 and s._teleport_ack_count==1 and not s._teleport_ack_tasks
    assert s.send_packet.call_args.args[0]==build_teleport_ack(s._last_server_position)
    await s.close()


def test_malformed_move_is_not_an_acknowledged_teleport():
    with pytest.raises(EOFError):read_player_teleport(move()[:-2])
    with pytest.raises(ValueError):build_teleport_ack((float('nan'),0,0))


async def test_teleport_notifications_distinguish_importer_moves_and_other_players():
    s=BedrockSession(None);s._netease_mode=True;s._netease_protocol=860;s._entity_runtime_id=7;s._teleport_ack_supported=True
    s.send_packet=AsyncMock();queue=s.subscribe_player_teleports()
    s._import_teleport_target=(12027,88,12228)
    s._dispatch_packet(move(runtime=8));assert queue.empty()
    s._dispatch_packet(move());assert (await queue.get())['importer_move']
    s._import_teleport_target=None
    s._dispatch_packet(move(position=(-.5,81,-16.5)))
    event=await queue.get()
    assert event['position']==(-.5,81,-16.5) and not event['importer_move']
    s.unsubscribe_player_teleports(queue)
    await asyncio.sleep(0);await s.close()
