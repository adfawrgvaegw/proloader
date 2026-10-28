"""Legacy NetEase palette names → current command names, preserving slab geometry.

The server still advertises old aliases, but testforblock rejects them even when
its world stores the equivalent modern block. Smooth quartz single/double were
verified on 2026-10-20 inside the authorized airport target.
"""
WIRE_REVISION='legacy-names-v4-nemc-stone'
MATERIALS={
 'stone_slab_type':{'smooth_stone':'smooth_stone','sandstone':'sandstone','cobblestone':'cobblestone',
                   'brick':'brick','stone_brick':'stone_brick','quartz':'quartz','nether_brick':'nether_brick'},
 'stone_slab_type_2':{'red_sandstone':'red_sandstone','purpur':'purpur','prismarine_rough':'prismarine',
                    'prismarine_dark':'dark_prismarine','prismarine_brick':'prismarine_brick',
                    'mossy_cobblestone':'mossy_cobblestone','smooth_sandstone':'smooth_sandstone',
                    'red_nether_brick':'red_nether_brick'},
 'stone_slab_type_3':{s:s for s in ('end_stone_brick','smooth_red_sandstone','polished_andesite','andesite',
                                  'diorite','polished_diorite','granite','polished_granite')},
 'stone_slab_type_4':{**{s:s for s in ('mossy_stone_brick','smooth_quartz','cut_sandstone','cut_red_sandstone')},'stone':'normal_stone'},
}


def command_state(name,states=None,probe=False):
    states=dict(states or {})
    base=name.removeprefix('minecraft:')
    if probe and base=='dirt' and states.get('dirt_type') in ('normal','coarse'):
        name='minecraft:'+('coarse_dirt' if states.pop('dirt_type')=='coarse' else 'dirt')
    if probe and base=='cobblestone_wall' and states.get('wall_block_type')=='cobblestone':
        states.pop('wall_block_type')
    if probe and base.endswith('_door') and 'direction' in states:
        direction=states.get('direction')
        if type(direction) is int and 0<=direction<4:
            states.pop('direction')
            states['minecraft:cardinal_direction']=('south','west','north','east')[direction]
    # NetEase stores light_block as per-level names (verified live 2026-10-24 on
    # 12345678: placed via states but testforblock only matches light_block_15).
    # Writes keep the modern states form; probes must use the level-suffixed id.
    if probe and base=='light_block':
        level=states.get('block_light_level')
        if isinstance(level,int) or (isinstance(level,str) and level.isdigit()):
            return 'minecraft:light_block_%d'%int(level),{}
    # Split legacy compound IDs into exact modern variants for both writes
    # and name probes. Never fall back to another material for unknown values.
    variants = {
        'stonebrick': ('stone_brick_type', {
            'default': 'stone_bricks', 'mossy': 'mossy_stone_bricks',
            'cracked': 'cracked_stone_bricks', 'chiseled': 'chiseled_stone_bricks'}),
        'quartz_block': ('chisel_type', {
            'default':'quartz_block','smooth':'smooth_quartz',
            'lines':'quartz_pillar','chiseled':'chiseled_quartz_block'}),
    }
    if base in variants:
        key, values = variants[base]
        if states.get(key) in values:
            name = 'minecraft:' + values[states.pop(key)]
    double=base.startswith('double_')
    original=base.removeprefix('double_')
    keys={'stone_block_slab':'stone_slab_type', 'stone_block_slab2':'stone_slab_type_2',
          'stone_block_slab3':'stone_slab_type_3', 'stone_block_slab4':'stone_slab_type_4'}
    key=keys.get(original)
    if key and states.get(key) in MATERIALS[key]:
        material=MATERIALS[key][states.pop(key)]
        name='minecraft:'+material+('_double_slab' if double else '_slab')
    return name,states


def verification_state(name,states=None):
    """Persistent material/state requirements; wall joins are computed by the game."""
    name,states=command_state(name,states,probe=True)
    if name.endswith('_wall'):
        for key in ('wall_connection_type_east','wall_connection_type_north',
                    'wall_connection_type_south','wall_connection_type_west','wall_post_bit'):
            states.pop(key,None)
    if name in ('minecraft:daylight_detector','minecraft:daylight_detector_inverted'):
        states.pop('redstone_signal',None)
    return name,states


def cell_states(structure,local):
    """Resolve paired BDX door halves, whose legacy upper data has no direction."""
    block=structure.palette.get(int(structure.region.blocks[local]))
    states=block.state.states;name=block.state.name
    if structure.source_info.get('source_format')!='BDX' or not name.endswith('_door'):
        return states
    x,y,z=local
    if y>0:
        below=structure.palette.get(int(structure.region.blocks[x,y-1,z]))
        if below.state.name==name and not below.state.states.get('upper_block_bit',False):
            states=dict(states,upper_block_bit=True)
            for key in ('direction','minecraft:cardinal_direction','open_bit'):
                if key in below.state.states:states[key]=below.state.states[key]
            return states
    if not states.get('upper_block_bit',False) and y+1<structure.size[1]:
        above=structure.palette.get(int(structure.region.blocks[x,y+1,z]))
        if above.state.name==name and above.state.states.get('upper_block_bit',False):
            states=dict(states,door_hinge_bit=above.state.states.get('door_hinge_bit',False))
    return states
