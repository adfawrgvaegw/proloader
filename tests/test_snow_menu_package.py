import pytest

from tools.package_snow_menu import bundle


def test_relocation_retains_connections_and_replaces_only_home():
    old, _ = bundle('123', (1, 100, 2), (12000, 280, 12000))
    new, manifest = bundle('456', (-100, 70, 300), (18000, 200, 19000))
    assert len(new['blocks']) == 58
    assert [b['at'] for b in new['blocks']] == [b['at'] for b in old['blocks']]
    changed = [(a,b) for a,b in zip(old['blocks'],new['blocks']) if a != b]
    assert len(changed) == 1
    assert changed[0][1]['command'].endswith('-100 70 300')
    heads = [b for b in new['blocks'] if b['type'] == 'repeat']
    assert sorted(b['delay'] for b in heads) == [0, 20]
    vectors = dict(east=(1,0,0), west=(-1,0,0), south=(0,0,1))
    for chain in (new['blocks'][:56], new['blocks'][56:]):
        for a,b in zip(chain, chain[1:]):
            assert [x+y for x,y in zip(a['at'],vectors[a['facing']])] == b['at']
    assert manifest['prepare'][0].endswith('plsm_state 0')
    assert manifest['platform'].endswith(' keep')
    assert manifest['platform_bounds'][1] == [18011,199,19010]


def test_invalid_height_rejected_before_creating_bundle():
    with pytest.raises(ValueError):
        bundle('123', (0, 400, 0), (12000, 280, 12000))
    with pytest.raises(ValueError):
        bundle('123', (0, 80, 0), (12000, -64, 12000))
