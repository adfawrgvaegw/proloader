import asyncio
from dataclasses import replace
from types import SimpleNamespace
from unittest.mock import AsyncMock

import pytest
from proloader.command_block_delivery import stage_command_blocks,read_batch
from proloader.command_blocks import CommandBlockData


def block(x):
    return CommandBlockData((x,80,0),'minecraft:command_block',{'facing_direction':5},
                            'say test','test',True,False,True,0,False)


def nbt(b):
    return dict(x=b.position[0],y=80,z=0,id='CommandBlock',Command=b.command,
                CustomName=b.name,auto=0,TrackOutput=1,TickDelay=0,ExecuteOnFirstTick=0)


async def test_repair_shell_retry_read_and_resume_without_blind_writes():
    b=block(1);data=nbt(b);state={'shell':True,'reads':0};job={};packets=[]
    async def read(*a):
        state['reads']+=1
        if state['reads']==1:raise asyncio.TimeoutError()
        return dict(nbt=[data])
    async def place(*a,**kw):state['shell']=True
    session=SimpleNamespace(read_subchunk=read,_raise_if_failed=lambda:None)
    session.send_packet=AsyncMock(side_effect=lambda p:packets.append(p))
    world=SimpleNamespace(check_command_block=AsyncMock(side_effect=lambda b:state['shell']),
                          place=place,send=AsyncMock())
    await stage_command_blocks([b],session,world,'overworld',job,lambda:None,AsyncMock(),lambda *a,**kw:None)
    assert job['command_blocks_next']==1 and state['shell']
    assert not packets  # The data was correct after a lost read; do not replay it.
    assert len(job['command_blocks_recoveries'])==1


async def test_nearby_writes_share_reads_and_do_not_advance_until_all_verified():
    blocks=[block(1),block(2),block(3)];data=[dict(nbt(b),Command='old') for b in blocks];calls=[]
    async def read(*a):calls.append('read');return dict(nbt=[dict(n) for n in data])
    async def send(p):
        calls.append('write');data[calls.count('write')-1]['Command']='say test'
    session=SimpleNamespace(read_subchunk=read,send_packet=send,_raise_if_failed=lambda:None,
        _raknet=SimpleNamespace(_send_queue=asyncio.Queue(),_pending_acks={}))
    world=SimpleNamespace(check_command_block=AsyncMock(return_value=True),send=AsyncMock())
    job={};saved=[]
    await stage_command_blocks(blocks,session,world,'overworld',job,lambda:saved.append(dict(job)),AsyncMock(),lambda *a,**kw:None)
    assert calls==['read','write','write','write','read']
    assert job['command_blocks_next']==3
    assert all(s.get('command_blocks_next',0)==0 for s in saved[:-1])


async def test_unrelated_actor_error_is_isolated_but_target_error_is_rejected():
    b=block(1)
    session=SimpleNamespace(read_subchunk=AsyncMock(return_value=dict(nbt=[nbt(b),dict(x=4,y=80,z=0,_decode_error='unknown')])))
    assert await read_batch(session,[b],'overworld')==[nbt(b)]
    session.read_subchunk.return_value['nbt'][0]['_decode_error']='bad target'
    with pytest.raises(ValueError):await read_batch(session,[b],'overworld')


async def test_nbt_window_readiness_failure_keeps_cursor_and_sends_no_updates():
    session=SimpleNamespace(send_packet=AsyncMock(),read_subchunk=AsyncMock())
    world=SimpleNamespace(chunk_window=SimpleNamespace(ensure=AsyncMock(side_effect=asyncio.TimeoutError())))
    job={}
    with pytest.raises(asyncio.TimeoutError):
        await stage_command_blocks([block(1)],session,world,'overworld',job,lambda:None,AsyncMock(),lambda *a,**kw:None)
    session.send_packet.assert_not_awaited();session.read_subchunk.assert_not_awaited()
    assert job.get('command_blocks_next',0)==0


async def test_batch_reach_uses_actual_player_height_instead_of_first_block_height():
    first=replace(block(0),position=(0,85,0));second=replace(block(3),position=(3,80,0))
    data=[dict(nbt(b),y=b.position[1]) for b in (first,second)]
    session=SimpleNamespace(read_subchunk=AsyncMock(return_value={'nbt':data}),send_packet=AsyncMock(),_raise_if_failed=lambda:None)
    world=SimpleNamespace(check_command_block=AsyncMock(return_value=True),send=AsyncMock())
    move=AsyncMock();job={}
    await stage_command_blocks([first,second],session,world,'overworld',job,lambda:None,move,lambda *a,**kw:None)
    from proloader.command_block_delivery import reachable
    assert move.await_count==1
    assert all(reachable(move.call_args.args[1],b.position) for b in (first,second))
    assert job['command_blocks_next']==2
    session.send_packet.assert_not_awaited()
