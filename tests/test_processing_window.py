import asyncio
from types import SimpleNamespace
from unittest.mock import AsyncMock

import pytest

from proloader.pacing import ProcessingWindow,CommandPacer
from proloader.importer import WorldCommands,CommandUnconfirmedError


async def test_pipeline_keeps_sending_until_limit_and_drains_partial_tail():
    gate=asyncio.Event();calls=[];metrics={}
    async def probe(begin,end,phase):
        calls.append((begin,end,phase));await gate.wait()
    async with ProcessingWindow(probe,metrics) as window:
        for i in range(768):await window.before();await window.after(i)
        assert len(calls)==3 and metrics['max_processing_window']==768
        blocked=asyncio.create_task(window.before());await asyncio.sleep(0)
        assert not blocked.done()
        gate.set();await blocked
        for i in range(768,800):await window.before();await window.after(i)
        await window.drain()
        assert window.sent==window.released==800
    assert calls==[(0,256,'initial'),(256,512,'initial'),(512,768,'initial'),(768,800,'initial')]


async def test_probe_ranges_remain_exact_when_resuming_and_switching_to_repair():
    calls=[]
    async def probe(*args):calls.append(args)
    async with ProcessingWindow(probe,{},interval=2,limit=4) as window:
        for i in range(900,903):await window.before();await window.after(i)
        await window.drain()
        for i in range(3):await window.before();await window.after(i,'repair')
        await window.drain()
    assert calls==[(900,902,'initial'),(902,903,'initial'),(0,2,'repair'),(2,3,'repair')]


async def test_context_exit_cancels_and_joins_pending_probes():
    cancelled=[];started=asyncio.Event()
    async def probe(*args):
        started.set()
        try:await asyncio.Event().wait()
        finally:cancelled.append(True)
    async with ProcessingWindow(probe,{},interval=1,limit=2) as window:
        await window.after(0);await started.wait()
        tasks=[t for _,t in window.pending]
    assert cancelled and all(t.done() for t in tasks)


async def test_terminal_probe_error_reaches_sender_before_more_commands():
    async def probe(*args):raise RuntimeError('disconnected')
    async with ProcessingWindow(probe,{},interval=1,limit=2) as window:
        await window.after(0)
        with pytest.raises(RuntimeError,match='disconnected'):await window.before()


async def test_pacer_does_not_accumulate_huge_burst_after_idle(monkeypatch):
    import proloader.pacing as module
    now=[0.]
    monkeypatch.setattr(module.time,'monotonic',lambda:now[0])
    async def sleep(delay):now[0]+=delay
    monkeypatch.setattr(module.asyncio,'sleep',sleep)
    pacer=CommandPacer(4000)
    for _ in range(10):await pacer.wait()
    assert now[0]==0
    await pacer.wait();assert now[0]>0
    now[0]=100
    await pacer.wait();assert pacer.tokens==9
    for _ in range(10):await pacer.wait()
    assert now[0]>100


async def test_fast_probe_records_failure_without_three_long_retries():
    session=SimpleNamespace(_raise_if_failed=lambda:None,run_command_ai=AsyncMock(return_value=None))
    with pytest.raises(CommandUnconfirmedError):
        await WorldCommands(session,'bot',None).send('testfor @s',fast_probe=True)
    session.run_command_ai.assert_awaited_once()
    assert session.run_command_ai.call_args.kwargs['timeout']==12


async def test_teleport_wait_reduced_only_after_position_confirmation(monkeypatch):
    import proloader.importer as module
    events=[]
    async def command(cmd,**kwargs):events.append(cmd);return ''
    async def sleep(delay):events.append(delay)
    monkeypatch.setattr(module.asyncio,'sleep',sleep)
    session=SimpleNamespace(_raise_if_failed=lambda:None,run_command_ai=command)
    await WorldCommands(session,'bot',SimpleNamespace(dimension='overworld')).teleport(1,80,2)
    assert 'tp @s 1 80 2' in events[0]
    assert 'testfor @s[x=1,y=80,z=2,r=3]' in events[1]
    assert events[2]==.25
