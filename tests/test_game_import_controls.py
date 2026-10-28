import asyncio
from contextlib import asynccontextmanager
import io
import json
from types import SimpleNamespace
from unittest.mock import AsyncMock

import pytest
from rich.console import Console
from proloader.cli.ui import menu_bounds,menu_view
from proloader.game_ui import GameConsole,ProgressMeter,hud,WELCOME,READY
from proloader.net.bedrock.chat import parse_chat,ChatMessage
from proloader.net.raknet.wire import ByteWriter
from proloader.net.bedrock.session import BedrockSession,BedrockLoginError
from proloader.importer import execute_import,CommandUnconfirmedError
from tests.test_rental_importer import plan


def text_packet(source,message,kind=1):
    writer=ByteWriter();writer.write_unsigned_varint(9);writer.write_byte(kind);writer.write_bool(False)
    if kind in (1,7,8):writer.write_string(source)
    writer.write_string(message)
    for value in ('xuid','',''):writer.write_string(value)
    return writer.to_bytes()


@pytest.mark.parametrize('height',[14,24,40])
def test_every_file_is_reachable_and_visible(height):
    options=[f'building-{i:03}.schem' for i in range(50)]
    for selected in range(50):
        start,end=menu_bounds(50,selected,height)
        assert start<=selected<end
        out=io.StringIO();c=Console(file=out,width=100,height=height,color_system=None)
        c.print(menu_view('选择建筑文件',options,['size']*50,selected,height))
        assert options[selected] in out.getvalue()
        assert len(out.getvalue().splitlines())<=height


def test_chat_parser_does_not_treat_system_notifications_as_controls():
    assert parse_chat(text_packet('player','repair'))==ChatMessage('player','repair','xuid')
    assert parse_chat(text_packet('','done',kind=0)) is None
    with pytest.raises(EOFError):parse_chat(text_packet('player','done')[:-1])


async def test_chat_dispatch_is_bounded_and_malformed_packets_are_ignored():
    queue=asyncio.Queue(maxsize=2)
    session=SimpleNamespace(_terminal_error=None,_chat_subscribers={queue},
                            _read_packet_id=lambda packet:(9,1))
    for message in ('repair','done','repair'):
        BedrockSession._dispatch_packet(session,text_packet('player',message))
    BedrockSession._dispatch_packet(session,b'\x09\x01')
    assert queue.qsize()==2
    assert (await queue.get()).message=='done'


async def test_controls_ignore_early_done_and_bot_echoes():
    queue=asyncio.Queue(maxsize=128)
    session=SimpleNamespace(subscribe_chat=lambda:queue,unsubscribe_chat=lambda q:None,
        _raise_if_failed=lambda:None,_raknet=SimpleNamespace(_send_queue=asyncio.Queue()))
    world=SimpleNamespace(session=session,name='actual-bot',send=AsyncMock())
    plan=SimpleNamespace(commands=[1],structure=SimpleNamespace(name='fixture'))
    job={'state':'placing','submitted':0}
    async with GameConsole(world,plan,job,enabled=True) as game:
        queue.put_nowait(ChatMessage('player','done'));await asyncio.sleep(0)
        assert game.events.empty()
        job.update(state='awaiting_action',delivery_complete=True)
        queue.put_nowait(ChatMessage('actual-bot','done'))
        queue.put_nowait(ChatMessage('player','please repair now'))
        queue.put_nowait(ChatMessage('player',' RePaIr '))
        assert await asyncio.wait_for(game.next_action(),1)==('repair','player')
    commands=[call.args[0] for call in world.send.call_args_list]
    assert any(WELCOME in cmd for cmd in commands)
    assert game.tasks and all(t.done() for t in game.tasks)


async def test_external_teleports_trigger_local_repair_and_coalesce_without_feedback_loop():
    from unittest.mock import Mock
    chat=asyncio.Queue();teleports=asyncio.Queue()
    session=SimpleNamespace(subscribe_chat=lambda:chat,unsubscribe_chat=lambda q:None,
        subscribe_player_teleports=lambda:teleports,unsubscribe_player_teleports=Mock(),
        _raise_if_failed=lambda:None,_raknet=SimpleNamespace(_send_queue=asyncio.Queue()))
    world=SimpleNamespace(session=session,name='bot',send=AsyncMock(),_verification_chunk=(0,0))
    p=SimpleNamespace(commands=[1],structure=SimpleNamespace(name='fixture'))
    job={'state':'placing','submitted':0}
    async with GameConsole(world,p,job,enabled=True) as game:
        teleports.put_nowait(dict(position=(1,80,1),importer_move=False));await asyncio.sleep(0)
        assert game.events.empty()
        job.update(state='awaiting_action',delivery_complete=True)
        teleports.put_nowait(dict(position=(1,80,1),importer_move=True));await asyncio.sleep(0)
        assert game.events.empty()
        for pos in ((1,80,1),(-.5,81,-16.5)):
            teleports.put_nowait(dict(position=pos,importer_move=False))
        await asyncio.sleep(0)
        assert game.events.qsize()==1
        assert await game.next_action()==('repair_here',(-.5,81,-16.5))
        assert world._verification_chunk is None
        job.update(state='repairing',local_repair_active=True)
        teleports.put_nowait(dict(position=(32,80,32),importer_move=False))
        await asyncio.sleep(0)
        assert await game.next_action()==('repair_here',(32,80,32))
    session.unsubscribe_player_teleports.assert_called_once()


async def test_local_action_only_rechecks_local_nbt_and_preserves_global_cursors(tmp_path,monkeypatch):
    import proloader.importer as module
    import proloader.command_block_delivery as delivery
    import proloader.differential_repair as differential
    from proloader.command_blocks import CommandBlockData
    from proloader.block_entities import BlockEntityData
    p=plan(tmp_path)
    def cb(pos):return CommandBlockData(pos,'minecraft:command_block',{},'say test','',True,False,False,0,False)
    def chest(pos):return BlockEntityData(pos,'minecraft:chest',{},'container',{'slots':27,'items':[]})
    local=(-84,78,352);other=(-60,78,352)
    p.command_blocks=[cb(local),cb(other)];p.block_entities=[chest(local),chest(other)]
    calls=[];closed=[];actions=iter([('repair_here',(-84,79,352)),('done','player')])
    session=SimpleNamespace(_raise_if_failed=lambda:None,read_subchunk=AsyncMock(),
        _raknet=SimpleNamespace(_closed=False,_send_queue=asyncio.Queue(),_pending_acks={}))
    @asynccontextmanager
    async def connect(*a,**kw):
        yield session,'bot';closed.append(True)
    class World:
        def __init__(self,*a):self.name='bot';self.session=session
        async def send(self,*a,**kw):return ''
        async def teleport(self,*a):pass
        async def place(self,*a,**kw):pass
    class Console:
        def __init__(self,*a,**kw):pass
        async def __aenter__(self):return self
        async def __aexit__(self,*a):pass
        async def notice(self,*a):pass
        async def next_action(self):return next(actions)
    async def cb_stage(ops,s,w,dim,job,*a):
        if job.get('command_blocks_next',0)<len(ops):calls.append(('cb',[o.position for o in ops]))
        job['command_blocks_next']=len(ops)
    async def other_stage(ops,s,w,dim,job,*a):
        if job.get('block_entities_next',0)<len(ops):calls.append(('other',[o.position for o in ops]))
        job['block_entities_next']=len(ops)
    repair=AsyncMock(return_value=True)
    monkeypatch.setattr(module,'rental_session',connect);monkeypatch.setattr(module,'WorldCommands',World)
    monkeypatch.setattr(module,'GameConsole',Console)
    monkeypatch.setattr(delivery,'stage_command_blocks',cb_stage)
    monkeypatch.setattr(module,'apply_block_entities',other_stage)
    monkeypatch.setattr(differential,'repair_building',repair)
    result=await execute_import(p,None,SimpleNamespace(id='fixture'),'server','',verify='none',
        stay_online=True,live_console=True,jobs_root=tmp_path/'jobs',command_block_mode='stage')
    assert calls==[('cb',[local,other]),('other',[local,other]),('cb',[local]),('other',[local])]
    assert result['command_blocks_next']==result['block_entities_next']==2
    assert result['local_repair_data']['command_blocks_next']==result['local_repair_data']['block_entities_next']==1
    assert repair.call_args.kwargs['chunk']==(-6,22)
    assert closed and result['local_repair_status']=='verified'


def test_measured_speed_eta_and_rollback(monkeypatch):
    import proloader.game_ui as module
    now=[0.];monkeypatch.setattr(module.time,'monotonic',lambda:now[0])
    meter=ProgressMeter(1000)
    now[0]=2;values=meter.read(100)
    assert values['rate']==50 and values['eta_seconds']==18
    now[0]=3;assert meter.read(90)['rate']==0
    p=SimpleNamespace(commands=[None]*1000,structure=SimpleNamespace(name='castle'))
    panel=hud(p,dict(submitted=100,telemetry=values))
    assert '100 / 1,000' in panel and '50.0' in panel and '18秒' in panel


@pytest.mark.parametrize('exit_notice_fails',[False,True])
async def test_done_keeps_connection_until_full_send_and_repair(tmp_path,monkeypatch,exit_notice_fails):
    import proloader.importer as module
    placements=[];closed=[];notices=[];actions=iter(('repair','done'))
    session=SimpleNamespace(_raise_if_failed=lambda:None,
        _raknet=SimpleNamespace(_closed=False,_send_queue=asyncio.Queue(),_pending_acks={}))
    @asynccontextmanager
    async def connect(*a,**kw):
        yield session,'bot'
        closed.append(True)
    class World:
        def __init__(self,*args):self.session=session;self.name='bot'
        async def send(self,*a,**kw):return ''
        async def teleport(self,*a):pass
        async def place(self,command,**kw):placements.append(command)
        async def check(self,*a):return True
    class Console:
        def __init__(self,world,p,job,**kw):self.job=job
        async def __aenter__(self):return self
        async def __aexit__(self,*a):pass
        async def notice(self,message):
            notices.append(message)
            if exit_notice_fails and '即将退出' in message:raise BedrockLoginError('connection closed')
        async def next_action(self):
            assert self.job['submitted']==len(p.commands) and not closed
            return next(actions),'player'
    monkeypatch.setattr(module,'rental_session',connect)
    monkeypatch.setattr(module,'WorldCommands',World)
    monkeypatch.setattr(module,'GameConsole',Console)
    p=plan(tmp_path)
    result=await execute_import(p,None,SimpleNamespace(id='fixture'),'server','',verify='none',
        stay_online=True,live_console=True,jobs_root=tmp_path/'jobs')
    assert result['state']=='finished' and result['delivery_state']=='sent'
    assert result['manual_repairs'][0]['commands']==len(p.commands)
    assert placements[1:1+len(p.commands)]==p.commands
    assert placements[1+len(p.commands):]==p.commands
    assert notices.count(READY)==2 and closed
    assert result['verification']=='not_performed_after_manual_repair'


async def test_bulk_unconfirmed_probe_is_recorded_not_an_exit(tmp_path,monkeypatch):
    import proloader.importer as module
    placements=[];probes=[0]
    session=SimpleNamespace(_raise_if_failed=lambda:None,
        _raknet=SimpleNamespace(_closed=False,_send_queue=asyncio.Queue(),_pending_acks={}))
    @asynccontextmanager
    async def connect(*a,**kw):yield session,'bot'
    class World:
        def __init__(self,*args):pass
        async def send(self,*a,**kw):
            probes[0]+=1
            if probes[0]==2:raise CommandUnconfirmedError('unconfirmed')
            return ''
        async def teleport(self,*a):pass
        async def place(self,c,**kw):placements.append(c)
        async def check(self,*a):raise AssertionError('verification disabled')
    monkeypatch.setattr(module,'rental_session',connect);monkeypatch.setattr(module,'WorldCommands',World)
    p=plan(tmp_path);p.commands*=200
    result=await execute_import(p,None,SimpleNamespace(id='fixture'),'server','',verify='none',jobs_root=tmp_path/'jobs')
    assert result['state']=='sent' and placements[1:]==p.commands
    assert result['unconfirmed_batches'][0]['end']==256


async def test_reconnect_reuses_task_and_preserves_exception_record(tmp_path,monkeypatch):
    import proloader.importer as module
    from proloader.bots import private_json
    calls=[]
    async def attempt(p,store,bot,server,password,**kw):
        calls.append(kw)
        path=tmp_path/'jobs'/(kw['reserved_id']+'.json')
        if len(calls)==1:
            private_json(path,dict(id=kw['reserved_id'],submitted=123,state='failed'))
            raise BedrockLoginError('connection lost')
        assert kw['resume']==kw['reserved_id']
        return json.loads(path.read_text())
    monkeypatch.setattr(module,'_execute_attempt',attempt)
    monkeypatch.setattr(module.asyncio,'sleep',AsyncMock())
    result=await execute_import(None,None,None,'server','',reconnect_attempts=1,jobs_root=tmp_path/'jobs')
    assert len(calls)==2 and result['connection_interruptions'][0]['submitted']==123
    assert len(list((tmp_path/'jobs').glob('*.json')))==1


async def test_negative_results_are_compact_unverified_anomalies():
    results=asyncio.Queue(maxsize=4096)
    session=SimpleNamespace(_command_result_dropped=0)
    job={};game=GameConsole(SimpleNamespace(session=session),None,job)
    game.results=results
    task=asyncio.create_task(game._results())
    try:
        for index in range(100):results.put_nowait(dict(index=index,phase='initial',position=[index,0,0],block='minecraft:stone'))
        await asyncio.sleep(0)
        assert job['negative_result_count']==100
        assert len(job['command_result_anomalies'])==1
        assert job['command_result_anomalies'][0]['end']==99
        assert job['command_result_anomalies'][0]['kind']=='server_negative_unverified'
    finally:
        task.cancel()
        with pytest.raises(asyncio.CancelledError):await task


async def test_session_result_context_is_delivered_only_for_negative_reply():
    queue=asyncio.Queue(maxsize=2)
    session=SimpleNamespace(_ai_result_contexts={'a':{'index':1},'b':{'index':2}},
        _command_result_subscribers={queue},_command_result_dropped=0,_ai_command_futures={})
    BedrockSession._handle_after_execute_command(session,{b'uuid':b'a',b'executeResult':False})
    BedrockSession._handle_after_execute_command(session,{b'uuid':b'b',b'executeResult':True})
    assert queue.qsize()==1 and (await queue.get())=={'index':1}
    assert not session._ai_result_contexts


async def test_reconnect_budget_pauses_without_a_crash_or_false_completion(tmp_path,monkeypatch):
    import proloader.importer as module
    from proloader.bots import private_json
    calls=[]
    async def attempt(*a,**kw):
        calls.append(kw)
        path=tmp_path/'jobs'/(kw['reserved_id']+'.json')
        job=json.loads(path.read_text()) if path.exists() else dict(id=kw['reserved_id'],submitted=123)
        private_json(path,job)
        raise BedrockLoginError('connection lost')
    monkeypatch.setattr(module,'_execute_attempt',attempt)
    monkeypatch.setattr(module.asyncio,'sleep',AsyncMock())
    result=await execute_import(None,None,None,'server','',reconnect_attempts=1,jobs_root=tmp_path/'jobs')
    assert len(calls)==2 and result['state']=='paused'
    assert len(result['connection_interruptions'])==2


async def test_manual_repair_disconnect_resumes_without_initial_rebuild(tmp_path,monkeypatch):
    import proloader.importer as module
    placements=[];interrupted=[False];actions=iter(('repair','done'));connections=[]
    session=SimpleNamespace(_raise_if_failed=lambda:None,
        _raknet=SimpleNamespace(_closed=False,_send_queue=asyncio.Queue(),_pending_acks={}))
    @asynccontextmanager
    async def connect(*a,**kw):connections.append(True);yield session,'bot'
    class World:
        def __init__(self,*a):self.session=session;self.name='bot'
        async def send(self,*a,**kw):return ''
        async def teleport(self,*a):pass
        async def check(self,*a):return True
        async def place(self,c,**kw):
            if getattr(self,'command_phase',None)=='manual_repair' and not interrupted[0]:
                interrupted[0]=True;raise BedrockLoginError('connection lost')
            placements.append(c)
    class Console:
        def __init__(self,*a,**kw):pass
        async def __aenter__(self):return self
        async def __aexit__(self,*a):pass
        async def notice(self,*a):pass
        async def next_action(self):return next(actions),'player'
    monkeypatch.setattr(module,'WorldCommands',World);monkeypatch.setattr(module,'GameConsole',Console)
    monkeypatch.setattr(module,'rental_session',connect);monkeypatch.setattr(module.asyncio,'sleep',AsyncMock())
    p=plan(tmp_path)
    result=await execute_import(p,None,SimpleNamespace(id='fixture'),'server','',verify='none',
        stay_online=True,reconnect_attempts=1,jobs_root=tmp_path/'jobs')
    assert result['state']=='finished' and len(connections)==2
    assert len(placements)==1+2*len(p.commands)
    assert len(result['connection_interruptions'])==1
    assert not result['manual_repair_in_progress']


async def test_failed_console_entry_cleans_up_tasks_and_subscription():
    queue=asyncio.Queue(maxsize=128);unsubscribed=[]
    def failed():raise BedrockLoginError('terminal connection failure')
    session=SimpleNamespace(subscribe_chat=lambda:queue,unsubscribe_chat=unsubscribed.append,
        _raise_if_failed=failed,_raknet=SimpleNamespace(_send_queue=asyncio.Queue()))
    world=SimpleNamespace(session=session,name='bot',send=AsyncMock(side_effect=BedrockLoginError('closed')))
    p=SimpleNamespace(commands=[],structure=SimpleNamespace(name='fixture'))
    game=GameConsole(world,p,{},enabled=True)
    with pytest.raises(BedrockLoginError):
        async with game:raise AssertionError('entry must not succeed')
    assert unsubscribed==[queue] and all(task.done() for task in game.tasks)


@pytest.mark.parametrize('stage,counter,total,label',[
 ('verifying','checked',100,'世界校验'),
 ('command_block_data','command_blocks_next',77,'命令方块NBT'),
 ('command_block_repair','command_blocks_next',77,'命令方块修复'),
 ('block_entity_data','block_entities_next',82,'容器/告示牌NBT'),
])
def test_hud_reports_current_phase_instead_of_completed_placement(stage,counter,total,label):
    p=SimpleNamespace(commands=[None]*500,block_count=100,structure=SimpleNamespace(name='冰河禁区'))
    j=dict(state=stage,submitted=500,verify_mode='complete',verification_total=100,
           plan=dict(command_blocks=77,block_entities=82),phase_telemetry=dict(rate=10,eta_seconds=5))
    j[counter]=12
    text=hud(p,j)
    assert label in text and f'12 / {total}' in text and '500 / 500' not in text


async def test_game_panel_refreshes_every_half_second(monkeypatch):
    import proloader.game_ui as module
    delays=[]
    async def sleep(seconds):delays.append(seconds);raise asyncio.CancelledError()
    monkeypatch.setattr(module.asyncio,'sleep',sleep)
    session=SimpleNamespace(_raknet=SimpleNamespace(_send_queue=asyncio.Queue()))
    w=SimpleNamespace(session=session,send=AsyncMock())
    p=SimpleNamespace(commands=[1],structure=SimpleNamespace(name='fixture'))
    game=GameConsole(w,p,dict(state='placing',submitted=0))
    with pytest.raises(asyncio.CancelledError):await game._panel()
    assert len(delays)==1 and .45<=delays[0]<=.5
    assert 'titleraw @a actionbar' in w.send.call_args.args[0]


def test_hud_keeps_unsupported_nbt_visible_after_supported_data_is_verified():
    p=SimpleNamespace(commands=[1],structure=SimpleNamespace(name='fixture'))
    text=hud(p,dict(state='awaiting_action',submitted=1,plan=dict(unsupported_block_entities=4),block_entities_state='readback_verified'))
    assert '未支持NBT：4' in text
