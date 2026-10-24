from pathlib import Path
import json
import numpy as np
import pytest

from proloader.fatalder import native_plan, native_binary, parse_command, validate_coverage
from proloader.pacing import CommandPacer, DEFAULT_SPEED
from proloader.model.block import Block, BlockState
from proloader.model.palette import BlockPalette
from proloader.model.region import Region
from proloader.model.structure import Structure


def test_native_cross_group_negative_origin_and_air_preservation():
    palette=BlockPalette()
    stone=palette.add(Block(BlockState('minecraft:stone')))
    blocks=np.zeros((66,3,74),dtype=np.uint32)
    blocks[:,0,:]=stone
    blocks[0,2,0]=stone
    blocks[65,2,73]=stone
    structure=Structure('groups',(66,3,74),palette,Region(palette,blocks))
    origin=(-85,78,351)
    commands=native_plan(structure,origin,native_binary())
    assert sum((c[4]-c[1]+1)*(c[5]-c[2]+1)*(c[6]-c[3]+1) for c in commands)==66*74+2
    assert all(c[4]-c[1]<48 and c[6]-c[3]<48 for c in commands)
    assert len(commands)<20


def test_validation_rejects_air_widening_and_duplicate_writes():
    palette=BlockPalette();stone=palette.add(Block(BlockState('minecraft:stone')))
    blocks=np.zeros((2,1,2),dtype=np.uint32);blocks[0,0,0]=stone
    structure=Structure('sparse',(2,1,2),palette,Region(palette,blocks))
    c=('setblock',0,0,0,0,0,0,'minecraft:stone',{})
    validate_coverage(structure,(0,0,0),[c])
    with pytest.raises(ValueError):validate_coverage(structure,(0,0,0),[c,c])
    with pytest.raises(ValueError):validate_coverage(structure,(0,0,0),[('fill',0,0,0,1,0,1,'minecraft:stone',{})])


def test_native_state_parse_keeps_typed_values():
    c=parse_command('fill -1 78 2 3 79 4 minecraft:test ["name"="a=b","v"=false,"n"=2]')
    assert c[8]=={'name':'a=b','v':False,'n':2}
    with pytest.raises(ValueError):parse_command('setblock 0 0 0 minecraft:stone [invalid]')


async def test_user_default_rate_keeps_initial_burst_small(monkeypatch):
    import proloader.pacing as pacing
    now=[100.]
    sleeps=[]
    monkeypatch.setattr(pacing.time,'monotonic',lambda:now[0])
    async def sleep(seconds):sleeps.append(seconds);now[0]+=seconds
    monkeypatch.setattr(pacing.asyncio,'sleep',sleep)
    pacer=CommandPacer()
    assert pacer.speed==DEFAULT_SPEED==4000
    for _ in range(10):await pacer.wait()
    assert not sleeps
    await pacer.wait()
    assert sleeps==[pytest.approx(1/4000)]


def test_legacy_slab_wire_names_keep_half_and_double_geometry():
    from proloader.block_names import command_state
    assert command_state('minecraft:stone_block_slab4',{'stone_slab_type_4':'smooth_quartz','minecraft:vertical_half':'top'}) == ('minecraft:smooth_quartz_slab',{'minecraft:vertical_half':'top'})
    assert command_state('minecraft:double_stone_block_slab3',{'stone_slab_type_3':'polished_andesite','minecraft:vertical_half':'bottom'}) == ('minecraft:polished_andesite_double_slab',{'minecraft:vertical_half':'bottom'})
    assert command_state('minecraft:stone_block_slab4',{'stone_slab_type_4':'unknown'}) == ('minecraft:stone_block_slab4',{'stone_slab_type_4':'unknown'})


def test_file_backed_exact_palette_keeps_new_names_and_states(tmp_path):
    palette=BlockPalette()
    state=BlockState('minecraft:leaf_litter',{'growth':2,'minecraft:cardinal_direction':'south'})
    leaf=palette.add(Block(state))
    raw=tmp_path/'blocks.bin'
    np.array([leaf,leaf],dtype='<u4').tofile(raw)
    blocks=np.memmap(raw,dtype='<u4',mode='r',shape=(2,1,1))
    structure=Structure('modern-world',(2,1,1),palette,Region(palette,blocks),
        source_info={'blocks_file':str(raw),'exact_states':True})
    commands=native_plan(structure,(130,77,-483),native_binary())
    assert len(commands)==1
    assert commands[0][7:]==(state.name,state.states)
    validate_coverage(structure,(130,77,-483),commands)
