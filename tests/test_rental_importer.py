import asyncio
from contextlib import asynccontextmanager
import json
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import AsyncMock

import numpy as np
import pytest

from proloader.bots import BotStore
from proloader.errors import ParseError
from proloader.importer import ImportPlan, WorldCommands, prepare_import, execute_import
from proloader.model.block import Block, BlockState
from proloader.model.palette import BlockPalette
from proloader.model.region import Region
from proloader.model.structure import Structure
from proloader.net.bedrock.session import BedrockSession, BedrockLoginError
from proloader.parser.sponge import decode_indices


def structure():
    palette = BlockPalette()
    stone = palette.add(Block(BlockState('minecraft:stone')))
    blocks = np.zeros((4,2,3), dtype=np.uint32)
    blocks[0:3,0,0:2] = stone
    blocks[3,1,2] = stone
    return Structure('test',(4,2,3),palette,Region(palette,blocks))


def plan(tmp_path):
    source = tmp_path/'example.mcstructure'
    source.write_bytes(b'fixture')
    st = structure()
    return ImportPlan(source,st,(-85,78,351),'overworld',
                      BedrockSession._generate_merged_commands(st,(-85,78,351)),'example-hash')


@pytest.mark.parametrize('data,count', [(b'\x80',1),(b'\x01',2),(b'\x80'*5+b'\0',1)])
def test_broken_schem_data_never_becomes_air(data,count):
    with pytest.raises(ParseError):
        decode_indices(data,count)


def test_merged_plan_covers_each_non_air_position_once_and_stays_inside(tmp_path):
    p=plan(tmp_path)
    observed=[]
    for command in p.commands:
        _,x,y,z,ex,ey,ez,name,states=command
        observed.extend((a,b,c) for a in range(x,ex+1) for b in range(y,ey+1) for c in range(z,ez+1))
        assert (ex-x+1)*(ey-y+1)*(ez-z+1) <= 32768
    expected={(int(x)-85,int(y)+78,int(z)+351) for x,y,z in np.argwhere(p.structure.region.blocks)}
    assert len(observed)==len(set(observed))
    assert set(observed)==expected


async def test_negative_setblock_requires_world_confirmation(tmp_path):
    session=SimpleNamespace(_raise_if_failed=lambda:None,run_command_ai=AsyncMock(
        side_effect=[BedrockLoginError('aiCommand 执行失败 (uuid=test)'), '']))
    p=plan(tmp_path)
    commands=WorldCommands(session,'bot',p)
    await commands.place(p.commands[0],wait=True)
    sent=[c.args[0] for c in session.run_command_ai.call_args_list]
    assert 'positioned -85 78 351' in sent[0]
    assert 'testforblock' in sent[1]


async def test_unconfirmed_negative_placement_stops(tmp_path):
    session=SimpleNamespace(_raise_if_failed=lambda:None,run_command_ai=AsyncMock(
        side_effect=[BedrockLoginError('aiCommand 执行失败'),BedrockLoginError('aiCommand 执行失败')]))
    with pytest.raises(BedrockLoginError):
        await WorldCommands(session,'bot',plan(tmp_path)).place(plan(tmp_path).commands[0],wait=True)


async def test_import_waits_for_permission_before_teleport_or_placement(tmp_path, monkeypatch):
    import proloader.importer as module
    events=[]
    stages=[]
    session=SimpleNamespace(_raise_if_failed=lambda:None,
        _raknet=SimpleNamespace(_closed=False,_send_queue=asyncio.Queue(),_pending_acks={}))
    @asynccontextmanager
    async def connect(*args,**kwargs):yield session,'permission-bot'
    class FakeWorld:
        def __init__(self,session,name,plan):self.name=name;self.probes=0
        async def send(self,command,**kwargs):
            if self.probes<3:
                assert command=='testfor @s'
                self.probes+=1
                events.append('probe')
                if self.probes<3:raise module.CommandUnconfirmedError('unconfirmed')
                events.append('granted')
            return ''
        async def teleport(self,*args):
            assert 'granted' in events
            events.append('teleport')
        async def place(self,*args,**kwargs):
            assert 'granted' in events
            events.append('place')
    real_sleep=asyncio.sleep
    async def no_delay(seconds):await real_sleep(0)
    monkeypatch.setattr(module,'rental_session',connect)
    monkeypatch.setattr(module,'WorldCommands',FakeWorld)
    monkeypatch.setattr(module.asyncio,'sleep',no_delay)
    job=await execute_import(plan(tmp_path),None,SimpleNamespace(id='test-bot'),'server','',
        verify='none',jobs_root=tmp_path/'jobs',progress=lambda j:stages.append((j['state'],j['detail'],j['submitted'])))
    assert events[:4]==['probe','probe','probe','granted']
    assert job['state']=='sent'
    waiting=[s for s in stages if s[0]=='waiting_permission']
    assert len(waiting)==2 and all('permission-bot' in s[1] and s[2]==0 for s in waiting)


@pytest.mark.parametrize('failure',[BedrockLoginError('连接已断开'),asyncio.CancelledError()])
async def test_permission_wait_propagates_disconnect_and_cancellation(failure):
    from proloader.importer import wait_for_import_permission
    session=SimpleNamespace(_raise_if_failed=lambda:None)
    world=SimpleNamespace(name='bot',send=AsyncMock(side_effect=failure))
    with pytest.raises(type(failure)):
        await wait_for_import_permission(world,session,lambda *args,**kwargs:None)
    assert world.send.await_count==1


async def test_permission_probe_requires_positive_response(tmp_path,monkeypatch):
    import proloader.importer as module
    real_sleep=asyncio.sleep
    async def no_delay(seconds):await real_sleep(0)
    monkeypatch.setattr(module.asyncio,'sleep',no_delay)
    session=SimpleNamespace(_raise_if_failed=lambda:None,
        run_command_ai=AsyncMock(side_effect=[None,'']))
    stages=[]
    await module.wait_for_import_permission(WorldCommands(session,'bot',plan(tmp_path)),session,
        lambda stage,**kwargs:stages.append(stage))
    assert stages==['checking_permission','waiting_permission','preflight']
    assert session.run_command_ai.await_count==2


async def test_no_verification_sends_every_command_and_reports_sent_not_verified(tmp_path,monkeypatch):
    import proloader.importer as module
    monkeypatch.setattr(module,'PROCESSING_WINDOW',128)
    placements=[]
    processing_barriers=[]
    session=SimpleNamespace(_raise_if_failed=lambda:None,
            _raknet=SimpleNamespace(_closed=False,_send_queue=asyncio.Queue(),_pending_acks={}))
    @asynccontextmanager
    async def connect(*args,**kwargs):
        yield session,'bot'
    class FakeWorld:
        def __init__(self,*args):pass
        async def send(self,*args,**kwargs):processing_barriers.append(args[0]);return ''
        async def teleport(self,*args):pass
        async def place(self,c,wait=False):placements.append(c)
        async def check(self,*args):raise AssertionError('no world verification was requested')
    monkeypatch.setattr(module,'rental_session',connect)
    monkeypatch.setattr(module,'WorldCommands',FakeWorld)
    p=plan(tmp_path)
    p.commands *= 200  # Cross server-processing barriers with block checks off.
    job=await execute_import(p,None,SimpleNamespace(id='test-bot'),'server','private-password',
                             verify='none',commands_per_second=200,jobs_root=tmp_path/'jobs')
    assert job['state']=='sent'
    assert job['submitted']==len(p.commands)
    assert placements[1:]==p.commands
    assert processing_barriers.count('testfor @s') >= 3
    assert all('commandblocksenabled' not in command for command in processing_barriers)
    assert job['checked']==0 and job['verification']=='not_performed'
    assert 'private-password' not in next((tmp_path/'jobs').glob('*.json')).read_text()


async def test_checked_pipeline_overlaps_responses_but_keeps_failed_batch_for_repair(tmp_path,monkeypatch):
    import proloader.importer as module
    gate=asyncio.Event();placements=[];checks=[0];failed_at=[]
    session=SimpleNamespace(_raise_if_failed=lambda:None,
        _raknet=SimpleNamespace(_closed=False,_send_queue=asyncio.Queue(),_pending_acks={}))
    @asynccontextmanager
    async def connect(*a,**kw):yield session,'bot'
    class World:
        def __init__(self,*a):pass
        async def send(self,*a,**kw):return ''
        async def teleport(self,*a):pass
        async def place(self,c,**kw):
            placements.append(c)
            if len(placements)>=65:gate.set()
        async def check(self,*a):
            checks[0]+=1
            if checks[0]==2:
                # A synchronous batch checker would deadlock here at 32.
                await asyncio.wait_for(gate.wait(),2)
                failed_at.append(len(placements))
                return False
            return True
    monkeypatch.setattr(module,'rental_session',connect)
    monkeypatch.setattr(module,'WorldCommands',World)
    p=plan(tmp_path);p.commands*=64
    result=await execute_import(p,None,SimpleNamespace(id='fixture'),'server','',
        verify='sample',jobs_root=tmp_path/'jobs')
    assert failed_at and failed_at[0]>=65
    assert result['mismatches_detected']==32
    assert not result['repair_pending'] and result['state']=='completed'
    assert result['checked_processing_window']==module.CHECKED_PROCESSING_LIMIT
    assert result['metrics']['max_processing_window']<=module.CHECKED_PROCESSING_LIMIT


async def test_checked_pipeline_stops_at_durable_bound_when_all_responses_stall(tmp_path,monkeypatch):
    import proloader.importer as module
    gate=asyncio.Event();placements=[];checks=[0]
    session=SimpleNamespace(_raise_if_failed=lambda:None,
        _raknet=SimpleNamespace(_closed=False,_send_queue=asyncio.Queue(),_pending_acks={}))
    @asynccontextmanager
    async def connect(*a,**kw):yield session,'bot'
    class World:
        def __init__(self,*a):pass
        async def send(self,*a,**kw):return ''
        async def teleport(self,*a):pass
        async def place(self,c,**kw):placements.append(c)
        async def check(self,*a):
            checks[0]+=1
            if checks[0]>1:await gate.wait()
            return True
    monkeypatch.setattr(module,'rental_session',connect)
    monkeypatch.setattr(module,'WorldCommands',World)
    p=plan(tmp_path);p.commands*=512
    task=asyncio.create_task(execute_import(p,None,SimpleNamespace(id='fixture'),'server','',
        verify='sample',jobs_root=tmp_path/'jobs'))
    limit=module.CHECKED_PROCESSING_LIMIT
    try:
        deadline=asyncio.get_running_loop().time()+3
        while len(placements)<limit+1 and asyncio.get_running_loop().time()<deadline:
            await asyncio.sleep(.001)
        await asyncio.sleep(.01)
        assert len(placements)==limit+1  # Includes the separate first probe.
        saved=json.loads(next((tmp_path/'jobs').glob('*.json')).read_text())
        assert saved['submitted']==limit and saved['checked_processing_window']==limit
        gate.set()
        result=await asyncio.wait_for(task,3)
        assert result['state']=='completed' and not result['repair_pending']
        assert result['metrics']['max_processing_window']==limit
    finally:
        gate.set()
        if not task.done():task.cancel()
        await asyncio.gather(task,return_exceptions=True)


@pytest.mark.parametrize('verify',['none','sample'])
async def test_covered_main_layer_never_teleports_and_uses_one_area(tmp_path,monkeypatch,verify):
    import proloader.importer as module
    p=plan(tmp_path)
    palette=BlockPalette();stone=palette.add(Block(BlockState('minecraft:stone')))
    p.structure=Structure('three-groups',(100,1,1),palette,
        Region(palette,np.full((100,1,1),stone,dtype=np.uint32)))
    p.origin=(0,80,0)
    p.commands=[('fill',x,80,0,min(x+47,99),80,0,'minecraft:stone',{}) for x in (0,48,96)]
    moves=[];sent=[]
    session=SimpleNamespace(_raise_if_failed=lambda:None,read_chunk_window=AsyncMock(return_value=[]),
        _raknet=SimpleNamespace(_closed=False,_send_queue=asyncio.Queue(),_pending_acks={}))
    @asynccontextmanager
    async def connect(*a,**kw):yield session,'bot'
    class World(module.WorldCommands):
        async def send(self,command,**kw):sent.append(command);return ''
        async def teleport(self,*position):moves.append(position)
    monkeypatch.setattr(module,'rental_session',connect)
    monkeypatch.setattr(module,'WorldCommands',World)
    result=await execute_import(p,None,SimpleNamespace(id='fixture'),'server','',
        verify=verify,jobs_root=tmp_path/'jobs')
    assert result['submitted']==3 and result['state']==('sent' if verify=='none' else 'completed')
    assert moves==[]
    assert session.read_chunk_window.await_count==0
    assert result['loaded_windows']==1 and 'loading_area_pending' not in result
    assert result['loading_strategy']=='stationary_tickingarea_100'


async def test_sample_checks_reload_each_source_slice_without_teleporting(tmp_path,monkeypatch):
    import proloader.importer as module
    p=plan(tmp_path)
    palette=BlockPalette();stone=palette.add(Block(BlockState('minecraft:stone')))
    cells=np.zeros((300,1,300),dtype=np.uint32)
    for x in (0,200):
        for z in (0,200):cells[x,0,z]=stone
    p.structure=Structure('four-slices',(300,1,300),palette,Region(palette,cells))
    p.origin=(0,80,0)
    p.commands=[('setblock',x,80,z,x,80,z,'minecraft:stone',{}) for x in (0,200) for z in (0,200)]
    session=SimpleNamespace(_raise_if_failed=lambda:None,read_chunk_window=AsyncMock(),
        _raknet=SimpleNamespace(_closed=False,_send_queue=asyncio.Queue(),_pending_acks={}))
    @asynccontextmanager
    async def connect(*a,**kw):yield session,'bot'
    class World(module.WorldCommands):
        async def send(self,*a,**kw):return ''
        async def teleport(self,*a):raise AssertionError('remote sample must not move')
    monkeypatch.setattr(module,'rental_session',connect)
    monkeypatch.setattr(module,'WorldCommands',World)
    result=await execute_import(p,None,SimpleNamespace(id='fixture'),'server','',
        verify='sample',jobs_root=tmp_path/'jobs')
    assert result['checked']==result['passed']==4
    assert result['loaded_windows']==8  # Four construction slices, four fresh samples.
    assert not result.get('loading_area_pending') and not result['repair_pending']


async def test_changed_schedule_cannot_reuse_old_cursor_or_connect(tmp_path,monkeypatch):
    import proloader.importer as module
    from proloader.bots import private_json
    p=plan(tmp_path);task='c'*32;root=tmp_path/'jobs'
    previous=p.summary();previous['plan_sha256']='old-order'
    original=dict(id=task,plan=previous,server='server',bot_id='fixture',submitted=1)
    private_json(root/f'{task}.json',original)
    connect=AsyncMock(side_effect=AssertionError('must reject before login'))
    monkeypatch.setattr(module,'rental_session',connect)
    with pytest.raises(ValueError,match='不能沿用旧命令下标'):
        await execute_import(p,None,SimpleNamespace(id='fixture'),'server','',
            verify='none',resume=task,jobs_root=root)
    connect.assert_not_called()
    assert json.loads((root/f'{task}.json').read_text())==original


def test_bot_identity_deduplicates_and_keeps_credentials_private(tmp_path):
    store=BotStore(tmp_path/'bots')
    cookie=json.dumps({'sauth_json':json.dumps(dict(gameid='x19',sdkuid='account',sessionid='secret',deviceid='device',udid='u'))})
    bot=store.add('first',cookie)
    assert store.add('second',cookie).id==bot.id
    assert len(store.list())==1
    files=list((tmp_path/'bots').glob('*/*.json'))
    assert all(p.stat().st_mode & 0o777 == 0o600 for p in files)
    profile=json.loads((tmp_path/'bots'/bot.id/'profile.json').read_text())
    assert 'secret' not in json.dumps(profile)


def test_same_identity_cannot_be_leased_twice(tmp_path):
    store=BotStore(tmp_path/'bots')
    cookie=json.dumps({'sauth_json':json.dumps(dict(gameid='x19',sdkuid='account',sessionid='secret',deviceid='device',udid='u'))})
    bot=store.add('bot',cookie)
    with store.lease(bot):
        assert store.is_busy(bot)
        with pytest.raises(ValueError,match='另一个任务'):
            with store.lease(bot):pass
        with pytest.raises(ValueError,match='正在执行任务'):
            store.remove(bot)
    assert not store.is_busy(bot)
    store.remove(bot)
    assert not store.list()


async def test_false_tp_requires_positive_position_probe(tmp_path):
    session=SimpleNamespace(_raise_if_failed=lambda:None,run_command_ai=AsyncMock(side_effect=[None,'']))
    await WorldCommands(session,'bot',plan(tmp_path)).teleport(1000,78,1000)
    assert 'testfor @s[x=1000,y=78,z=1000,r=3]' in session.run_command_ai.call_args_list[1].args[0]


async def test_false_tp_and_failed_position_probe_stops(tmp_path,monkeypatch):
    import proloader.importer as module
    clock=iter((0,9))
    monkeypatch.setattr(module.time,"monotonic",lambda:next(clock,9))
    session=SimpleNamespace(_raise_if_failed=lambda:None,run_command_ai=AsyncMock(return_value=None))
    with pytest.raises(BedrockLoginError):
        await WorldCommands(session,'bot',plan(tmp_path)).teleport(1000,78,1000)


@pytest.mark.parametrize('scenario', ['batch','probe','final_sample','persistent'])
async def test_mismatch_is_deferred_until_all_commands_sent(tmp_path,monkeypatch,scenario):
    import proloader.importer as module
    placements=[]
    stages=[]
    session=SimpleNamespace(_raise_if_failed=lambda:None,
        _raknet=SimpleNamespace(_closed=False,_send_queue=asyncio.Queue(),_pending_acks={}))
    @asynccontextmanager
    async def connect(*args,**kwargs):yield session,'bot'
    class FakeWorld:
        def __init__(self,*args):self.probes=0
        async def send(self,*args,**kwargs):return ''
        async def teleport(self,*args):pass
        async def place(self,c,**kwargs):placements.append(c)
        async def check(self,*args):
            self.probes+=1
            failed_at={'batch':2,'probe':1,'final_sample':4}.get(scenario)
            return scenario!='persistent' and self.probes!=failed_at
    monkeypatch.setattr(module,'rental_session',connect)
    monkeypatch.setattr(module,'WorldCommands',FakeWorld)
    p=plan(tmp_path)
    p.commands = (p.commands*32)[:64]
    def progress(j):
        stages.append(j['state'])
        if j['state']=='repairing':
            assert j['submitted']==64
            assert placements[1:65]==p.commands
    job=await execute_import(p,None,SimpleNamespace(id='fixture'),'server','SECRET',
                             jobs_root=tmp_path/'jobs', progress=progress)
    assert job['submitted']==64
    assert 'repairing' in stages
    assert job['mismatches_detected']>0
    if scenario=='persistent':
        assert job['state']=='verification_failed'
        assert len(job['repair_rounds'])==2
        assert job['repair_pending'] and job['failures']
    else:
        assert job['state']=='completed'
        assert len(job['repair_rounds'])==1
        assert job['repair_rounds'][0]['commands']==(32 if scenario=='batch' else 1)
        assert not job['repair_pending']
        assert job['checked']==job['passed']
    saved=next((tmp_path/'jobs').glob('*.json')).read_text()
    assert 'SECRET' not in saved
    assert json.loads(saved)['repair_pending']==job['repair_pending']


async def test_connection_failure_is_not_treated_as_repairable_mismatch(tmp_path,monkeypatch):
    import proloader.importer as module
    @asynccontextmanager
    async def connect(*args,**kwargs):yield SimpleNamespace(_raise_if_failed=lambda:None),'bot'
    class FakeWorld:
        def __init__(self,*args):pass
        async def send(self,*args,**kwargs):return ''
        async def teleport(self,*args):pass
        async def place(self,*args,**kwargs):pass
        async def check(self,*args):raise BedrockLoginError('连接已断开')
    monkeypatch.setattr(module,'rental_session',connect)
    monkeypatch.setattr(module,'WorldCommands',FakeWorld)
    with pytest.raises(BedrockLoginError,match='连接已断开'):
        await execute_import(plan(tmp_path),None,SimpleNamespace(id='fixture'),'server','SECRET',jobs_root=tmp_path/'jobs')
    job=json.loads(next((tmp_path/'jobs').glob('*.json')).read_text())
    assert job['state']=='failed'
    assert not job['repair_pending']
    assert job['submitted']==0


@pytest.mark.parametrize('variant,modern', [('default','stone_bricks'),('mossy','mossy_stone_bricks'),('cracked','cracked_stone_bricks'),('chiseled','chiseled_stone_bricks')])
def test_stonebrick_split_preserves_variant_and_input(variant,modern):
    from proloader.block_names import command_state
    states={'stone_brick_type':variant}
    assert command_state('minecraft:stonebrick',states)==('minecraft:'+modern,{})
    assert states=={'stone_brick_type':variant}
    assert command_state('minecraft:stonebrick',{'stone_brick_type':'unknown'})==('minecraft:stonebrick',{'stone_brick_type':'unknown'})


def test_light_block_probe_maps_to_level_name_and_write_keeps_states():
    from proloader.block_names import command_state
    states={'block_light_level':15}
    # Probe (verification) must use the per-level id the server stores.
    assert command_state('minecraft:light_block',states,probe=True)==('minecraft:light_block_15',{})
    # Writes keep the modern states form so the light level survives placement.
    assert command_state('minecraft:light_block',states)==('minecraft:light_block',{'block_light_level':15})
    assert states=={'block_light_level':15}
    # Unknown level value stays unmapped rather than guessed.
    assert command_state('minecraft:light_block',{'block_light_level':'x'},probe=True)==('minecraft:light_block',{'block_light_level':'x'})
    # light_block without level states is untouched in both modes.
    assert command_state('minecraft:light_block',None,probe=True)==('minecraft:light_block',{})


async def test_interrupted_repair_retains_pending_and_resumes(tmp_path,monkeypatch):
    import proloader.importer as module
    placements=[]
    probes=[0]
    interrupted=[False]
    session=SimpleNamespace(_raise_if_failed=lambda:None,
        _raknet=SimpleNamespace(_closed=False,_send_queue=asyncio.Queue(),_pending_acks={}))
    @asynccontextmanager
    async def connect(*args,**kwargs):yield session,'bot'
    class FakeWorld:
        def __init__(self,*args):pass
        async def send(self,*args,**kwargs):return ''
        async def teleport(self,*args):pass
        async def place(self,c,**kwargs):
            if len(placements)==65 and not interrupted[0]:
                interrupted[0]=True
                raise BedrockLoginError('连接已断开')
            placements.append(c)
        async def check(self,*args):
            probes[0]+=1
            return probes[0]!=2
    monkeypatch.setattr(module,'rental_session',connect)
    monkeypatch.setattr(module,'WorldCommands',FakeWorld)
    p=plan(tmp_path);p.commands=(p.commands*32)[:64]
    kwargs=dict(jobs_root=tmp_path/'jobs')
    bot=SimpleNamespace(id='fixture')
    with pytest.raises(BedrockLoginError):
        await execute_import(p,None,bot,'server','SECRET',**kwargs)
    file=next((tmp_path/'jobs').glob('*.json'));job=json.loads(file.read_text())
    assert job['submitted']==64 and len(job['repair_pending'])==32
    result=await execute_import(p,None,bot,'server','SECRET',resume=job['id'],**kwargs)
    assert result['state']=='completed'
    assert not result['repair_pending']
    assert 'error_type' not in result


async def test_verification_resume_does_not_replace_any_building_blocks(tmp_path,monkeypatch):
    import proloader.importer as module
    from proloader.bots import private_json
    p=plan(tmp_path);bot=SimpleNamespace(id='fixture')
    task_id='a'*32;root=tmp_path/'jobs'
    private_json(root/f'{task_id}.json',dict(id=task_id,plan=p.summary(),server='server',
        bot_id=bot.id,state='paused',submitted=len(p.commands),checked=1,passed=1,
        verify_mode='complete',placement_finished_seconds=1))
    session=SimpleNamespace(_raise_if_failed=lambda:None)
    @asynccontextmanager
    async def connect(*args,**kwargs):yield session,'bot'
    class FakeWorld:
        def __init__(self,*args):pass
        async def send(self,*args,**kwargs):return ''
        async def teleport(self,*args):pass
        async def place(self,*args,**kwargs):raise AssertionError('verification resumed placement')
        async def check(self,*args):return True
    monkeypatch.setattr(module,'rental_session',connect)
    monkeypatch.setattr(module,'WorldCommands',FakeWorld)
    result=await execute_import(p,None,bot,'server','',resume=task_id,
        verify='complete',jobs_root=root)
    assert result['state']=='completed'
    assert result['checked']==int(np.count_nonzero(p.structure.region.blocks))
    assert result['verification_checkpoint']['cursor']==result['checked']


async def test_processing_barrier_retries_only_readonly_probe(tmp_path,monkeypatch):
    import proloader.importer as module
    monkeypatch.setattr(module.asyncio,'sleep',AsyncMock())
    session=SimpleNamespace(_raise_if_failed=lambda:None,
                            run_command_ai=AsyncMock(side_effect=[None,None,'']))
    assert await WorldCommands(session,'bot',plan(tmp_path)).send('testfor @s')==''
    assert session.run_command_ai.call_count==3
    assert all(c.args[0]=='testfor @s' and c.kwargs['timeout']==30
               for c in session.run_command_ai.call_args_list)


async def test_processing_barrier_stops_on_terminal_failure(tmp_path):
    check=iter((None,BedrockLoginError('连接已断开')))
    def failed():
        result=next(check)
        if result:raise result
    session=SimpleNamespace(_raise_if_failed=failed,run_command_ai=AsyncMock(return_value=None))
    with pytest.raises(BedrockLoginError,match='连接已断开'):
        await WorldCommands(session,'bot',plan(tmp_path)).send('testfor @s')
    assert session.run_command_ai.call_count==1


async def test_processing_barrier_failure_is_bounded(tmp_path,monkeypatch):
    import proloader.importer as module
    monkeypatch.setattr(module.asyncio,'sleep',AsyncMock())
    session=SimpleNamespace(_raise_if_failed=lambda:None,run_command_ai=AsyncMock(return_value=None))
    with pytest.raises(BedrockLoginError,match='3次只读确认'):
        await WorldCommands(session,'bot',plan(tmp_path)).send('testfor @s')
    assert session.run_command_ai.call_count==3


async def test_confirmed_teleport_moves_relative_command_frame(tmp_path,monkeypatch):
    import proloader.importer as module
    async def no_delay(*a):pass
    monkeypatch.setattr(module.asyncio,'sleep',no_delay)
    session=SimpleNamespace(_raise_if_failed=lambda:None,run_command_ai=AsyncMock(return_value=''))
    w=WorldCommands(session,'bot',plan(tmp_path))
    await w.teleport(12027,118,12267)
    await w.check(12027,117,12267,'minecraft:chain_command_block')
    command=session.run_command_ai.call_args.args[0]
    assert 'positioned 12027 118 12267' in command
    assert 'testforblock ~ ~-1 ~' in command


async def test_real_bulk_repair_path_reuses_source_fills_and_rechecks_affected_cubes(tmp_path,monkeypatch):
    import proloader.importer as module
    from proloader.bots import private_json
    p=plan(tmp_path);bot=SimpleNamespace(id='fixture');task='b'*32;root=tmp_path/'jobs'
    expected={tuple(int(c[i])+p.origin[i] for i in range(3)) for c in np.argwhere(p.structure.region.blocks)}
    missing=tuple(p.commands[0][1:4]);actual=expected-{missing};writes=[];reads=[]
    async def cubes(pos,dim,heights,timeout=8,**kwargs):
        reads.append((pos,list(heights)));entries=[]
        for ytile in heights:
            ids=[0]*4096
            for x,y,z in actual:
                if x//16==pos[0]//16 and z//16==pos[2]//16 and y//16==ytile:
                    ids[((x%16)*16+y%16)*16+z%16]=1
            entries.append(dict(offset=[0,ytile,0],result=1,blocks=ids,palette={
                '0':dict(name='minecraft:air',states={}), '1':dict(name='minecraft:stone',states={})}))
        return entries
    session=SimpleNamespace(read_subchunks=cubes,read_subchunk=AsyncMock(),_raise_if_failed=lambda:None,
        _raknet=SimpleNamespace(_closed=False,_send_queue=asyncio.Queue(),_pending_acks={}))
    @asynccontextmanager
    async def connect(*a,**kw):yield session,'bot'
    class World(module.WorldCommands):
        async def send(self,*a,**kw):return ''
        async def teleport(self,*a):pass
        async def place(self,c,**kw):
            writes.append(c)
            for x in range(c[1],c[4]+1):
                for y in range(c[2],c[5]+1):
                    for z in range(c[3],c[6]+1):actual.add((x,y,z))
    monkeypatch.setattr(module,'rental_session',connect);monkeypatch.setattr(module,'WorldCommands',World)
    private_json(root/f'{task}.json',dict(id=task,plan=p.summary(),server='server',bot_id=bot.id,
        state='sent',submitted=len(p.commands),checked=0,passed=0,verify_mode='complete',placement_finished_seconds=1))
    result=await execute_import(p,None,bot,'server','',verify='complete',resume=task,jobs_root=root)
    assert result['state']=='completed' and result['checked']==result['passed']==len(expected)
    assert not result['repair_pending'] and actual==expected
    assert writes==[p.commands[0]]  # One merged source fill, not a whole-building replay.
    assert result['repair_rounds'][0]['differences']==1 and reads


def test_approved_coordinate_replacements_survive_resume_without_changing_source(tmp_path,monkeypatch):
    import proloader.importer as module
    from proloader.cli.commands import resume_plan
    source=tmp_path/'approved.mcstructure';source.write_bytes(b'unchanged source')
    monkeypatch.setattr(module,'load_structure',lambda file:structure())
    replacements=[dict(local=[0,0,0],from_name='minecraft:stone',name='minecraft:white_concrete')]
    original=prepare_import(source,(0,80,0),engine='legacy')
    adjusted=prepare_import(source,(0,80,0),engine='legacy',block_replacements=replacements)
    assert source.read_bytes()==b'unchanged source'
    assert adjusted.structure.palette.get(int(adjusted.structure.region.blocks[0,0,0])).state.name=='minecraft:white_concrete'
    assert adjusted.structure.palette.get(int(adjusted.structure.region.blocks[1,0,0])).state.name=='minecraft:stone'
    assert original.summary()['plan_sha256']!=adjusted.summary()['plan_sha256']
    assert resume_plan(dict(state='failed',plan=adjusted.summary())).summary()==adjusted.summary()
    with pytest.raises(ValueError,match='不符'):
        prepare_import(source,(0,80,0),engine='legacy',block_replacements=[dict(replacements[0],local=[0,1,0])])


def test_approved_profiles_require_exact_file_hash_and_do_not_migrate_legacy_resumes(tmp_path,monkeypatch):
    import hashlib
    import proloader.importer as module
    source=tmp_path/'same-name.mcstructure';source.write_bytes(b'approved exact source')
    profile=tmp_path/'profiles.json'
    profile.write_text(json.dumps({hashlib.sha256(source.read_bytes()).hexdigest():[
        dict(local=[0,0,0],from_name='minecraft:stone',name='minecraft:white_concrete')]}))
    monkeypatch.setattr(module,'APPROVED_REPLACEMENTS',profile)
    monkeypatch.setattr(module,'load_structure',lambda file:structure())
    approved=prepare_import(source,(0,80,0),engine='legacy')
    assert len(approved.summary()['block_replacements'])==1
    legacy=prepare_import(source,(0,80,0),engine='legacy',block_replacements=())
    assert 'block_replacements' not in legacy.summary()
    source.write_bytes(b'different source, same name')
    assert 'block_replacements' not in prepare_import(source,(0,80,0),engine='legacy').summary()


async def test_confirmed_external_teleport_invalidates_verification_location_marker(monkeypatch):
    import proloader.importer as module
    monkeypatch.setattr(module.asyncio,'sleep',AsyncMock())
    w=WorldCommands(None,'bot',SimpleNamespace(dimension='overworld'))
    w.send=AsyncMock(return_value='');w._verification_chunk=(1,2)
    await w.teleport(100,80,100)
    assert w._command_origin==(100,80,100) and w._verification_chunk is None


async def test_new_external_teleport_blocks_old_local_writes_and_importer_moves():
    w=WorldCommands(SimpleNamespace(),'bot',SimpleNamespace())
    w.send=AsyncMock();w._active_local_repair_revision=1;w._external_teleport_revision=2
    with pytest.raises(ValueError,match='新的传送'):
        await w.place(('setblock',0,80,0,0,80,0,'minecraft:stone',{}))
    with pytest.raises(ValueError,match='新的传送'):await w.teleport(0,80,0)
    w.send.assert_not_awaited()
