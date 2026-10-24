"""AI-facing CLI: one JSON response per call, no embedded paid model required."""
import argparse
import asyncio
import json
import logging
from pathlib import Path
import sys
import uuid

from proloader.ai.program import load,compile_program,layout
from proloader.ai.bridge import socket_path,call,run_daemon


def main(argv=None):
    logging.disable(logging.CRITICAL)
    parser = argparse.ArgumentParser(prog='proloader ai',description=__doc__)
    parser.add_argument('--socket',type=Path,default=socket_path())
    sub = parser.add_subparsers(dest='action',required=True)
    p = sub.add_parser('spec',help='查询内置网易手机版兼容性规范与证据范围')
    from proloader.ai.spec import PROFILE,lint,lint_program
    p.add_argument('--topic',choices=tuple(PROFILE['topics']))
    p = sub.add_parser('lint',help='保守静态检查；不能代替实服验证')
    p.add_argument('file',type=Path)
    p.add_argument('--context',choices=('bot','block'),default='block')
    p = sub.add_parser('compile',help='离线JSON→mcstructure，保留源配置')
    p.add_argument('file',type=Path)
    p.add_argument('--output',type=Path,required=True)
    p = sub.add_parser('layout',help='mcfunction自动排成蛇形命令链，生成JSON源文件')
    p.add_argument('file',type=Path)
    p.add_argument('--output',type=Path,required=True)
    p.add_argument('--origin',type=int,nargs=3,required=True)
    p.add_argument('--width',type=int,default=12)
    p.add_argument('--head',choices=('impulse','repeat'),default='impulse')
    p.add_argument('--delay',type=int,default=0)
    p.add_argument('--dimension',choices=('overworld','nether','the_end'),default='overworld')
    p = sub.add_parser('serve',help='常驻单身份假人；前台运行，可用nohup常驻')
    p.add_argument('--server',required=True)
    p.add_argument('--bot')
    p.add_argument('--password-stdin',action='store_true')
    for op in ('status','stop','command','batch','inspect','deploy','write','arm','disarm','verify','rpc'):
        p = sub.add_parser(op)
        p.add_argument('--id',default=None,help='重试时复用原id；同进程最近512个结果去重')
        if op=='command':p.add_argument('command')
        if op in ('batch','deploy','write','arm','disarm','verify','rpc'):p.add_argument('file',type=Path)
        if op=='inspect':
            p.add_argument('position',type=int,nargs=3)
            p.add_argument('--dimension',default='overworld',choices=('overworld','nether','the_end'))
        if op=='verify':p.add_argument('--active',action='store_true')
    args = parser.parse_args(argv)
    try:
        if args.action=='spec':
            result=dict(ok=True,result=PROFILE if not args.topic else dict(
                profile=PROFILE['id'],policy=PROFILE['policy'],topic=PROFILE['topics'][args.topic]))
        elif args.action=='lint':
            text=args.file.read_text()
            if args.file.suffix=='.json':
                data=json.loads(text)
                if isinstance(data,dict):
                    # Lint before strict validation so Java-specific diagnostics remain visible.
                    issues=lint_program(data)
                    if not any(e['severity']=='error' for e in issues):load(args.file)
                elif isinstance(data,list):
                    issues=[dict(line=i+1,**e) for i,c in enumerate(data) for e in lint(c,args.context)]
                else:raise ValueError('需要程序JSON或命令字符串数组')
            else:
                issues=[dict(line=i+1,**e) for i,c in enumerate(text.splitlines())
                        if c.strip() and not c.lstrip().startswith('#') for e in lint(c,args.context)]
            result=dict(ok=not any(e['severity']=='error' for e in issues),issues=issues,
                        profile=PROFILE['id'],note='仅静态启发式检查；通过不代表网易服务器支持')
        elif args.action=='compile':
            result = dict(ok=True,result=compile_program(load(args.file),args.output))
        elif args.action=='layout':
            program=layout(args.file.read_text(),args.origin,args.width,args.head,args.delay,args.dimension)
            args.output.write_text(json.dumps(program,ensure_ascii=False,indent=2)+'\n')
            result=dict(ok=True,result=dict(file=str(args.output.resolve()),blocks=len(program['blocks'])))
        elif args.action=='serve':
            password = sys.stdin.readline().rstrip('\r\n') if args.password_stdin else ''
            asyncio.run(run_daemon(args.bot,args.server,password,args.socket))
            return 0
        else:
            request = dict(op=args.action,id=args.id or uuid.uuid4().hex)
            if args.action=='command':request['command']=args.command
            if args.action=='batch':request['commands']=json.loads(args.file.read_text())
            if args.action=='inspect':request.update(position=args.position,dimension=args.dimension)
            if args.action in ('deploy','write','arm','disarm','verify'):request['program']=load(args.file)
            if args.action=='verify':request['active']=args.active
            if args.action=='rpc':
                request=json.loads(args.file.read_text())
                request.setdefault('id',args.id or uuid.uuid4().hex)
            result = asyncio.run(call(request,args.socket))
        print(json.dumps(result,ensure_ascii=False))
        return 0 if result.get('ok') else 2
    except (KeyboardInterrupt,asyncio.CancelledError):
        print(json.dumps(dict(ok=False,error='已中断；若客户端中断，服务器端操作可能仍在继续，请先查询状态')))
        return 130
    except Exception as exc:
        print(json.dumps(dict(ok=False,error_type=type(exc).__name__,error=str(exc)),ensure_ascii=False))
        return 2


if __name__=='__main__':sys.exit(main())
