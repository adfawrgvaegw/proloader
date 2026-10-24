"""Real rental import through the same engine as the interactive CLI."""
import argparse
import asyncio
import getpass
import json
import logging
import sys
from pathlib import Path

from proloader.bots import BotStore
from proloader.importer import prepare_import, execute_import, WorldCommands
from proloader.net.netease.runtime import rental_session


async def run(args, password):
    store = BotStore()
    bot = next((bot for bot in store.list() if bot.id == args.bot), None)
    if bot is None:
        raise ValueError('未找到机器人')
    plan = prepare_import(args.file, tuple(map(int, args.origin.split(','))), args.dimension)
    print(json.dumps(dict(stage='plan', **plan.summary()), ensure_ascii=False), flush=True)
    if args.probe_only:
        async with rental_session(store,bot,args.server,password,
                                  on_stage=lambda text: print(json.dumps(dict(stage='connect',step=text),ensure_ascii=False),flush=True)) as (session,name):
            commands = WorldCommands(session,name,plan)
            await commands.send('testfor @s')
            print(json.dumps(dict(stage='permission',ok=True)),flush=True)
            await commands.teleport(*plan.origin)
            print(json.dumps(dict(stage='teleport',ok=True)),flush=True)
            first = plan.commands[0]
            await commands.place(first, wait=True)
            ok = await commands.check(first[1], first[2], first[3], first[7])
            print(json.dumps(dict(stage='first_building_block',ok=ok,position=list(first[1:4]),block=first[7]),ensure_ascii=False),flush=True)
            return 0 if ok else 1
    previous = [None, -100]
    def progress(job):
        if job['state'] != previous[0] or job['submitted'] - previous[1] >= 100:
            print(json.dumps({k:job[k] for k in ('id','state','submitted','checked','passed')},ensure_ascii=False),flush=True)
            previous[:] = [job['state'],job['submitted']]
    job = await execute_import(plan,store,bot,args.server,password,verify=args.verify,
                               commands_per_second=args.rate,progress=progress,resume=args.resume)
    print(json.dumps(dict(stage='result',**job),ensure_ascii=False),flush=True)
    return 0 if job['state'] in ('completed','sent') else 1


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--bot',required=True)
    p.add_argument('--server',required=True)
    p.add_argument('--file',type=Path,required=True)
    p.add_argument('--origin',required=True)
    p.add_argument('--dimension',default='overworld')
    p.add_argument('--verify',choices=('sample','complete','none'),default='sample')
    p.add_argument('--rate',type=int,default=5000)
    p.add_argument('--password-stdin',action='store_true')
    p.add_argument('--probe-only',action='store_true')
    p.add_argument('--resume')
    a=p.parse_args()
    logging.disable(logging.CRITICAL)
    try:
        return asyncio.run(run(a,sys.stdin.readline().rstrip('\r\n') if a.password_stdin else getpass.getpass('租赁服密码（隐藏输入）：')))
    except Exception as exc:
        print(json.dumps(dict(ok=False,error=type(exc).__name__,detail=str(exc)[:300]),ensure_ascii=False))
        return 1


if __name__=='__main__':
    raise SystemExit(main())
