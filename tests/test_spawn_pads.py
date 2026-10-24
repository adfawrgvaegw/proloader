from tools.build_spawn_pads import make_program,TYPES
from proloader.ai.program import validate


def test_player_specific_teleports_have_cooldown_before_transfer():
    p=validate(make_program());assert len(p['blocks'])==20
    cs=[b['command'] for b in p['blocks'][:14]]
    assert all('@p' not in c for c in cs)
    assert '603,y=144,z=200' in cs[4] and 'pl_pad_cd=0' in cs[4]
    assert '593,y=144,z=200' in cs[5]
    assert 'pl_pad_cd 100' in cs[6]
    assert 'resistance 30 255 true' in cs[8]
    assert cs[10].endswith('spreadplayers 598 201 200 10000 @s')
    assert cs[11].endswith('tp @a[tag=pl_pad_fixed] 395 124 410')
    assert len(TYPES)==6 and all(t not in TYPES for t in ('player','villager','wolf','cat'))
    assert all('name=!宠物' in b['command'] for b in p['blocks'][14:])


def test_two_independent_chains_keep_connected_arrows():
    p=make_program();vectors={'east':(1,0,0),'west':(-1,0,0),'south':(0,0,1)}
    for chain in (p['blocks'][:14],p['blocks'][14:]):
        assert chain[0]['type']=='repeat'
        for a,b in zip(chain,chain[1:]):
            assert [v+d for v,d in zip(a['at'],vectors[a['facing']])]==b['at']
    assert p['blocks'][14]['delay']==20
