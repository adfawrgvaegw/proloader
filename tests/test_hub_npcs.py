from tools.build_hub_npcs import make_program,ROLES
from proloader.ai.program import validate


def test_only_tagged_villagers_protected_and_shop_has_destination():
    p=validate(make_program());cs=[b['command'] for b in p['blocks']]
    assert len(p['blocks'])==74
    assert {r[2] for r in ROLES}=={389,392,395,398,401}
    assert all('tag=pl_hub_npc' in c for c in cs if 'effect @e' in c)
    assert all('447 66 403' not in c for c in cs)
    assert sum('title §e建设中' in c for c in cs)==5
    targets=[c for c in cs if 'add pl_npc_trigger' in c]
    assert len(targets)==5 and all('@p[tag=!pl_sm_bot,r=4]' in c for c in targets)
    for start in range(29,74,9):
        detect=p['blocks'][start:start+9]
        assert [b['conditional'] for b in detect]==[False]*4+[True]*4+[False]
        assert 'destroy iron_boots 1 1145' in detect[1]['command']
        assert 'keep iron_boots 1 1145' in detect[3]['command']


def test_stability_and_instant_title_settings():
    p=validate(make_program());cs=[b['command'] for b in p['blocks']]
    assert sum('r=0.05] run tp @s' in c for c in cs)==5
    assert all('facing @p' not in c for c in cs)
    assert any('slowness 1 255 true' in c for c in cs)
    assert all(b['delay']==0 for b in p['blocks'] if b['type']=='repeat' and b['at'][2]>=3)
    for start in range(29,74,9):
        assert 'times 0 20 0' in p['blocks'][start+6]['command']
        assert 'title §e建设中' in p['blocks'][start+7]['command']
