import asyncio
import copy
import json
from pathlib import Path
import tempfile
from unittest.mock import AsyncMock

import pytest

from proloader.ai.program import load, validate, compile_program
from proloader.ai.program import layout
from proloader.ai.spec import lint
from proloader.ai.bridge import Controller, serve, call
from proloader.command_blocks import plan_command_blocks
from proloader.parser.mcstructure import parse_mcstructure_bytes


def source():
    return load('examples/ai/counter.json')


def test_compile_roundtrip_preserves_commands_and_geometry(tmp_path):
    p = source()
    p['blocks'][0].update(type='repeat',facing='up',auto=True,delay=17,first_tick=True)
    file = tmp_path/'test.mcstructure'
    compile_program(p,file)
    s = parse_mcstructure_bytes(file.read_bytes())
    entries = plan_command_blocks(s,tuple(p['origin']))
    assert len(entries)==2
    assert entries[0].command==p['blocks'][0]['command']
    assert entries[0].block_name=='minecraft:repeating_command_block'
    assert entries[0].states['facing_direction']==1
    assert entries[0].tick_delay==17
    assert entries[0].needs_redstone is False
    assert entries[0].execute_on_first_tick is True
    assert entries[1].conditional is True
    assert entries[1].position==(12001,284,12014)


@pytest.mark.parametrize('change',[
    lambda p:p.update(edition='java'),
    lambda p:p['blocks'].append(copy.deepcopy(p['blocks'][0])),
    lambda p:p['blocks'][0].update(at=[True,0,0]),
    lambda p:p['blocks'][0].update(at=[2000001,0,0]),
    lambda p:p['blocks'][0].update(auto='false'),
    lambda p:p['blocks'][0].update(facing='eats'),
    lambda p:p['blocks'][0].update(command='say hi\nkill @a'),
    lambda p:p['blocks'][0].update(delai=20),
    lambda p:p.update(origin=[0,320,0]),
])
def test_invalid_program_refused(change):
    p = source()
    change(p)
    with pytest.raises(ValueError):validate(p)


def controller():
    class Session:
        def _raise_if_failed(self):pass
        run_command_ai=AsyncMock(return_value='')
    return Controller(Session(),'test')


async def test_retry_id_does_not_repeat_non_idempotent_command():
    c = controller()
    r = dict(id='same',op='command',command='scoreboard players add x foo 1')
    assert (await c.request(r))['ok']
    assert (await c.request(r))['ok']
    c.session.run_command_ai.assert_awaited_once()
    assert not (await c.request(dict(r,command='kill @s')))['ok']


async def test_batch_prevalidate_and_stop_on_unknown_result():
    c = controller()
    r = await c.request(dict(id='invalid',op='batch',commands=['say valid','say bad\nkill @a']))
    assert not r['ok']
    c.session.run_command_ai.assert_not_awaited()
    c.session.run_command_ai.side_effect=['',None,'']
    r = await c.request(dict(id='batch',op='batch',commands=['say a','say b','say c']))
    assert r['result']['submitted']==2
    assert not r['result']['all_confirmed']


async def test_local_socket_lifecycle_and_singleton(tmp_path,capsys):
    directory = Path(tempfile.mkdtemp(prefix='plai-',dir='/tmp'))
    path=directory/'ai.sock'
    c=controller()
    task=asyncio.create_task(serve(c,path))
    try:
        for _ in range(100):
            if path.exists():break
            await asyncio.sleep(.01)
        assert (path.stat().st_mode & 0o777)==0o600
        r=await call(dict(id='status',op='status'),path)
        assert r['ok'] and r['result']['online']
        with pytest.raises(BlockingIOError):await serve(controller(),path)
        assert path.exists()
        await call(dict(id='stop',op='stop'),path)
        await asyncio.wait_for(task,2)
        assert not path.exists()
    finally:
        c.stopped.set()
        await task
        (directory/'ai.sock.lock').unlink(missing_ok=True)
        directory.rmdir()


async def test_deploy_checks_all_air_before_first_write(monkeypatch):
    from proloader.ai import bridge
    c=controller()
    world=type('World',(),{})()
    world.check=AsyncMock(side_effect=[True,False])
    world.place=AsyncMock()
    monkeypatch.setattr(c,'near',AsyncMock(return_value=(0,80,0)))
    monkeypatch.setattr(c,'world',lambda p:world)
    r=await c.request(dict(id='deploy',op='deploy',program=source()))
    assert not r['ok']
    world.place.assert_not_awaited()


async def test_arm_checks_every_block_before_activation(monkeypatch):
    c=controller()
    monkeypatch.setattr(c,'near',AsyncMock())
    monkeypatch.setattr(c,'check',AsyncMock(side_effect=[{},ValueError('mismatch')]))
    update=AsyncMock()
    monkeypatch.setattr(c,'update',update)
    r=await c.request(dict(id='arm',op='arm',program=source()))
    assert not r['ok']
    update.assert_not_awaited()


def test_large_serpentine_layout_points_to_next_chain(tmp_path):
    p=layout('\n'.join(f'scoreboard players add p{i} plai_test 1' for i in range(1024)),[0,80,0])
    directions={'east':(1,0,0),'west':(-1,0,0),'south':(0,0,1)}
    for a,b in zip(p['blocks'],p['blocks'][1:]):
        assert tuple(x+d for x,d in zip(a['at'],directions[a['facing']]))==tuple(b['at'])
    file=tmp_path/'large.mcstructure'
    compile_program(p,file)
    assert len(plan_command_blocks(parse_mcstructure_bytes(file.read_bytes()),(0,80,0)))==1024


@pytest.mark.parametrize('text',[
    'execute as @e[limit=1] run say hi',
    'execute store result score n obj run time query daytime',
    'data merge block 0 80 0 {Command:"say hi"}',
    'give @s stone{foo:1}',
    'setblock 0 80 0 command_block {Command:"say hi"}',
])
def test_java_mistakes_are_flagged(text):
    assert any(e['severity']=='error' for e in lint(text))


def test_quoted_payload_not_mistaken_for_java_syntax():
    assert lint('tellraw @a {"rawtext":[{"text":"execute store distance= limit= nbt="}]}')==[]
    assert lint('execute as @a[tag=!pl_sm_bot] at @s run tp @s ~ ~1 ~','block')==[]
    assert lint('tp @s ~ ~1 ~','block')[0]['code']=='block-self'


async def test_status_during_long_operation_is_nonblocking():
    c=controller()
    async with c.lock:
        r=await c.request(dict(id='status-busy',op='status'))
        assert r['result']['busy']


async def test_cancelled_write_id_is_not_replayed_after_reconnect():
    c=controller()
    c.session.run_command_ai.side_effect=asyncio.CancelledError
    r=dict(id='interrupted',op='command',command='give @s stone')
    with pytest.raises(asyncio.CancelledError):await c.request(r)
    result=await c.request(r)
    assert result['error_type']=='CancelledError'
    c.session.run_command_ai.assert_awaited_once()


async def test_reconnect_does_not_replay_and_explicit_stop_does_not_reconnect(monkeypatch,tmp_path):
    from contextlib import asynccontextmanager
    from types import SimpleNamespace
    from proloader.ai import bridge
    from proloader.net.netease import runtime
    from proloader.net.bedrock.session import BedrockLoginError
    import proloader.bots
    monkeypatch.setattr(proloader.bots,'BotStore',lambda:SimpleNamespace(list=lambda:[SimpleNamespace(id='b',label='bot')]))
    @asynccontextmanager
    async def rental(*args,**kwargs):yield controller().session,'bot'
    monkeypatch.setattr(runtime,'rental_session',rental)
    calls=[]
    async def fake_serve(c,path):
        calls.append(c)
        if len(calls)==1:
            await c.request(dict(id='once',op='command',command='give @s stone'))
            raise BedrockLoginError('disconnected')
        r=await c.request(dict(id='once',op='command',command='give @s stone'))
        assert r['ok']
        c.session.run_command_ai.assert_not_awaited()
    monkeypatch.setattr(bridge,'serve',fake_serve)
    monkeypatch.setattr(asyncio.get_running_loop(),'add_signal_handler',lambda *a:None)
    monkeypatch.setattr(bridge.asyncio,'sleep',AsyncMock())
    await bridge.run_daemon(None,'server','',tmp_path/'socket')
    assert len(calls)==2
