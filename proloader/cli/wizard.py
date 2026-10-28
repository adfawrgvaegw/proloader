"""The two-entry CLI: manage identities, or guide a real rental import."""
from __future__ import annotations
import asyncio
import json
import logging
from pathlib import Path
import re
import shlex
import time
import warnings

warnings.filterwarnings('ignore',message='urllib3 v2 only supports OpenSSL.*')

from rich.console import Group
from rich.live import Live
from rich.panel import Panel
from rich.text import Text

from proloader.bots import BotStore, private_json
from proloader.cli import ui
from proloader.importer import prepare_import, execute_import, PlacementCheckError
from proloader.pacing import DEFAULT_BURST
from proloader.net.netease.runtime import verify_bot, RentalError
from proloader.net.netease.mobile_login import create_classic_mobile_provider, MobileLoginError

PROJECT=Path(__file__).resolve().parents[2]
PREFERENCES=Path.home()/'.proloader'/'preferences.json'
JOBS=Path.home()/'.proloader'/'imports'


def parse_coordinates(text):
    values=re.split(r'[\s,，]+',text.strip())
    if len(values)!=3:
        raise ValueError('坐标需要三个整数，例如 -85 78 351。')
    try:return tuple(map(int,values))
    except ValueError:raise ValueError('坐标需要三个整数，例如 -85 78 351。') from None


def input_path(text):
    text=text.strip()
    if Path(text).expanduser().exists():return Path(text).expanduser()
    parts=shlex.split(text)
    if len(parts)!=1:raise ValueError('请拖入一个文件，或输入完整路径。')
    return Path(parts[0]).expanduser()


def pending_jobs():
    result=[]
    seen=set()
    if not JOBS.exists():return result
    for path in sorted(JOBS.glob('*.json'),key=lambda p:p.stat().st_mtime,reverse=True):
        try:
            job=json.loads(path.read_text())
            key=(job['server'],job['bot_id'],job['plan']['sha256'],tuple(job['plan']['origin']),job['plan']['dimension'])
            if key in seen:continue
            seen.add(key)
            if job['state'] in ('paused','failed','placing','connecting','preflight','loading','verifying','repairing','manual_repair','awaiting_action','reconnecting','verification_failed','command_block_data','command_block_repair','block_entity_data','manual_repair_check','repair_verifying','cleanup'):
                result.append(job)
        except (OSError,ValueError,KeyError,TypeError):continue
    return result


def preferences():
    try:return json.loads(PREFERENCES.read_text())
    except (OSError,ValueError):return {}


def pause():
    ui.console.input('[muted]按 Enter 继续。[/muted]')


def show_error(exc):
    if isinstance(exc,MobileLoginError):
        message=f'网易登录未完成：阶段 {exc.stage}'
        if type(exc.code) is int:message+=f'，错误码 {exc.code}'
        message+='。不会自动重发短信。'
    elif isinstance(exc,(RentalError,ValueError,PlacementCheckError)) or exc.__class__.__name__ in ('ParseError','BedrockLoginError','CommandUnconfirmedError'):
        message=str(exc)[:400]
    else:
        message=f'这一步未能完成（{type(exc).__name__}）。已保存的机器人和导入断点仍可使用。'
    ui.notice('这一步没有完成',message,'bad')
    if isinstance(exc,MobileLoginError) and exc.verify_url:
        if ui.choose('网易要求完成官方验证',['在浏览器打开验证页','返回'])==0:
            import webbrowser
            webbrowser.open(exc.verify_url)
    pause()


def add_bot(store):
    method=ui.choose('添加一个机器人',['手机号登录','导入已有登录文件'],
                     descriptions=['收取一次网易官方短信，无需安装游戏客户端。','已有登录材料时使用，不会重新发短信。'])
    if method<0:return
    ui.screen(*ui.chrome('给你的机器人起个名字'),Text('这个名字只用于本机管理，不会更改游戏昵称。',style='muted'),Text(''))
    label=ui.ask('机器人名称','建筑助手')
    provider=None
    try:
        if method==0:
            ui.screen(*ui.chrome('手机号登录 · 1 / 3'),Text('输入已有网易账号的手机号。输入内容会隐藏。',style='muted'),Text(''))
            phone=ui.ask('手机号',secret=True)
            if not re.fullmatch(r'1[3-9][0-9]{9}',phone):raise ValueError('请输入有效的中国大陆手机号。')
            if ui.choose(f'验证码发送至 ****{phone[-4:]}',['发送一条登录短信','返回'])!=0:return
            with ui.console.status('[accent]准备登录环境并请求短信…'):
                provider=create_classic_mobile_provider()
                provider.send_sms(phone)
            phone=''
            ui.screen(*ui.chrome('手机号登录 · 2 / 3'),Text('网易已接受短信请求，请输入收到的验证码。',style='muted'),Text(''))
            code=ui.ask('短信验证码',secret=True)
            with ui.console.status('[accent]验证登录…'):
                provider.verify_sms(code)
            code=''
            cookie=provider.fetch_cookie().cookie
        else:
            ui.screen(*ui.chrome('导入登录文件'),Text('将 cookie JSON 文件拖到这里，或输入路径。',style='muted'),Text(''))
            path=input_path(ui.ask('文件路径'))
            cookie=path.read_text()
        bot=store.add(label,cookie)
        cookie=''
        ui.screen(*ui.chrome('验证机器人 · 3 / 3'))
        with ui.console.status('[accent]确认游戏身份…'):
            name=verify_bot(store,bot)
        ui.notice('机器人已添加',f'{bot.label}\n游戏昵称：{name}\n现在可以从「开始导入」选择它。','good')
        pause()
    finally:
        if provider:provider.close()


def manage_bots(store):
    while True:
        bots=store.list()
        labels=[b.label for b in bots]+['添加机器人']
        details=[('使用中 · ' if store.is_busy(b) else '')+(f'游戏昵称：{b.game_name}' if b.game_name else '已保存登录材料 · 待验证') for b in bots]+['手机号登录，或导入已有登录文件。']
        choice=ui.choose('管理机器人',labels,descriptions=details)
        if choice<0:return
        try:
            if choice==len(bots):
                add_bot(store)
                continue
            bot=bots[choice]
            action=ui.choose(bot.label,['重新验证身份','移除本机机器人','返回'],
                             descriptions=[bot.game_name or '尚未取得游戏昵称','只移除本机保存的登录材料，不删除网易账号。',''])
            if action==0:
                with ui.console.status('[accent]验证身份…'):name=verify_bot(store,bot)
                ui.notice('身份验证通过',f'游戏昵称：{name}','good')
                pause()
            elif action==1:
                if ui.choose('移除这个机器人？',['保留','移除'],descriptions=[bot.label,'本机登录材料将被删除。'])==1:
                    store.remove(bot)
        except ui.Back:continue
        except KeyboardInterrupt:continue
        except Exception as exc:show_error(exc)


def select_file():
    candidates=sorted((PROJECT/'buildings').glob('*')) if (PROJECT/'buildings').exists() else []
    candidates=[p for p in candidates if p.is_file() and p.suffix.lower() in ('.bdx','.schem','.schematic','.mcstructure','.mcworld')]
    choice=ui.choose('选择建筑文件',[p.name for p in candidates]+['选择其他文件'],
                     descriptions=[f'{p.stat().st_size/1024:.1f} KB' for p in candidates]+['拖入文件，或输入完整路径。'])
    if choice<0:raise ui.Back()
    if choice<len(candidates):return candidates[choice]
    ui.screen(*ui.chrome('开始导入',step=2),Text('将 .bdx / .schem / .schematic / .mcstructure / .mcworld 文件拖到终端。',style='muted'),Text(''))
    return input_path(ui.ask('建筑文件'))


def review(plan,bot,server,verify,rate=4000,command_block_mode='reject'):
    s=plan.summary()
    mode={'sample':'抽样核对 64 个方块类型','complete':'逐个核对全部方块类型','none':'不校验，执行整套填充'}[verify]
    def render(selected):
        details=ui.pairs([('机器人',bot.label),('租赁服',server),('建筑',s['name']),
            ('尺寸',f"{s['size'][0]} × {s['size'][1]} × {s['size'][2]}"),
            ('维度',{'overworld':'主世界','nether':'下界','the_end':'末地'}[s['dimension']]),
            ('起点','  '.join(map(str,s['origin']))),('终点','  '.join(map(str,s['end']))),
            ('非空气方块',f"{s['blocks']:,}"),('合并后命令',f"{s['commands']:,}"),
            ('发送上限',f'{rate:,} 条/秒 · 突发最多 {min(rate,DEFAULT_BURST):,} 条'),('空气处理','跳过空气，保留建筑空白处原有内容'),('世界校验',mode)])
        warnings=[Panel(Text(w),border_style='warning',title='兼容性说明') for w in s['warnings']]
        if s.get('command_blocks'):
            warnings.insert(0,Panel(Text('命令方块：'+('暂存内部指令并读回；需要红石、首刻执行关闭。' if command_block_mode=='stage' else '仅放外壳，跳过内部指令。')),title='本次处理方式'))
        return Group(*ui.chrome('最后核对一次',step=5),Panel(details,border_style='accent',padding=(1,2)),
            *warnings,ui.card('开始导入','将覆盖目标范围内与建筑非空气方块重合的位置。',selected==0,1),
            ui.card('返回修改','还不开始放置。',selected==1,2))
    return ui.choose('最后核对一次',['开始导入','返回修改'],renderer=render)==0


async def perform(plan,store,bot,server,password,verify,resume=None,rate=4000,command_block_mode='reject'):
    progress=ui.ImportProgress(plan)
    ui.console.clear()
    with Live(progress.render(),console=ui.console,refresh_per_second=8,transient=True) as live:
        last=[0,None]
        def update(job):
            progress.update(job)
            now=time.monotonic()
            if now-last[0]>=0.125 or last[1]!=job['state']:
                live.update(progress.render())
                last[:]=[now,job['state']]
        return await execute_import(plan,store,bot,server,password,verify=verify,
                                     progress=update,resume=resume,commands_per_second=rate,live_console=True,stay_online=True,reconnect_attempts=0,
                                     command_block_mode=command_block_mode)


def show_result(result):
    status=result['state']
    title={'finished':'操作结束 · 用户已确认','paused':'任务暂停 · 断点已保存','completed':'导入完成 · 世界校验通过','sent':'整套填充已发送 · 未做完整性校验',
           'verification_failed':'发送与补齐已结束 · 仍有差异'}.get(status,'任务已保存')
    ui.screen(*ui.chrome(title),Panel(ui.pairs([
        ('建筑',result['plan']['name']),('已发送命令',f"{result['submitted']:,} / {result['plan']['commands']:,}"),
        ('世界校验',f"{result['passed']} / {result['checked']} 通过" if result['checked'] else '未执行'),
        ('用时',f"{result.get('elapsed_seconds',0):.1f} 秒"),('任务记录',result['id'])]),border_style='good' if status=='completed' else 'accent',padding=(1,2)))
    if status in ('paused','failed','reconnecting'):
        details = result.get('error_details',{})
        reason = details.get('message') or result.get('detail') or result.get('error_type','原因未记录')
        ui.notice('暂停原因',reason,'warning')
        if result.get('error_type'):
            ui.notice('错误类型',result['error_type'],'warning')
    for warning in result['plan']['warnings']:ui.notice('兼容性说明',warning,'warning')
    pause()


def choose_rate(default=4000):
    options=[f'使用默认速度 · {default:,} 条/秒','中速 · 2000 条/秒','较保守 · 1000 条/秒','自定义速度']
    selected=ui.choose('选择发送上限',options,descriptions=['默认4000；平滑突发10条；实际吞吐受服务器和校验影响。','降低发送上限，实际速度仍取决于服务器。','适合较慢的服务器。','范围1～5000，发送速率不等于方块落地速度。'])
    if selected<0:raise ui.Back()
    if selected<3:return (default,2000,1000)[selected]
    from proloader.cli.main import speed
    import argparse
    try:return speed(ui.ask('每秒命令数',str(default)))
    except argparse.ArgumentTypeError as exc:raise ValueError(str(exc)) from None


def start_import(store):
    bots=store.list()
    if not bots:
        ui.screen(*ui.chrome('先添加一个机器人'),Text('机器人账号用于进入服务器、加载区块和放置建筑。',style='muted'))
        pause()
        add_bot(store)
        return
    pending=pending_jobs()
    resume=None
    if pending:
        action=ui.choose('开始导入',['新建导入','继续中断的导入'],descriptions=['选择建筑，设置新的放置位置。',f'{len(pending)} 个任务可以继续。'])
        if action<0:return
        if action==1:
            choice=ui.choose('选择要继续的任务',[j['plan']['name'] for j in pending],
                         descriptions=[f"{j['submitted']:,} / {j['plan']['commands']:,} 条 · 服务器 {j['server']}" for j in pending])
            if choice<0:return
            resume=pending[choice]
    if resume:
        bot=next((b for b in bots if b.id==resume['bot_id']),None)
        if not bot:raise ValueError('原任务的机器人已移除，请先恢复该账号。')
        server=resume['server']
        file=Path(resume['plan']['file'])
        origin=tuple(resume['plan']['origin'])
        dimension=resume['plan']['dimension']
        ui.screen(*ui.chrome('继续导入',step=1),Text(f'目标服务器：{server}  ·  密码不会保存。',style='muted'),Text(''))
        password=ui.ask('服务器密码（无密码直接回车）',secret=True)
    else:
        chosen=0 if len(bots)==1 else ui.choose('选择执行导入的机器人',[b.label for b in bots],descriptions=[b.game_name for b in bots])
        if chosen<0:return
        bot=bots[chosen]
        ui.screen(*ui.chrome('开始导入',step=1),Text('输入游戏内看到的租赁服编号。',style='muted'),Text(''))
        server=ui.ask('租赁服编号',preferences().get('server'))
        if not re.fullmatch(r'\d{5,12}',server):raise ValueError('租赁服编号应为 5～12 位数字。')
        password=ui.ask('服务器密码（无密码直接回车）',secret=True)
        file=select_file()
        ui.screen(*ui.chrome('开始导入',step=3),Text('起点对应建筑边界的最小 X / Y / Z。',style='muted'),Text(''))
        origin=parse_coordinates(ui.ask('起点坐标 X Y Z'))
        dim=ui.choose('选择放置维度',['主世界','下界','末地'])
        if dim<0:raise ui.Back()
        dimension=('overworld','nether','the_end')[dim]
    if resume:
        with ui.console.status('[accent]读取建筑并转换方块状态…'):
            from proloader.cli.commands import resume_plan
            plan=resume_plan(resume)
    else:
        from proloader.cli.commands import prepare_with_height_correction
        plan=prepare_with_height_correction(file,origin,dimension,planner=prepare_import)
    command_block_mode = resume.get('command_block_mode','reject') if resume else 'reject'
    if plan.command_blocks and not resume:
        selected=ui.choose('命令方块内部数据', ['暂存指令并读回核对','只放外壳，跳过内部数据','返回'],
            descriptions=['需要红石、关闭首刻执行；外部红石仍可触发。Java指令暂不支持。',
                          '明确跳过内部指令，不会报告NBT完成。','暂不连接服务器。'])
        if selected not in (0,1):return
        command_block_mode = ('stage','skip')[selected]
    mode=ui.choose('选择校验方式',['抽样校验 · 推荐','逐块校验','关闭校验 · 只发送建筑'],
        descriptions=['失败先记录，发送完成后补齐，最多两轮；抽样核对64点。',
                      '核对全部非空气方块类型，发现差异后补齐，最多两轮。',
                      '不会因为单块检查不一致暂停；结果记录为已发送，未验收。'])
    if mode<0:raise ui.Back()
    verify=('sample','complete','none')[mode]
    rate=choose_rate(resume.get('speed',{}).get('rate',4000) if resume else 4000)
    nbt_options = dict(command_block_mode=command_block_mode) if plan.command_blocks else {}
    if not review(plan,bot,server,verify,rate,**nbt_options):return
    private_json(PREFERENCES,{'server':server})
    result=asyncio.run(perform(plan,store,bot,server,password,verify,resume['id'] if resume else None,rate,**nbt_options))
    password=''
    show_result(result)


def run_tui():
    logging.disable(logging.CRITICAL)
    store=BotStore()
    while True:
        try:
            choice=ui.choose('', ['管理机器人','开始导入'],renderer=lambda current:ui.home(len(store.list()),len(pending_jobs()),current))
            if choice<0:
                ui.screen(*ui.chrome(),Text('下次见。你的机器人和任务记录已保存在本机。',style='muted'))
                return
            if choice==0:manage_bots(store)
            else:start_import(store)
        except (ui.Back,KeyboardInterrupt):
            continue
        except EOFError:
            return
        except Exception as exc:
            show_error(exc)
