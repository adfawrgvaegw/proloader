"""Build only the user's pasted third-revision UI supplement. No old menu code."""
import argparse
import hashlib
import json
import re
from pathlib import Path

from proloader.ai.program import layout, compile_program


def build(source, folder):
    commands=[]
    for line in source.splitlines():
        line=line.strip()
        if not line.startswith(('execute ', 'scoreboard players ')):
            continue
        line=line.replace('~~~','~ ~ ~')
        line=line.replace('scores={dh=3..8,dh=!5..6}', 'scores={dh=3..8}')
        if 'block.bell.hit' in line:
            line=line.replace(' at @s run ', ' at @s unless score @s dh matches 5..6 run ')
        line=line.replace('scores={dh=9..14,dh=!10..13}', 'scores={dh=9..14}')
        if 'tile.piston.in' in line:
            line=line.replace(' at @s run ', ' at @s unless score @s dh matches 10..13 run ')
        line=line.replace('scores={菜单=1..,菜单=!6}', 'scores={菜单=1..5}')
        line=line.replace('@a[互传=1]', '@a[scores={互传=1}]')
        if line=='scoreboard players add @a[scores={dh=1..79,dh=!50}] dh 1':
            line='execute as @a[scores={dh=1..79}] unless score @s dh matches 50 run scoreboard players add @s dh 1'
        line=line.replace('dh=!..34', 'dh=35..')
        # Negated score ranges are not valid Bedrock selectors.
        line=line.replace(' unless entity @s[scores={菜单=!1..}]', '')
        if ' actionbar ' in line:
            json.loads(line.split(' actionbar ',1)[1])
        commands.append(line)
    assert len(commands)==23, len(commands)
    front=json.loads(Path(__file__).with_name('snow_menu_front_20261024.json').read_text())
    assert len(front)==38
    front[10]=front[10].replace('0 0 0','-68 118 246')
    front[15]=front[15].replace('scores={菜单=1..,菜单=!5..6}', 'scores={菜单=1..4}')
    front[32]='execute if entity @a[scores={互传=1}] at @a[scores={菜单=6},rx=-89] as @a[c=2] unless score @s 菜单 matches 1..5 run title @s actionbar §l║║║§a传送成功§f║║║'
    # Capture the projectile and start animations before the front chain kills
    # it; capture close/confirmation before the front chain resets menu scores.
    main=[front[0],commands[21],commands[22],*front[2:6],*commands[4:7]]
    main += [c for i,c in enumerate(front[7:36],start=7) if i != 14]
    main += [*commands[:4],*commands[7:18],commands[20],commands[19],commands[18]]
    def exclude_bot(c):
        c=c.replace('@a[','@a[tag=!pl_sm_bot,').replace('@p[','@p[tag=!pl_sm_bot,')
        c=re.sub(r'@a(?![\w\[])','@a[tag=!pl_sm_bot]',c)
        return c
    main=[exclude_bot(c) for c in main]
    prefix='execute if score plsm_enabled plsm_state matches 1 run '
    p=layout('\n'.join(prefix+c for c in main),[12000,280,12000],width=12,head='repeat')
    for b in p['blocks']:
        b['name']='新版UI 附加指令'
        b['track_output']=True
    vectors={'east':(1,0,0),'west':(-1,0,0),'south':(0,0,1)}
    for a,b in zip(p['blocks'],p['blocks'][1:]):
        assert [x+y for x,y in zip(a['at'],vectors[a['facing']])]==b['at']
    timer=layout('\n'.join(prefix+exclude_bot(c) for c in front[36:38]),[12000,280,12010],width=12,head='repeat',delay=20)
    for b in timer['blocks']:
        b['at'][2]+=10
        b['name']='新版UI 互传计时'
        p['blocks'].append(b)
    folder=Path(folder);folder.mkdir(parents=True,exist_ok=True)
    (folder/'snow-menu.ai.json').write_text(json.dumps(p,ensure_ascii=False,indent=2)+'\n')
    (folder/'main.mcfunction').write_text('\n'.join(prefix+c for c in main)+'\n')
    (folder/'setup.mcfunction').write_text('scoreboard objectives add 菜单 dummy\nscoreboard objectives add 互传 dummy\nscoreboard objectives add dh dummy\nscoreboard objectives add dh1 dummy\n')
    (folder/'seconds.mcfunction').write_text('\n'.join(prefix+exclude_bot(c) for c in front[36:38])+'\n')
    (folder/'start.mcfunction').write_text('scoreboard players set plsm_enabled plsm_state 1\n')
    (folder/'stop.mcfunction').write_text('scoreboard players set plsm_enabled plsm_state 0\n')
    for name in ('minutes.mcfunction',):
        (folder/name).unlink(missing_ok=True)
    report=compile_program(p,folder/'snow-menu.mcstructure')
    (folder/'README.md').write_text(f'''# 用户两次新粘贴指令：38条前置 + 三改UI

服务器12345678；指令区12000 280 12000；常加载proloader_snow_menu。
粘贴原文SHA256：{hashlib.sha256(source.encode()).hexdigest()}。
全部逻辑来自本次用户两份粘贴；没有使用旧版生成器或项目三份UI文件。

主链{len(main)}块，第1块循环，其余无条件连锁；保持开启、延迟0、首次执行关闭。
每行12块蛇形，行末箭头向南，其余东西交替连接下一块。
第二排在(12000,280,12010)，循环头延迟20刻，后接1块连锁，负责互传秒计时。
总开关plsm_enabled/plsm_state。假人通过pl_sm_bot标签排除。
原文非法的重复scores/否定范围/互传选择器/~~~已作语法修正。

前置第2条基础UI由新动画UI替换；按附加说明关闭前置第7、15条提示。
动画捕捉在清雪球和重置菜单分数前；UI渲染放链尾；收起动画移除错误菜单分数门槛。
前置第11条回城改为-68 118 246。没有在线时间、金币或之前菜单的任何其他功能。
前置第1条会持续将所有真人快捷栏第9格换成锁定雪球，遵照用户原指令。
扔雪球开启/切换，抬头确认，低头关闭；互传60秒。实服结果另见部署记录。
''')
    print(json.dumps(report,ensure_ascii=False))


if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source',type=Path);parser.add_argument('output',type=Path)
    args=parser.parse_args();build(args.source.read_text(),args.output)
