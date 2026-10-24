"""Real rental imports: preflight → permission probe → bounded placement → world verification.

Progress counts submitted commands separately from verified world samples.
Checkpoints contain no server password, cookie or JWT. Replayed writes are idempotent.
"""
from __future__ import annotations
import asyncio
from dataclasses import dataclass, field
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import time
import uuid

from proloader.bots import BotStore, BotProfile, private_json
from proloader.parser import load_structure
from proloader.fatalder import native_plan, native_binary
from proloader.pacing import (DEFAULT_SPEED, DEFAULT_BURST, PROCESSING_LIMIT,
                             CommandPacer, ProcessingWindow, transport_pressure)
from proloader.block_names import command_state, WIRE_REVISION
from proloader.game_ui import GameConsole, ProgressMeter, READY
from proloader.net.bedrock.session import BedrockSession, BedrockLoginError
from proloader.net.netease.runtime import rental_session
from proloader.command_blocks import plan_command_blocks, operation_hash, REVISION as NBT_REVISION

PROCESSING_WINDOW = 256

DIMENSIONS = {'主世界': 'overworld', '下界': 'nether', '末地': 'the_end',
              'overworld': 'overworld', 'nether': 'nether', 'the_end': 'the_end'}


class CommandUnconfirmedError(BedrockLoginError):
    pass


class HeightLimitError(ValueError):
    """Recoverable preflight error, with the nearest valid placement height."""
    def __init__(self, origin, height, dimension, low, high):
        max_y = high - height + 1
        self.suggested_origin = ((origin[0], min(max(origin[1], low), max_y), origin[2])
                                 if max_y >= low else None)
        self.details = dict(kind='height_limit', origin=list(origin), height=height,
                            dimension=dimension, world_y_range=[low, high],
                            valid_start_y_range=[low, max_y] if max_y >= low else None,
                            suggested_origin=list(self.suggested_origin) if self.suggested_origin else None)
        if self.suggested_origin:
            message = (f'建筑高度{height}格，当前起点Y={origin[1]}，超出维度范围{low}～{high}。'
                       f'建议起点Y={self.suggested_origin[1]}；可用起点范围{low}～{max_y}。')
        else:
            message = (f'建筑高度{height}格，超过此维度可容纳的{high-low+1}格；'
                       '仅调整Y无法放下，请更换维度或选择裁剪后的建筑文件。')
        super().__init__(message)


class PlacementCheckError(RuntimeError):
    """Safe, structured world mismatch; no remote response or credentials."""
    def __init__(self, index, command):
        name, _ = command_state(command[7], command[8])
        self.details = dict(kind='block_mismatch', command_index=index,
                            position=list(command[1:4]), block=name)
        x, y, z = command[1:4]
        super().__init__(f'第 {index} 条命令未通过世界校验：{name}，坐标 {x} {y} {z}。已保留断点。')


@dataclass
class ImportPlan:
    file: Path
    structure: object
    origin: tuple
    dimension: str
    commands: list
    source_hash: str
    engine: str = "legacy"
    command_blocks: list = field(init=False, repr=False)

    def __post_init__(self):
        self.command_blocks = plan_command_blocks(self.structure, self.origin)

    @property
    def block_count(self):
        return sum((c[4]-c[1]+1)*(c[5]-c[2]+1)*(c[6]-c[3]+1) for c in self.commands)

    def summary(self):
        sx, sy, sz = self.structure.size
        x, y, z = self.origin
        result = dict(file=str(self.file), name=self.structure.name, size=[sx, sy, sz],
                    origin=[x, y, z], end=[x+sx-1, y+sy-1, z+sz-1],
                    dimension=self.dimension, blocks=self.block_count, commands=len(self.commands),
                    sha256=self.source_hash, engine=self.engine, wire_revision=WIRE_REVISION,
                    plan_sha256=hashlib.sha256(json.dumps([self.engine,self.origin,self.dimension,self.commands],sort_keys=True,separators=(',',':')).encode()).hexdigest(), warnings=list(self.structure.source_info.get('warnings', [])))
        if self.command_blocks:
            result.update(command_blocks=len(self.command_blocks), nbt_revision=NBT_REVISION,
                          command_blocks_sha256=operation_hash(self.command_blocks))
            result['plan_sha256'] = hashlib.sha256((result['plan_sha256']+result['command_blocks_sha256']).encode()).hexdigest()
            result['warnings'].append('含命令方块：须显式选择暂存内部数据或仅放外壳；Java指令暂不转换。暂存仍可能被外部红石触发。')
        return result


def prepare_import(file, origin, dimension='overworld', *, engine='fatalder') -> ImportPlan:
    file = Path(file).expanduser().resolve()
    if len(origin) != 3 or any(type(v) is not int for v in origin):
        raise ValueError('请输入三个整数坐标 X Y Z。')
    if dimension not in DIMENSIONS:
        raise ValueError('请选择主世界、下界或末地。')
    dimension = DIMENSIONS[dimension]
    structure = load_structure(file)
    low, high = (-64, 319) if dimension == 'overworld' else (0, 127 if dimension == 'nether' else 255)
    if origin[1] < low or origin[1] + structure.size[1] - 1 > high:
        raise HeightLimitError(origin, structure.size[1], dimension, low, high)
    if any(abs(v) > 29_999_000 for v in (origin[0], origin[2], origin[0]+structure.size[0], origin[2]+structure.size[2])):
        raise ValueError('建筑超出世界坐标范围。')
    if engine not in ('fatalder','legacy'):raise ValueError('未知施工引擎')
    commands = native_plan(structure, tuple(origin), native_binary()) if engine=='fatalder' else BedrockSession._generate_merged_commands(structure, tuple(origin))
    # Place structural blocks first, then attachment/fragile decorations.
    fragile = ('torch', 'ladder', 'button', 'lever', 'banner', 'flower', 'sapling', 'carpet',
               'tripwire', 'coral', 'lantern', 'end_rod', 'bell', 'bed', 'door', 'vine', 'bamboo')
    if engine=='legacy':
        commands.sort(key=lambda c: (any(part in c[7] for part in fragile), c[2], c[1], c[3]))
    return ImportPlan(file, structure, tuple(origin), dimension, commands,
                      hashlib.sha256(file.read_bytes()).hexdigest(), engine)


class WorldCommands:
    def __init__(self, session, name, plan):
        self.session, self.name, self.plan = session, name, plan

    async def send(self, command, wait=True, result_context=None, fast_probe=False):
        # Only the idempotent processing barrier may be repeated. World writes
        # and block probes keep their existing semantics; terminal failures stop.
        barrier = wait and command == 'testfor @s'
        attempts = 3 if barrier and not fast_probe else 1
        for attempt in range(attempts):
            self.session._raise_if_failed()
            tracking = {'result_context':result_context} if result_context is not None and hasattr(self.session,'subscribe_command_results') else {}
            result = await self.session.run_command_ai(command, timeout=(12 if fast_probe else 30) if barrier else 8,
                          await_response=wait, dimensional=True, bot_name=self.name, **tracking)
            if not wait or result is not None:
                return result
            self.session._raise_if_failed()
            if attempt+1 < attempts:
                await asyncio.sleep(0.5*(attempt+1))
        raise CommandUnconfirmedError(f'服务器命令处理确认未收到（{attempts}次只读确认），断点已保存' if barrier else 'aiCommand 执行失败或未确认')

    def relative(self, x, y, z):
        return ' '.join('~' if d == 0 else f'~{d}' for d in (x-self.plan.origin[0], y-self.plan.origin[1], z-self.plan.origin[2]))

    def context(self, command):
        x, y, z = self.plan.origin
        return f'execute in {self.plan.dimension} positioned {x} {y} {z} run {command}'

    async def teleport(self, x, y, z):
        try:
            await self.send(f'execute in {self.plan.dimension} run tp @s {x} {y} {z}')
        except BedrockLoginError as exc:
            self.session._raise_if_failed()
            if not str(exc).startswith('aiCommand 执行失败'):
                raise
        # Confirm location instead of allowing a failed teleport to shift an import.
        deadline=time.monotonic()+8
        while True:
            try:
                await self.send(f'execute in {self.plan.dimension} run testfor @s[x={x},y={y},z={z},r=3]')
                break
            except BedrockLoginError as exc:
                self.session._raise_if_failed()
                if not str(exc).startswith('aiCommand 执行失败') or time.monotonic()>=deadline:
                    raise
                await asyncio.sleep(0.25)
        await asyncio.sleep(0.25)

    async def check(self, x, y, z, name, states=None):
        name, _ = command_state(name,states)
        try:
            result = await self.send(self.context(f'testforblock {self.relative(x,y,z)} {name} []'))
            return result is not None
        except BedrockLoginError as exc:
            # Only a known command-negative response means mismatch. Session/timeout
            # failures must stop the job, never masquerade as an empty world block.
            if str(exc).startswith('aiCommand 执行失败'):
                return False
            raise

    async def check_command_block(self, block):
        """Unlike normal name sampling, command blocks require exact states."""
        states = dict(block.states)
        states.setdefault('conditional_bit', block.conditional)
        tile = BedrockSession._states_to_str(states)
        try:
            result = await self.send(self.context(f'testforblock {self.relative(*block.position)} {block.block_name} {tile}'))
            return result is not None
        except BedrockLoginError as exc:
            self.session._raise_if_failed()
            if str(exc).startswith('aiCommand 执行失败'):return False
            raise

    async def place(self, command, wait=False):
        kind, x, y, z, ex, ey, ez, name, states = command
        wire_name, wire_states=command_state(name,states)
        positions = self.relative(x,y,z)
        if kind == 'fill':
            positions += ' ' + self.relative(ex,ey,ez)
        text = self.context(f'{kind} {positions} {wire_name} {BedrockSession._states_to_str(wire_states)}')
        try:
            context = dict(index=getattr(self,'command_index',0),phase=getattr(self,'command_phase','initial'),position=[x,y,z],block=wire_name)
            await self.send(text, wait=wait, result_context=context)
        except BedrockLoginError as exc:
            if wait and str(exc).startswith('aiCommand 执行失败'):
                # Known NetEase false result: only accept after observing the block.
                if await self.check(x, y, z, name, states):
                    return
            raise


async def wait_for_import_permission(world, session, update):
    """Require a positive read-only command response before any import writes.

    NetEase reports both permission rejection and missing responses as unconfirmed.
    Neither is permission evidence; keep the current connection and let the owner
    grant access. Terminal session errors and cancellation still propagate.
    """
    update('checking_permission', detail='检查机器人命令权限')
    while True:
        session._raise_if_failed()
        try:
            await world.send('testfor @s', fast_probe=True)
        except BedrockLoginError as exc:
            session._raise_if_failed()
            if not isinstance(exc, CommandUnconfirmedError) and not str(exc).startswith('aiCommand 执行失败'):
                raise
            update('waiting_permission', detail=f'等待授予权限：请将机器人 {world.name} 设为操作员（OP）；确认权限后自动开始导入，Ctrl+C 可暂停')
            await asyncio.sleep(2)
        else:
            session._raise_if_failed()
            update('preflight', detail='命令权限已确认，准备导入')
            return


async def _execute_attempt(plan: ImportPlan, store: BotStore, bot: BotProfile,
                         server_code: str, password: str, *, verify='sample',
                         commands_per_second=DEFAULT_SPEED, progress=None, resume=None, jobs_root=None,
                         live_console=False, stay_online=False, reserved_id=None, command_block_mode='reject'):
    if verify not in ('sample', 'complete', 'none') or not 1 <= commands_per_second <= 5000:
        raise ValueError('无效校验方式或导入速度。')
    if command_block_mode not in ('reject', 'stage', 'skip'):
        raise ValueError('无效命令方块处理方式。')
    if plan.command_blocks:
        if command_block_mode == 'reject':
            raise ValueError('建筑含命令方块：请用 --command-blocks stage 暂存内部数据，或 skip 明确只放外壳。')
        if command_block_mode == 'stage':
            # Validate edition and encoding and build the offline reader before login.
            for block in plan.command_blocks:block.packet()
            native_binary('nbt')
    root = Path(jobs_root) if jobs_root else Path.home() / '.proloader' / 'imports'
    job_id = resume or reserved_id or uuid.uuid4().hex
    if len(job_id) != 32 or any(c not in '0123456789abcdef' for c in job_id):
        raise ValueError('无效任务编号。')
    path = root / f'{job_id}.json'
    if resume:
        job = json.loads(path.read_text())
        if job['plan'] != plan.summary() or job['server'] != server_code or job['bot_id'] != bot.id:
            raise ValueError('文件、坐标、服务器或机器人与中断任务不一致。')
        if plan.command_blocks and job.get('command_block_mode') != command_block_mode:
            raise ValueError('命令方块处理方式与中断任务不一致。')
        replay = job.get('processing_window', 1024) if job.get('verify_mode') == 'none' else 32
        start = max(0, job['submitted'] - max(32, replay))
        if (job['submitted'] == len(plan.commands) and verify != 'none'
                and 'placement_finished_seconds' in job):
            # Verification-only recovery must not replay placement commands.
            start = len(plan.commands)
    else:
        job = dict(id=job_id, plan=plan.summary(), server=server_code, bot_id=bot.id,
                   state='preparing', submitted=0, checked=0, passed=0,
                   created_at=datetime.now(timezone.utc).isoformat())
        start = 0
    job.pop('error_type', None)
    job.pop('error_details', None)
    job['processing_window']=PROCESSING_LIMIT
    job['processing_probe_interval']=PROCESSING_WINDOW
    job['verify_mode']=verify
    job['speed']=dict(rate=commands_per_second,burst=min(DEFAULT_BURST,commands_per_second))
    if plan.command_blocks:job['command_block_mode'] = command_block_mode
    private_json(path, job)

    meter=ProgressMeter(len(plan.commands),job['submitted'])

    def update(stage, **values):
        previous=job['state']
        job.update(values)
        job['state'] = stage
        job['telemetry']=meter.read(job['submitted'],force=stage!=previous)
        if stage!=previous:private_json(path,job)
        if progress:
            progress(dict(job))

    started = time.monotonic()
    metrics = dict(teleports=0, teleport_seconds=0.0, placement_seconds=0.0,
                   backpressure_seconds=0.0, verification_seconds=0.0)
    pacer = CommandPacer(commands_per_second)
    pending = job.setdefault('repair_pending', {})
    job.setdefault('mismatches_detected', 0)
    job.setdefault('repair_rounds', [])

    def record_mismatch(key, command):
        pending[key] = list(command)
        job['mismatches_detected'] += 1


    async def move(world, position):
        tick = time.monotonic()
        update('loading',detail='确认施工窗口与机器人位置',target=list(position))
        await world.teleport(*position)
        metrics['teleports']+=1
        metrics['teleport_seconds']+=time.monotonic()-tick
    def anchor(command):
        if plan.engine=='fatalder':
            return (plan.origin[0]+((command[1]-plan.origin[0])//48)*48+23,plan.origin[1],
                    plan.origin[2]+((command[3]-plan.origin[2])//48)*48+23)
        return ((command[1]+command[4])//2,plan.origin[1],(command[3]+command[6])//2)
    flow = None

    async def record_processing_probe(begin,end,phase):
        try:
            await world.send('testfor @s', fast_probe=True)
        except CommandUnconfirmedError:
            session._raise_if_failed()
            ranges=job.setdefault('unconfirmed_batches',[])
            ranges.append(dict(phase=phase,start=begin,end=end))
            pacer.speed=min(pacer.speed,250)
            job['throttled_rate']=pacer.speed
            private_json(path,job)

    async def processing_barrier():
        if flow is not None and flow.sent>flow.released:
            await flow.drain()
            return
        try:
            await world.send('testfor @s')
        except CommandUnconfirmedError:
            session._raise_if_failed()
            end=job.get('manual_repair_submitted',0) if job.get('manual_repair_in_progress') else job['submitted'];begin=max(0,end-PROCESSING_WINDOW)
            ranges=job.setdefault('unconfirmed_batches',[])
            entry=dict(phase='repair' if job.get('manual_repair_in_progress') else 'initial',start=begin,end=end)
            if not ranges or ranges[-1]!=entry:ranges.append(entry)
            # Store compact ranges rather than thousands of duplicate commands.
            private_json(path,job)
            await asyncio.sleep(0.5)

    async def manual_repair():
        nonlocal current
        total=len(plan.commands)
        offset=max(0,job.get('manual_repair_submitted',0)-PROCESSING_LIMIT) if job.get('manual_repair_in_progress') else 0
        job['manual_repair_in_progress']=True
        if plan.command_blocks:
            # A replayed shell may clear its configuration. Always reapply/read
            # every staged block after rebuilding; never reuse old NBT cursors.
            job['command_blocks_next'] = 0
            job['command_blocks_state'] = 'pending_after_repair'
            private_json(path, job)
        repair_meter=ProgressMeter(total,offset)
        update('manual_repair',detail='修复中：重新发送建筑全部主层命令')
        await game.notice(f'proloader_bot：开始修复，共 {total} 条命令。')
        for index in range(offset,total):
            command=plan.commands[index]
            if max(abs(command[1]-current[0]),abs(command[4]-current[0]),
                   abs(command[3]-current[2]),abs(command[6]-current[2]))>32:
                await processing_barrier()
                current=anchor(command);await move(world,current)
            await flow.before()
            await transport_pressure(session);await pacer.wait()
            world.command_index=index;world.command_phase='manual_repair'
            await world.place(command)
            job['manual_repair_submitted']=index+1
            await flow.after(index,'repair')
            job['repair_telemetry']=repair_meter.read(index+1)
            if (index+1)%PROCESSING_WINDOW==0:
                private_json(path,job)
            update('manual_repair',detail='修复中：重新发送建筑全部主层命令')
        await processing_barrier()
        job['manual_repair_in_progress']=False
        job.setdefault('manual_repairs',[]).append(dict(commands=total,finished_at=datetime.now(timezone.utc).isoformat()))
        job['verification']='not_performed_after_manual_repair'
        job['delivery_state']='sent';job['checked']=job['passed']=0
        private_json(path,job)
        await apply_command_blocks()

    async def apply_command_blocks():
        nonlocal current
        if not plan.command_blocks:return
        if command_block_mode == 'skip':
            job['command_blocks_state'] = 'skipped_shells_only'
            private_json(path, job)
            return
        if job['state'] == 'verification_failed':
            raise ValueError('主层方块仍有差异，命令方块数据尚未写入。')
        nbt_started = time.monotonic()
        if not job.get('nbt_phase_started'):
            job['block_delivery_state'] = job['state']
            job['nbt_phase_started'] = True
        update('command_block_data', detail='写入并读回命令方块内部数据（需要红石、首刻执行关闭）')
        job['command_blocks_state'] = 'staging'
        private_json(path, job)
        for index in range(job.get('command_blocks_next', 0), len(plan.command_blocks)):
            block = plan.command_blocks[index]
            await move(world, block.position)
            current = block.position
            # Confirm the shell name and full source states, including facing.
            if not await world.check_command_block(block):
                raise ValueError('命令方块外壳、朝向或状态未确认，停止内部数据写入。')
            # Read before write on resume: a lost response must not force a
            # blind resend. A failed/missing read is a failure, never a match.
            old = await session.read_block_entity(block.position, plan.dimension)
            if block.differences(old):
                job['command_blocks_inflight'] = index
                private_json(path, job)
                await transport_pressure(session)
                await session.send_packet(block.packet())
                await world.send('testfor @s')
                observed = await session.read_block_entity(block.position, plan.dimension)
                differences = block.differences(observed)
                if not await world.check_command_block(block):differences.append('block_states')
                if differences:
                    job['command_block_mismatch'] = dict(index=index, position=list(block.position), fields=differences)
                    private_json(path, job)
                    raise ValueError('命令方块内部数据读回不一致，断点已保存。')
            job['command_blocks_next'] = index+1
            job.pop('command_blocks_inflight', None)
            job.pop('command_block_mismatch', None)
            update('command_block_data', detail=f'内部数据已读回 {index+1}/{len(plan.command_blocks)}')
            private_json(path, job)
        job['command_blocks_state'] = 'staged_readback_verified'
        job['command_blocks_activation'] = 'requires_redstone_first_tick_off'
        job['command_blocks_seconds'] = round(job.get('command_blocks_seconds',0)+time.monotonic()-nbt_started,3)
        update(job.get('block_delivery_state', 'sent'), detail='命令方块数据暂存并读回通过；源自动执行模式未启用')
        private_json(path, job)

    async def finish_session():
        await apply_command_blocks()
        job['elapsed_seconds'] = round(time.monotonic()-started,2)
        job['delivery_complete']=True
        job.setdefault('delivery_state',job['state'])
        if not stay_online:
            private_json(path,job)
            return job
        while True:
            update('awaiting_action',detail='已发完：游戏聊天输入repair修复，done退出')
            await game.notice(READY)
            action,sender=await game.next_action()
            job['last_control']=dict(action=action,sender=sender)
            if action=='repair':
                await manual_repair()
                continue
            job['user_done']=True
            job['finished_at']=datetime.now(timezone.utc).isoformat()
            job['elapsed_seconds']=round(time.monotonic()-started,2)
            update('finished',detail='用户输入done，操作结束，机器人退出')
            try:await game.notice('proloader_bot：操作已结束，机器人即将退出。')
            except Exception:job.setdefault('ui_anomalies',{})['exit_notice_unconfirmed']=1
            await asyncio.sleep(0.3)
            private_json(path,job)
            return job

    try:
        async with rental_session(store, bot, server_code, password,
                                  on_stage=lambda text: update('connecting', detail=text)) as (session, name):
            world = WorldCommands(session, name, plan)
            await wait_for_import_permission(world, session, update)
            async with GameConsole(world,plan,job,enabled=live_console or stay_online) as game, \
                    ProcessingWindow(record_processing_probe,metrics,interval=PROCESSING_WINDOW) as flow:
                await move(world,plan.origin)
                sx, sy, sz = plan.structure.size
                current = (plan.origin[0]+sx//2, plan.origin[1], plan.origin[2]+sz//2)
                await move(world,current)
                if not plan.commands:
                    raise ValueError('建筑没有可放置的非空气方块。')
                if job.get('delivery_complete') and stay_online:
                    if job.get('manual_repair_in_progress'):await manual_repair()
                    return await finish_session()
                if job.get('nbt_phase_started') and not job.get('manual_repair_in_progress'):
                    # Preserve already staged data. Resume the NBT phase without
                    # replaying the first probe or any shell-placement commands.
                    return await finish_session()
                # The probe is the building's first real command, entirely inside its bounds.
                first = plan.commands[0]
                if max(abs(first[1]-current[0]),abs(first[4]-current[0]),abs(first[3]-current[2]),abs(first[6]-current[2]))>32:
                    current=anchor(first)
                    await move(world,current)
                if start < len(plan.commands):
                    await world.place(first)
                probe_good = start == len(plan.commands) or (verify != 'none' and await world.check(first[1], first[2], first[3], first[7], first[8]))
                if verify != 'none' and not probe_good:
                    record_mismatch('command:0', first)
                    private_json(path, job)
                update('placing', detail='放置方块', probe_verified=probe_good)
                batch_start = start
                for index in range(start, len(plan.commands)):
                    command = plan.commands[index]
                    x, z = (command[1]+command[4])//2, (command[3]+command[6])//2
                    if max(abs(command[1]-current[0]),abs(command[4]-current[0]),
                           abs(command[3]-current[2]),abs(command[6]-current[2])) > 32:
                        if verify == 'none':
                            await processing_barrier()
                        current = anchor(command)
                        await move(world,current)
                    if verify == 'none':await flow.before()
                    tick=time.monotonic()
                    await transport_pressure(session)
                    metrics['backpressure_seconds']+=time.monotonic()-tick
                    await pacer.wait()
                    tick=time.monotonic()
                    world.command_index=index;world.command_phase='initial'
                    await world.place(command)
                    metrics['placement_seconds']+=time.monotonic()-tick
                    if index%32==0:await asyncio.sleep(0)
                    job['submitted'] = index + 1
                    if verify == 'none':await flow.after(index)
                    if (index+1) % 32 == 0 or index+1 == len(plan.commands):
                        # A world check acts as a barrier and detects a failed batch.
                        if verify != 'none' and not await world.check(command[1], command[2], command[3], command[7], command[8]):
                            # The sampled command may represent a lost batch. Keep
                            # every command in that batch for deferred idempotent repair.
                            for suspect in range(batch_start, index+1):
                                record_mismatch(f'command:{suspect}', plan.commands[suspect])
                        batch_start = index+1
                        private_json(path, job)
                    # Keep the server command queue bounded even when block checks
                    # are disabled. This barrier checks command processing, not blocks.
                    update('placing', detail='放置方块')

                if verify == 'none':
                    await processing_barrier()
                    # Let the reliable send queue drain and receive transport ACKs before
                    # closing; a completed enqueue loop alone does not prove transmission.
                    until = time.monotonic() + 15
                    while time.monotonic() < until:
                        session._raise_if_failed()
                        if session._raknet._closed:
                            raise RuntimeError('发送期间连接已断开。')
                        if session._raknet._send_queue.empty() and not session._raknet._pending_acks:
                            break
                        await asyncio.sleep(0.1)
                    else:
                        raise RuntimeError('末尾命令尚未获得传输确认，已保留断点。')
                    job['elapsed_seconds'] = round(time.monotonic()-started, 2)
                    job['metrics']={k:round(v,3) for k,v in metrics.items()}
                    job['speed']=dict(rate=commands_per_second,burst=min(DEFAULT_BURST,commands_per_second))
                    job['verification'] = 'not_performed'
                    update('sent', detail='填充命令已全部发送，未校验完整性')
                    private_json(path, job)
                    return await finish_session()
                await processing_barrier()
                job['placement_finished_seconds']=round(time.monotonic()-started,3)
                verify_started=time.monotonic()
                # Samples cover names, not all states/NBT. All recorded batch
                # commands are repaired too, including unsampled points in that batch.
                import numpy as np
                from proloader.verification import ordered_coords, check_window, REVISION, BATCH, WINDOW
                coords = ordered_coords(plan.structure.region.blocks, plan.structure.palette.air_id)
                selected = coords if verify == 'complete' else coords[np.linspace(0, len(coords)-1, min(64, len(coords)), dtype=int)]

                async def ensure_window(command):
                    nonlocal current
                    if max(abs(command[1]-current[0]), abs(command[4]-current[0]),
                           abs(command[3]-current[2]), abs(command[6]-current[2])) > 32:
                        await processing_barrier()
                        current = anchor(command)
                        await move(world,current)

                async def verify_samples(fresh=False):
                    nonlocal current
                    checkpoint = job.get('verification_checkpoint', {})
                    continuing = (not fresh and checkpoint.get('revision') == REVISION
                                  and checkpoint.get('mode') == verify
                                  and checkpoint.get('total') == len(selected))
                    cursor = checkpoint['cursor'] if continuing else 0
                    if not 0 <= cursor <= len(selected):raise ValueError('核对断点超出范围')
                    if not continuing:job['checked'] = job['passed'] = 0
                    update('verifying', detail='核对实际世界方块')
                    saved_at=time.monotonic()
                    while cursor < len(selected):
                        # Never move while checks are in flight. All concurrent
                        # probes belong to a single 16x16 horizontal region.
                        end = min(cursor+BATCH*WINDOW, len(selected))
                        tile = selected[cursor][[0,2]]//16
                        for i in range(cursor+1,end):
                            if not np.array_equal(selected[i][[0,2]]//16,tile):
                                end=i;break
                        points=[]
                        for local in selected[cursor:end]:
                            lx,ly,lz=map(int,local)
                            x,y,z=(plan.origin[0]+lx,plan.origin[1]+ly,plan.origin[2]+lz)
                            block=plan.structure.palette.get(int(plan.structure.region.blocks[lx,ly,lz]))
                            points.append((x,y,z,block.state.name,block.state.states))
                        x,y,z,name,states=points[0]
                        await ensure_window(('setblock',x,y,z,x,y,z,name,states))
                        if verify == 'complete' and hasattr(world,'relative'):
                            results=await check_window(world,points)
                        else:
                            results=[await world.check(*point) for point in points]
                        for point,good in zip(points,results):
                            x,y,z,name,states=point
                            job['checked']+=1;job['passed']+=int(good)
                            if not good:record_mismatch(f'block:{x},{y},{z}',('setblock',x,y,z,x,y,z,name,states))
                        cursor=end
                        job['verification_checkpoint']=dict(revision=REVISION,mode=verify,total=len(selected),cursor=cursor)
                        if time.monotonic()-saved_at >= 2:
                            private_json(path,job)
                            saved_at=time.monotonic()
                        update('verifying', detail='核对实际世界方块')
                    private_json(path,job)

                await verify_samples()
                # Bounded repair, always after the entire original plan was sent.
                # Pending entries remain durable until positively rechecked, so a
                # disconnect during repair cannot discard the work still needed.
                for repair_round in range(1,3):
                    if not pending:
                        break
                    work = list(pending.items())
                    round_info = dict(round=repair_round, commands=len(work), sent=0)
                    job['repair_rounds'].append(round_info)
                    update('repairing', detail=f'补齐差异 · 第 {repair_round}/2 轮')
                    private_json(path,job)
                    for offset, (key, command) in enumerate(work):
                        await ensure_window(command)
                        await transport_pressure(session)
                        await pacer.wait()
                        world.command_index=offset;world.command_phase='automatic_repair'
                        await world.place(command)
                        round_info['sent'] = offset+1
                        if (offset+1) % PROCESSING_WINDOW == 0:
                            await processing_barrier()
                            private_json(path,job)
                        update('repairing',detail=f'补齐差异 · 第 {repair_round}/2 轮')
                    await processing_barrier()
                    for key, command in work:
                        await ensure_window(command)
                        points = {tuple(command[1:4]), tuple(command[4:7])}
                        good = True
                        for x,y,z in sorted(points):
                            if not await world.check(x,y,z,command[7],command[8]):
                                good = False
                        if good:
                            pending.pop(key, None)
                    round_info['remaining'] = len(pending)
                    private_json(path,job)
                    await verify_samples(fresh=True)
                failures = [list(c[1:4])+[c[7]] for c in pending.values()]
                metrics['verification_seconds']=time.monotonic()-verify_started
                job['metrics']={k:round(v,3) for k,v in metrics.items()}
                job['speed']=dict(rate=commands_per_second,burst=min(DEFAULT_BURST,commands_per_second))
                job['failures'] = failures
                job['elapsed_seconds'] = round(time.monotonic()-started, 2)
                job['verification'] = 'all_block_names' if verify == 'complete' else 'sample_64_block_names'
                update('completed' if not failures else 'verification_failed', detail='导入完成' if not failures else '已完成发送与两轮补齐，仍有差异，详情已保存')
                private_json(path, job)
                return await finish_session()
    except BaseException as exc:
        if job.get('user_done'):
            job.update(state='finished',cleanup_error_type=type(exc).__name__)
            private_json(path,job)
            return job
        job['state'] = 'paused' if isinstance(exc, (KeyboardInterrupt, asyncio.CancelledError)) else 'failed'
        job['error_type'] = type(exc).__name__
        if isinstance(exc, PlacementCheckError):
            job['error_details'] = exc.details
        job['elapsed_seconds']=round(time.monotonic()-started,3)
        job['metrics']={k:round(v,3) for k,v in metrics.items()}
        private_json(path, job)
        raise


async def execute_import(plan,store,bot,server_code,password,*,verify='sample',
                         commands_per_second=DEFAULT_SPEED,progress=None,resume=None,jobs_root=None,
                         live_console=False,stay_online=False,reconnect_attempts=0,command_block_mode='reject'):
    if type(reconnect_attempts) is not int or not 0<=reconnect_attempts<=20:
        raise ValueError('自动重连次数需要0～20之间的整数。')
    if stay_online and not live_console:live_console=True
    root=Path(jobs_root) if jobs_root else Path.home()/'.proloader/imports'
    active=resume or uuid.uuid4().hex
    kwargs=dict(verify=verify,commands_per_second=commands_per_second,progress=progress,
                jobs_root=jobs_root,live_console=live_console,stay_online=stay_online,reserved_id=active,
                command_block_mode=command_block_mode)
    # A new ID is reserved once, then all retries retain that same durable task.
    first_resume=resume
    for attempt in range(reconnect_attempts+1):
        def observe(job):
            nonlocal active
            active=job['id']
            if progress:progress(job)
        try:
            return await _execute_attempt(plan,store,bot,server_code,password,
                **dict(kwargs,progress=observe,resume=first_resume if attempt==0 else active))
        except (BedrockLoginError,OSError,RuntimeError) as exc:
            retryable=isinstance(exc,(BedrockLoginError,OSError)) or str(exc) in ('施工传输已断开','施工传输确认积压超时','末尾命令尚未获得传输确认，已保留断点。','发送期间连接已断开。')
            if not retryable or reconnect_attempts==0:raise
            path=root/f'{active}.json'
            if not path.exists():raise
            job=json.loads(path.read_text())
            job.setdefault('connection_interruptions',[]).append(dict(attempt=attempt+1,error_type=type(exc).__name__,submitted=job['submitted']))
            exhausted=attempt>=reconnect_attempts
            job.update(state='paused' if exhausted else 'reconnecting',
                detail='自动重连暂未成功，已保留全部断点，可继续任务' if exhausted else f'连接中断，保存断点后自动重连 {attempt+1}/{reconnect_attempts}')
            private_json(path,job)
            if progress:progress(dict(job))
            if exhausted:return job
            await asyncio.sleep(min(30,2**attempt))
