"""Shared real CLI actions. Output never includes cookie/PE files or passwords."""
from __future__ import annotations
import asyncio
import getpass
import importlib.util
import json
from pathlib import Path
import shutil
import sys
import time

from proloader.bots import BotStore
from proloader.cli import ui
from proloader.importer import prepare_import,execute_import,HeightLimitError

JOBS=Path.home()/'.proloader'/'imports'


def emit(value):
    print(json.dumps(value,ensure_ascii=False),flush=True)


def select_bot(store,selector=None):
    bots=store.list()
    if selector:
        matched=[b for b in bots if selector in (b.id,b.label)]
        if len(matched)!=1:raise ValueError('机器人不存在或名称重复，请使用 bots 中显示的完整ID。')
        return matched[0]
    if len(bots)!=1:raise ValueError('请先添加机器人，或使用 --bot 指定机器人ID。')
    return bots[0]


def read_job(task_id):
    if len(task_id)!=32 or any(c not in '0123456789abcdef' for c in task_id):
        raise ValueError('任务ID应为32位小写十六进制，可用 tasks 查看。')
    try:
        job=json.loads((JOBS/f'{task_id}.json').read_text())
    except (OSError,ValueError):raise ValueError('任务不存在或记录损坏。') from None
    if job.get('id')!=task_id:raise ValueError('任务记录与ID不一致。')
    return job


def jobs():
    if not JOBS.exists():return []
    result=[]
    for path in sorted(JOBS.glob('*.json'),key=lambda p:p.stat().st_mtime,reverse=True):
        try:result.append(read_job(path.stem))
        except ValueError:continue
    return result


def resume_plan(job):
    if job['state'] in ('sent','completed','finished'):
        raise ValueError('任务已经结束；如需重建，请新建导入。')
    saved=job['plan']
    if not saved.get('plan_sha256') or not saved.get('wire_revision'):
        raise ValueError('这是旧版施工计划，无法安全沿用断点。请用原文件和坐标新建导入。')
    plan=prepare_import(saved['file'],tuple(saved['origin']),saved['dimension'],engine=saved['engine'])
    if plan.summary()!=saved:
        raise ValueError('建筑文件或施工计划已变化，不能按旧下标续传。请新建导入。')
    return plan


def display_plan(plan,rate=None,verify=None):
    summary=plan.summary()
    ui.screen(*ui.chrome('导入计划'),ui.pairs([
        ('建筑',summary['name']),('起点',', '.join(map(str,summary['origin']))),
        ('终点',', '.join(map(str,summary['end']))),('维度',summary['dimension']),
        ('目标方块',f"{summary['blocks']:,}"),('命令',f"{summary['commands']:,}"),
        ('发送速率',f'{rate}/秒' if rate else '尚未连接'),('校验',verify or '离线预检'),
        ('覆盖方式','覆盖建筑非空气目标；保留空白处原有内容')]))
    for warning in summary['warnings']:ui.notice('兼容性说明',warning,'warning')


def password(args):
    if args.password_stdin:
        line=sys.stdin.readline()
        if line=='':raise ValueError('标准输入没有密码行；无密码服务器请提供空行。')
        return line.rstrip('\r\n')
    if not sys.stdin.isatty():
        raise ValueError('非交互运行请使用 --password-stdin；密码不应放进命令参数。')
    return getpass.getpass('租赁服密码（隐藏输入，无密码直接回车）：')


def prepare_with_height_correction(file, origin, dimension, *, engine='fatalder', interactive=True, planner=None):
    planner = planner or prepare_import
    while True:
        try:
            with ui.console.status('[accent]读取建筑并转换方块状态…'):
                return planner(file, origin, dimension, engine=engine)
        except HeightLimitError as exc:
            if not interactive:raise
            ui.notice('调整放置高度', str(exc), 'warning')
            if exc.suggested_origin is not None:
                action=ui.choose('建筑超出限高，调整后继续',
                    [f'使用建议高度 · Y={exc.suggested_origin[1]}','手动修改起始高度','返回'],
                    descriptions=['保留X/Z、服务器、机器人等设置。','只修改Y坐标，重新检查高度。','暂不连接服务器。'])
                if action==0:
                    origin=exc.suggested_origin
                    continue
                if action==1:
                    value=ui.ask('新的起始高度 Y',str(exc.suggested_origin[1]))
                    try:y=int(value)
                    except ValueError:
                        ui.notice('高度格式不正确','请输入整数Y坐标。','warning')
                        continue
                    origin=(origin[0],y,origin[2])
                    continue
            else:
                action=ui.choose('建筑本身超过维度高度', ['更换维度','更换建筑文件','返回'])
                if action==0:
                    selected=ui.choose('选择放置维度',['主世界','下界','末地'])
                    if selected in (0,1,2):dimension=('overworld','nether','the_end')[selected]
                    continue
                if action==1:
                    from proloader.cli.wizard import select_file
                    file=select_file()
                    continue
            raise ui.Back()


async def run_import(args,store,bot,plan,server,secret,rate,verify,resume=None):
    previous=[None,-100]
    last_check=[-1,0.0]
    def progress(job):
        check_update=(job.get('checked',0)!=last_check[0] and time.monotonic()-last_check[1]>=1)
        if job['state']!=previous[0] or job['submitted']-previous[1]>=100 or check_update:
            event={k:job[k] for k in ('id','state','submitted','checked','passed','detail','telemetry') if k in job}
            if args.json:emit(dict(event='progress',**event))
            else:print(f"{job['detail']} · {job['submitted']:,}/{len(plan.commands):,} 条 · 校验 {job['passed']}/{job['checked']}",flush=True)
            previous[:]=[job['state'],job['submitted']]
            last_check[:]=[job.get('checked',0),time.monotonic()]
    job=await execute_import(plan,store,bot,server,secret,verify=verify,
                             commands_per_second=rate,progress=progress,resume=resume,live_console=True,
                             stay_online=not getattr(args,'exit_after_send',False),reconnect_attempts=5,
                             command_block_mode=getattr(args,'command_blocks',None) or 'reject')
    if args.json:emit(dict(event='result',**job))
    else:
        ui.notice('任务结束',f"状态：{job['state']}\n已发送 {job['submitted']:,} 条；校验 {job['passed']}/{job['checked']}\n用时 {job.get('elapsed_seconds',0):.2f} 秒\n任务ID：{job['id']}",
                  'good' if job['state']=='completed' else 'warning')
    return 0 if job['state'] in ('completed','sent','finished') else 1


def dispatch(args):
    store=BotStore()
    if args.command=='bots':
        if args.bot_action in (None,'list'):
            for bot in store.list():
                print(f'{bot.id}  {bot.label}  {bot.game_name}'+('  使用中' if store.is_busy(bot) else ''))
            if not store.list():print('暂无机器人。运行 proloader 打开菜单添加。')
        elif args.bot_action=='add':
            bot=store.add(args.name,args.file.expanduser().read_text())
            print(f'已保存机器人 {bot.label}  {bot.id}；首次进服时验证身份。')
        else:
            if not args.yes:raise ValueError('移除本机身份需加 --yes，或在交互菜单中操作。')
            bot=select_bot(store,args.bot);store.remove(bot);print('已移除本机机器人。')
        return 0
    if args.command in ('tasks','list'):
        data=read_job(args.id) if args.id else jobs()
        if args.json:emit(data)
        else:
            for job in ([data] if isinstance(data,dict) else data):
                print(f"{job['id']}  {job['state']}  {job['plan']['name']}  {job['submitted']}/{job['plan']['commands']} 条  {job.get('elapsed_seconds',0):.2f}秒")
            if not data:print('暂无导入任务。')
        return 0
    if args.command=='doctor':return doctor(args.json)
    if args.command=='plan':
        plan=prepare_with_height_correction(args.file,args.to,args.dimension,engine=args.engine,
            interactive=sys.stdin.isatty() and not args.json)
        if args.json:emit(plan.summary())
        else:display_plan(plan)
        return 0
    if args.command=='resume':
        job=read_job(args.id);plan=resume_plan(job)
        if args.command_blocks is None:args.command_blocks = job.get('command_block_mode','reject')
        bot=select_bot(store,job['bot_id']);server=job['server']
        rate=args.rate or job.get('speed',{}).get('rate',4000)
        verify=args.verify or job.get('verify_mode','sample')
        resume=args.id
    else:
        if not args.server.isdigit() or not 5<=len(args.server)<=12:raise ValueError('服务器编号应为5～12位数字。')
        bot=select_bot(store,args.bot);server=args.server
        plan=prepare_with_height_correction(args.file,args.to,args.dimension,engine=args.engine,
            interactive=sys.stdin.isatty() and not args.json and not args.password_stdin)
        rate,verify,resume=args.rate,args.verify,None
    if store.is_busy(bot):raise ValueError('机器人正在执行其他任务，请等待结束。')
    if args.json:emit(dict(event='plan',**plan.summary(),rate=rate,verify=verify,command_block_mode=args.command_blocks))
    else:
        display_plan(plan,rate,verify)
        if getattr(plan,'command_blocks',None):
            print('命令方块处理：'+{'stage':'暂存内部指令并读回；需要红石、首刻执行关闭。','skip':'仅放外壳，跳过内部指令。','reject':'尚未选择，默认停止。'}.get(args.command_blocks,'尚未选择，默认停止。'))
    if not args.yes:
        if not sys.stdin.isatty() or args.password_stdin:
            raise ValueError('非交互执行需加 --yes 确认上述覆盖范围。')
        if input('按上述计划开始覆盖？输入 y 确认：').strip().lower()!='y':
            print('已取消，未连接服务器。');return 0
    secret=password(args)
    try:return asyncio.run(run_import(args,store,bot,plan,server,secret,rate,verify,resume))
    finally:secret=''


from proloader.cli.health import doctor


def print_error(exc,as_json=False):
    safe=isinstance(exc,(ValueError,FileNotFoundError)) or exc.__class__.__name__ in ('PlacementCheckError','RentalError','ParseError','UnsupportedFormatError','BedrockLoginError','BedrockDisconnectedError')
    message=str(exc)[:400] if safe else f'操作未完成（{type(exc).__name__}）；请用 tasks 查看已保存的任务。'
    if as_json:emit(dict(event='error',error=type(exc).__name__,message=message,
                        **({'details':exc.details} if isinstance(exc,HeightLimitError) else {})))
    else:print('未完成：'+message,file=sys.stderr)
