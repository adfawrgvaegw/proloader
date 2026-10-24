from proloader.ai.program import validate
from tools.build_server_systems import make_program, setup


def test_system_chains_connect_and_only_second_chain_runs_every_tick():
    p=validate(make_program())
    assert len(p['blocks'])==33
    assert [(b['at'],b['delay']) for b in p['blocks'] if b['type']=='repeat']==[([0,0,0],20),([0,0,8],0)]
    vectors=dict(east=(1,0,0),west=(-1,0,0),south=(0,0,1))
    for chain in (p['blocks'][:25],p['blocks'][25:]):
        for a,b in zip(chain,chain[1:]):
            assert [x+y for x,y in zip(a['at'],vectors[a['facing']])]==b['at']


def test_currency_backup_wraps_offline_hiding_and_reward():
    commands=[b['command'] for b in make_program()['blocks'][:25]]
    backup=[i for i,c in enumerate(commands) if '金币1 = @s 金币' in c]
    reset=next(i for i,c in enumerate(commands) if 'reset * 金币' in c)
    restore=next(i for i,c in enumerate(commands) if '金币 = @s 金币1' in c)
    award=next(i for i,c in enumerate(commands) if '金币 70' in c)
    assert backup[0]<reset<restore<award<backup[-1]
    assert 'tag=!pl_sm_bot' in commands[award]
    assert all('cookie' not in c for c in commands)
    assert any('在线奖励" 信息栏 999' in c for c in setup())


def test_uid_assigns_only_one_pending_player_per_tick():
    commands=[b['command'] for b in make_program()['blocks'][25:]]
    assert 'c=1' in commands[2]
    assert '注册人数 uid 1' in commands[3]
    assert 'uid = 注册人数 uid' in commands[4]
    assert commands[1].endswith('remove pluid_pending')
    assert commands[-1].endswith('remove pluid_pending')
