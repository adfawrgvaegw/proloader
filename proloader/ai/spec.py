"""Evidence-scoped NetEase mobile command guidance and conservative linting.

This is NOT a complete parser or an equivalence claim with latest Bedrock.
Keep server observations separate from upstream documentation.
"""
import re

PROFILE = {
    'id': 'netease-mobile-rental-observed-860-v1',
    'scope': '网易手机版租赁服；当前proloader使用860协议；不是网易Java版/国际版最新版兼容声明',
    'updated': '2026-10-22',
    'policy': '逐条语法、参数、执行通道分别验证。静态通过不代表服务器支持或玩法正确。',
    'version_note': '网易启动器版本、补丁号、基岩引擎版本、网络协议号不是同一个版本；860不能推导完整指令规范。',
    'references': [
        {'source':'docs/SNOW_MENU_LIVE_20261021.md','kind':'project_live_observation',
         'scope':'雪球菜单实服执行与NBT；不意味着每个参数组合均支持'},
        {'source':'docs/AI_CLI_20261022.md','kind':'project_live_observation',
         'scope':'脉冲/条件连锁计数、热修改；具体执行见报告'},
        {'source':'https://learn.microsoft.com/en-us/minecraft/creator/reference/content/commandsreference/examples/commands/execute?view=minecraft-bedrock-stable',
         'kind':'upstream_reference_only','scope':'国际版基岩语法参考，不能作为网易支持证据'},
        {'source':'https://learn.microsoft.com/en-us/minecraft/creator/documents/commandsnewexecute?view=minecraft-bedrock-stable',
         'kind':'upstream_reference_only','scope':'执行上下文与rawtext参考'},
    ],
    'topics': {
        'execute': {
            'status':'observed_subset',
            'forms':['execute as <selector> at @s run <command>',
                     'execute if|unless entity <selector> run <command>',
                     'execute if|unless score <holder> <objective> matches <range> run <command>',
                     'execute in overworld positioned <x> <y> <z> run <command>'],
            'notes':['as修改执行者，at修改坐标/维度，二者不能混用。',
                     '新版execute上述子集已在当前网易服可用；旧教程的execute <目标> ~ ~ ~ ...不能直接照抄。',
                     'Java execute store和NBT谓词不属于本工具支持的语法。未列出的新版子命令待测。'],
        },
        'selectors': {
            'status':'observed_subset',
            'examples':['@a[tag=!pl_sm_bot]','@a[scores={plsm_menu=1..6}]',
                        '@p[r=3,c=1]','@a[rxm=89]','@a[hasitem={item=snowball,data=1,quantity=1..}]'],
            'notes':['基岩常用r/rm、c、rx/rxm；不要套用Java的distance、limit、sort、nbt。',
                     '命令方块不是实体，直接@s通常找不到玩家。先execute as <目标>，需要玩家位置时再at @s。',
                     '假人也是玩家；全服选择器可能选到假人，玩法应显式排除其标签。'],
        },
        'blocks': {
            'status':'observed_subset',
            'examples':['setblock 12000 284 12014 minecraft:command_block ["facing_direction"=5,"conditional_bit"=false]',
                        'testforblock 12000 284 12014 minecraft:command_block ["facing_direction"=5,"conditional_bit"=false]'],
            'notes':['朝向：down=0/up=1/north=2/south=3/west=4/east=5。',
                     '方块名/states按当前服务器核对，旧名称走proloader/block_names.py；未知名称不猜。',
                     'Java setblock ... {Command:...}不是本链路的NBT写入方式；内部数据走CommandBlockUpdate并读回。',
                     '普通放块的负回执可能误报，需精确方块探针；NBT写入必须检查内部数据。'],
        },
        'scoreboard': {
            'status':'observed_subset',
            'examples':['scoreboard objectives add plai_test dummy',
                        'scoreboard players set pulse plai_test 0',
                        'scoreboard players add pulse plai_test 1',
                        'scoreboard players test pulse plai_test 1 1'],
            'notes':['目标分数应先初始化；使用专属objective和假名，避免改动现有玩法。'],
        },
        'text': {
            'status':'observed_subset',
            'examples':['tellraw @a {"rawtext":[{"text":"你好"}]}',
                        'titleraw @a actionbar {"rawtext":[{"text":"调试"}]}'],
            'notes':['使用基岩rawtext，不能直接拷贝Java文本组件。保留JSON转义。'],
        },
        'command_blocks': {
            'status':'observed_subset',
            'fields':{'type':['impulse','chain','repeat'],'facing':['down','up','north','south','west','east'],
                      'conditional':'条件模式：依赖链中前一块执行成功，不是任意if条件',
                      'auto':'true=保持开启；false=需要红石',
                      'delay':'非负整数，游戏刻；20刻约1秒，卡服时墙钟时间更长',
                      'first_tick':'首次执行配置；stage/write强制false，arm才应用源配置'},
            'notes':['箭头应指向下一连锁块；连锁顺序由世界布局决定，不由JSON数组顺序决定。',
                     '循环块、条件连锁、延迟须分别验收；不能以NBT写入成功当作玩法成功。',
                     'disarm仅切换需要红石；外部红石仍可触发。需要可靠总停时应给玩法加专属计分板开关。',
                     '常加载需要确认服务器tickingarea及命令方块开关；不自动占用新的常加载名额。'],
        },
        'transport': {
            'status':'project_observed',
            'notes':['假人命令通过网易aiCommand；不是Java RCON，也不是租赁服控制台。',
                     'aiCommand会绑定假人为执行者；命令方块的执行者和位置上下文不同，必须分别测试。',
                     '空字符串回执可代表成功；None可能负回执或超时，不足以证明写入未生效。',
                     '只读探针可以有界重试；加分、give等写命令不得自动重放。',
                     'OP、世界作弊/命令方块开关、服务器插件都会影响可执行性。'],
        },
    },
}


def tokens(text):
    """Split only at top-level whitespace, leaving JSON/selector/string payloads intact."""
    result, start, quote, escaped, depth = [], None, None, False, 0
    for i,ch in enumerate(text):
        if start is None:
            if ch.isspace():continue
            start=i
        if escaped:escaped=False;continue
        if quote:
            if ch=='\\':escaped=True
            elif ch==quote:quote=None
        elif ch in ('"',"'"):quote=ch
        elif ch in '[{(':depth+=1
        elif ch in ']})':depth=max(0,depth-1)
        elif ch.isspace() and depth==0:
            result.append(text[start:i]);start=None
    if start is not None:result.append(text[start:])
    return result


def lint(text, context='bot'):
    issues=[]
    def issue(severity,code,message):issues.append(dict(severity=severity,code=code,message=message))
    parts=tokens(text.strip().lstrip('/'))
    bound = context=='bot'
    while parts:
        root=parts[0].removeprefix('minecraft:')
        # Inspect only selector arguments, never raw JSON/text payloads.
        for arg in parts[1:]:
            if arg.startswith('@') and re.search(r'\b(distance|limit|sort|nbt|predicate|advancements)\s*=',arg):
                issue('error','java-selector','检测到Java选择器参数；按网易当前基岩子集改写，不自动猜测。')
        if root!='execute':break
        if len(parts)>1 and parts[1].startswith('@'):
            issue('warning','legacy-execute','旧版execute形式；请依据当前网易环境迁移并实测。')
        if 'run' not in parts:
            if 'store' in parts:issue('error','java-execute-store','execute store属于Java语法，本环境未支持。')
            break
        cut=parts.index('run')
        if 'store' in parts[1:cut]:issue('error','java-execute-store','execute store属于Java语法，本环境未支持。')
        if 'as' in parts[1:cut]:bound=True
        parts=parts[cut+1:]
    if parts:
        root=parts[0].removeprefix('minecraft:')
        if root=='data' and len(parts)>1 and parts[1] in ('get','merge','modify','remove'):
            issue('error','java-data','Java /data不能作为网易命令方块NBT编辑接口；请用CLI write/inspect。')
        if root=='give' and len(parts)>2 and '{' in parts[2]:
            issue('error','java-item-nbt','物品ID后直接拼NBT是Java写法；网易组件支持需单独核对。')
        block_index = {'setblock':4,'fill':7}.get(root)
        if block_index is not None and len(parts)>block_index+1 and parts[block_index+1].startswith('{'):
            issue('error','java-block-nbt','不能用Java SNBT后缀写命令方块内部数据；使用CLI结构化字段。')
        if context=='block' and not bound and any(p.startswith('@s') for p in parts[1:]):
            issue('warning','block-self','命令方块没有玩家执行者；请明确execute as目标，必要时at @s。')
    return issues


def lint_program(program):
    issues=[]
    for i,e in enumerate(program['blocks']):
        issues.extend(dict(block=i,at=e['at'],**item) for item in lint(e['command'],'block'))
        if e['type']=='repeat' and e['auto'] and not ('if score ' in e['command'] or 'unless score ' in e['command']):
            issues.append(dict(block=i,at=e['at'],severity='warning',code='repeat-gate',message='自动循环块建议增加专属计分板总开关。'))
    return issues
