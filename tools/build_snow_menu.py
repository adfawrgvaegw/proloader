"""Convert the attributed snowball-menu design into an importable Bedrock program.

The source document is data, never shell/Python instructions. This is a corrected
adaptation: dedicated objectives, serialized mutual teleport, fixed syntax,
three timer groups, and an in-world master switch. No network access here.
"""
from __future__ import annotations
import argparse, hashlib, json
from pathlib import Path
import numpy as np

from proloader.model.block import Block, BlockState
from proloader.model.palette import BlockPalette
from proloader.model.region import Region
from proloader.model.structure import Structure
from proloader.parser.mcstructure import write_mcstructure
from proloader.parser.nbt import NBTByte

BOT_TAG='pl_sm_bot'
P='@a[tag=!pl_sm_bot'

def players(extra=''):
    return P+(','+extra if extra else '')+']'


def make_program(source: str, home=(0,80,0), origin=(12000,280,12000)):
    raw=next(line for line in source.splitlines() if line.startswith('execute ') and 'run titleraw @s actionbar ' in line)
    display=json.loads(raw.split('run titleraw @s actionbar ',1)[1])
    def map_display(v):
        if isinstance(v,dict):
            return {k:({'在线时间':'plsm_online','金币':'plsm_coin'}.get(x,x) if k=='objective' else map_display(x)) for k,x in v.items()}
        if isinstance(v,list):return [map_display(x) for x in v]
        if isinstance(v,str) and v.startswith('@s['):return v.replace('菜单=','plsm_menu=')
        return v
    display=map_display(display)
    ui=json.dumps(display,ensure_ascii=False,separators=(',',':'))
    user=players();sel=lambda score,look='rx=-89':players(f'scores={{plsm_menu={score}}},{look}')
    pending=players('scores={plsm_tpa=1}')
    main=[]
    add=main.append
    for objective in ('plsm_menu','plsm_tpa','plsm_coin','plsm_online'):
        add(f'scoreboard players add {user} {objective} 0')
    add(f'execute as {players("hasitem={item=snowball,data=1,quantity=0}")} run give @s snowball 16 1 '+json.dumps({'minecraft:keep_on_death':{},'minecraft:item_lock':{'mode':'lock_in_inventory'}},separators=(',',':')))
    add(f'execute as {players("scores={plsm_menu=1..6}")} at @s run titleraw @s actionbar {ui}')
    near='@p[tag=!pl_sm_bot,r=3,hasitem={item=snowball,data=1,quantity=1..}]'
    add(f'execute as @e[type=snowball] at @s if entity {near} run tag @s add plsm_throw')
    add(f'execute as @e[type=snowball,tag=plsm_throw] at @s as {near} unless score @s plsm_tpa matches 1 run scoreboard players add @s plsm_menu 1')
    add(f'execute at @e[type=snowball,tag=plsm_throw] run playsound block.end_portal_frame.fill {near}')
    add('kill @e[type=snowball,tag=plsm_throw]')
    add(f'scoreboard players set {players("scores={plsm_menu=7..}")} plsm_menu 1')
    add(f'title {sel("1..6","rxm=89")} actionbar §7菜单已关闭')
    add(f'scoreboard players set {sel("1..6","rxm=89")} plsm_menu 0')
    add(f'scoreboard players set {players("scores={plsm_tpa=1},rxm=89")} plsm_tpa 0')
    add(f'execute unless entity {pending} run scoreboard players set plsm_timer plsm_state 0')
    add(f'playsound random.levelup {sel("1..6")}')
    add(f'tp {sel("1")} {home[0]} {home[1]} {home[2]}')
    add(f'gamemode survival {sel("2")}')
    add(f'kill {sel("3")}')
    add(f'execute as {sel("4")} at @s run spawnpoint @s ~ ~ ~')
    add(f'title {sel("1..4")} actionbar §a执行成功')
    add(f'scoreboard players set {sel("1..4")} plsm_menu 0')
    add(f'execute if entity {pending} run title {sel("5")} actionbar §c互传通道占用中，请稍候')
    add(f'execute if entity {pending} run scoreboard players set {sel("5")} plsm_menu 0')
    add(f'execute unless entity {pending} run tag {players("scores={plsm_menu=5},rx=-89,c=1")} add plsm_newreq')
    add('scoreboard players set @a[tag=plsm_newreq] plsm_tpa 1')
    broadcast={'rawtext':[{'text':'[§d雪球菜单§r] '},{'selector':'@a[tag=plsm_newreq]'}, {'text':' 发起了互传，60秒内选择“接受传送”可将其传送到你身边。'}]}
    add(f'execute if entity @a[tag=plsm_newreq] run tellraw {user} '+json.dumps(broadcast,ensure_ascii=False,separators=(',',':')))
    add('execute if entity @a[tag=plsm_newreq] run scoreboard players set plsm_timer plsm_state 60')
    add('scoreboard players set @a[tag=plsm_newreq] plsm_menu 0')
    add('tag @a[tag=plsm_newreq] remove plsm_newreq')
    tpa={'rawtext':[{'text':'§d互传等待中 §f剩余 '},{'score':{'name':'plsm_timer','objective':'plsm_state'}},{'text':' 秒；低头取消'}]}
    add(f'titleraw {pending} actionbar '+json.dumps(tpa,ensure_ascii=False,separators=(',',':')))
    add(f'execute if entity {pending} if score plsm_timer plsm_state matches ..0 run tellraw {user} '+json.dumps({'rawtext':[{'text':'§7互传已超时，通道释放。'}]},ensure_ascii=False,separators=(',',':')))
    add(f'execute if score plsm_timer plsm_state matches ..0 run scoreboard players set {pending} plsm_tpa 0')
    add(f'execute unless entity {pending} run title {sel("6")} actionbar §c无人开启互传通道')
    add(f'execute if entity {pending} run tag {players("scores={plsm_menu=6},rx=-89,c=1")} add plsm_accept')
    add(f'execute at @a[tag=plsm_accept,c=1] run tp {pending} ~ ~ ~')
    add('execute if entity @a[tag=plsm_accept] run title @a[tag=plsm_accept] actionbar §a传送成功')
    add(f'execute if entity @a[tag=plsm_accept] run scoreboard players set {pending} plsm_tpa 0')
    add('execute if entity @a[tag=plsm_accept] run scoreboard players set plsm_timer plsm_state 0')
    add(f'scoreboard players set {sel("6")} plsm_menu 0')
    add('tag @a[tag=plsm_accept] remove plsm_accept')
    clock=[f'execute if entity {pending} if score plsm_timer plsm_state matches 1.. run scoreboard players remove plsm_timer plsm_state 1']
    minute=[f'scoreboard players add {user} plsm_online 1']
    groups=[dict(name='main',delay=0,z=0,commands=main),dict(name='seconds',delay=20,z=7,commands=clock),dict(name='minutes',delay=1200,z=10,commands=minute)]
    entries=[]
    for group in groups:
        commands=group['commands']
        for i,cmd in enumerate(commands):
            row,col=divmod(i,12);x=col if row%2==0 else 11-col
            z=group['z']+row
            facing=3 if col==11 and i+1<len(commands) else (5 if row%2==0 else 4)
            position=[origin[0]+x,origin[1],origin[2]+z]
            entries.append(dict(group=group['name'],index=i,position=position,
                block='minecraft:repeating_command_block' if i==0 else 'minecraft:chain_command_block',
                facing=facing,conditional=False,command='execute if score plsm_enabled plsm_state matches 1 run '+cmd,
                auto=True,delay=group['delay'] if i==0 else 0,first_tick=False))
    setup=[f'scoreboard objectives add {name} dummy' for name in ('plsm_menu','plsm_tpa','plsm_state','plsm_online','plsm_coin')]
    setup+=['scoreboard players set plsm_enabled plsm_state 0','scoreboard players set plsm_timer plsm_state 0']
    return dict(format='proloader-command-program-v1',attribution='Adapted from 肆陵雪球菜单; author 肆陵, player-teleport inspiration credited to 晴渊蓝 in source.',
                source_sha256=hashlib.sha256(source.encode()).hexdigest(),origin=list(origin),home=list(home),
                dimension='overworld',size=[16,1,16],ticking_area='proloader_snow_menu',bot_tag=BOT_TAG,
                setup=setup,entries=entries,enable='scoreboard players set plsm_enabled plsm_state 1',
                disable='scoreboard players set plsm_enabled plsm_state 0')


def write_program(program,folder):
    folder=Path(folder);folder.mkdir(parents=True,exist_ok=True)
    (folder/'snow-menu.command.json').write_text(json.dumps(program,ensure_ascii=False,indent=2)+'\n')
    palette=BlockPalette();blocks=np.zeros(tuple(program['size']),dtype=np.uint32);entities={}
    for entry in program['entries']:
        local=tuple(a-b for a,b in zip(entry['position'],program['origin']))
        states={'conditional_bit':False,'facing_direction':entry['facing']}
        bid=palette.add(Block(BlockState(entry['block'],states)));blocks[local]=bid
        entities[local]=dict(id='CommandBlock',Command=entry['command'],CustomName='雪球菜单 '+entry['group']+' '+str(entry['index']+1),
            auto=NBTByte(1),TickDelay=entry['delay'],ExecuteOnFirstTick=NBTByte(0),TrackOutput=NBTByte(0),conditionalMode=NBTByte(0))
    s=Structure('肆陵雪球菜单_基岩适配',tuple(program['size']),palette,Region(palette,blocks,offset=tuple(program['origin'])),entities)
    (folder/'snow-menu.mcstructure').write_bytes(write_mcstructure(s))
    (folder/'setup.mcfunction').write_text('\n'.join(['# Source: 肆陵雪球菜单; adaptation by proloader',*program['setup']])+'\n')
    (folder/'stop.mcfunction').write_text(program['disable']+'\n')
    (folder/'start.mcfunction').write_text(program['enable']+'\n')
    for group in ('main','seconds','minutes'):
        lines=[e['command'] for e in program['entries'] if e['group']==group]
        (folder/f'{group}.mcfunction').write_text('\n'.join(lines)+'\n')
    return folder


def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('source',type=Path);p.add_argument('--output',type=Path,required=True)
    p.add_argument('--home',type=int,nargs=3,required=True);p.add_argument('--origin',type=int,nargs=3,default=[12000,280,12000])
    args=p.parse_args();program=make_program(args.source.read_text(),tuple(args.home),tuple(args.origin));write_program(program,args.output)
    print(json.dumps(dict(entries=len(program['entries']),groups=3,output=str(args.output)),ensure_ascii=False))
if __name__=='__main__':main()
