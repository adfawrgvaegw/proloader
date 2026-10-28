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


def test_nemc_normal_stone_slabs_preserve_material_and_geometry():
    from proloader.block_names import command_state
    for name in ('minecraft:stone_block_slab4','minecraft:double_stone_block_slab4'):
        states={'minecraft:vertical_half':'top','stone_slab_type_4':'stone'}
        expected='minecraft:normal_stone_'+('double_slab' if name.startswith('minecraft:double_') else 'slab')
        assert command_state(name,states)==(expected,{'minecraft:vertical_half':'top'})
    assert command_state('minecraft:quartz_block',{'chisel_type':'smooth','pillar_axis':'y'})==(
        'minecraft:smooth_quartz',{'pillar_axis':'y'})


def test_bdx_literal_supports_and_in_memory_exact_states_are_preserved():
    palette=BlockPalette()
    support=palette.add(Block(BlockState('minecraft:structure_void',{'structure_void_type':'air'})))
    carpet=palette.add(Block(BlockState('minecraft:gray_carpet')))
    furnace=palette.add(Block(BlockState('minecraft:furnace',{'minecraft:cardinal_direction':'north'})))
    blocks=np.array([support,carpet,furnace],dtype=np.uint32).reshape(1,3,1)
    structure=Structure('bdx',(1,3,1),palette,Region(palette,blocks),
        source_info={'source_format':'BDX','exact_states':True})
    commands=native_plan(structure,(0,80,0),native_binary())
    validate_coverage(structure,(0,80,0),commands)
    assert commands[0][7]=='minecraft:structure_void'
    assert commands[-1][7]=='minecraft:gray_carpet'
    assert next(c for c in commands if c[7]=='minecraft:furnace')[8]=={'minecraft:cardinal_direction':'north'}


def test_scattered_supports_are_merged_without_air_widening_or_window_thrash():
    palette=BlockPalette()
    support=palette.add(Block(BlockState('minecraft:structure_void',{'structure_void_type':'air'})))
    carpet=palette.add(Block(BlockState('minecraft:gray_carpet')))
    stone=palette.add(Block(BlockState('minecraft:stone')))
    blocks=np.zeros((100,3,100),dtype=np.uint32)
    # X-major source traversal used to bounce between Z windows every row.
    blocks[:,0,:]=support
    blocks[1,0,20]=0  # A deliberate source gap may never be filled.
    blocks[:,1,:]=carpet
    blocks[:,2,:]=stone
    s=Structure('scattered',(100,3,100),palette,Region(palette,blocks),
        source_info={'source_format':'BDX','exact_states':True})
    origin=(-19,80,37)
    commands=native_plan(s,origin,native_binary())
    validate_coverage(s,origin,commands)
    visits=[]
    for c in commands:
        key=((c[1]-origin[0])//48,(c[3]-origin[2])//48)
        if not visits or visits[-1]!=key:visits.append(key)
        assert (c[4]-origin[0])//48==key[0] and (c[6]-origin[2])//48==key[1]
    assert len(visits)<=18  # Nine groups, structure pass then attachment pass.
    assert visits[:3]==[(0,0),(1,0),(2,0)]
    assert visits[3:6]==[(2,1),(1,1),(0,1)]
    assert sum(c[7]=='minecraft:structure_void' for c in commands)<400
    first_carpet=next(i for i,c in enumerate(commands) if c[7]=='minecraft:gray_carpet')
    assert all(c[7]=='minecraft:gray_carpet' for c in commands[first_carpet:])


async def test_live_wall_hash_is_resolved_without_accepting_other_materials():
    from types import SimpleNamespace
    from proloader.importer import WorldCommands
    palette=BlockPalette()
    props=dict(wall_block_type='cobblestone',wall_connection_type_east='none',wall_connection_type_north='none',
               wall_connection_type_south='none',wall_connection_type_west='none',wall_post_bit=True)
    wall=palette.add(Block(BlockState('minecraft:cobblestone_wall',props)))
    blocks=np.full((1,1,1),wall,dtype=np.uint32)
    s=Structure('wall',(1,1,1),palette,Region(palette,blocks))
    hashes=await WorldCommands(None,'bot',SimpleNamespace(structure=s)).runtime_hashes()
    ids=hashes[json.dumps(['minecraft:cobblestone_wall',props],sort_keys=True)]
    assert 3867560979 in ids  # Live east/west tall join, post disabled.
    assert 3690217760 not in ids  # Air never passes the wall material check.


async def test_bdx_paired_door_uses_lower_orientation_and_repairs_duplicate_lower_flag():
    from types import SimpleNamespace
    from proloader.importer import WorldCommands
    from proloader.block_names import cell_states
    palette=BlockPalette()
    lower=palette.add(Block(BlockState('minecraft:warped_door',dict(direction=3,door_hinge_bit=False,open_bit=False,upper_block_bit=False))))
    upper=palette.add(Block(BlockState('minecraft:warped_door',dict(direction=0,door_hinge_bit=False,open_bit=False,upper_block_bit=False))))
    blocks=np.array([lower,upper],dtype=np.uint32).reshape(1,2,1)
    s=Structure('door',(1,2,1),palette,Region(palette,blocks),source_info={'source_format':'BDX'})
    expected=cell_states(s,(0,1,0))
    assert expected['upper_block_bit'] and expected['direction']==3
    assert not palette.get(upper).state.states['upper_block_bit']
    hashes=await WorldCommands(None,'bot',SimpleNamespace(structure=s)).runtime_hashes()
    assert 3808820045 in hashes[json.dumps(['minecraft:warped_door',expected],sort_keys=True)]
