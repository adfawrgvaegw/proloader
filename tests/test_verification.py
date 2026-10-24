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
