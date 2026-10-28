import asyncio
from types import SimpleNamespace
from unittest.mock import AsyncMock
import pytest
from proloader.chunk_loading import ChunkWindow
from proloader.chunk_loading import StationaryWindow


def window(job=None):
    session=SimpleNamespace(read_chunk_window=AsyncMock(return_value=[]),read_subchunk=AsyncMock(return_value={}),_raise_if_failed=lambda:None)
    world=SimpleNamespace(session=session,send=AsyncMock(return_value=''),teleport=AsyncMock())
    plan=SimpleNamespace(origin=(4,80,4),dimension='overworld',structure=SimpleNamespace(size=(100,10,100)))
    job=job if job is not None else dict(id='a'*32)
    return ChunkWindow(world,plan,job,lambda:None,AsyncMock(),lambda *a,**kw:None)


async def test_temporary_area_is_owned_bounded_read_proven_and_removed():
    w=window();c=('fill',4,80,4,19,80,19,'minecraft:stone',{})
    async with w:
        await w.ensure(c);await w.ensure(c)
        assert w.world.session.read_chunk_window.call_count==1
        commands=[call.args[0] for call in w.world.send.call_args_list]
        assert commands==[f'tickingarea add 4 80 4 51 80 51 {w.name}',f'tickingarea preload {w.name} true']
        assert w.job['loading_area_pending']['name']==w.name
    assert w.world.send.call_args.args[0]==f'tickingarea remove {w.name}'
    assert 'loading_area_pending' not in w.job


async def test_unavailable_chunk_is_retried_before_any_caller_write():
    w=window();w.world.session.read_chunk_window.side_effect=[ValueError('unloaded'),[]]
    async with w:await w.ensure(('setblock',4,80,4,4,80,4,'minecraft:stone',{}))
    assert w.world.session.read_chunk_window.call_count==1
    assert w.world.session.read_subchunk.call_count==16
    assert w.job['loading_single_fallbacks']==1


async def test_unconfirmed_cleanup_keeps_exact_owned_area_for_resume():
    from proloader.net.bedrock.session import BedrockLoginError
    w=window(dict(id='b'*32,loading_area_pending=dict(name='plimp_'+'b'*32)))
    w.world.send.side_effect=BedrockLoginError('unconfirmed')
    await w.__aexit__(None,None,None)
    assert w.job['loading_area_pending'] and w.job['loading_area_cleanup_error']=='remove_unconfirmed'
    assert w.world.send.call_args.args[0]=='tickingarea remove plimp_'+'b'*32


async def test_failed_neighbour_window_can_progress_only_on_proven_target_columns():
    w=window();w.world.session.read_chunk_window.side_effect=ValueError('neighbour unavailable')
    async def read(pos,*args,**kwargs):
        if pos[1]!=85:raise ValueError('unavailable lower cube')
        return {}
    w.world.session.read_subchunk.side_effect=read
    first=('setblock',4,85,4,4,85,4,'minecraft:stone',{})
    second=('setblock',20,85,4,20,85,4,'minecraft:stone',{})
    async with w:
        await w.ensure(first)
        assert not w.full_window and w.proven_columns=={(0,0)}
        before=w.world.session.read_subchunk.await_count
        await w.ensure(first)
        assert w.world.session.read_subchunk.await_count==before
        await w.ensure(second)
        assert w.proven_columns=={(0,0),(1,0)}
        assert w.world.session.read_subchunk.await_count==before+1
    assert not w.proven_columns and not w.job.get('loading_area_pending')


async def test_empty_native_area_requires_positive_server_query_inside_source_footprint():
    w=window();w.world.session.read_subchunk.side_effect=ValueError('no native payload')
    c=('fill',4,85,4,19,85,4,'minecraft:stone',{})
    await w.prove_footprint(c)
    queries=[call.args[0] for call in w.world.send.call_args_list]
    assert queries==['execute in overworld run testforblocks 4 85 4 4 85 4 4 85 4 all',
                     'execute in overworld run testforblocks 16 85 4 16 85 4 16 85 4 all']
    assert w.world.session.read_subchunk.await_count==6
    assert w.proven_columns=={(0,0),(1,0)} and w.job['loading_command_probes']==2


async def test_unconfirmed_server_query_never_marks_empty_native_column_ready():
    w=window();w.world.session.read_subchunk.side_effect=ValueError('no native payload')
    w.world.send.return_value=None
    with pytest.raises(ValueError,match='未确认'):
        await w.prove_footprint(('setblock',4,85,4,4,85,4,'minecraft:stone',{}))
    assert not w.proven_columns and not w.job.get('loading_command_probes')


def stationary(commands,size=(300,10,300),origin=(-19,80,37)):
    w=window()
    w.plan.origin=origin;w.plan.structure.size=size;w.plan.commands=commands
    return StationaryWindow(w.world,w.plan,w.job,w.save,w.drain,w.update)


async def test_stationary_coverage_reuses_whole_footprint_without_native_reads_or_movement():
    commands=[('fill',-19,80,37,28,80,84,'minecraft:stone',{}),
              ('setblock',120,85,160,120,85,160,'minecraft:stone',{})]
    w=stationary(commands)
    async with w:
        for c in commands:await w.ensure(c)
        assert len(w.proven_columns)<=100 and w.full_window
        assert w.job['loaded_windows']==1
        w.world.teleport.assert_not_awaited()
        w.world.session.read_chunk_window.assert_not_awaited()
        queries=[c.args[0] for c in w.world.send.call_args_list if 'if blocks' in c.args[0]]
        assert sum(q.count('if blocks') for q in queries)==len(w.proven_columns)
    assert 'loading_area_pending' not in w.job


async def test_stationary_large_source_slices_respect_world_chunk_limit_and_negative_origins():
    commands=[('setblock',-19,80,37,-19,80,37,'minecraft:stone',{}),
              ('setblock',270,80,326,270,80,326,'minecraft:stone',{})]
    w=stationary(commands)
    async with w:
        for c in commands:
            await w.ensure(c)
            assert len(w.proven_columns)<=100
            assert w.contains(w.key,c)
        assert w.job['loaded_windows']==2
        w.world.teleport.assert_not_awaited()


async def test_stationary_unknown_readiness_stops_before_placement_and_cleans_owned_area():
    commands=[('setblock',4,80,4,4,80,4,'minecraft:stone',{})]
    w=stationary(commands,size=(100,10,100),origin=(4,80,4))
    w.readiness_attempts=2
    async def send(command,**kw):return None if ('if blocks' in command or 'testforblocks' in command) else ''
    w.world.send.side_effect=send
    with pytest.raises(ValueError,match='只读确认'):
        async with w:await w.ensure(commands[0])
    assert not w.full_window and not w.proven_columns
    assert 'loading_area_pending' not in w.job
    w.world.teleport.assert_not_awaited()


async def test_stationary_empty_neighbours_only_prove_actual_columns_without_moving():
    commands=[('setblock',-19,85,37,-19,85,37,'minecraft:stone',{}),
              ('setblock',120,85,160,120,85,160,'minecraft:stone',{})]
    w=stationary(commands)
    async def send(command,**kw):
        if 'if blocks' in command:raise ValueError('empty neighbour unavailable')
        return ''
    w.world.send.side_effect=send
    async with w:
        await w.ensure(commands[0])
        assert not w.full_window and w.proven_columns=={(-2,2)}
        before=w.world.send.await_count
        await w.ensure(commands[0])
        assert w.world.send.await_count==before
        await w.ensure(commands[1])
        assert w.proven_columns=={(-2,2),(7,10)} and not w.full_window
        assert w.job['loaded_windows']==1
        w.world.teleport.assert_not_awaited()
