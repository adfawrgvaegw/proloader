"""Local spawn/command-area protection; no global spawning or PvP rule changes."""
import json
from pathlib import Path
from proloader.ai.program import layout, compile_program

HOME=(598,143,201)
ORIGIN=(12000,280,12028)
GATE='execute if score plprot_enabled plsys_state matches 1 run '


def make_program():
    center='execute in overworld positioned 598 143 201 run '
    modes=[
        'tag @a[tag=pl_spawn_inside] remove pl_spawn_inside',
        center+'tag @a[tag=!pl_sm_bot,r=500] add pl_spawn_inside',
        'tag @a[tag=pl_spawn_inside,m=0] add pl_spawn_managed',
        'gamemode a @a[tag=pl_spawn_inside,m=0]',
        'gamemode s @a[tag=!pl_spawn_inside,tag=pl_spawn_managed,m=2]',
        'tag @a[tag=!pl_spawn_inside,tag=pl_spawn_managed] remove pl_spawn_managed',
    ]
    mobs=[
        'execute in overworld positioned 598 143 201 as @e[family=monster,r=500] at @s run tp @s ~ -100 ~',
        'execute in overworld positioned 12006 280 12014 as @e[family=monster,r=64] at @s run tp @s ~ -100 ~',
    ]
    effects=[
        center+'effect @e[tag=!pl_sm_bot,r=800] resistance 5 255 true',
        'scoreboard players test plprot_enabled plsys_state 1 1',
        'scoreboard players test plprot_enabled plsys_state 1 1',
    ]
    p=layout('\n'.join(GATE+c for c in modes),ORIGIN,width=12,head='repeat',delay=5)
    for z,delay,commands in [(1,20,mobs),(2,40,effects)]:
        q=layout('\n'.join(GATE+c for c in commands),ORIGIN,width=12,head='repeat',delay=delay)
        for b in q['blocks']:b['at'][2]+=z
        p['blocks']+=q['blocks']
    for b in p['blocks']:
        b['name']=('生命提升已停用' if b['at']==[1,0,2] else '虚弱已停用' if b['at']==[2,0,2] else '主城800格防打架' if 'run effect ' in b['command'] else '主城500格保护')
        b['track_output']=True
    return p


if __name__=='__main__':
    folder=Path('cache/spawn_protection_20261024');folder.mkdir(exist_ok=False)
    p=make_program();(folder/'program.json').write_text(json.dumps(p,ensure_ascii=False,indent=2)+'\n')
    compile_program(p,folder/'spawn-protection.mcstructure')
    print({'blocks':len(p['blocks']),'origin':p['origin']})
