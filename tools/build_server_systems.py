"""User-provided notice, UID and currency systems, with 70-point rewards."""
import json
from pathlib import Path

from proloader.ai.program import layout, compile_program

PLAYERS='@a[tag=!pl_sm_bot]'
GATE='execute if score plsys_enabled plsys_state matches 1 run '


def make_program():
    # Preserve backup balances for returning players before hiding offline names.
    seconds=[
        f'scoreboard players add {PLAYERS} 金币1 0',
        f'execute as {PLAYERS} run scoreboard players operation @s 金币1 = @s 金币',
        'scoreboard players reset * 金币',
        f'execute as {PLAYERS} run scoreboard players operation @s 金币 = @s 金币1',
        'scoreboard players set " §d●§7 在线人数" 信息栏 0',
        f'execute as {PLAYERS} run scoreboard players add " §d●§7 在线人数" 信息栏 1',
        'scoreboard players set " §c●§7 实体数量" 信息栏 0',
        'execute as @e[type=!player,type=!item] run scoreboard players add " §c●§7 实体数量" 信息栏 1',
        'scoreboard players set " §a●§7 掉落物数" 信息栏 0',
        'execute as @e[type=item] run scoreboard players add " §a●§7 掉落物数" 信息栏 1',
        'scoreboard players set " §b●§7 创造人数" 信息栏 0',
        'execute as @a[tag=!pl_sm_bot,m=1] run scoreboard players add " §b●§7 创造人数" 信息栏 1',
        'scoreboard players remove " §e●§7 定时清理" 信息栏 1',
        'execute if score " §e●§7 定时清理" 信息栏 matches 0 run tellraw @a {"rawtext":[{"text":"§a●§f〉 §7突然刮起了一阵强劲的龙卷风，把整个世界的掉落物都吹走了！"}]}',
        f'execute if score " §e●§7 定时清理" 信息栏 matches 0 run playsound item.trident.riptide_3 {PLAYERS}',
        'execute if score " §e●§7 定时清理" 信息栏 matches 0 run kill @e[type=item]',
        'execute if score " §e●§7 定时清理" 信息栏 matches ..0 run scoreboard players set " §e●§7 定时清理" 信息栏 999',
        'execute if score " §c●§7 实体数量" 信息栏 matches 500.. run tellraw @a {"rawtext":[{"text":"§c●§f〉 实体数量达到500，清理未命名为宠物且不在保护名单中的实体（仅当前加载范围）。"}]}',
        'execute if score " §c●§7 实体数量" 信息栏 matches 500.. as @e[type=!player,type=!npc,type=!armor_stand,type=!painting,type=!netease:pet,type=!villager,type=!zombie_villager,type=!iron_golem,type=!snow_golem,type=!wolf,type=!cat,type=!zombie_horse,type=!skeleton_horse,type=!leash_knot,type=!ender_dragon,type=!ender_crystal,type=!minecart,type=!hopper_minecart,type=!chest_minecart,type=!thrown_trident,name=!宠物] at @s unless entity @s[x=2944,y=-64,z=2944,dx=255,dy=383,dz=255] run tp @s ~ -100 ~',
        'scoreboard players remove " §6●§7 在线奖励" 信息栏 1',
        f'execute if score " §6●§7 在线奖励" 信息栏 matches 0 run scoreboard players add {PLAYERS} 金币 70',
        'execute if score " §6●§7 在线奖励" 信息栏 matches 0 run tellraw @a {"rawtext":[{"text":"§6●§f〉§7 已发放 §e70积分"}]}',
        f'execute if score " §6●§7 在线奖励" 信息栏 matches 0 run playsound mob.chicken.plop {PLAYERS}',
        'execute if score " §6●§7 在线奖励" 信息栏 matches ..0 run scoreboard players set " §6●§7 在线奖励" 信息栏 999',
        f'execute as {PLAYERS} run scoreboard players operation @s 金币1 = @s 金币',
    ]
    uid=[
        f'execute as {PLAYERS} unless score @s uid matches 1.. run scoreboard players add @s uid 0',
        'tag @a[tag=pluid_pending] remove pluid_pending',
        'tag @a[tag=!pl_sm_bot,scores={uid=..0},c=1] add pluid_pending',
        'execute if entity @a[tag=pluid_pending] run scoreboard players add 注册人数 uid 1',
        'scoreboard players operation @a[tag=pluid_pending] uid = 注册人数 uid',
        f'execute if entity @a[tag=pluid_pending] run playsound random.levelup {PLAYERS}',
        'execute as @a[tag=pluid_pending] run tellraw @a {"rawtext":[{"text":"[§d§l服务器§r§f] 欢迎玩家§a➩§e"},{"selector":"@s"},{"text":"§f成为第 §l§c"},{"score":{"name":"注册人数","objective":"uid"}},{"text":" §r§f名加入服务器的玩家，祝您§a玩的开心"}]}',
        'tag @a[tag=pluid_pending] remove pluid_pending',
    ]
    p=layout('\n'.join(GATE+c for c in seconds),[12000,280,12016],width=12,head='repeat',delay=20)
    q=layout('\n'.join(GATE+c for c in uid),[12000,280,12024],width=12,head='repeat')
    for b in q['blocks']:b['at'][2]+=8
    p['blocks']+=q['blocks']
    for b in p['blocks']:b['name']='公告UID金币';b['track_output']=True
    return p


def setup():
    return [
        'scoreboard objectives add plsys_state dummy',
        'scoreboard objectives add 信息栏 dummy "§l§4ᴍ§cɪ§6ɴ§eᴇ§aᴄ§bʀ§3ᴀ§dғ§uᴛ"',
        'scoreboard objectives add uid dummy',
        'scoreboard objectives add 金币 dummy "§e金币§g¥"',
        'scoreboard objectives add 金币1 dummy',
        'scoreboard players set plsys_enabled plsys_state 0',
        'scoreboard players add 注册人数 uid 0',
        'scoreboard objectives setdisplay sidebar 信息栏 ascending',
        'scoreboard objectives setdisplay list 金币 descending',
        'scoreboard players set "§7----------------§r" 信息栏 -6',
        'scoreboard players set "§o§bঌ§l§f ɴᴏᴛɪᴄᴇ ɪɴғᴏ" 信息栏 -5',
        'scoreboard players set " §7[§b服主§7] OWNER_PLACEHOLDER" 信息栏 -4',
        'scoreboard players set " §7[§a服号§7] 12345678" 信息栏 -3',
        'scoreboard players set " §7[§d交流§7] CONTACT_PLACEHOLDER" 信息栏 -2',
        'scoreboard players set "§o§bঌ§l§f ᴏɴʟɪɴᴇ ɪɴғᴏ" 信息栏 -1',
        'scoreboard players set "§7----------------" 信息栏 999',
        'scoreboard players set " §e●§7 定时清理" 信息栏 999',
        'scoreboard players set " §6●§7 在线奖励" 信息栏 999',
        'tickingarea add 12000 280 12016 12011 280 12024 pl_server_systems true',
    ]


if __name__=='__main__':
    folder=Path('cache/server_systems_20261024');folder.mkdir(exist_ok=False)
    p=make_program()
    for name,obj in [('program.json',p),('setup.json',setup())]:
        (folder/name).write_text(json.dumps(obj,ensure_ascii=False,indent=2)+'\n')
    compile_program(p,folder/'server-systems.mcstructure')
    print(json.dumps({'blocks':len(p['blocks']),'folder':str(folder)},ensure_ascii=False))
