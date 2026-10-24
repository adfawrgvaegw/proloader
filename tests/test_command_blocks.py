"""Cross-language codec, standard-file, and durable NBT integration checks."""
import asyncio
import base64
from contextlib import asynccontextmanager
from dataclasses import replace
import json
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import AsyncMock

import numpy as np
import pytest

from proloader.command_blocks import CommandBlockData, plan_command_blocks, decode_subchunk
from proloader.errors import ParseError
from proloader.fatalder import native_binary
from proloader.importer import ImportPlan, WorldCommands, execute_import
from proloader.model.block import Block, BlockState
from proloader.model.palette import BlockPalette
from proloader.model.region import Region
from proloader.model.structure import Structure
from proloader.net.bedrock.command_block import build_command_block_update, build_subchunk_request
from proloader.net.bedrock.packets import wrap_packet, unwrap_packet
from proloader.net.bedrock.session import BedrockSession, BedrockLoginError
from proloader.parser.mcstructure import parse_mcstructure_bytes, write_mcstructure
from proloader.parser.nbt import dump_nbt, parse_nbt_auto

ROOT = Path(__file__).resolve().parents[1]


def raw_structure():
    # Independently specified file order, not Proloader's writer/index helpers.
    names = ['minecraft:stone', 'minecraft:dirt', 'minecraft:gold_block']
    indices = [0,1,2,0, 1,2,0,1, 2,0,1,2, 0,1,2,0, 1,2,0,1, 2,0,1,2]
    nbt = dict(id='CommandBlock', Command='say 你好', CustomName='test', auto=1,
               TickDelay=7, ExecuteOnFirstTick=1, TrackOutput=1, conditionalMode=0,
               x=101, y=-58, z=202)
    return dict(format_version=1, size=[2,3,4], structure_world_origin=[100,-60,200],
        structure=dict(block_indices=[indices, [-1]*24], palette=dict(default=dict(
            block_palette=[dict(name=n, states={}, version=18153475) for n in names],
            block_position_data={'23':dict(block_entity_data=nbt)}))))


def test_standard_xyz_index_and_numeric_entity_index():
    root = raw_structure()
    data = dump_nbt(root)
    data = data[:1]+b'\0\0'+data[1:]
    s = parse_mcstructure_bytes(data)
    names = ['minecraft:stone', 'minecraft:dirt', 'minecraft:gold_block']
    for x in range(2):
        for y in range(3):
            for z in range(4):
                assert s.palette.get(int(s.region.blocks[x,y,z])).state.name == names[(x*12+y*4+z)%3]
    assert set(s.block_entities) == {(1,2,3)}
    assert s.block_entities[1,2,3]['Command'] == 'say 你好'
    encoded = write_mcstructure(s)
    assert encoded[:3] == b'\x0a\x00\x00'
    out = parse_nbt_auto(encoded)
    positions = out['structure']['palette']['default']['block_position_data']
    assert positions['23']['block_entity_data']['x'] == 101
    assert positions['23']['block_entity_data']['y'] == -58
    assert positions['23']['block_entity_data']['z'] == 203
    out_indices = out['structure']['block_indices'][0]
    out_palette = out['structure']['palette']['default']['block_palette']
    assert [out_palette[i]['name'] for i in out_indices] == [names[i] for i in root['structure']['block_indices'][0]]


def test_real_upstream_command_block_keeps_data_and_source_auto():
    p = ROOT/'third_party/fatalder/modules/FunShuttler/core/minecraft/protocol/block_actors/test/mcstructure/command_block/command_block.mcstructure'
    s = parse_mcstructure_bytes(p.read_bytes())
    assert len(s.block_entities) == 1
    assert s.block_entities[0,0,0]['auto'] == 1
    data = plan_command_blocks(s, (-17,-60,-33))[0]
    assert not data.needs_redstone
    assert data.position == (-17,-60,-33) and data.block_name == 'minecraft:chain_command_block'


@pytest.mark.parametrize('index', ['-1','24','bad'])
def test_bad_entity_positions_are_not_silently_dropped(index):
    r = raw_structure()
    r['structure']['palette']['default']['block_position_data'] = {index:dict(block_entity_data={})}
    with pytest.raises(ParseError):parse_mcstructure_bytes(dump_nbt(r))


@pytest.mark.parametrize('protocol', [686,860])
@pytest.mark.parametrize('mode', [0,1,2])
@pytest.mark.parametrize('position', [(-17,-60,-33),(123,78,456)])
def test_update_bytes_match_real_upstream_go_marshal(mode, position, protocol):
    import subprocess
    update = dict(Block=True, Position=position, Mode=mode, NeedsRedstone=True, Conditional=mode==2,
        Command='say 你好', LastOutput='', Name='中文名称', ShouldTrackOutput=False,
        TickDelay=2147483647, ExecuteOnFirstTick=False)
    result = subprocess.run([str(native_binary('nbt'))], input=json.dumps(dict(action='encode_update',update=update,protocol=protocol)),
        capture_output=True,text=True,check=True)
    packet = build_command_block_update(position,mode=mode,needs_redstone=True,conditional=mode==2,
        command='say 你好',name='中文名称',track_output=False,tick_delay=2147483647,protocol=protocol)
    assert unwrap_packet(packet)[1].hex() == json.loads(result.stdout)['hex']


@pytest.mark.parametrize('position,dimension', [((-17,-60,-33),0),((48,78,67),1),((1,200,1),2)])
def test_subchunk_request_matches_upstream(position,dimension):
    import subprocess
    result = subprocess.run([str(native_binary('nbt'))],input=json.dumps(dict(action='encode_request',position=position,dimension=dimension)),capture_output=True,text=True,check=True)
    assert unwrap_packet(build_subchunk_request(position,dimension))[1].hex() == json.loads(result.stdout)['hex']


@pytest.mark.parametrize('kind', ['plain','tagged'])
def test_native_decodes_two_distinct_upstream_entities(kind):
    fixture = json.loads((ROOT/f'tests/fixtures/command_block_wire/{kind}.json').read_text())
    decoded = decode_subchunk(wrap_packet(174,base64.b64decode(fixture['data'])))
    assert decoded == fixture['decoded']
    nbt = decoded['entries'][0]['nbt']
    assert [n['x'] for n in nbt] == [-17,-16]
    assert all(n['Command']=='say hello' for n in nbt)
    if kind=='tagged':assert 'LPCommandMode' in nbt[0] and '__tag' not in nbt[0]


def make_plan(tmp_path, edition='bedrock'):
    palette = BlockPalette()
    bid = palette.add(Block(BlockState('minecraft:chain_command_block', {'facing_direction':5,'conditional_bit':True})))
    blocks = np.full((1,1,1),bid,dtype=np.uint32)
    source = tmp_path/'command.mcstructure';source.write_bytes(b'fixture')
    nbt = dict(Command='say 你好', CustomName='test', auto=1, TickDelay=7,
               ExecuteOnFirstTick=1, TrackOutput=1, conditionalMode=0)
    s = Structure('command',(1,1,1),palette,Region(palette,blocks),{(0,0,0):nbt})
    if edition=='java':s.source_info['source_format']='Sponge v3'
    return ImportPlan(source,s,(-17,78,-33),'overworld',
        [('setblock',-17,78,-33,-17,78,-33,'minecraft:chain_command_block',{'facing_direction':5,'conditional_bit':True})],'source-hash')


def expected_nbt(block):
    return dict(id='CommandBlock',Command=block.command,CustomName=block.name,auto=0,
        TickDelay=block.tick_delay,ExecuteOnFirstTick=0,TrackOutput=int(block.track_output),
        LPCondionalMode=int(block.conditional),LPCommandMode=2)


def test_nbt_fingerprint_includes_command_activation_delay_tracking_and_facing(tmp_path):
    p = make_plan(tmp_path)
    before = p.summary()['plan_sha256']
    for change in [dict(command='say changed'),dict(needs_redstone=True),dict(tick_delay=8),
                   dict(track_output=False),dict(execute_on_first_tick=False),dict(states={'facing_direction':3})]:
        block = p.command_blocks[0]
        p.command_blocks[0] = replace(block,**change)
        assert p.summary()['plan_sha256'] != before
        p.command_blocks[0] = block


def test_stage_does_not_enable_source_auto_and_missing_readback_fields_fail(tmp_path):
    block = make_plan(tmp_path).command_blocks[0]
    assert not block.needs_redstone and block.execute_on_first_tick
    assert not block.differences(expected_nbt(block))
    assert block.differences({})
    with pytest.raises(ValueError,match='Java'):make_plan(tmp_path,'java').command_blocks[0].packet()


async def test_exact_command_block_check_includes_facing_and_condition(tmp_path):
    p = make_plan(tmp_path)
    session = SimpleNamespace(run_command_ai=AsyncMock(return_value=''),_raise_if_failed=lambda:None)
    assert await WorldCommands(session,'bot',p).check_command_block(p.command_blocks[0])
    cmd = session.run_command_ai.call_args.args[0]
    assert '"facing_direction"=5' in cmd and '"conditional_bit"=true' in cmd


@pytest.mark.parametrize('fail', ['none','mismatch','disconnect','timeout'])
async def test_staging_is_separate_and_readback_controls_durable_completion(tmp_path,monkeypatch,fail):
    import proloader.importer as module
    p = make_plan(tmp_path);writes=[];shells=[];reads=[]
    current = dict(expected_nbt(p.command_blocks[0]),Command='old')
    class Session:
        _raknet=SimpleNamespace(_closed=False,_send_queue=asyncio.Queue(),_pending_acks={})
        def _raise_if_failed(self):pass
        async def send_packet(self,packet):writes.append(packet);current.update(expected_nbt(p.command_blocks[0]))
        async def read_block_entity(self,*args):
            reads.append(args)
            if len(reads)>1:
                if fail=='mismatch':return dict(current,TickDelay=999)
                if fail=='disconnect':raise BedrockLoginError('closed')
                if fail=='timeout':raise asyncio.TimeoutError
            return dict(current)
    @asynccontextmanager
    async def connection(*a,**kw):yield Session(),'bot'
    class World:
        def __init__(self,*a):pass
        async def send(self,*a,**kw):return ''
        async def teleport(self,*a):pass
        async def place(self,c,**kw):shells.append(c)
        async def check(self,*a):return True
        async def check_command_block(self,*a):return True
    monkeypatch.setattr(module,'rental_session',connection)
    monkeypatch.setattr(module,'WorldCommands',World)
    monkeypatch.setattr(module,'native_binary',lambda *a:Path('/unused'))
    kwargs=dict(verify='none',jobs_root=tmp_path/'jobs',command_block_mode='stage')
    if fail=='none':
        job=await execute_import(p,None,SimpleNamespace(id='bot'),'server','SECRET',**kwargs)
        assert job['state']=='sent' and job['command_blocks_state']=='staged_readback_verified'
        assert job['verification']=='not_performed' and job['command_blocks_next']==1
    else:
        with pytest.raises((ValueError,BedrockLoginError,asyncio.TimeoutError)):
            await execute_import(p,None,SimpleNamespace(id='bot'),'server','SECRET',**kwargs)
        path=next((tmp_path/'jobs').glob('*.json'));job=json.loads(path.read_text())
        assert job['state']=='failed' and job.get('command_blocks_next',0)==0
        assert job['command_blocks_inflight']==0
        # Lost ack after applying: resume reads the world and does not replay
        # shell placement or the already-correct update packet.
        reads.clear();count=len(shells)
        job=await execute_import(p,None,SimpleNamespace(id='bot'),'server','SECRET',resume=job['id'],**kwargs)
        assert len(shells)==count and len(writes)==1
        assert job['command_blocks_state']=='staged_readback_verified'
    assert len(writes)==1 and reads
    saved=next((tmp_path/'jobs').glob('*.json')).read_text()
    assert 'SECRET' not in saved and 'say 你好' not in saved


async def test_default_refuses_command_blocks_before_login(tmp_path,monkeypatch):
    import proloader.importer as module
    connect=AsyncMock();monkeypatch.setattr(module,'rental_session',connect)
    with pytest.raises(ValueError,match='--command-blocks'):
        await execute_import(make_plan(tmp_path),None,SimpleNamespace(id='bot'),'server','')
    connect.assert_not_called()


async def test_readback_routes_by_dimension_and_chunk_and_cleans_timeout(monkeypatch):
    session=BedrockSession(None)
    body=b'\x00\x00\x03\x00\x05' # cache=false, dimension0, center(-2,0,-3)
    packet=wrap_packet(174,body)
    async def send(request):
        session._dispatch_packet(wrap_packet(174,b'\x00\x02\x03\x00\x05')) # wrong dimension
        assert not session._subchunk_waiters[0,-2,0,-3].done()
        session._dispatch_packet(packet)
    monkeypatch.setattr(session,'send_packet',send)
    import proloader.command_blocks as module
    monkeypatch.setattr(module,'decode_subchunk',lambda p:dict(dimension=0,position=[-2,0,-3],entries=[dict(offset=[0,-4,0],result=1,nbt=[dict(x=-17,y=-60,z=-33,id='CommandBlock')])]))
    assert (await session.read_block_entity((-17,-60,-33)))['id']=='CommandBlock'
    assert not session._subchunk_waiters
    monkeypatch.setattr(session,'send_packet',AsyncMock())
    with pytest.raises(asyncio.TimeoutError):await session.read_block_entity((-17,-60,-33),timeout=0.01)
    assert not session._subchunk_waiters


async def test_disconnect_wakes_subchunk_waiter():
    session=BedrockSession(None)
    session.send_packet=AsyncMock()
    task=asyncio.create_task(session.read_block_entity((1,78,1),timeout=5))
    await asyncio.sleep(0)
    session._fail_session(BedrockLoginError('disconnected'))
    with pytest.raises(BedrockLoginError):await task
    assert not session._subchunk_waiters


def test_python_export_is_readable_by_real_waterstructure(tmp_path):
    import os
    import subprocess
    from proloader.parser.nbt import NBTByte, NBTLong
    root = raw_structure()
    nbt = root['structure']['palette']['default']['block_position_data']['23']['block_entity_data']
    nbt.update(Command='say test',auto=NBTByte(1),LastExecution=NBTLong(1<<40))
    s = parse_mcstructure_bytes(dump_nbt(root))
    output = tmp_path/'export.mcstructure';output.write_bytes(write_mcstructure(s))
    result = subprocess.run(['go','test','./cmd/proloader-nbt','-run','^TestMCStructureExport$','-count=1'],
        cwd=ROOT/'third_party/fatalder',capture_output=True,text=True,timeout=90,
        env=dict(os.environ,PROLOADER_MCSTRUCTURE_EXPORT=str(output)))
    assert result.returncode==0, result.stdout+result.stderr


@pytest.mark.parametrize('edition', ['bedrock','java'])
async def test_shell_only_opt_in_never_sends_nbt_packets(tmp_path,monkeypatch,edition):
    import proloader.importer as module
    p=make_plan(tmp_path,edition)
    packets=AsyncMock()
    @asynccontextmanager
    async def connect(*a,**kw):
        yield SimpleNamespace(_raise_if_failed=lambda:None,send_packet=packets,
            _raknet=SimpleNamespace(_closed=False,_send_queue=asyncio.Queue(),_pending_acks={})), 'bot'
    class World:
        def __init__(self,*a):pass
        async def send(self,*a,**kw):return ''
        async def teleport(self,*a):pass
        async def place(self,*a,**kw):pass
        async def check(self,*a):raise AssertionError('no checks expected')
    monkeypatch.setattr(module,'rental_session',connect)
    monkeypatch.setattr(module,'WorldCommands',World)
    job=await execute_import(p,None,SimpleNamespace(id='bot'),'server','',verify='none',
        command_block_mode='skip',jobs_root=tmp_path/'jobs')
    assert job['state']=='sent' and job['command_blocks_state']=='skipped_shells_only'
    packets.assert_not_called()


async def test_java_staging_is_rejected_before_login(tmp_path,monkeypatch):
    import proloader.importer as module
    connect=AsyncMock();monkeypatch.setattr(module,'rental_session',connect)
    with pytest.raises(ValueError,match='Java'):
        await execute_import(make_plan(tmp_path,'java'),None,SimpleNamespace(id='bot'),'server','',command_block_mode='stage')
    connect.assert_not_called()


async def test_manual_repair_reapplies_configuration_after_shell_replay(tmp_path,monkeypatch):
    import proloader.importer as module
    p=make_plan(tmp_path);writes=[];shells=[];actions=iter(('repair','done'))
    current=dict(expected_nbt(p.command_blocks[0]),Command='')
    session=SimpleNamespace(_raise_if_failed=lambda:None,
        _raknet=SimpleNamespace(_closed=False,_send_queue=asyncio.Queue(),_pending_acks={}))
    async def read(*a):return dict(current)
    async def send(packet):writes.append(packet);current.update(expected_nbt(p.command_blocks[0]))
    session.read_block_entity=read;session.send_packet=send
    @asynccontextmanager
    async def connect(*a,**kw):yield session,'bot'
    class World:
        def __init__(self,*a):pass
        async def send(self,*a,**kw):return ''
        async def teleport(self,*a):pass
        async def place(self,c,**kw):shells.append(c);current['Command']=''
        async def check_command_block(self,*a):return True
    class Console:
        def __init__(self,*a,**kw):pass
        async def __aenter__(self):return self
        async def __aexit__(self,*a):pass
        async def notice(self,*a):pass
        async def next_action(self):return next(actions),'player'
    monkeypatch.setattr(module,'rental_session',connect);monkeypatch.setattr(module,'WorldCommands',World)
    monkeypatch.setattr(module,'GameConsole',Console);monkeypatch.setattr(module,'native_binary',lambda *a:Path('/unused'))
    job=await execute_import(p,None,SimpleNamespace(id='bot'),'server','',verify='none',
        stay_online=True,command_block_mode='stage',jobs_root=tmp_path/'jobs')
    assert job['state']=='finished' and len(writes)==2 and len(shells)==3
    assert current['Command']=='say 你好' and job['command_blocks_next']==1


@pytest.mark.parametrize('change', ['truncated','palette_reference','duplicate_entity'])
def test_malformed_standard_structure_does_not_silently_lose_blocks_or_nbt(change):
    root=raw_structure()
    if change=='truncated':root['structure']['block_indices'][0].pop()
    if change=='palette_reference':root['structure']['block_indices'][0][0]=99
    if change=='duplicate_entity':
        data=root['structure']['palette']['default']['block_position_data']
        data['023']=data['23']
    with pytest.raises(ParseError):parse_mcstructure_bytes(dump_nbt(root))


def test_netease_ui_mode_cache_is_not_used_as_block_state_evidence(tmp_path):
    block=make_plan(tmp_path).command_blocks[0]
    observed=dict(expected_nbt(block),LPCommandMode=0,LPCondionalMode=0,conditionalMode=0)
    assert not block.differences(observed)
    # Mode, condition, and facing are checked with testforblock states separately.
    observed['TickDelay']=999
    assert block.differences(observed)==['TickDelay']
