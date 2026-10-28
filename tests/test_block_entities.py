import asyncio
from contextlib import asynccontextmanager
import json
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import AsyncMock

import numpy as np
import pytest

from proloader.block_entities import (BlockEntityData, plan_block_entities, operation_hash,
    apply_block_entities, sign_packets, UnsupportedNBT)
from proloader.errors import ParseError
from proloader.model.block import Block, BlockState
from proloader.model.palette import BlockPalette
from proloader.model.region import Region
from proloader.model.structure import Structure
from proloader.net.bedrock.session import BedrockSession
from proloader.net.bedrock.packets import unwrap_packet
from proloader.net.raknet.wire import ByteReader


def structure(name='chest', nbt=None, edition='bedrock'):
    palette = BlockPalette()
    bid = palette.add(Block(BlockState('minecraft:'+name)))
    blocks = np.full((1,1,1),bid,dtype=np.uint32)
    return Structure('fixture',(1,1,1),palette,Region(palette,blocks),
        block_entities={(0,0,0): nbt if nbt is not None else dict(Items=[dict(Slot=5,Name='stone',Count=3,Damage=0)])},
        source_info={'source_edition':edition})


def operation():
    operations, unsupported = plan_block_entities(structure(),(100,80,-100))
    assert not unsupported
    return operations[0]


def test_container_normalization_complete_slot_replacement_and_fingerprint():
    op = operation()
    assert op.position == (100,80,-100)
    assert len(op.commands()) == 27
    assert op.commands()[5] == 'replaceitem block 100 80 -100 slot.container 5 minecraft:stone 3 0'
    assert op.commands()[0].endswith('minecraft:air 1 0')
    other, _ = plan_block_entities(structure(),(101,80,-100))
    assert operation_hash([op]) != operation_hash(other)
    assert not op.differences({'Items':[dict(Slot=5,Name='minecraft:stone',Count=3,Damage=0)]})
    assert op.differences({'Items':[]})
    assert op.differences({})  # Missing is not empty/success.


@pytest.mark.parametrize('item',[
    dict(Slot=0,Name='stone',Count=1,tag={'ench':[{'id':0,'lvl':1}]}),
    dict(Slot=0,Name='stone',Count=1,tag={'display':{'Name':'custom'}}),
    dict(Slot=0,Name='written_book',Count=1,tag={'pages':[]}),
    dict(Slot=0,Name='shulker_box',Count=1,tag={'Items':[]}),
    dict(Slot=0,Name='stone run kill @e',Count=1),
])
def test_complex_item_excludes_whole_container_without_simplification(item):
    operations, unsupported = plan_block_entities(structure(nbt={'Items':[item]}),(0,80,0))
    assert not operations and len(unsupported)==1


@pytest.mark.parametrize('items',[
    [dict(Slot=27,Name='stone',Count=1)], [dict(Slot=0,Name='stone',Count=0)],
    [dict(Slot=0,Name='stone',Count=1)]*2, 'not-a-list', [dict(Slot=True,Name='stone',Count=1)],
])
def test_bad_slot_or_count_stops_preflight(items):
    with pytest.raises(ParseError):plan_block_entities(structure(nbt={'Items':items}),(0,80,0))


@pytest.mark.parametrize('extra',[{'CustomName':'箱子'},{'LootTable':'loot'},{'pairx':100}])
def test_container_attributes_are_reported_not_dropped(extra):
    operations, unsupported = plan_block_entities(structure(nbt={'Items':[],**extra}),(0,80,0))
    assert not operations and unsupported


def test_redundant_item_block_state_must_match_replaceitem_metadata():
    block = dict(name='minecraft:stone',states={'stone_type':'stone'},val=0,version=1)
    item = dict(Slot=0,Name='minecraft:stone',Damage=0,Count=1,Block=block)
    operations, unsupported = plan_block_entities(structure(nbt={'Items':[item]}),(0,80,0))
    assert len(operations)==1 and not unsupported
    item['Block'] = dict(name='minecraft:dirt',states={'dirt_type':'normal'},val=0,version=1)
    operations, unsupported = plan_block_entities(structure(nbt={'Items':[item]}),(0,80,0))
    assert not operations and unsupported


def test_transient_container_open_state_and_zero_runtime_fields_do_not_drop_items():
    nbt = dict(Items=[dict(Slot=0,Name='stone',Count=1)],IsOpened=1,IsIgnoreShuffle=0)
    operations, unsupported = plan_block_entities(structure(nbt=nbt),(0,80,0))
    assert len(operations)==1 and not unsupported and operations[0].data['items']


def test_java_and_unknown_types_never_enter_bedrock_writer():
    for s in (structure(edition='java'),structure('mob_spawner',{})):
        operations, unsupported = plan_block_entities(s,(0,80,0))
        assert not operations and unsupported


def sign():
    s = structure('wall_sign',dict(FrontText={'Text':'前面\n二哈'},BackText={'Text':'背面'},IsWaxed=0))
    s.palette.get(int(s.region.blocks[0,0,0])).state.states.update(facing_direction=2)
    operations, unsupported = plan_block_entities(s,(100,80,-100))
    assert not unsupported
    return operations[0]


def test_sign_legacy_and_modern_text_and_attributes():
    op = sign()
    assert not op.differences({'FrontText':{'Text':'前面\n二哈'},'BackText':{'Text':'背面'},'IsWaxed':0})
    assert op.differences({'Text':'前面\n二哈'})  # back text missing
    assert op.differences({'FrontText':{'Text':'前面\n二哈'},'BackText':{'Text':'背面'},'IsWaxed':1})
    legacy, _ = plan_block_entities(structure('standing_sign',{'Text':'old'}),(0,80,0))
    assert legacy[0].data == {'FrontText':'old','BackText':'',
        'FrontTextPersistFormatting':1,'BackTextPersistFormatting':1}
    for nbt in ({'Text':'x','IgnoreLighting':1},{'Text':'x','SignTextColor':123},{'Text':'x','IsWaxed':1}):
        operations, unsupported = plan_block_entities(structure('wall_sign',nbt),(0,80,0))
        assert not operations and unsupported


def test_real_sign_codecs_and_upstream_actor_profile_agreement():
    op = sign()
    packets = sign_packets(op,'sign_click',(101,81.62,-100))
    assert [unwrap_packet(p)[0] for p in packets] == [48,30]
    assert unwrap_packet(sign_packets(op,'auth',(101,81.62,-100),123)[0])[0] == 144
    packet, = sign_packets(op,'sign_text',(101,81.62,-100))
    pid,body = unwrap_packet(packet)
    assert pid == 56
    reader = ByteReader(body)
    assert (reader.read_varint(),reader.read_unsigned_varint(),reader.read_varint()) == op.position
    assert '二哈'.encode() in body
    with pytest.raises(ParseError,match='860'):
        sign_packets(op,'sign_click',(101,81.62,-100),protocol=686)


@pytest.mark.parametrize('failure',['none','mismatch','disconnect','timeout'])
async def test_nbt_completion_requires_readback_and_resume_reads_before_writing(monkeypatch,failure):
    import proloader.block_entities as module
    op = operation(); current={'Items':[]};writes=[];saved=[];job={};reads=0
    expected_writes=3 if failure=='mismatch' else 1
    async def write(*args):
        writes.append(args);current.update(Items=op.data['items'])
    monkeypatch.setattr(module,'write_block_entity',write)
    class Session:
        _raise_if_failed = staticmethod(lambda:None)
        _states_to_str = staticmethod(BedrockSession._states_to_str)
        async def read_block_entity(self,*args):
            nonlocal reads
            reads += 1
            if reads>1:
                if failure=='mismatch':return {'Items':[]}
                if failure=='disconnect':raise ConnectionError('closed')
                if failure=='timeout':raise asyncio.TimeoutError()
            return dict(current)
    world=SimpleNamespace(send=AsyncMock(return_value=''))
    save=lambda:saved.append(dict(job))
    move=AsyncMock()
    kwargs=( [op],Session(),world,'overworld',job,save,move,lambda *a,**kw:None )
    if failure=='none':
        await apply_block_entities(*kwargs)
    else:
        with pytest.raises((ValueError,ConnectionError,asyncio.TimeoutError)):
            await apply_block_entities(*kwargs)
        assert job.get('block_entities_next',0)==0 and job['block_entities_inflight']==0
        failure='none';reads=0
        await apply_block_entities(*kwargs)
    assert len(writes)==expected_writes and job['block_entities_state']=='readback_verified'
    assert job['block_entities_next']==1
    assert any(s.get('block_entities_inflight')==0 for s in saved)


async def test_container_writer_uses_target_dimension_and_complete_slots():
    from proloader.block_entities import write_block_entity
    op = operation();world=SimpleNamespace(send=AsyncMock())
    await write_block_entity(op,world,None,'nether')
    commands=[call.args[0] for call in world.send.call_args_list]
    assert len(commands)==28 and commands[0].startswith('execute in nether run replaceitem block 100 80 -100')
    assert commands[-1]=='testfor @s'


async def test_container_delta_only_clears_stale_slots_and_waits_once():
    from proloader.block_entities import write_block_entity
    op=operation()
    observed={'Items':op.data['items']+[dict(Slot=2,Name='minecraft:dirt',Count=1,Damage=0)]}
    world=SimpleNamespace(send=AsyncMock())
    await write_block_entity(op,world,None,'overworld',observed)
    calls=world.send.call_args_list
    assert len(calls)==2 and 'slot.container 2 minecraft:air' in calls[0].args[0]
    assert calls[0].kwargs=={'wait':False} and calls[-1].args==('testfor @s',)
    assert not op.commands({'Items':op.data['items']})
    # Unknown item tags must not become an incomplete partial replacement.
    assert len(op.commands({'Items':[dict(Slot=1,Name='minecraft:stone',Count=1,tag={'custom':1})]}))==27


async def test_sign_writer_reuses_session_and_orders_click_auth_text(monkeypatch):
    import proloader.block_entities as module
    events=[]
    def packets(op,action,position,tick=0,protocol=860):
        events.append((action,tick));return [action.encode()]
    monkeypatch.setattr(module,'sign_packets',packets)
    session=SimpleNamespace(_netease_protocol=860,send_packet=AsyncMock())
    world=SimpleNamespace(send=AsyncMock(),teleport=AsyncMock())
    for _ in range(2):await module.write_block_entity(sign(),world,session,'overworld')
    assert events == [('sign_click',0),('auth',1),('auth',2),('sign_text',0),
                      ('sign_click',0),('auth',3),('auth',4),('sign_text',0)]
    world.teleport.assert_called_with(101,80,-100)
    assert any('slot.hotbar 0 minecraft:air' in c.args[0] for c in world.send.call_args_list)


async def test_waxed_sign_stops_before_any_write(monkeypatch):
    import proloader.block_entities as module
    write=AsyncMock();monkeypatch.setattr(module,'write_block_entity',write)
    session=SimpleNamespace(_states_to_str=BedrockSession._states_to_str,
        read_block_entity=AsyncMock(return_value={'IsWaxed':1,'Text':'old'}))
    with pytest.raises(ValueError,match='涂蜡'):
        await apply_block_entities([sign()],session,SimpleNamespace(send=AsyncMock()),
            'overworld',{},lambda:None,AsyncMock(),lambda *a,**k:None)
    write.assert_not_called()


@pytest.mark.parametrize('fail',['none','disconnect','repair','unsupported'])
async def test_importer_integrates_phase_and_resumes_without_shell_replay(tmp_path,monkeypatch,fail):
    import proloader.importer as module
    from proloader.importer import ImportPlan, execute_import
    s=structure();p=tmp_path/'fixture.bdx';p.write_bytes(b'fixture')
    plan=ImportPlan(p,s,(100,80,-100),'overworld',
        BedrockSession._generate_merged_commands(s,(100,80,-100)),'hash')
    if fail=='unsupported':plan.unsupported_block_entities.append(dict(position=[101,80,-100],reason='complex item'))
    shells=[];writes=[];current={'Items':[]};reads=0
    class Session:
        _raknet=SimpleNamespace(_closed=False,_send_queue=asyncio.Queue(),_pending_acks={})
        _states_to_str=staticmethod(BedrockSession._states_to_str)
        def _raise_if_failed(self):pass
        async def read_block_entity(self,*a):
            nonlocal reads
            reads+=1
            if fail=='disconnect' and reads==2:raise ConnectionError('closed')
            return dict(current)
    @asynccontextmanager
    async def connect(*args,**kw):yield Session(),'bot'
    class World:
        def __init__(self,*args):pass
        async def send(self,command,**kw):
            if 'replaceitem block' in command:
                writes.append(command)
                if 'slot.container 5 ' in command:current['Items']=plan.block_entities[0].data['items']
            return ''
        async def teleport(self,*args):pass
        async def place(self,c,**kw):shells.append(c);current['Items']=[]
        async def check(self,*args):return True
    monkeypatch.setattr(module,'rental_session',connect)
    monkeypatch.setattr(module,'WorldCommands',World)
    monkeypatch.setattr(module,'native_binary',lambda *a:Path('/unused'))
    kwargs=dict(verify='none',jobs_root=tmp_path/'jobs')
    if fail=='repair':
        actions=iter(['repair','done'])
        class Game:
            def __init__(self,*a,**kw):pass
            async def __aenter__(self):return self
            async def __aexit__(self,*a):pass
            async def notice(self,*a):pass
            async def next_action(self):return next(actions),'player'
        monkeypatch.setattr(module,'GameConsole',Game)
        kwargs['stay_online']=True
    if fail in ('none','repair','unsupported'):
        job=await execute_import(plan,None,SimpleNamespace(id='bot'),'server','SECRET',**kwargs)
    else:
        with pytest.raises(ConnectionError):
            await execute_import(plan,None,SimpleNamespace(id='bot'),'server','SECRET',**kwargs)
        job=json.loads(next((tmp_path/'jobs').glob('*.json')).read_text())
        assert job['nbt_phase_started'] and job['block_entities_inflight']==0
        count=len(shells);fail='none';reads=0
        job=await execute_import(plan,None,SimpleNamespace(id='bot'),'server','SECRET',resume=job['id'],**kwargs)
        assert len(shells)==count
    assert job['block_entities_next']==1 and job['block_entities_state']=='readback_verified'
    assert len(writes)==(2 if fail=='repair' else 1)
    assert job['delivery_complete']==(fail!='unsupported')
    if fail=='unsupported':assert job['block_entity_delivery']=='supported_subset_verified'
    if fail=='repair':assert len(job['manual_repairs'])==1
    assert 'SECRET' not in next((tmp_path/'jobs').glob('*.json')).read_text()


@pytest.mark.parametrize('first',[False,asyncio.TimeoutError(),ValueError('unavailable cube')])
async def test_shell_recovery_rechecks_unknown_reads_before_replacing(first):
    op=operation();job={}
    world=SimpleNamespace(check_block_entity_shell=AsyncMock(side_effect=[first,True,True]),
                          place=AsyncMock(),send=AsyncMock())
    session=SimpleNamespace(_states_to_str=BedrockSession._states_to_str,
        _raise_if_failed=lambda:None,
        read_block_entity=AsyncMock(return_value={'Items':op.data['items']}))
    await apply_block_entities([op],session,world,'overworld',job,lambda:None,
                              AsyncMock(),lambda *a,**kw:None)
    assert world.place.await_count==(1 if first is False else 0)
    assert job['block_entities_next']==1


async def test_native_shell_proof_does_not_downgrade_state_mismatch_to_name_match():
    from proloader.importer import WorldCommands
    op=operation();x,y,z=op.position
    blocks=[0]*4096;blocks[((x%16)*16+y%16)*16+z%16]=111
    session=SimpleNamespace(read_subchunk=AsyncMock(return_value={'blocks':blocks}))
    world=WorldCommands(session,'bot',SimpleNamespace(dimension='overworld'))
    world.runtime_hashes=AsyncMock(return_value={json.dumps([op.block_name,op.states],sort_keys=True):{222}})
    world.send=AsyncMock(return_value='')
    assert not await world.check_block_entity_shell(op)
    world.send.assert_not_awaited()
    blocks[((x%16)*16+y%16)*16+z%16]=222
    assert await world.check_block_entity_shell(op)


@pytest.mark.parametrize('changed',[False,True])
async def test_shell_and_actor_share_one_fresh_read_and_never_reuse_after_write(monkeypatch,changed):
    import proloader.block_entities as module
    from proloader.importer import WorldCommands
    op=operation();x,y,z=op.position;job={};current={'Items':[] if changed else op.data['items']}
    blocks=[0]*4096;blocks[((x%16)*16+y%16)*16+z%16]=222
    async def read(*a,**kw):return dict(blocks=blocks,nbt=[dict(x=x,y=y,z=z,**current)])
    session=SimpleNamespace(read_subchunk=AsyncMock(side_effect=read),read_block_entity=AsyncMock(),
        _raise_if_failed=lambda:None,_states_to_str=BedrockSession._states_to_str)
    world=WorldCommands(session,'bot',SimpleNamespace(dimension='overworld'))
    world.runtime_hashes=AsyncMock(return_value={json.dumps([op.block_name,op.states],sort_keys=True):{222}})
    async def write(*a):current['Items']=op.data['items']
    writer=AsyncMock(side_effect=write);monkeypatch.setattr(module,'write_block_entity',writer)
    await apply_block_entities([op],session,world,'overworld',job,lambda:None,AsyncMock(),lambda *a,**kw:None)
    assert session.read_subchunk.await_count==(2 if changed else 1)
    session.read_block_entity.assert_not_awaited()
    assert writer.await_count==int(changed) and job['block_entities_next']==1
    assert world.consume_entity_snapshot(op.position) is None


async def test_spatial_nbt_schedule_preserves_completed_legacy_prefix():
    from dataclasses import replace
    ops=[replace(operation(),position=pos) for pos in [(100,80,100),(200,80,200),(101,80,101)]]
    job={'block_entities_next':1};moves=[]
    current={'Items':ops[0].data['items']}
    world=SimpleNamespace(check_block_entity_shell=AsyncMock(return_value=True))
    session=SimpleNamespace(read_block_entity=AsyncMock(return_value=current),
        _states_to_str=BedrockSession._states_to_str,_raise_if_failed=lambda:None)
    async def move(w,pos):moves.append(pos)
    await apply_block_entities(ops,session,world,'overworld',job,lambda:None,move,lambda *a,**kw:None)
    assert job['block_entities_schedule']==[0,2,1]
    assert moves==[ops[2].position,ops[1].position] and job['block_entities_next']==3


async def test_nbt_lost_postwrite_read_recovers_in_same_call_without_resending(monkeypatch):
    import proloader.block_entities as module
    op=operation();job={};current={'Items':[]};reads=0
    async def read(*args):
        nonlocal reads
        reads+=1
        if reads==2:raise asyncio.TimeoutError()
        return dict(current)
    async def write(*args):current['Items']=op.data['items']
    writer=AsyncMock(side_effect=write);monkeypatch.setattr(module,'write_block_entity',writer)
    world=SimpleNamespace(check_block_entity_shell=AsyncMock(return_value=True))
    session=SimpleNamespace(read_block_entity=read,_raise_if_failed=lambda:None,
                            _states_to_str=BedrockSession._states_to_str)
    await apply_block_entities([op],session,world,'overworld',job,lambda:None,AsyncMock(),lambda *a,**kw:None)
    writer.assert_awaited_once()
    assert job['block_entities_next']==1 and len(job['block_entities_recoveries'])==1


async def test_shell_repair_after_write_invalidates_earlier_nbt_read(monkeypatch):
    import proloader.block_entities as module
    op=operation();job={};current={'Items':[]}
    async def write(*args):current['Items']=op.data['items']
    async def place(*args,**kw):current['Items']=[]
    writer=AsyncMock(side_effect=write);monkeypatch.setattr(module,'write_block_entity',writer)
    checks=iter([True,False,True,True,True])
    world=SimpleNamespace(check_block_entity_shell=AsyncMock(side_effect=lambda op:next(checks)),place=place)
    session=SimpleNamespace(read_block_entity=AsyncMock(side_effect=lambda *a:dict(current)),
        _raise_if_failed=lambda:None,_states_to_str=BedrockSession._states_to_str)
    await apply_block_entities([op],session,world,'overworld',job,lambda:None,AsyncMock(),lambda *a,**kw:None)
    assert writer.await_count==2 and current['Items']==op.data['items']
    assert job['block_entities_next']==1


@pytest.mark.parametrize('name',['phantom_membrane','diamond','lodestone_compass','banner'])
def test_known_generated_nonblock_metadata_requires_exact_generator_profile(name):
    item=dict(Slot=0,Name='minecraft:'+name,Count=1,Damage=0,
              Block=dict(name='minecraft:'+name,states={},val=0,version=17959425))
    operations,unsupported=plan_block_entities(structure(nbt={'Items':[item]}),(0,80,0))
    assert len(operations)==1 and not unsupported
    assert operations[0].data['items'][0]['Name']=='minecraft:'+name
    item['Block']['states']={'unexpected':1}
    operations,unsupported=plan_block_entities(structure(nbt={'Items':[item]}),(0,80,0))
    assert not operations and unsupported
    item['Block']['states']={};item['tag']={'custom':'must survive'}
    operations,unsupported=plan_block_entities(structure(nbt={'Items':[item]}),(0,80,0))
    assert not operations and unsupported


@pytest.mark.parametrize('tag,accepted',[({'Type':0},True),({'Type':1},False),({'Type':0,'Patterns':[]},False)])
def test_banner_readback_allows_only_server_generated_plain_default(tag,accepted):
    op=plan_block_entities(structure(nbt={'Items':[dict(Name='minecraft:banner',Slot=0,Count=1,Damage=0)]}),(0,80,0))[0][0]
    observed={'Items':[dict(Name='minecraft:banner',Slot=0,Count=1,Damage=0,tag=tag)]}
    assert (not op.differences(observed))==accepted
    _,unsupported=plan_block_entities(structure(nbt=observed),(0,80,0))
    assert unsupported  # Source tags still require an explicit implementation.
