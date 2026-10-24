"""Real rental CLI; no arguments opens the guided terminal interface."""
from __future__ import annotations
import argparse
import asyncio
import logging
import sys
import warnings
warnings.filterwarnings("ignore",message="urllib3 v2 only supports OpenSSL.*")
from pathlib import Path

from proloader import __version__


def coordinates(value):
    from proloader.cli.wizard import parse_coordinates
    try:
        return parse_coordinates(value)
    except ValueError as exc:
        raise argparse.ArgumentTypeError(str(exc)) from None


def speed(value):
    try:
        result=int(value)
    except ValueError:
        raise argparse.ArgumentTypeError('速度需要整数') from None
    if not 1<=result<=5000:
        raise argparse.ArgumentTypeError('速度范围为1～5000，默认4000')
    return result


def build_parser():
    parser=argparse.ArgumentParser(prog='proloader',description='网易租赁服建筑导入器；不带参数打开交互菜单。')
    parser.add_argument('-V','--version',action='version',version=f'proloader {__version__}')
    sub=parser.add_subparsers(dest='command',required=True)
    p=sub.add_parser('ai',help='AI常驻假人控制、网易指令规范、命令方块编译与调试')
    p.add_argument('arguments',nargs=argparse.REMAINDER)
    sub.add_parser('menu',help='打开交互菜单')
    p=sub.add_parser('bots',help='列出、添加或移除本机机器人')
    q=p.add_subparsers(dest='bot_action')
    q.add_parser('list',help='列出机器人')
    a=q.add_parser('add',help='从已有cookie文件添加；短信登录请用菜单')
    a.add_argument('file',type=Path)
    a.add_argument('--name',default='建筑助手')
    a=q.add_parser('remove',help='仅移除本机身份文件')
    a.add_argument('bot')
    a.add_argument('--yes',action='store_true')
    p=sub.add_parser('plan',help='离线预检建筑、边界和命令计划，不连接服务器')
    _plan_options(p)
    p.add_argument('--json',action='store_true')
    p=sub.add_parser('import',help='导入建筑到真实网易租赁服')
    _plan_options(p)
    p.add_argument('--server',required=True,help='租赁服编号')
    p.add_argument('--bot',help='机器人ID或唯一名称；只有一个时可省略')
    _run_options(p)
    p=sub.add_parser('tasks',aliases=['list'],help='查看真实导入任务与耗时')
    p.add_argument('id',nargs='?',help='查看某个任务的完整记录')
    p.add_argument('--json',action='store_true')
    p=sub.add_parser('resume',help='按原文件、坐标、服务器和计划继续任务')
    p.add_argument('id')
    _run_options(p,defaults=False)
    p=sub.add_parser('doctor',help='检查本机依赖与原生引擎，不登录、不发短信')
    p.add_argument('--json',action='store_true')
    p=sub.add_parser('simulate',help='开发用模拟导入、diff、rollback、preview等旧工具')
    p.add_argument('arguments',nargs=argparse.REMAINDER)
    return parser


def _plan_options(parser):
    parser.add_argument('file',type=Path,help='.schem / .schematic / .mcstructure / .mcworld 文件')
    parser.add_argument('--to',type=coordinates,required=True,help='建筑最小坐标，例如 1200,78,1000；负数用 --to=-85,78,351')
    parser.add_argument('--dimension',choices=('overworld','nether','the_end'),default='overworld')
    parser.add_argument('--engine',choices=('fatalder','legacy'),default='fatalder',help='默认Fatalder原生规划')


def _run_options(parser,defaults=True):
    parser.add_argument('--command-blocks', choices=('reject','stage','skip'), default='reject' if defaults else None,
                        help='含命令方块时必选：stage写入并读回，需要红石且关闭首刻执行（外部红石仍可触发）；skip仅放外壳；默认停止')
    parser.add_argument('--rate',type=speed,default=4000 if defaults else None,help='每秒命令数，默认4000；平滑突发最多10条')
    parser.add_argument('--verify',choices=('sample','complete','none'),default='sample' if defaults else None,
                        help='sample=64点名称抽样；complete=全部名称；none=只确认发送')
    parser.add_argument('--exit-after-send',action='store_true',help='发送结束即退出；默认留服等待游戏聊天repair/done')
    parser.add_argument('--password-stdin',action='store_true',help='从标准输入读取一行密码；不通过命令参数传递密码')
    parser.add_argument('--yes',action='store_true',help='按所列计划直接开始，跳过交互确认')
    parser.add_argument('--json',action='store_true',help='逐行输出JSON进度和最终结果')


def app():
    logging.disable(logging.CRITICAL)
    if len(sys.argv)>1 and sys.argv[1]=='ai':
        from proloader.ai.__main__ import main as ai_main
        raise SystemExit(ai_main(sys.argv[2:]))
    if len(sys.argv)==1:
        from proloader.cli.wizard import run_tui
        run_tui()
        return
    parser=build_parser()
    args=parser.parse_args()
    try:
        if args.command=='menu':
            from proloader.cli.wizard import run_tui
            run_tui()
            return
        if args.command=='simulate':
            from proloader.cli.simulation import app as simulation
            sys.argv=[sys.argv[0]]+args.arguments
            simulation()
            return
        if args.command=='doctor':
            from proloader.cli.health import doctor
            code=doctor(args.json)
        else:
            from proloader.cli.commands import dispatch
            code=dispatch(args)
    except (KeyboardInterrupt, asyncio.CancelledError):
        print('已暂停；已开始的导入任务已保存断点。',file=sys.stderr)
        code=130
    except Exception as exc:
        from proloader.cli.commands import print_error
        print_error(exc,getattr(args,'json',False))
        code=2
    if code:
        raise SystemExit(code)


if __name__=='__main__':
    app()
