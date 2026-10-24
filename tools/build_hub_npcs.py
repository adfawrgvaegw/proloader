"""Five scoped villager markers; original boot-break detection shows 建设中."""
import json
from pathlib import Path
from proloader.ai.program import layout, compile_program

GATE='execute if score plnpc_enabled plsys_state matches 1 run '
ROLES=[('shop','商店',395),('dungeon','副本',392),('artifact','神器',389),
       ('trade','玩家交易所',398),('games','小游戏',401)]
TYPE='villager'


def make_program():
    spawn=[];protect=[]
    for key,name,x in ROLES:
        tag='plnpc_'+key
        spawn += [
            f'execute in overworld positioned {x}.5 125 395.5 unless entity @e[tag={tag}] unless entity @e[type={TYPE},name="{name}",r=2] run summon {TYPE} "{name}" {x}.5 125 395.5',
            f'execute in overworld positioned {x}.5 125 395.5 run tag @e[type={TYPE},name="{name}",r=2,c=1] add {tag}',
            f'tag @e[tag={tag}] add pl_hub_npc',
        ]
        protect += [
            f'execute in overworld as @e[tag={tag}] unless entity @s[x={x}.5,y=125,z=395.5,r=0.05] run tp @s {x}.5 125 395.5 0 0',
            f'execute in overworld as @e[tag={tag}] at @s run tp @s ~ ~ ~ 0 0',
        ]
    protect += [f'effect @e[type={TYPE},tag=pl_hub_npc] {effect} 1 255 true'
                for effect in ('health_boost','instant_health','resistance','slowness')]
    detectors=[]
    for key,name,x in ROLES:
        tag='plnpc_'+key
        detectors.append([
            'tag @a[tag=pl_npc_trigger] remove pl_npc_trigger',
            f'execute as @e[tag={tag}] unless score @s plnpc_ready matches 1 run replaceitem entity @s slot.armor.feet 0 destroy iron_boots 1 1145',
            f'scoreboard players set @e[tag={tag}] plnpc_ready 1',
            f'execute as @e[tag={tag}] at @s run replaceitem entity @s slot.armor.feet 0 keep iron_boots 1 1145',
            f'scoreboard players add plnpc_{key}_events plsys_state 1',
            f'execute at @e[tag={tag},c=1] run tag @p[tag=!pl_sm_bot,r=4] add pl_npc_trigger',
            'title @a[tag=pl_npc_trigger] times 0 20 0',
            'title @a[tag=pl_npc_trigger] title §e建设中',
            'tag @a[tag=pl_npc_trigger] remove pl_npc_trigger',
        ])
    p=None
    chains=[(0,20,spawn,'五入口村民生成'),(3,0,protect,'五入口村民保护')]
    chains += [(6+i,0,commands,ROLES[i][1]+'击打提示') for i,commands in enumerate(detectors)]
    for z,delay,commands,label in chains:
        q=layout('\n'.join(GATE+c for c in commands),[12000,250,12008],width=12,head='repeat',delay=delay)
        for i,b in enumerate(q['blocks']):
            b['at'][2]+=z;b['name']=label;b['track_output']=True
            if z>=6 and 4<=i<=7:b['conditional']=True
        if p is None:p=q
        else:p['blocks']+=q['blocks']
    return p


if __name__=='__main__':
    folder=Path('cache/hub_npc_20261024');folder.mkdir(exist_ok=False)
    p=make_program();(folder/'program.json').write_text(json.dumps(p,ensure_ascii=False,indent=2)+'\n')
    compile_program(p,folder/'hub-npcs.mcstructure');print({'blocks':len(p['blocks'])})
