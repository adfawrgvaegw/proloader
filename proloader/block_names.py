"""Legacy NetEase palette names → current command names, preserving slab geometry.

The server still advertises old aliases, but testforblock rejects them even when
its world stores the equivalent modern block. Smooth quartz single/double were
verified on 2026-10-20 inside the authorized airport target.
"""
WIRE_REVISION='legacy-names-v2'
MATERIALS={
 'stone_slab_type':{'smooth_stone':'smooth_stone','sandstone':'sandstone','cobblestone':'cobblestone',
                   'brick':'brick','stone_brick':'stone_brick','quartz':'quartz','nether_brick':'nether_brick'},
 'stone_slab_type_2':{'red_sandstone':'red_sandstone','purpur':'purpur','prismarine_rough':'prismarine',
                    'prismarine_dark':'dark_prismarine','prismarine_brick':'prismarine_brick',
                    'mossy_cobblestone':'mossy_cobblestone','smooth_sandstone':'smooth_sandstone',
                    'red_nether_brick':'red_nether_brick'},
 'stone_slab_type_3':{s:s for s in ('end_stone_brick','smooth_red_sandstone','polished_andesite','andesite',
                                  'diorite','polished_diorite','granite','polished_granite')},
 'stone_slab_type_4':{s:s for s in ('mossy_stone_brick','smooth_quartz','stone','cut_sandstone','cut_red_sandstone')},
}


def command_state(name,states=None):
    states=dict(states or {})
    base=name.removeprefix('minecraft:')
    # Split legacy compound IDs into exact modern variants for both writes
    # and name probes. Never fall back to another material for unknown values.
    variants = {
        'stonebrick': ('stone_brick_type', {
            'default': 'stone_bricks', 'mossy': 'mossy_stone_bricks',
            'cracked': 'cracked_stone_bricks', 'chiseled': 'chiseled_stone_bricks'}),
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
