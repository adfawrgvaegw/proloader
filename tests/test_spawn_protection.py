from proloader.ai.program import validate
from tools.build_spawn_protection import make_program


def test_protection_has_explicit_centers_and_connected_independent_chains():
    p=validate(make_program());assert len(p['blocks'])==11
    heads=[b for b in p['blocks'] if b['type']=='repeat']
    assert [b['delay'] for b in heads]==[5,20,40]
    for row in (0,1,2):
        chain=[b for b in p['blocks'] if b['at'][2]==row]
        assert all(a['facing']=='east' and b['at'][0]==a['at'][0]+1 for a,b in zip(chain,chain[1:]))
    commands=[b['command'] for b in p['blocks']]
    assert any('598 143 201 as @e[family=monster,r=500]' in c for c in commands)
    assert any('12006 280 12014 as @e[family=monster,r=64]' in c for c in commands)
    assert all('at @s run tp @s ~ -100 ~' in c for c in commands if 'family=monster' in c)
    restore=next(c for c in commands if 'gamemode s' in c)
    assert 'tag=!pl_spawn_inside,tag=pl_spawn_managed,m=2' in restore
    assert all('5 255 true' in c and 'r=800' in c for c in commands if 'effect ' in c)
    assert all('gamerule' not in c for c in commands)
