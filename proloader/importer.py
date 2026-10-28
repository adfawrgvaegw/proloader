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
from proloader.fatalder import native_plan, native_binary, skipped_names
from proloader.pacing import (DEFAULT_SPEED, DEFAULT_BURST, PROCESSING_LIMIT,
                             CommandPacer, ProcessingWindow, transport_pressure)
from proloader.block_names import command_state, verification_state, cell_states, WIRE_REVISION
from proloader.game_ui import GameConsole, ProgressMeter, READY, phase_progress
from proloader.net.bedrock.session import BedrockSession, BedrockLoginError
from proloader.net.netease.runtime import rental_session
from proloader.command_blocks import plan_command_blocks, operation_hash, REVISION as NBT_REVISION, world_reader_binary
from proloader.block_entities import (plan_block_entities, operation_hash as block_entities_hash,
                                     apply_block_entities, sign_packets)

PROCESSING_WINDOW = 256

DIMENSIONS = {'主世界': 'overworld', '下界': 'nether', '末地': 'the_end',
              'overworld': 'overworld', 'nether': 'nether', 'the_end': 'the_end'}


CHECKED_PROCESSING_LIMIT = 512
APPROVED_REPLACEMENTS = Path(__file__).resolve().parents[1]/'assets'/'approved_block_replacements.json'


class CommandUnconfirmedError(BedrockLoginError):
    pass


def same_plan(saved,current):
    """Informational warning wording must not invalidate semantic checkpoints."""
    return {k:v for k,v in saved.items() if k!='warnings'}=={k:v for k,v in current.items() if k!='warnings'}


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
    block_entities: list = field(init=False, repr=False)
    unsupported_block_entities: list = field(init=False, repr=False)

    def __post_init__(self):
        self.command_blocks = plan_command_blocks(self.structure, self.origin)
        self.block_entities, self.unsupported_block_entities = plan_block_entities(self.structure, self.origin)

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
        if self.structure.source_info.get('block_replacements'):
            result['block_replacements']=self.structure.source_info['block_replacements']
        if self.command_blocks:
            result.update(command_blocks=len(self.command_blocks), nbt_revision=NBT_REVISION,
                          command_blocks_sha256=operation_hash(self.command_blocks))
            result['plan_sha256'] = hashlib.sha256((result['plan_sha256']+result['command_blocks_sha256']).encode()).hexdigest()
            result['warnings'].append('含命令方块：须显式选择暂存内部数据或仅放外壳；Java指令暂不转换。暂存仍可能被外部红石触发。')
        if self.block_entities or self.unsupported_block_entities:
            result['block_entities'] = len(self.block_entities)
            result['block_entities_sha256'] = block_entities_hash(self.block_entities)
            result['unsupported_block_entities'] = len(self.unsupported_block_entities)
            result['unsupported_block_entity_examples'] = self.unsupported_block_entities[:10]
            result['plan_sha256'] = hashlib.sha256((result['plan_sha256']+result['block_entities_sha256']).encode()).hexdigest()
            if self.block_entities:
                result['warnings'].append(f'{len(self.block_entities)}个容器/告示牌附加数据将写入并逐块读回。')
                if any(op.kind=='sign' for op in self.block_entities):
                    result['warnings'].append('告示牌仅写入预检支持的文字/格式字段；编辑会清空假人第1快捷栏。')
            if self.unsupported_block_entities:
                reasons = sorted({v['reason'] for v in self.unsupported_block_entities})
                result['warnings'].append(f'{len(self.unsupported_block_entities)}个附加数据尚不支持写入：'+ '；'.join(reasons))
        return result


def prepare_import(file, origin, dimension='overworld', *, engine='fatalder', block_replacements=None) -> ImportPlan:
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
    source_hash=hashlib.sha256(file.read_bytes()).hexdigest()
    if block_replacements is None:
        profiles=json.loads(APPROVED_REPLACEMENTS.read_text()) if APPROVED_REPLACEMENTS.exists() else {}
        block_replacements=profiles.get(source_hash,())
    if block_replacements:
        from proloader.model.block import Block,BlockState
        import re
        structure.region.blocks=structure.region.blocks.copy()
        structure.source_info.pop('blocks_file',None)
        applied=[];seen=set()
        for replacement in block_replacements:
            local=tuple(replacement['local'])
            if (len(local)!=3 or any(type(v) is not int or not 0<=v<n for v,n in zip(local,structure.size))
                    or local in seen or local in structure.block_entities):
                raise ValueError('材质替换坐标无效、重复或包含方块实体。')
            seen.add(local)
            old=structure.palette.get(int(structure.region.blocks[local])).state
            name=replacement['name'];states=dict(replacement.get('states',{}))
            if (old.name!=replacement['from_name'] or old.name=='minecraft:air'
                    or not re.fullmatch(r'minecraft:[a-z0-9_]+',name)
                    or name in ('minecraft:air','minecraft:command_block','minecraft:chain_command_block','minecraft:repeating_command_block')):
                raise ValueError('材质替换与原图不符或目标名称无效。')
            structure.region.blocks[local]=structure.palette.add(Block(BlockState(name,states)))
            applied.append(dict(local=list(local),from_name=old.name,name=name,states=states))
        structure.source_info['block_replacements']=sorted(applied,key=lambda r:r['local'])
        structure.source_info.setdefault('warnings',[]).append(f'本任务按明确选择替换{len(applied)}格材质；原始文件保持原样。')
    commands = native_plan(structure, tuple(origin), native_binary()) if engine=='fatalder' else BedrockSession._generate_merged_commands(structure, tuple(origin))
    # Place structural blocks first, then attachment/fragile decorations.
    fragile = ('torch', 'ladder', 'button', 'lever', 'banner', 'flower', 'sapling', 'carpet',
               'tripwire', 'coral', 'lantern', 'end_rod', 'bell', 'bed', 'door', 'vine', 'bamboo')
    if engine=='legacy':
        commands.sort(key=lambda c: (any(part in c[7] for part in fragile), c[2], c[1], c[3]))
    return ImportPlan(file, structure, tuple(origin), dimension, commands,
                      source_hash, engine)


class WorldCommands:
    def __init__(self, session, name, plan):
        self.session, self.name, self.plan = session, name, plan

    def assert_local_repair_current(self):
        expected=getattr(self,'_active_local_repair_revision',None)
        if expected is not None and expected!=getattr(self,'_external_teleport_revision',0):
            raise ValueError('收到新的传送，旧区块停止补写，切换到最新位置。')

    async def runtime_hashes(self):
        if not hasattr(self,'_runtime_hashes'):
            from proloader.command_blocks import world_reader_binary
            palette=[];keys=[];spans=[]
            entries=[(b.state.name,b.state.states) for b in self.plan.structure.palette]
            import numpy as np
            for i,b in enumerate(self.plan.structure.palette):
                if b.state.name.endswith('_door'):
                    for pos in np.argwhere(self.plan.structure.region.blocks==i):
                        entries.append((b.state.name,cell_states(self.plan.structure,tuple(map(int,pos)))))
            unique={json.dumps([n,s],sort_keys=True):(n,s) for n,s in entries}
            for name,states in unique.values():
                begin=len(palette)
                keys.append(json.dumps([name,states],sort_keys=True))
                wire,props=command_state(name,states)
                probe,probe_states=command_state(name,states,probe=True)
                alternate=dict(name=wire,states=props)
                if name in ('minecraft:stone_block_slab4','minecraft:double_stone_block_slab4') and states.get('stone_slab_type_4')=='stone':
                    alternate=dict(name='minecraft:normal_stone_'+('double_slab' if name.startswith('minecraft:double_') else 'slab'),
                                   states={k:v for k,v in states.items() if k!='stone_slab_type_4'})
                palette.extend([dict(name=name,states=states),dict(name=wire,states=props),dict(name=probe,states=probe_states),alternate])
                wall,stable=verification_state(name,states)
                if wall.endswith('_wall'):
                    import itertools
                    for joins in itertools.product(('none','short','tall'),repeat=4):
                        for post in (False,True):
                            actual=dict(stable,**dict(zip(('wall_connection_type_east','wall_connection_type_north',
                                'wall_connection_type_south','wall_connection_type_west'),joins)),wall_post_bit=post)
                            palette.append(dict(name=wall,states=actual))
                if wall in ('minecraft:daylight_detector','minecraft:daylight_detector_inverted'):
                    palette.extend(dict(name=wall,states=dict(stable,redstone_signal=n)) for n in range(16))
                spans.append((begin,len(palette)))
            def run():
                from proloader.world_codec import request
                return request(world_reader_binary(),dict(action='hash_states',palette=palette))['hashes']
            hashes=await asyncio.to_thread(run)
            if len(hashes)!=len(palette):raise ValueError('方块状态哈希数量不一致。')
            self._runtime_hashes={k:set(hashes[a:b]) for k,(a,b) in zip(keys,spans)}
        return self._runtime_hashes

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
        origin = getattr(self, '_command_origin', self.plan.origin)
        return ' '.join('~' if d == 0 else f'~{d}' for d in (x-origin[0], y-origin[1], z-origin[2]))

    def context(self, command):
        x, y, z = getattr(self, '_command_origin', self.plan.origin)
        return f'execute in {self.plan.dimension} positioned {x} {y} {z} run {command}'

    async def teleport(self, x, y, z):
        self.assert_local_repair_current()
        if self.session is None:return await self._teleport_confirmed(x,y,z)
        previous=getattr(self.session,'_import_teleport_target',None)
        self.session._import_teleport_target=(x,y,z)
        try:
            return await self._teleport_confirmed(x,y,z)
        finally:
            self.session._import_teleport_target=previous

    async def _teleport_confirmed(self, x, y, z):
        self.__dict__.pop('_entity_snapshot',None)
        try:
            # TP acknowledgements are unreliable on NetEase. Submit once, then
            # prove position instead of spending eight seconds waiting for an
            # acknowledgement that cannot certify the destination anyway.
            await self.send(f'execute in {self.plan.dimension} run tp @s {x} {y} {z}',wait=False)
        except BedrockLoginError as exc:
            self.session._raise_if_failed()
            if not str(exc).startswith('aiCommand 执行失败'):
                raise
        # Confirm location instead of allowing a failed teleport to shift an import.
        deadline=time.monotonic()+8
        while True:
            self.assert_local_repair_current()
            try:
                await self.send(f'execute in {self.plan.dimension} run testfor @s[x={x},y={y},z={z},r=3]')
                break
            except BedrockLoginError as exc:
                self.session._raise_if_failed()
                if not str(exc).startswith('aiCommand 执行失败') or time.monotonic()>=deadline:
                    raise
                await asyncio.sleep(0.25)
        await asyncio.sleep(0.25)
        self.assert_local_repair_current()
        # Keep predicates and placement relative to the confirmed loaded window.
        # A distant fixed building origin can make NetEase reject valid targets.
        self._command_origin = (x, y, z)
        self._last_import_position=(x,y,z)
        self._verification_chunk = None

    async def check(self, x, y, z, name, states=None):
        if (name in ('minecraft:stone_block_slab4','minecraft:double_stone_block_slab4')
                and (states or {}).get('stone_slab_type_4')=='stone' and hasattr(self.session,'read_subchunk')
                and not getattr(self,'_stationary_placement',False)):
            try:
                entry=await self.session.read_subchunk((x,y,z),self.plan.dimension)
                ids=entry.get('blocks',[])
                hashes=(await self.runtime_hashes()).get(json.dumps([name,states],sort_keys=True),set())
                if len(ids)==4096 and ids[((x%16)*16+y%16)*16+z%16] in hashes:return True
                if entry.get('result')==6:return False
            except (ValueError,asyncio.TimeoutError):self.session._raise_if_failed()
        name, props = verification_state(name,states)
        tile=(BedrockSession._states_to_str(props) if self.plan.structure.source_info.get('exact_states') or
              (name in ('minecraft:stone_block_slab4','minecraft:double_stone_block_slab4')
              and props.get('stone_slab_type_4')=='stone') else '[]')
        try:
            result = await self.send(self.context(f'testforblock {self.relative(x,y,z)} {name} {tile}'))
            return result is not None
        except BedrockLoginError as exc:
            # Only a known command-negative response means mismatch. Session/timeout
            # failures must stop the job, never masquerade as an empty world block.
            if str(exc).startswith('aiCommand 执行失败'):
                return False
            raise

    async def check_block_entity_shell(self, operation):
        """Prove exact source states independently of command-state aliases."""
        x,y,z=operation.position
        entry=await self.session.read_subchunk(operation.position,self.plan.dimension)
        self._entity_snapshot=(tuple(operation.position),entry)
        ids=entry.get('blocks',[])
        hashes=(await self.runtime_hashes()).get(
            json.dumps([operation.block_name,operation.states],sort_keys=True),set())
        if len(ids)==4096 and hashes:
            rid=ids[((x%16)*16+y%16)*16+z%16]
            if rid in hashes:return True
            observed=entry.get('palette',{}).get(str(rid))
            if observed:
                expected,states=command_state(operation.block_name,operation.states,probe=True)
                actual,props=command_state(observed['name'],observed.get('states'),probe=True)
                return expected==actual and all(props.get(k)==v for k,v in states.items())
            return False
        if entry.get('result')==6:return False
        name,states=command_state(operation.block_name,operation.states,probe=True)
        try:
            result=await self.send(self.context(f'testforblock {self.relative(x,y,z)} {name} {BedrockSession._states_to_str(states)}'))
            return result is not None
        except BedrockLoginError as exc:
            self.session._raise_if_failed()
            if str(exc).startswith('aiCommand 执行失败'):return False
            raise

    def consume_entity_snapshot(self, position):
        """Consume actor data from the exact fresh read used for shell checks."""
        snapshot=self.__dict__.pop('_entity_snapshot',None)
        if snapshot is None or snapshot[0]!=tuple(position):return None
        matches=[n for n in snapshot[1].get('nbt',[])
                 if tuple(n.get(k) for k in ('x','y','z'))==tuple(position)]
        if len(matches)!=1 or '_decode_error' in matches[0]:
            raise ValueError('目标方块NBT缺失、重复或解码失败，未确认内部数据。')
        return matches[0]

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

    async def ensure_command_block_access(self):
        try:
            await self.send('testfor @s[m=1]')
            return
        except CommandUnconfirmedError:self.session._raise_if_failed()
        try:await self.send('gamemode creative')
        except CommandUnconfirmedError:self.session._raise_if_failed()
        for attempt in range(3):
            try:
                await self.send('testfor @s[m=1]')
                return
            except CommandUnconfirmedError:
                self.session._raise_if_failed()
                if attempt==2:raise
                await asyncio.sleep(.15*(attempt+1))

    async def place(self, command, wait=False):
        self.assert_local_repair_current()
        self.__dict__.pop('_entity_snapshot',None)
        if getattr(self,'chunk_window',None) is not None:await self.chunk_window.ensure(command)
        if hasattr(self,'_scan_subchunks'):self._scan_subchunks.clear()
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
            world_reader_binary()
    if plan.block_entities:
        world_reader_binary()
        for operation in plan.block_entities:
            if operation.kind == 'sign':
                for action in ('sign_click','sign_text','auth'):
                    sign_packets(operation,action,(operation.position[0]+1,operation.position[1]+1.62,operation.position[2]))
    root = Path(jobs_root) if jobs_root else Path.home() / '.proloader' / 'imports'
    job_id = resume or reserved_id or uuid.uuid4().hex
    if len(job_id) != 32 or any(c not in '0123456789abcdef' for c in job_id):
        raise ValueError('无效任务编号。')
    path = root / f'{job_id}.json'
    if resume:
        job = json.loads(path.read_text())
        current_plan=plan.summary()
        previous_plan=job['plan']
        if (previous_plan.get('wire_revision')=='legacy-names-v3'
                and current_plan.get('wire_revision')=='legacy-names-v4-nemc-stone'
                and same_plan(dict(previous_plan,wire_revision=current_plan['wire_revision']),current_plan)):
            job.setdefault('compatibility_migrations',[]).append(dict(kind='stone_slab_wire_fix',from_revision='legacy-names-v3'))
            job['plan']=current_plan
            job.pop('verification_checkpoint',None)
        if (not same_plan(job['plan'],current_plan)
                and job['plan'].get('sha256')==current_plan.get('sha256')
                and job['plan'].get('origin')==current_plan.get('origin')
                and job['plan'].get('plan_sha256')!=current_plan.get('plan_sha256')):
            raise ValueError('施工计划已更新，不能沿用旧命令下标续传。原任务保留，请重新创建导入任务；已有方块仍在世界中。')
        if not same_plan(job['plan'],current_plan) or job['server'] != server_code or job['bot_id'] != bot.id:
            raise ValueError('文件、坐标、服务器或机器人与中断任务不一致。')
        job['plan']=current_plan
        if plan.command_blocks and job.get('command_block_mode') != command_block_mode:
            raise ValueError('命令方块处理方式与中断任务不一致。')
        replay = job.get('processing_window', 1024) if job.get('verify_mode') == 'none' else job.get('checked_processing_window',32)
        start = max(0, job['submitted'] - max(32, replay))
        if (job['submitted'] == len(plan.commands) and verify != 'none'
                and 'placement_finished_seconds' in job):
            # Verification-only recovery must not replay placement commands.
            start = len(plan.commands)
        if job.get('manual_repair_in_progress') and job.get('manual_repair_chunk') is not None:
            start=len(plan.commands)  # Never replay shells for an interrupted local action.
    else:
        job = dict(id=job_id, plan=plan.summary(), server=server_code, bot_id=bot.id,
                   state='preparing', submitted=0, checked=0, passed=0,
                   created_at=datetime.now(timezone.utc).isoformat())
        start = 0
    job.pop('error_type', None)
    job.pop('error_details', None)
    job.pop('error_stage', None)
    job['processing_window']=PROCESSING_LIMIT
    job['processing_probe_interval']=PROCESSING_WINDOW
    if verify != 'none':
        job['checked_processing_window']=CHECKED_PROCESSING_LIMIT
        job['processing_probe_interval']=32
    job['verify_mode']=verify
    job['speed']=dict(rate=commands_per_second,burst=min(DEFAULT_BURST,commands_per_second))
    if plan.command_blocks:job['command_block_mode'] = command_block_mode
    private_json(path, job)

    meter=ProgressMeter(len(plan.commands),job['submitted'])

    phase_meter = None
    phase_key = None

    def update(stage, **values):
        nonlocal phase_meter, phase_key
        previous=job['state']
        job.update(values)
        job['state'] = stage
        if stage != 'loading':job['active_phase'] = stage
        phase=phase_progress(plan,job)
        token=(phase['key'],phase['total'])
        if token!=phase_key:
            phase_key=token;phase_meter=ProgressMeter(phase['total'],phase['done'])
        job['phase_telemetry']=phase_meter.read(phase['done'])
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
        if verify != 'none' and phase=='initial':
            # Keep every 32-command sample, but overlap its response with the
            # next bounded batches. Drain before any move or loading-area change.
            command=plan.commands[end-1]
            if not await world.check(command[1],command[2],command[3],command[7],command[8]):
                for suspect in range(begin,end):
                    record_mismatch(f'command:{suspect}',plan.commands[suspect])
                private_json(path,job)
            return
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

    async def manual_repair(position=None):
        nonlocal current
        if hasattr(session,'read_subchunk'):
            from proloader.differential_repair import repair_building
            import math
            if position is None:position=getattr(session,'_last_server_position',None)
            if position is None or len(position)!=3 or not all(math.isfinite(v) for v in position):
                raise ValueError('请先把机器人传送到需要修复的位置，再输入repair。')
            chunk=(math.floor(position[0])//16,math.floor(position[2])//16)
            revision=getattr(world,'_external_teleport_revision',0)
            # Radius selector is evaluated in the source dimension. A stale
            # teleport or a player in another dimension cannot authorize work.
            await world.send(f'execute in {plan.dimension} positioned '+
                ' '.join(str(float(v)) for v in position)+f' run testfor @a[name={json.dumps(name,ensure_ascii=False)},r=3]')
            if getattr(world,'_external_teleport_revision',0)!=revision:
                raise ValueError('收到新的传送，切换到最新修复位置。')
            world._active_local_repair_revision=revision
            job['manual_repair_chunk']=list(chunk)
            job['local_repair_active']=True
            job['local_repair_status']='running';job['local_repair_progress']={}
            job['manual_repair_in_progress']=True;job['delivery_complete']=False
            private_json(path,job)
            try:
                good=await repair_building(plan,session,world,job,lambda:private_json(path,job),move,update,pacer,chunk=chunk)
                if good:
                    # Only actors in this column are reread/repaired. Global
                    # delivery cursors must never be reset by a local action.
                    inside=lambda op:op.position[0]//16==chunk[0] and op.position[2]//16==chunk[1]
                    cb=[op for op in plan.command_blocks if inside(op)]
                    other=[op for op in plan.block_entities if inside(op)]
                    local=job.setdefault('local_repair_data',{})
                    local.clear();local.update(id=job['id'],plan=dict(command_blocks=len(cb),block_entities=len(other)))
                    def local_update(stage,**values):
                        job['local_repair_progress']=dict(stage=stage,command_blocks_next=local.get('command_blocks_next',0),
                            command_blocks_total=len(cb),block_entities_next=local.get('block_entities_next',0),block_entities_total=len(other))
                        update(stage,**values)
                    if cb and command_block_mode=='stage':
                        from proloader.command_block_delivery import stage_command_blocks
                        await stage_command_blocks(cb,session,world,plan.dimension,local,lambda:private_json(path,job),move,local_update)
                    if other:
                        await apply_block_entities(other,session,world,plan.dimension,local,lambda:private_json(path,job),move,local_update)
            except Exception:
                job['local_repair_status']='pending'
                private_json(path,job)
                raise
            finally:
                job['local_repair_active']=False
                world.__dict__.pop('_active_local_repair_revision',None)
            if not good:
                job['manual_repair_in_progress']=False
                job['local_repair_status']='pending'
                update('verification_failed',detail='差异补全已尝试3轮，仍有差异；输入repair继续修复')
                private_json(path,job)
                return
            job['manual_repair_in_progress']=False
            job['delivery_complete']=not bool(pending)
            job['local_repair_status']='verified'
            job.pop('recovery_error',None)
            update('awaiting_action',detail=('当前区块没有本建筑内容，等待下一次传送' if job.get('local_repair_empty') else
                f'区块 {chunk[0]}, {chunk[1]} 修复完成；传送机器人到下一处继续'))
            private_json(path,job)
            return
        total=len(plan.commands)
        offset=max(0,job.get('manual_repair_submitted',0)-PROCESSING_LIMIT) if job.get('manual_repair_in_progress') else 0
        job['manual_repair_in_progress']=True
        if plan.command_blocks:
            # A replayed shell may clear its configuration. Always reapply/read
            # every staged block after rebuilding; never reuse old NBT cursors.
            job['command_blocks_next'] = 0
            job['command_blocks_state'] = 'pending_after_repair'
            private_json(path, job)
        if plan.block_entities:
            job['block_entities_next'] = 0
            job['block_entities_state'] = 'pending_after_repair'
            private_json(path,job)
        repair_meter=ProgressMeter(total,offset)
        update('manual_repair',detail='修复中：重新发送建筑全部主层命令')
        await game.notice(f'proloader_bot：开始修复，共 {total} 条命令。')
        for index in range(offset,total):
            command=plan.commands[index]
            actual=getattr(world,'_command_origin',current)
            if not getattr(getattr(world,'chunk_window',None),'stationary',False) and max(abs(command[1]-actual[0]),abs(command[4]-actual[0]),
                   abs(command[3]-actual[2]),abs(command[6]-actual[2]))>32:
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
        await apply_other_block_entities()

    async def apply_command_blocks():
        nonlocal current
        if not plan.command_blocks:return
        if command_block_mode == 'skip':
            job['command_blocks_state'] = 'skipped_shells_only'
            private_json(path, job)
            return
        if not job.get('nbt_phase_started'):
            job['block_delivery_state'] = job['state']
            job['nbt_phase_started'] = True
        update('command_block_data', detail='写入并读回命令方块内部数据（需要红石、首刻执行关闭）')
        job['command_blocks_state'] = 'staging'
        private_json(path, job)
        if hasattr(world,'ensure_command_block_access'):
            await world.ensure_command_block_access()
        from proloader.command_block_delivery import stage_command_blocks
        await stage_command_blocks(plan.command_blocks,session,world,plan.dimension,job,
            lambda: private_json(path,job),move,update)
        job['command_blocks_state'] = 'staged_readback_verified'
        job['command_blocks_activation'] = 'requires_redstone_first_tick_off'
        update(job.get('block_delivery_state', 'sent'), detail='命令方块数据暂存并读回通过；源自动执行模式未启用')
        private_json(path, job)

    async def apply_other_block_entities():
        if not plan.block_entities:return
        if not job.get('nbt_phase_started'):
            job['block_delivery_state'] = job['state']
            job['nbt_phase_started'] = True
        await apply_block_entities(plan.block_entities,session,world,plan.dimension,job,
            lambda: private_json(path,job),move,update)
        update(job.get('block_delivery_state','sent'),detail='支持的方块附加数据已写入并读回')
        private_json(path,job)

    async def finish_session():
        while True:
            data_error=None
            try:
                await apply_command_blocks()
                await apply_other_block_entities()
                if job.get('local_repair_status') not in ('pending','interrupted'):job.pop('recovery_error',None)
            except (ValueError,asyncio.TimeoutError,CommandUnconfirmedError) as exc:
                session._raise_if_failed()
                if not stay_online:raise
                data_error=f'{type(exc).__name__}：{str(exc)[:160]}'
                job['recovery_error']=data_error
            job['elapsed_seconds'] = round(time.monotonic()-started,2)
            unsupported=len(plan.unsupported_block_entities)
            job['delivery_complete']=not bool(pending) and not data_error and not unsupported and job.get('local_repair_status') not in ('pending','interrupted','running')
            if (plan.block_entities or plan.unsupported_block_entities) and not data_error:
                job['unsupported_block_entities'] = len(plan.unsupported_block_entities)
                job['block_entity_delivery'] = ('supported_subset_verified' if plan.unsupported_block_entities
                                                else 'all_planned_verified')
            job.setdefault('delivery_state',job['state'])
            if not stay_online:
                private_json(path,job)
                return job
            detail=(f'局部恢复暂未通过：{data_error}；连接保留，输入repair继续，done退出' if data_error else
                    ('区块修复尚未完成；重新传送机器人或输入repair重试，done结束' if job.get('local_repair_status') in ('pending','interrupted') else
                    (f'仍有{len(pending)}处差异：传送机器人到目标区块补齐，done退出' if pending else
                     (f'支持部分已读回；{unsupported}项复杂NBT未支持，详见预检清单；repair补全，done退出' if unsupported else
                      READY))))
            update('awaiting_action',detail=detail)
            private_json(path,job)
            await game.notice(detail if data_error or unsupported else READY)
            action,sender=await game.next_action()
            job['last_control']=dict(action=action,sender=sender)
            if action in ('repair','repair_here'):
                try:await manual_repair(sender if action=='repair_here' else None)
                except (ValueError,asyncio.TimeoutError,CommandUnconfirmedError) as exc:
                    session._raise_if_failed()
                    job['local_repair_status']='pending'
                    job['recovery_error']=f'{type(exc).__name__}：{str(exc)[:160]}'
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
            if any(op.kind=='sign' for op in plan.block_entities) and getattr(session,'_netease_protocol',None)!=860:
                raise ValueError('告示牌写入只支持当前860协议，已在施工前停止。')
            world = WorldCommands(session, name, plan)
            world.nbt_pacer=pacer
            await wait_for_import_permission(world, session, update)
            from contextlib import AsyncExitStack
            from proloader.chunk_loading import StationaryWindow
            async with GameConsole(world,plan,job,enabled=live_console or stay_online) as game, \
                    ProcessingWindow(record_processing_probe,metrics,
                        interval=PROCESSING_WINDOW if verify=='none' else 32,
                        limit=PROCESSING_LIMIT if verify=='none' else CHECKED_PROCESSING_LIMIT) as flow, AsyncExitStack() as loaders:
                if hasattr(session,'read_chunk_window'):
                    world.chunk_window=await loaders.enter_async_context(StationaryWindow(world,plan,job,
                        lambda:private_json(path,job),processing_barrier,update))
                if getattr(world,'chunk_window',None) is None:
                    await move(world,plan.origin)
                sx, sy, sz = plan.structure.size
                current = (plan.origin[0]+sx//2, plan.origin[1], plan.origin[2]+sz//2)
                # The loader owns positioning for grouped construction. Moving
                # here and again in ensure() doubles every window transition.
                if getattr(world,'chunk_window',None) is None:
                    await move(world,current)
                if not plan.commands:
                    raise ValueError('建筑没有可放置的非空气方块。')
                if job.get('manual_repair_in_progress') and job.get('manual_repair_chunk') is not None:
                    job['manual_repair_in_progress']=False;job['local_repair_active']=False
                    job['local_repair_status']='interrupted'
                    update('awaiting_action',detail='上次区块修复未完成，请重新传送机器人到目标处继续')
                    private_json(path,job)
                    return await finish_session()
                if job.get('delivery_complete') and stay_online:
                    if job.get('manual_repair_in_progress'):await manual_repair()
                    return await finish_session()
                if job.get('nbt_phase_started') and not job.get('manual_repair_in_progress'):
                    # Preserve already staged data. Resume the NBT phase without
                    # replaying the first probe or any shell-placement commands.
                    return await finish_session()
                # The probe is the building's first real command, entirely inside its bounds.
                first = plan.commands[0]
                world._stationary_placement=getattr(getattr(world,'chunk_window',None),'stationary',False)
                if getattr(world,'chunk_window',None) is None and max(abs(first[1]-current[0]),abs(first[4]-current[0]),abs(first[3]-current[2]),abs(first[6]-current[2]))>32:
                    current=anchor(first)
                    await move(world,current)
                if start < len(plan.commands):
                    await world.place(first)
                probe_good = start == len(plan.commands) or (verify != 'none' and await world.check(first[1], first[2], first[3], first[7], first[8]))
                if verify != 'none' and not probe_good:
                    record_mismatch('command:0', first)
                    private_json(path, job)
                update('placing', detail='放置方块', probe_verified=probe_good)
                for index in range(start, len(plan.commands)):
                    command = plan.commands[index]
                    x, z = (command[1]+command[4])//2, (command[3]+command[6])//2
                    if getattr(world,'chunk_window',None) is None and max(abs(command[1]-current[0]),abs(command[4]-current[0]),
                           abs(command[3]-current[2]),abs(command[6]-current[2])) > 32:
                        await processing_barrier()
                        current = anchor(command)
                        await move(world,current)
                    await flow.before()
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
                    await flow.after(index)
                    if (index+1) % 32 == 0 or index+1 == len(plan.commands):
                        private_json(path, job)
                    # Keep the server command queue bounded even when block checks
                    # are disabled. This barrier checks command processing, not blocks.
                    update('placing', detail='放置方块')

                if verify == 'none':
                    await processing_barrier()
                    world._stationary_placement=False
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
                world._stationary_placement=False
                job['placement_finished_seconds']=round(time.monotonic()-started,3)
                verify_started=time.monotonic()
                # Samples cover names, not all states/NBT. All recorded batch
                # commands are repaired too, including unsampled points in that batch.
                import numpy as np
                from proloader.verification import ordered_coords, check_window, check_subchunk, REVISION, BATCH, WINDOW
                excluded=[i for i,b in enumerate(plan.structure.palette) if b.state.name in skipped_names(plan.structure)]
                coords = ordered_coords(plan.structure.region.blocks, plan.structure.palette.air_id,plan.origin,excluded)
                for key,c in list(pending.items()):
                    if c[7] in skipped_names(plan.structure):pending.pop(key)
                selected = coords if verify == 'complete' else coords[np.linspace(0, len(coords)-1, min(64, len(coords)), dtype=int)]

                async def ensure_window(command):
                    nonlocal current
                    if getattr(getattr(world,'chunk_window',None),'stationary',False):
                        await world.chunk_window.ensure(command)
                        return
                    actual=getattr(world,'_command_origin',current)
                    if max(abs(command[1]-actual[0]), abs(command[4]-actual[0]),
                           abs(command[3]-actual[2]), abs(command[6]-actual[2])) > 32:
                        await processing_barrier()
                        current = anchor(command)
                        await move(world,current)

                async def verify_samples(fresh=False, only_cubes=None):
                    nonlocal current
                    world._scan_subchunks={}
                    checkpoint = job.get('verification_checkpoint', {})
                    continuing = (not fresh and checkpoint.get('revision') == REVISION
                                  and checkpoint.get('mode') == verify
                                  and checkpoint.get('total') == len(selected))
                    cursor = checkpoint['cursor'] if continuing else 0
                    if not 0 <= cursor <= len(selected):raise ValueError('核对断点超出范围')
                    if not continuing:job['checked'] = job['passed'] = 0
                    job['verification_total']=len(selected)
                    if only_cubes is not None:
                        job['repair_checked']=0
                        job['repair_check_total']=sum(tuple(v) in only_cubes for v in ((selected+np.asarray(plan.origin))//16))
                    stage='repair_verifying' if only_cubes is not None else 'verifying'
                    update(stage, detail='复核已补全区块' if only_cubes is not None else '核对实际世界方块')
                    saved_at=time.monotonic()
                    while cursor < len(selected):
                        if only_cubes is not None and tuple((selected[cursor]+np.asarray(plan.origin))//16) not in only_cubes:
                            cursor+=1
                            continue
                        # Never move while checks are in flight. All concurrent
                        # probes belong to a single 16x16 horizontal region.
                        bulk = verify == 'complete' and hasattr(session,'read_subchunk')
                        end = min(cursor+(4096 if bulk else BATCH*WINDOW), len(selected))
                        tile = (selected[cursor]+np.asarray(plan.origin))//16
                        for i in range(cursor+1,end):
                            if not np.array_equal((selected[i]+np.asarray(plan.origin))//16,tile):
                                end=i;break
                        points=[]
                        for local in selected[cursor:end]:
                            lx,ly,lz=map(int,local)
                            x,y,z=(plan.origin[0]+lx,plan.origin[1]+ly,plan.origin[2]+lz)
                            block=plan.structure.palette.get(int(plan.structure.region.blocks[lx,ly,lz]))
                            points.append((x,y,z,block.state.name,cell_states(plan.structure,(lx,ly,lz))))
                        x,y,z,name,states=points[0]
                        await ensure_window(('setblock',x,y,z,x,y,z,name,states))
                        if bulk:
                            results=await check_subchunk(world,points)
                        elif verify == 'complete' and hasattr(world,'relative'):
                            results=await check_window(world,points)
                        else:
                            results=[await world.check(*point) for point in points]
                        for point,good in zip(points,results):
                            x,y,z,name,states=point
                            job['checked']+=1;job['passed']+=int(good)
                            if not good:record_mismatch(f'block:{x},{y},{z}',('setblock',x,y,z,x,y,z,name,states))
                            else:pending.pop(f'block:{x},{y},{z}',None)
                        cursor=end
                        job['verification_checkpoint']=dict(revision=REVISION,mode=verify,total=len(selected),cursor=cursor)
                        if time.monotonic()-saved_at >= 2:
                            private_json(path,job)
                            saved_at=time.monotonic()
                        if only_cubes is not None:job['repair_checked']=job['checked']
                        update(stage, detail='复核已补全区块' if only_cubes is not None else '核对实际世界方块')
                    private_json(path,job)

                recovery_cubes={tuple(c) for c in job.get('repair_recheck_cubes',[])}
                await verify_samples(fresh=bool(recovery_cubes),only_cubes=recovery_cubes or None)
                if recovery_cubes:
                    job['checked']=len(selected)
                    job['passed']=len(selected)-sum(k.startswith('block:') for k in pending)
                    job.pop('repair_recheck_cubes',None)
                    private_json(path,job)
                # Bounded repair, always after the entire original plan was sent.
                # Pending entries remain durable until positively rechecked, so a
                # disconnect during repair cannot discard the work still needed.
                for repair_round in range(1,3):
                    if not pending:
                        break
                    work = list(pending.items())
                    optimized=hasattr(session,'read_subchunks')
                    if optimized:
                        from proloader.differential_repair import coalesce_repairs
                        writes=coalesce_repairs(plan,work)
                    else:writes=[c for _,c in work]
                    affected=set()
                    if optimized:
                        for c in writes:
                            for cx in range(c[1]//16,c[4]//16+1):
                                for cy in range(c[2]//16,c[5]//16+1):
                                    for cz in range(c[3]//16,c[6]//16+1):affected.add((cx,cy,cz))
                        job['repair_recheck_cubes']=[list(c) for c in sorted(affected)]
                    round_info = dict(round=repair_round, commands=len(writes), differences=len(work), sent=0)
                    job['repair_rounds'].append(round_info)
                    update('repairing', detail=f'补齐差异 · 第 {repair_round}/2 轮')
                    private_json(path,job)
                    for offset, command in enumerate(writes):
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
                    if optimized:
                        world._scan_subchunks={}
                        groups={}
                        for key,c in work:
                            for pos in {tuple(c[1:4]),tuple(c[4:7])}:
                                groups.setdefault(tuple(v//16 for v in pos),[]).append((key,(*pos,c[7],c[8])))
                        from collections import Counter
                        remaining_checks=Counter(key for group in groups.values() for key,_ in group)
                        bad=set()
                        job['repair_checked']=0;job['repair_check_total']=sum(len(g) for g in groups.values())
                        update('repair_verifying',detail='正在读回补全结果，随后检查受影响区块')
                        for group in groups.values():
                            await ensure_window(('setblock',*group[0][1][:3],*group[0][1][:3],group[0][1][3],group[0][1][4]))
                            values=await check_subchunk(world,[p for _,p in group])
                            for (key,_),good in zip(group,values):
                                if not good:bad.add(key)
                                remaining_checks[key]-=1
                                if remaining_checks[key]==0 and key not in bad:pending.pop(key,None)
                            job['repair_checked']+=len(group)
                            update('repair_verifying',detail='正在读回补全结果，随后检查受影响区块')
                        for key,_ in work:
                            if key not in bad:pending.pop(key,None)
                    else:
                        for key, command in work:
                            await ensure_window(command)
                            points = {tuple(command[1:4]), tuple(command[4:7])}
                            good = True
                            for x,y,z in sorted(points):
                                if not await world.check(x,y,z,command[7],command[8]):good = False
                            if good:pending.pop(key,None)
                    round_info['remaining'] = len(pending)
                    private_json(path,job)
                    if optimized:
                        await verify_samples(fresh=True,only_cubes=affected)
                        job.pop('repair_recheck_cubes',None)
                        job['checked']=len(selected)
                        job['passed']=len(selected)-sum(k.startswith('block:') for k in pending)
                    else:await verify_samples(fresh=True)
                    round_info['remaining']=len(pending)
                    private_json(path,job)
                failures = [list(c[1:4])+[c[7]] for c in pending.values()]
                if verify=='complete':
                    job['checked']=len(selected)
                    job['passed']=len(selected)-len({tuple(c[1:4]) for k,c in pending.items() if k.startswith('block:')})
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
        job['error_stage'] = job.get('state','unknown')
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
