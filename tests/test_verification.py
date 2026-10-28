import asyncio
from types import SimpleNamespace
from unittest.mock import AsyncMock

import numpy as np
import pytest

from proloader.verification import ordered_coords, check_group, check_window
from proloader.net.bedrock.session import BedrockLoginError


def world():
    return SimpleNamespace(relative=lambda x,y,z:f'{x} {y} {z}', context=lambda c:c,
        send=AsyncMock(return_value=''), check=AsyncMock(return_value=True))


def points(n):
    return [(0,0,z,'minecraft:stone',{}) for z in range(n)]


def test_spatial_order_covers_every_nonair_coordinate_once():
    blocks=np.ones((33,2,33),dtype=np.uint32)
    blocks[0,0,0]=0
    result=ordered_coords(blocks,0)
    assert {tuple(c) for c in result}=={tuple(c) for c in np.argwhere(blocks)}
    tiles=[(int(c[0])//16,int(c[2])//16) for c in result]
    assert tiles==sorted(tiles)


async def test_positive_group_proves_all_32_without_single_probes():
    w=world()
    assert await check_group(w,points(32)) == [True]*32
    assert w.send.call_count==1 and w.check.call_count==0
    assert w.send.call_args.args[0].count('if block ')==32


async def test_failed_group_splits_and_identifies_only_actual_failure():
    w=world()
    w.send.side_effect=BedrockLoginError('aiCommand 执行失败')
    w.check.side_effect=lambda x,y,z,*args:z!=3
    assert await check_group(w,points(8))==[True,True,True,False,True,True,True,True]


async def test_connection_error_never_becomes_a_mismatch():
    w=world();w.send.side_effect=ConnectionError('disconnected')
    with pytest.raises(ConnectionError):await check_group(w,points(32))
    w.check.assert_not_called()


async def test_check_window_bounds_concurrency_and_drains_cancelled_tasks():
    w=world()
    with pytest.raises(ValueError):await check_window(w,points(257))
    active=0;maximum=0
    async def send(c):
        nonlocal active,maximum
        active+=1;maximum=max(maximum,active)
        try:await asyncio.sleep(0.01);return ''
        finally:active-=1
    w.send.side_effect=send
    assert await check_window(w,points(256))==[True]*256
    assert maximum==8 and active==0


async def test_chunk_verification_reads_once_and_falls_back_for_unknown_and_negative():
    from proloader.verification import check_subchunk
    w=world();ids=[9]*4096;ids[1]=999;ids[2]=10
    w.plan=SimpleNamespace(dimension='overworld')
    w.session=SimpleNamespace(read_subchunk=AsyncMock(return_value=dict(blocks=ids,palette={
        '9':dict(name='minecraft:stone',states={}), '10':dict(name='minecraft:dirt',states={})})))
    w.send.side_effect=BedrockLoginError('aiCommand 执行失败')
    w.check.side_effect=lambda x,y,z,*args:z!=2
    assert await check_subchunk(w,points(4))==[True,True,False,True]
    assert w.session.read_subchunk.call_count==1 and w.check.call_count==1


async def test_subchunk_error_retries_only_reads_and_never_counts_as_match():
    from proloader.verification import check_subchunk
    w=world();w.plan=SimpleNamespace(dimension='overworld')
    w.session=SimpleNamespace(read_subchunk=AsyncMock(side_effect=asyncio.TimeoutError),_raise_if_failed=lambda:None)
    w.send.side_effect=BedrockLoginError('aiCommand 执行失败')
    w.check.return_value=False
    assert await check_subchunk(w,points(2))==[False,False]
    assert w.session.read_subchunk.call_count==3
    assert w.check.call_count==2


async def test_all_air_cube_is_a_proven_difference_without_command_probes():
    from proloader.verification import check_subchunk
    w=world();w.plan=SimpleNamespace(dimension='overworld')
    w.session=SimpleNamespace(read_subchunk=AsyncMock(return_value=dict(result=6)))
    assert await check_subchunk(w,points(16))==[False]*16
    w.check.assert_not_called();w.send.assert_not_called()


async def test_unknown_runtime_id_can_match_exact_upstream_source_hash():
    import json
    from proloader.verification import check_subchunk
    w=world();w.plan=SimpleNamespace(dimension='overworld')
    w.runtime_hashes=AsyncMock(return_value={json.dumps(['minecraft:stone',{}],sort_keys=True):{98765}})
    w.session=SimpleNamespace(read_subchunk=AsyncMock(return_value=dict(blocks=[98765]*4096,palette={})))
    assert await check_subchunk(w,points(16))==[True]*16
    w.check.assert_not_called();w.send.assert_not_called()


async def test_scan_prefetch_reuses_only_current_horizontal_chunk():
    from proloader.verification import check_subchunk
    w=world();w.plan=SimpleNamespace(dimension='overworld',origin=(0,0,0),structure=SimpleNamespace(size=(32,32,16)))
    w._scan_subchunks={}
    def entries(*a,**kw):return [dict(offset=[0,y,0],result=6) for y in (0,1)]
    w.session=SimpleNamespace(read_subchunks=AsyncMock(side_effect=entries))
    assert await check_subchunk(w,[(0,0,0,'minecraft:stone',{})])==[False]
    assert await check_subchunk(w,[(0,16,0,'minecraft:stone',{})])==[False]
    assert w.session.read_subchunks.call_count==1
    await check_subchunk(w,[(16,0,0,'minecraft:stone',{})])
    assert w.session.read_subchunks.call_count==2
    assert set(w._scan_subchunks)=={(1,0,0),(1,1,0)}


async def test_exact_bdx_states_do_not_pass_a_wrong_furnace_direction():
    from proloader.verification import check_subchunk
    w=world();w.plan=SimpleNamespace(dimension='overworld',structure=SimpleNamespace(source_info={'exact_states':True}))
    w.session=SimpleNamespace(read_subchunk=AsyncMock(return_value=dict(blocks=[9]*4096,palette={
        '9':dict(name='minecraft:furnace',states={'minecraft:cardinal_direction':'south','custom_appearance':0})})))
    assert await check_subchunk(w,[(0,0,0,'minecraft:furnace',{'minecraft:cardinal_direction':'north'})])==[False]
    assert await check_subchunk(w,[(0,0,0,'minecraft:furnace',{'minecraft:cardinal_direction':'south'})])==[True]
    w.send.assert_not_called()


async def test_exact_source_fallback_group_keeps_requested_states():
    w=world();w.plan=SimpleNamespace(structure=SimpleNamespace(source_info={'exact_states':True}))
    p=[(0,0,z,'minecraft:furnace',{'minecraft:cardinal_direction':'north'}) for z in range(2)]
    assert await check_group(w,p)==[True,True]
    assert w.send.call_args.args[0].count('"minecraft:cardinal_direction"="north"')==2


async def test_mixed_height_prefetch_preserves_good_cube_and_probes_unavailable_one():
    from proloader.verification import check_subchunk
    w=world();w.plan=SimpleNamespace(dimension='overworld',origin=(0,0,0),structure=SimpleNamespace(size=(16,32,16)))
    w._scan_subchunks={}
    w.session=SimpleNamespace(read_subchunks=AsyncMock(return_value=[
        dict(offset=[0,0,0],result=1,blocks=[9]*4096,palette={'9':dict(name='minecraft:stone',states={})}),
        dict(offset=[0,1,0],result=2,blocks=[9]*4096,palette={'9':dict(name='minecraft:stone',states={})})]))
    assert await check_subchunk(w,[(0,0,0,'minecraft:stone',{})])==[True]
    w.check.return_value=False
    assert await check_subchunk(w,[(0,16,0,'minecraft:stone',{})])==[False]
    assert w.session.read_subchunks.await_count==1
    assert w.session.read_subchunks.call_args.kwargs['allow_partial']
    w.check.assert_awaited_once()


async def test_wall_joins_are_derived_but_materials_and_furnace_direction_remain_required():
    from proloader.verification import check_subchunk
    w=world();w.plan=SimpleNamespace(dimension='overworld',structure=SimpleNamespace(source_info={'exact_states':True}))
    w.session=SimpleNamespace(read_subchunk=AsyncMock(return_value=dict(blocks=[9]*4096,palette={
        '9':dict(name='minecraft:cobblestone_wall',states={'wall_connection_type_east':'tall','wall_post_bit':False})})))
    p=[(0,0,0,'minecraft:cobblestone_wall',{'wall_block_type':'cobblestone','wall_connection_type_east':'none','wall_post_bit':True})]
    assert await check_subchunk(w,p)==[True]
    w.session.read_subchunk.return_value['palette']['9']['name']='minecraft:polished_deepslate_wall'
    assert await check_subchunk(w,p)==[False]
