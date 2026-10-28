import asyncio
from types import SimpleNamespace
from unittest.mock import AsyncMock
import pytest
from proloader.differential_repair import repair_building
from tests.test_rental_importer import plan


async def test_local_chunk_scan_clips_fills_and_preserves_neighbour_damage(tmp_path):
    import numpy as np
    from proloader.model.block import Block,BlockState
    from proloader.model.palette import BlockPalette
    from proloader.model.structure import Structure
    from proloader.model.region import Region
    p=plan(tmp_path);palette=BlockPalette();stone=palette.add(Block(BlockState('minecraft:stone')))
    p.structure=Structure('local',(64,2,64),palette,Region(palette,np.full((64,2,64),stone,dtype=np.uint32)))
    p.origin=(-8,80,-8)
    p.commands=[('fill',-8,80,-8,55,81,55,'minecraft:stone',{})]
    inside=(-2,80,-2);outside=(1,80,1)
    actual={(x,y,z) for x in range(-8,56) for y in (80,81) for z in range(-8,56)}-{inside,outside}
    reads=[];writes=[];moves=[]
    async def read(pos,dim):
        reads.append(pos);ids=[0]*4096
        for x,y,z in actual:
            if (x//16,y//16,z//16)==tuple(v//16 for v in pos):ids[((x%16)*16+y%16)*16+z%16]=1
        return dict(blocks=ids,palette={'0':dict(name='minecraft:air',states={}), '1':dict(name='minecraft:stone',states={})})
    async def place(c):
        writes.append(c)
        actual.update((x,y,z) for x in range(c[1],c[4]+1) for y in range(c[2],c[5]+1) for z in range(c[3],c[6]+1))
    async def move(world,pos):moves.append(pos)
    session=SimpleNamespace(read_subchunk=read,_raise_if_failed=lambda:None,
        _raknet=SimpleNamespace(_send_queue=asyncio.Queue(),_pending_acks={}))
    world=SimpleNamespace(session=session,plan=p,place=place,teleport=AsyncMock(),send=AsyncMock(return_value=''))
    outside_command=['setblock',*outside,*outside,'minecraft:stone',{}]
    job={'repair_pending':{'outside':outside_command}}
    assert await repair_building(p,session,world,job,lambda:None,move,lambda *a,**kw:None,
        SimpleNamespace(wait=AsyncMock()),chunk=(-1,-1))
    assert inside in actual and outside not in actual
    assert job['checked']==job['passed']==128
    assert job['repair_pending']=={'outside':outside_command}
    assert writes and all(c[1]>=-16 and c[4]<=-1 and c[3]>=-16 and c[6]<=-1 for c in writes)
    assert all(pos[0]//16==pos[2]//16==-1 for pos in reads)
    world.teleport.assert_not_awaited()  # No duplicate positioning inside reader.
    assert job['verification']=='local_chunk_verified'


async def test_unreadable_local_scan_does_not_drop_saved_pending(tmp_path,monkeypatch):
    import proloader.differential_repair as module
    p=plan(tmp_path);c=p.commands[0];chunk=(c[1]//16,c[3]//16)
    job={'repair_pending':{'command:0':c}}
    monkeypatch.setattr(module,'check_subchunk',AsyncMock(side_effect=asyncio.TimeoutError()))
    world=SimpleNamespace()
    with pytest.raises(asyncio.TimeoutError):
        await repair_building(p,None,world,job,lambda:None,AsyncMock(),lambda *a,**kw:None,None,chunk=chunk)
    assert job['repair_pending']=={'command:0':c}

async def test_repair_discovers_missing_block_and_only_rewrites_difference(tmp_path):
    p=plan(tmp_path);placed=[]
    expected={tuple(p.origin[i]+int(c[i]) for i in range(3)) for c in __import__('numpy').argwhere(p.structure.region.blocks)}
    missing=next(iter(expected));actual=set(expected)-{missing}
    async def read(pos,dim):
        ids=[0]*4096
        for x,y,z in actual:
            if x//16==pos[0]//16 and y//16==pos[1]//16 and z//16==pos[2]//16:
                ids[((x%16)*16+y%16)*16+z%16]=1
        return dict(blocks=ids,palette={'0':dict(name='minecraft:air',states={}),
                    '1':dict(name='minecraft:stone',states={})})
    async def place(c):placed.append(c);actual.add(tuple(c[1:4]))
    session=SimpleNamespace(read_subchunk=read,_raise_if_failed=lambda:None,
                           _raknet=SimpleNamespace(_send_queue=asyncio.Queue(),_pending_acks={}))
    w=SimpleNamespace(session=session,plan=p,place=place,teleport=AsyncMock(),
        relative=lambda *p:' '.join(map(str,p)),context=lambda c:c,send=AsyncMock(side_effect=__import__('proloader.net.bedrock.session',fromlist=['BedrockLoginError']).BedrockLoginError('aiCommand 执行失败')),
        check=AsyncMock(side_effect=lambda x,y,z,*a:(x,y,z) in actual))
    # Processing barriers succeed; negative conjunctions split into exact probes.
    from proloader.net.bedrock.session import BedrockLoginError
    async def send(c):
        if c=='testfor @s':return ''
        raise BedrockLoginError('aiCommand 执行失败')
    w.send=send
    job={};stages=[]
    result=await repair_building(p,session,w,job,lambda:None,AsyncMock(),
        lambda stage,**kw:stages.append(stage),SimpleNamespace(wait=AsyncMock()))
    assert result and actual==expected and len(placed)==1
    assert tuple(placed[0][1:4])==missing
    assert job['checked']==job['passed']==len(expected)
    assert not job['repair_pending'] and 'manual_repair_check' in stages
