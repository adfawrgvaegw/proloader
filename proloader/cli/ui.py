"""A compact terminal design system: keyboard menus, quiet chrome, honest progress."""
from __future__ import annotations
import os
import select
import sys
import time

from proloader.game_ui import duration
from rich import box
from rich.align import Align
from rich.console import Console, Group
from rich.live import Live
from rich.panel import Panel
from rich.progress import Progress, BarColumn, TextColumn, TaskProgressColumn
from rich.table import Table
from rich.text import Text
from rich.theme import Theme

ACCENT = '#69e1c1'
MUTED = '#829b95'
LINE = '#28433b'
THEME = Theme({'accent':ACCENT,'muted':MUTED,'good':'#91e2ab',
               'warning':'#edc887','bad':'#f3949d','title':'bold #edf5f1'})
console = Console(theme=THEME, highlight=False)


class Back(Exception):
    pass


def chrome(title='', step=None):
    top = Table.grid(expand=True)
    top.add_column(ratio=1)
    top.add_column(justify='right')
    top.add_row(Text('▌ PROLOADER',style='bold '+ACCENT),Text('BUILD SOMETHING EXTRAORDINARY',style=MUTED))
    parts = [Text(''),top,Text('  把建筑带进你的世界。',style=MUTED),Text('')]
    if title:
        parts += [Text(title,style='bold #edf5f1'),Text('')]
    if step is not None:
        trail=Text()
        for i,label in enumerate(('服务器','建筑文件','坐标','校验','确认'),1):
            trail.append(('●' if i<step else f'{i:02}')+' '+label,
                         style=ACCENT if i==step else (MUTED if i<step else '#526b62'))
            if i<5:trail.append('  ─  ',style=LINE)
        parts += [trail,Text('')]
    return parts


def screen(*parts):
    if console.is_terminal:
        console.clear()
    console.print(Group(*parts))


def card(label, detail='', selected=False, number=None):
    label = Text(('› ' if selected else '  ')+(f'{number:02}  ' if number else '')+label,
                 style='bold '+('#e9fff6' if selected else '#bdcec8'))
    content = Group(label,Text('     '+detail,style=MUTED)) if detail else label
    return Panel(content,box=box.ROUNDED,border_style=ACCENT if selected else LINE,
                 padding=(1,2),style='on #10251e' if selected else '')


def home(bot_count=0, pending_count=0, selected=0):
    status=Text(f'本机身份  {bot_count} 个',style=MUTED)
    if pending_count:status.append(f'    ·    可继续任务  {pending_count} 个',style='#edc887')
    return Group(*chrome(),Text('从这里开始',style='bold #edf5f1'),Text(''),
                 card('管理机器人','添加账号，验证身份，随时准备出发。',selected==0,1),
                 card('开始导入','选择世界与建筑，预检，然后看它落地。',selected==1,2),
                 Text(''),status,Text(''),
                 Text('↑ ↓ 选择     Enter 进入     1 / 2 快选     Q 退出',style=MUTED))


def menu_bounds(count, current, height):
    page_size=max(1,min(9,(height-10)//4))
    start=(current//page_size)*page_size
    return start,min(count,start+page_size)


def menu_view(title, options, descriptions, current, height):
    start,end=menu_bounds(len(options),current,height)
    return Group(*chrome(title),*(Panel(Group(
        Text(f"{'›' if i==current else ' '} {i-start+1}  {options[i]}",style='accent' if i==current else 'bold',no_wrap=True,overflow='ellipsis'),
        Text(descriptions[i],style=MUTED,no_wrap=True,overflow='ellipsis')),border_style=ACCENT if i==current else LINE,padding=(0,1))
        for i in range(start,end)),
        Text(f'第 {start+1}–{end} 项 / 共 {len(options)} 项',style=MUTED),
        Text('↑↓ 选择  N/P 翻页  Home/End 首尾  数字选本页  Enter确认  Esc返回',style=MUTED))


def choose(title, options, *, descriptions=None, default=0, renderer=None):
    if not options:return -1
    descriptions=descriptions or ['']*len(options)
    current=max(0,min(default,len(options)-1))
    interactive=sys.stdin.isatty() and console.is_terminal and os.name=='posix' and os.environ.get('TERM')!='dumb'
    def render():
        if renderer:return renderer(current)
        if interactive:return menu_view(title,options,descriptions,current,console.size.height)
        return Group(*chrome(title),*(card(option,descriptions[i],i==current,i+1)
            for i,option in enumerate(options)))
    if not interactive:
        console.print(render())
        while True:
            raw=input(f'选择 1–{len(options)}（q 返回）：').strip()
            if not raw:return current
            if raw.lower() in ('q','b'):return -1
            if raw.isdigit() and 1<=int(raw)<=len(options):return int(raw)-1
            console.print('请输入有效的选项编号。',style='warning')
    import termios
    import tty
    old=termios.tcgetattr(sys.stdin.fileno())
    console.clear()
    try:
        tty.setcbreak(sys.stdin.fileno())
        with Live(render(),console=console,auto_refresh=False,transient=True) as live:
            while True:
                key=os.read(sys.stdin.fileno(),1).decode('ascii','ignore')
                start,end=(0,len(options)) if renderer else menu_bounds(len(options),current,console.size.height)
                size=max(1,end-start)
                if key in ('\r','\n'):return current
                if key in ('q','Q','\x03'):return -1
                if key=='\x1b':
                    if not select.select([sys.stdin],[],[],0.08)[0]:return -1
                    sequence=''
                    while select.select([sys.stdin],[],[],0.01)[0] and len(sequence)<8:
                        sequence+=os.read(sys.stdin.fileno(),1).decode('ascii','ignore')
                        if sequence.endswith(('A','B','H','F','~')):break
                    key={'[A':'k','[B':'j','[5~':'p','[6~':'n','[H':'g','[F':'G','[1~':'g','[4~':'G'}.get(sequence,'')
                if key in ('j','s'):current=(current+1)%len(options)
                elif key in ('k','w'):current=(current-1)%len(options)
                elif key in ('n','N'):current=min(len(options)-1,current+size)
                elif key in ('p','P'):current=max(0,current-size)
                elif key=='g':current=0
                elif key=='G':current=len(options)-1
                elif key.isdigit() and 1<=int(key)<=size:return start+int(key)-1
                live.update(render(),refresh=True)
    finally:
        termios.tcsetattr(sys.stdin.fileno(),termios.TCSADRAIN,old)


def ask(label,default=None,secret=False):
    if secret:
        import getpass
        return getpass.getpass(label+'：').strip()
    suffix=f' [muted]({default})[/muted]' if default is not None else ''
    raw=console.input(f'[accent]›[/accent] {label}{suffix}：').strip()
    if raw.lower() in ('q','返回'):raise Back()
    return raw or (str(default) if default is not None else '')


def pairs(items):
    table=Table.grid(padding=(0,3),expand=True)
    table.add_column(style=MUTED,width=16)
    table.add_column(style='#e4eee9',ratio=1)
    for label,value in items:table.add_row(Text(str(label)),Text(str(value)))
    return table


def notice(title,body,style='accent'):
    console.print(Panel(Text(str(body)),title=title,border_style=style,padding=(1,2),box=box.ROUNDED))


class ImportProgress:
    def __init__(self,plan):
        self.plan=plan
        self.block_count=plan.block_count
        self.started=time.monotonic()
        self.bar=Progress(TextColumn('[accent]{task.description}'),BarColumn(complete_style=ACCENT,finished_style=ACCENT),
                          TaskProgressColumn(),TextColumn('[muted]{task.completed:,.0f} / {task.total:,.0f}'),expand=True)
        self.task=self.bar.add_task('放置',total=len(plan.commands))
        self.job={'state':'connecting','detail':'准备连接','submitted':0,'checked':0,'passed':0}

    def render(self):
        j=self.job
        verifying=j['state']=='verifying' or (j['state']=='loading' and j['submitted']==len(self.plan.commands))
        total=self.block_count if j.get('verify_mode')=='complete' else min(64,self.block_count)
        repairing=j['state']=='repairing' or j.get('manual_repair_in_progress',False)
        nbt=j['state']=='command_block_data' or (j.get('command_blocks_state')=='staging' and j['state']=='loading')
        repair=j.get('repair_rounds',[{}])[-1] if j.get('repair_rounds') else {}
        if j.get('manual_repair_in_progress'):
            repair=dict(sent=j.get('manual_repair_submitted',0),commands=len(self.plan.commands))
        reading=j.get('repair_telemetry',{}) if j.get('manual_repair_in_progress') else j.get('telemetry',{})
        self.bar.update(self.task,description='命令方块数据' if nbt else ('补齐' if repairing else ('核对' if verifying else '发送')),
                        completed=j.get('command_blocks_next',0) if nbt else (repair.get('sent',0) if repairing else (j['checked'] if verifying else j['submitted'])),
                        total=j.get('plan',{}).get('command_blocks',0) if nbt else (repair.get('commands',0) if repairing else (total if verifying else len(self.plan.commands))))
        return Group(*chrome('建筑正在落地'),Panel(Group(
            Text(self.plan.structure.name,style='bold #edf5f1'),Text(''),
            pairs([('当前步骤',j.get('detail','准备中')),('目标位置',', '.join(map(str,self.plan.origin))),
                   ('已运行',f'{int(time.monotonic()-self.started)//60:02}:{int(time.monotonic()-self.started)%60:02}'),
                   ('世界校验',f"{j['passed']} / {j['checked']} 通过" if j['checked'] else '等待执行'),
                   ('待补齐记录',str(len(j.get('repair_pending',{})))),
                   ('实测速率',f"{reading.get('rate',0):,.1f} 条/秒"),
                   ('预计剩余',duration(reading.get('eta_seconds'))),
                   ('发送上限',f"{j.get('speed',{}).get('rate',4000):,} 条/秒")]),
            Text(''),self.bar),border_style=ACCENT,padding=(1,2),box=box.ROUNDED),
            Text('Ctrl+C 暂停并保存断点。发送进度与世界校验分开记录。',style=MUTED))

    def update(self,job):
        self.job=job
