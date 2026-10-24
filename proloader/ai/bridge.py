"""One game connection, serialized local RPC, explicit activation and fresh readback."""
import asyncio
from collections import OrderedDict
import fcntl
import json
import os
from pathlib import Path
import signal
import time
from types import SimpleNamespace

from proloader.ai.program import validate, position, command, fingerprint, TYPES, FACING
from proloader.command_blocks import MODES
from proloader.importer import WorldCommands
from proloader.net.bedrock.command_block import build_command_block_update

MAX_REQUEST = 4 * 1024 * 1024


def socket_path():
    return Path.home() / '.proloader' / 'ai' / 'control.sock'


class Controller:
    def __init__(self, session, name):
        self.session, self.name = session, name
        self.started = time.time()
        self.stopped = asyncio.Event()
        self.lock = asyncio.Lock()
        self.results = OrderedDict()
        self.progress = None

    def world(self, program):
        return WorldCommands(self.session,self.name,SimpleNamespace(
            origin=tuple(program['origin']),dimension=program['dimension']))

    async def check(self, program, entry, active=False):
        world = self.world(program)
        p = tuple(a+b for a,b in zip(program['origin'],entry['at']))
        shell = SimpleNamespace(position=p,block_name=TYPES[entry['type']],conditional=entry['conditional'],
            states=dict(conditional_bit=entry['conditional'],facing_direction=FACING[entry['facing']]))
        if not await world.check_command_block(shell):
            raise ValueError(f'外壳类型/朝向/条件不符：{p}')
        nbt = await self.session.read_block_entity(p, dimension=program['dimension'])
        expected = dict(Command=entry['command'],CustomName=entry['name'],
            auto=int(active and entry['auto']),TickDelay=entry['delay'],
            ExecuteOnFirstTick=int(active and entry['first_tick']),TrackOutput=int(entry['track_output']))
        differences = [k for k,v in expected.items() if k not in nbt or nbt[k] != v]
        if differences:
            raise ValueError(f'内部数据未确认 {p}: {differences}')
        return nbt

    async def near(self, program, entry):
        p = tuple(a+b for a,b in zip(program['origin'],entry['at']))
        await self.world(program).teleport(p[0],p[1]+1,p[2])
        return p

    async def update(self, program, entry, active=False):
        p = await self.near(program,entry)
        await self.session.send_packet(build_command_block_update(p,mode=MODES[TYPES[entry['type']]],
            needs_redstone=not(active and entry['auto']),conditional=entry['conditional'],
            command=entry['command'],name=entry['name'],track_output=entry['track_output'],
            tick_delay=entry['delay'],execute_on_first_tick=active and entry['first_tick']))
        await self.world(program).send('testfor @s')
        return await self.check(program,entry,active)

    async def request(self, request):
        request_id = request.get('id')
        if not isinstance(request_id,str) or not 1 <= len(request_id) <= 128:
            return dict(ok=False,error='请求需要1～128字符id')
        if request.get('op')=='status':
            self.session._raise_if_failed()
            return dict(id=request_id,ok=True,result=dict(name=self.name,online=True,
                uptime_s=round(time.time()-self.started,1),busy=self.lock.locked(),progress=self.progress))
        signature = json.dumps(request,sort_keys=True,ensure_ascii=False)
        # Admission is nonblocking: no unbounded waiting writers.
        if self.lock.locked():
            return dict(id=request_id,ok=False,error='busy；当前操作仍在执行，请稍后用同一id查询')
        async with self.lock:
            if request_id in self.results:
                old_sig, result = self.results[request_id]
                if old_sig != signature:
                    return dict(id=request_id,ok=False,error='id已被不同请求使用')
                return result
            cancelled = False
            try:
                self.session._raise_if_failed()
                self.progress = dict(id=request_id,operation=request.get('op'),completed=0)
                value = await self.execute(request)
                result = dict(id=request_id,ok=True,result=value)
            except asyncio.CancelledError:
                cancelled = True
                result = dict(id=request_id,ok=False,error_type='CancelledError',
                    error='操作中断；部分世界写入可能已执行。先读回，禁止盲目重放。')
            except Exception as exc:
                result = dict(id=request_id,ok=False,error_type=type(exc).__name__,error=str(exc),
                    warning='可能有部分操作已执行；先读回，不要换id盲目重发')
            self.results[request_id] = (signature,result)
            self.progress = None
            if len(self.results)>512:
                self.results.popitem(last=False)
            if cancelled:raise asyncio.CancelledError
            return result

    async def execute(self, r):
        op = r['op']
        if op == 'status':
            return dict(name=self.name,online=True,uptime_s=round(time.time()-self.started,1))
        if op == 'spec':
            from proloader.ai.spec import PROFILE
            return PROFILE
        if op == 'stop':
            self.stopped.set()
            return dict(stopping=True)
        if op == 'command':
            output = await self.session.run_command_ai(command(r['command']),timeout=12,dimensional=True,bot_name=self.name)
            return dict(confirmed=output is not None,output=output,
                        note='未确认可能是负回执或超时；写入可能已生效')
        if op == 'batch':
            commands = r['commands']
            if not isinstance(commands,list) or not 1 <= len(commands) <= 10000:
                raise ValueError('批量命令数范围1～10000')
            commands = [command(c) for c in commands]  # Validate the entire batch before writing.
            results = []
            for i,c in enumerate(commands):
                out = await self.session.run_command_ai(c,timeout=12,dimensional=True,bot_name=self.name)
                results.append(dict(index=i,confirmed=out is not None,output=out))
                if self.progress:self.progress['completed']=len(results)
                if out is None:
                    break  # Never replay unknown non-idempotent writes automatically.
            return dict(results=results,submitted=len(results),total=len(commands),
                        all_confirmed=len(results)==len(commands) and all(e['confirmed'] for e in results))
        if op == 'inspect':
            p = position(r['position'])
            dim = r.get('dimension','overworld')
            if dim not in ('overworld','nether','the_end'):raise ValueError('维度无效')
            world = self.world(dict(origin=p,dimension=dim))
            await world.teleport(p[0],p[1]+1,p[2])
            return await self.session.read_block_entity(p,dimension=dim)
        if op not in ('deploy','write','arm','disarm','verify'):
            raise ValueError('未知操作')
        program = validate(r['program'])
        world = self.world(program)
        completed = []
        try:
            if op == 'deploy':
                # Check every destination before ANY placement. Existing builds are never overwritten.
                for e in program['blocks']:
                    p = await self.near(program,e)
                    if not await world.check(*p,'minecraft:air'):
                        raise ValueError(f'目标不是已确认空气：{p}，请换位置或显式清理')
                for e in program['blocks']:
                    p = await self.near(program,e)
                    await world.place(('setblock',*p,*p,TYPES[e['type']],dict(
                        conditional_bit=e['conditional'],facing_direction=FACING[e['facing']])),wait=False)
                    await world.send('testfor @s')
                    await self.update(program,e,False)
                    completed.append(e['at'])
                    if self.progress:self.progress['completed']=len(completed)
            elif op == 'write':
                # Explicit hot edit: require matching shells before overwriting any command text.
                for e in program['blocks']:
                    p = await self.near(program,e)
                    shell = SimpleNamespace(position=p,block_name=TYPES[e['type']],conditional=e['conditional'],
                        states=dict(conditional_bit=e['conditional'],facing_direction=FACING[e['facing']]))
                    if not await world.check_command_block(shell):
                        raise ValueError(f'热改目标外壳不符：{p}；类型/朝向变化请先停机并显式重建')
                for e in sorted(program['blocks'],key=lambda e:e['type']=='chain'):
                    await self.update(program,e,False)
                    completed.append(e['at'])
                    if self.progress:self.progress['completed']=len(completed)
            elif op == 'arm':
                # Exact source must match staged world data before any activation.
                for e in program['blocks']:
                    await self.near(program,e)
                    await self.check(program,e,False)
                # Chain tails first; repeat/impulse heads last, regardless of input ordering.
                entries = sorted(program['blocks'],key=lambda e: e['type']!='chain')
                for e in entries:
                    await self.update(program,e,True)
                    completed.append(e['at'])
                    if self.progress:self.progress['completed']=len(completed)
            elif op == 'disarm':
                # Heads first, then tails. This changes redstone mode; external power can still trigger.
                for e in sorted(program['blocks'],key=lambda e:e['type']=='chain'):
                    await self.update(program,e,False)
                    completed.append(e['at'])
                    if self.progress:self.progress['completed']=len(completed)
            else:
                active = r.get('active',False)
                if type(active) is not bool:raise ValueError('active必须为布尔值')
                for e in program['blocks']:
                    await self.near(program,e)
                    await self.check(program,e,active)
                    completed.append(e['at'])
                    if self.progress:self.progress['completed']=len(completed)
        except Exception as exc:
            # Report the precise completed prefix; do not imply a transactional world rollback.
            raise RuntimeError(f'{op}中止，已确认{len(completed)}/{len(program["blocks"])}块；{exc}') from exc
        return dict(operation=op,verified=len(completed),sha256=fingerprint(program),
                    mode='source' if op=='arm' or (op=='verify' and r.get('active')) else 'needs_redstone',
                    note='外部红石仍可能触发；不保证服务器永久在线')


async def serve(controller, path):
    path = Path(path)
    path.parent.mkdir(parents=True,exist_ok=True,mode=0o700)
    info = path.parent.stat()
    if info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise ValueError('socket目录必须由当前用户拥有且权限为0700')
    fd = os.open(str(path)+'.lock',os.O_CREAT|os.O_RDWR,0o600)
    server = None
    handlers = set()
    owned = False
    try:
        fcntl.flock(fd,fcntl.LOCK_EX|fcntl.LOCK_NB)
        path.unlink(missing_ok=True)  # Only after exclusive ownership of this endpoint.
        owned = True
        async def handle(reader,writer):
            task = asyncio.current_task()
            handlers.add(task)
            try:
                line = await asyncio.wait_for(reader.readline(),10)
                request = json.loads(line)
                result = await controller.request(request)
                writer.write(json.dumps(result,ensure_ascii=False).encode()+b'\n')
                await writer.drain()
            except (Exception,) as exc:
                try:
                    writer.write(json.dumps(dict(ok=False,error_type=type(exc).__name__)).encode()+b'\n')
                    await writer.drain()
                except (ConnectionError,OSError):pass
            finally:
                handlers.discard(task)
                writer.close()
                await writer.wait_closed()
        server = await asyncio.start_unix_server(handle,path=str(path),limit=MAX_REQUEST)
        os.chmod(path,0o600)
        print(json.dumps(dict(event='ready',socket=str(path),name=controller.name)),flush=True)
        while not controller.stopped.is_set():
            controller.session._raise_if_failed()
            try:
                await asyncio.wait_for(controller.stopped.wait(),1)
            except asyncio.TimeoutError:pass
    finally:
        if server:
            server.close()
            await server.wait_closed()
        for task in list(handlers):task.cancel()
        if handlers:await asyncio.gather(*handlers,return_exceptions=True)
        if owned:path.unlink(missing_ok=True)
        os.close(fd)


async def call(request, path=None):
    reader,writer = await asyncio.open_unix_connection(str(path or socket_path()),limit=MAX_REQUEST * 8)
    try:
        writer.write(json.dumps(request,ensure_ascii=False).encode()+b'\n')
        await writer.drain()
        return json.loads(await reader.readline())
    finally:
        writer.close()
        await writer.wait_closed()


async def run_daemon(bot_id,server,password,path):
    from proloader.bots import BotStore
    from proloader.net.netease.runtime import rental_session
    from proloader.net.bedrock.session import BedrockLoginError
    from proloader.net.raknet.connection import RakNetHandshakeError
    store = BotStore()
    bots = [b for b in store.list() if not bot_id or bot_id in (b.id,b.label)]
    if len(bots)!=1:raise ValueError('请用--bot指定唯一机器人ID或名称')
    loop = asyncio.get_running_loop()
    task = asyncio.current_task()
    for sig in (signal.SIGINT,signal.SIGTERM):loop.add_signal_handler(sig,task.cancel)
    failures = 0
    results = OrderedDict()
    while True:
        ready_at = None
        try:
            async with rental_session(store,bots[0],server,password,on_stage=lambda s: print(
                    json.dumps(dict(event='stage',detail=s),ensure_ascii=False),flush=True)) as (session,name):
                ready_at = time.monotonic()
                controller = Controller(session,name)
                controller.results = results
                await serve(controller,path)
                return  # Explicit stop never reconnects.
        except (BedrockLoginError,RakNetHandshakeError,ConnectionError,asyncio.TimeoutError) as exc:
            if ready_at is not None and time.monotonic()-ready_at >= 60:failures=0
            failures += 1
            if failures>5:raise
            delay = min(30,2**failures)
            print(json.dumps(dict(event='reconnecting',attempt=failures,delay_s=delay,
                                  error_type=type(exc).__name__,replay=False)),flush=True)
            await asyncio.sleep(delay)
