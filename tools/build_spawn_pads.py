"""Two player-specific transfer pads and explicit local ambient mob cleanup."""
import json
from pathlib import Path
from proloader.ai.program import layout, compile_program

GATE='execute if score plpads_enabled plsys_state matches 1 run '
TYPES=('wandering_trader','pig','sheep','cow','chicken','rabbit')


def make_program():
    commands=[
        'scoreboard players remove @a[scores={pl_pad_cd=1..}] pl_pad_cd 1',
        'scoreboard players add @a pl_pad_cd 0',
        'tag @a[tag=pl_pad_random] remove pl_pad_random',
        'tag @a[tag=pl_pad_fixed] remove pl_pad_fixed',
        'execute in overworld run tag @a[tag=!pl_sm_bot,x=603,y=144,z=200,dx=0,dy=1,dz=0,scores={pl_pad_cd=0}] add pl_pad_random',
        'execute in overworld run tag @a[tag=!pl_sm_bot,x=593,y=144,z=200,dx=0,dy=1,dz=0,scores={pl_pad_cd=0}] add pl_pad_fixed',
        'scoreboard players set @a[tag=pl_pad_random] pl_pad_cd 100',
        'scoreboard players set @a[tag=pl_pad_fixed] pl_pad_cd 100',
        'effect @a[tag=pl_pad_random] resistance 30 255 true',
        'title @a[tag=pl_pad_random] title §b随机传送中...',
        'execute in overworld as @a[tag=pl_pad_random] run spreadplayers 598 201 200 10000 @s',
        'execute in overworld run tp @a[tag=pl_pad_fixed] 395 124 410',
        'tag @a[tag=pl_pad_random] remove pl_pad_random',
        'tag @a[tag=pl_pad_fixed] remove pl_pad_fixed',
    ]
    p=layout('\n'.join(GATE+c for c in commands),[12000,250,12000],width=12,head='repeat')
    cleanup=[f'execute in overworld positioned 598 143 201 as @e[type={t},r=500,name=!宠物] at @s run tp @s ~ -100 ~' for t in TYPES]
    q=layout('\n'.join(GATE+c for c in cleanup),p['origin'],width=12,head='repeat',delay=20)
    for b in q['blocks']:b['at'][2]+=4;b['name']='主城普通生物清理'
    for b in p['blocks']:b['name']='主城踩点传送'
    p['blocks']+=q['blocks']
    for b in p['blocks']:b['track_output']=True
    return p


if __name__=='__main__':
    folder=Path('cache/spawn_pads_20261024');folder.mkdir(exist_ok=False)
    p=make_program();(folder/'program.json').write_text(json.dumps(p,ensure_ascii=False,indent=2)+'\n')
    compile_program(p,folder/'spawn-pads.mcstructure')
    print({'blocks':len(p['blocks'])})
