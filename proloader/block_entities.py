"""Plan non-command Bedrock block data and require fresh server readback.

Container commands follow FunShuttler's ReplaceitemInContainerAsync. Sign
editing follows nbt_block/sign.go (empty hand, click, two auth inputs, actor
text update). Unsupported content is inventoried, never flattened to plain items.
"""
import asyncio
import time
from dataclasses import dataclass, asdict
from functools import lru_cache
import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess
from proloader.native_build import go_build_environment

from proloader.errors import ParseError
from proloader.net.bedrock.packets import wrap_packet

REVISION = 'other-block-data-v2-generated-item-block'
ROOT = Path(__file__).resolve().parents[1]
CONTAINERS = dict(chest=27, trapped_chest=27, barrel=27, dispenser=9, dropper=9,
    hopper=5, furnace=3, lit_furnace=3, blast_furnace=3, lit_blast_furnace=3,
    smoker=3, lit_smoker=3)
IDENTIFIER = re.compile(r'^(?:minecraft:)?[a-z0-9_]+$')
COMMAND_NAMES = {'command_block','chain_command_block','repeating_command_block'}
GENERATED_NONBLOCK_ITEMS={'minecraft:phantom_membrane','minecraft:diamond',
                         'minecraft:lodestone_compass','minecraft:banner'}
INERT = {'id','x','y','z','isMovable','Findable','IsOpened'}
ZERO_CONTAINER_FIELDS = {'IsIgnoreShuffle','MoveItemSpeed','TransferCooldown','BurnDuration',
                         'BurnTime','CookTime','StoredXPInt'}


class UnsupportedNBT(ValueError):
    pass


def _int(value, low, high, field):
    if type(value) is not int or not low <= value <= high:
        raise ParseError(f'NBT {field} 数值无效。')
    return value


def plain_items(raw, slots, *, observed=False):
    if not isinstance(raw, list):raise ParseError('NBT Items 必须为列表。')
    result = {}
    for item in raw:
        if not isinstance(item, dict):raise ParseError('NBT 物品格式无效。')
        if observed and (item.get('Count',0) == 0 or item.get('Name') in ('air','minecraft:air')):continue
        slot = _int(item.get('Slot'), 0, slots-1, 'Slot')
        if slot in result:raise ParseError('NBT 容器槽位重复。')
        name = item.get('Name')
        if not isinstance(name,str) or not IDENTIFIER.fullmatch(name):
            raise UnsupportedNBT('物品名称不是可直接使用的原版标识符')
        if ':' not in name:name='minecraft:'+name
        if name == 'minecraft:air':raise ParseError('NBT 空物品不能带非零数量。')
        # Rental replaceitem adds the ordinary-banner discriminator itself.
        # Only this exact readback default is equivalent to an absent source tag.
        if observed and name=='minecraft:banner' and item.get('tag')=={'Type':0}:
            item=dict(item,tag={})
        if item.get('tag') or any(k not in {'Name','Count','Damage','Slot','tag','WasPickedUp','Block'} for k in item):
            raise UnsupportedNBT('包含附魔、命名、书本、嵌套容器或其他复杂物品NBT')
        damage = _int(item.get('Damage',0),0,32767,'Damage')
        if item.get('Block'):
            block = item['Block']
            if not isinstance(block,dict) or set(block)-{'name','states','val','version'}:
                raise UnsupportedNBT('物品方块附加数据格式尚未接入')
            # WaterStructure fuHongItemBlockNBT synthesises this exact compound
            # even for non-block items. It carries no additional item state.
            # Match its full profile; other metadata still needs native proof.
            generated=(name in GENERATED_NONBLOCK_ITEMS and damage==0 and block==dict(
                name=name,states={},val=0,version=17959425))
            if not generated and not validate_item_block(name,damage,json.dumps(block,sort_keys=True)):
                raise UnsupportedNBT('物品方块状态无法通过名称/附加值准确重建')
        result[slot] = dict(Name=name, Count=_int(item.get('Count'),1,255,'Count'),
                            Damage=damage, Slot=slot)
    return [result[k] for k in sorted(result)]


def sign_text(raw):
    if not isinstance(raw,dict):raise ParseError('告示牌NBT格式无效。')
    texts = {}
    modern = 'FrontText' in raw or 'BackText' in raw
    allowed = INERT | {'FrontText','BackText','IsWaxed','Text','TextOwner','SignTextColor','IgnoreLighting',
                      'PersistFormatting','TextIgnoreLegacyBugResolved'}
    if any(k not in allowed for k in raw):raise UnsupportedNBT('告示牌含未接入的附加字段')
    if raw.get('IsWaxed',0):raise UnsupportedNBT('涂蜡告示牌尚未接入')
    for side in ('FrontText','BackText'):
        part = raw.get(side,{}) if modern else (raw if side=='FrontText' else {})
        if not isinstance(part,dict):raise ParseError('告示牌文字面格式无效。')
        if modern and any(k not in {'Text','TextOwner','SignTextColor','IgnoreLighting','PersistFormatting',
                                    'HideGlowOutline','FilteredText'} for k in part):
            raise UnsupportedNBT('告示牌文字面含未接入字段')
        if part.get('SignTextColor',-16777216) not in (-16777216,4278190080):
            raise UnsupportedNBT('告示牌染色尚未接入')
        if any(part.get(k,0) for k in ('IgnoreLighting','HideGlowOutline')):
            raise UnsupportedNBT('告示牌发光或格式属性尚未接入')
        text = part.get('Text','')
        if not isinstance(text,str) or len(text.encode('utf-8'))>16384:raise ParseError('告示牌文字无效或过长。')
        if part.get('FilteredText','') not in ('',text):
            raise UnsupportedNBT('告示牌过滤文本与原文不同，不能无损写入')
        texts[side] = text
        formatting = part.get('PersistFormatting',1)
        if type(formatting) not in (bool,int) or formatting not in (0,1):
            raise ParseError('告示牌格式标记无效。')
        texts[side+'PersistFormatting'] = int(formatting)
    return texts


@dataclass(frozen=True)
class BlockEntityData:
    position: tuple
    block_name: str
    states: dict
    kind: str
    data: dict

    def commands(self,observed=None):
        if self.kind != 'container':return []
        # Set ALL slots, so stale items do not survive replacing an existing shell.
        items = {v['Slot']:v for v in self.data['items']}
        slots=range(self.data['slots'])
        if observed is not None:
            try:
                previous={v['Slot']:v for v in plain_items(observed.get('Items'),self.data['slots'],observed=True)}
                slots=[slot for slot in slots if items.get(slot)!=previous.get(slot)]
            except (ParseError,UnsupportedNBT):pass  # Unknown inventory requires a full source replacement.
        x,y,z = self.position
        return [f"replaceitem block {x} {y} {z} slot.container {slot} " +
            (f"{items[slot]['Name']} {items[slot]['Count']} {items[slot]['Damage']}" if slot in items
             else 'minecraft:air 1 0') for slot in slots]

    def differences(self, observed):
        if not isinstance(observed,dict):return ['missing_nbt']
        try:
            if self.kind == 'container':
                return [] if plain_items(observed.get('Items'),self.data['slots'],observed=True)==self.data['items'] else ['Items']
            # Source fields are normalized across old Text and modern Front/Back.
            return [] if sign_text({k:v for k,v in observed.items() if k in INERT or k in (
                'FrontText','BackText','Text','IsWaxed','SignTextColor','IgnoreLighting','PersistFormatting')})==self.data else ['sign_text_or_attributes']
        except (ParseError,UnsupportedNBT):return ['invalid_or_complex_nbt']


def plan_block_entities(structure, origin):
    operations = []; unsupported = []
    java = structure.source_info.get('source_edition')=='java' or structure.source_info.get('source_format','').startswith('Sponge')
    for local, raw in sorted(structure.block_entities.items()):
        if len(local)!=3 or any(type(v) is not int or not 0<=v<s for v,s in zip(local,structure.size)):
            raise ParseError('方块实体本地坐标无效。')
        block = structure.palette.get(int(structure.region.blocks[local]))
        name = block.state.name.removeprefix('minecraft:')
        if name in COMMAND_NAMES:continue
        world = tuple(a+b for a,b in zip(local,origin))
        try:
            if java:raise UnsupportedNBT('Java方块实体NBT尚未转换为基岩版')
            if not isinstance(raw,dict):raise ParseError('方块实体NBT必须为对象。')
            if name in CONTAINERS:
                extra = set(raw) - INERT - {'Items'}
                extra -= {k for k in ZERO_CONTAINER_FIELDS if raw.get(k)==0}
                if extra:raise UnsupportedNBT('容器含未接入字段：'+','.join(sorted(extra)))
                if 'Items' not in raw:continue
                slots = CONTAINERS[name]
                data = dict(slots=slots,items=plain_items(raw['Items'],slots))
                kind = 'container'
            elif name.endswith(('_sign','_hanging_sign')):
                data = sign_text(raw);kind = 'sign'
            else:raise UnsupportedNBT('此方块类型的NBT写入尚未接入')
            operations.append(BlockEntityData(world,block.state.name,dict(block.state.states),kind,data))
        except UnsupportedNBT as exc:
            unsupported.append(dict(position=list(world),block=block.state.name,reason=str(exc)))
    return operations, unsupported


def operation_hash(operations):
    return hashlib.sha256(json.dumps([REVISION,[asdict(op) for op in operations]],
        sort_keys=True,ensure_ascii=False,separators=(',',':')).encode()).hexdigest()


def codec_binary():
    folder = ROOT/'tools/nbtcodec'
    files = [folder/n for n in ('main.go','go.mod','go.sum')]
    source = ROOT/'third_party/fatalder/modules/FunShuttler'
    # Include directly reused upstream codec/schema sources in the build identity.
    files += sorted((source/'core/minecraft/protocol').glob('*.go'))
    files += [source/'core/minecraft/protocol/packet/block_actor_data.go']
    revision = hashlib.sha256(b''.join(p.read_bytes() for p in files)).hexdigest()
    binary = ROOT/'cache'/('other-nbt-codec-'+revision[:16])
    if not binary.exists():
        go = shutil.which('go')
        if not go:raise ParseError('首次写入告示牌需要Go构建离线协议助手。')
        binary.parent.mkdir(exist_ok=True)
        try:
            result = subprocess.run([go,'build','-mod=readonly','-o',str(binary),'.'],cwd=folder,env=go_build_environment(),
                capture_output=True,timeout=180)
        except (OSError,subprocess.TimeoutExpired) as exc:raise ParseError('NBT协议助手构建失败。') from exc
        if result.returncode:raise ParseError('NBT协议助手构建失败：'+result.stderr.decode(errors='replace')[:400])
    return binary


@lru_cache(maxsize=4096)
def validate_item_block(name, damage, encoded):
    request = dict(action='validate_item_block',protocol=860,item_name=name,damage=damage,
                   block_nbt=json.loads(encoded))
    try:
        result = subprocess.run([str(codec_binary())],input=json.dumps(request),text=True,
            capture_output=True,timeout=10)
    except (OSError,subprocess.TimeoutExpired) as exc:
        raise ParseError('物品方块状态预检失败。') from exc
    return result.returncode == 0


def sign_packets(operation, action, player_position, tick=0, protocol=860):
    if protocol!=860:raise ParseError('告示牌编辑仅支持已验证的网易860协议。')
    request = dict(action=action,protocol=protocol,position=operation.position,
        player_position=player_position,name=operation.block_name,states=operation.states,
        front=operation.data['FrontText'],back=operation.data['BackText'],tick=tick,
        front_persist=operation.data.get('FrontTextPersistFormatting',1),
        back_persist=operation.data.get('BackTextPersistFormatting',1))
    try:
        from proloader.world_codec import request as codec_request
        result=codec_request(codec_binary(),request)['packets']
        return [wrap_packet(v['id'],bytes.fromhex(v['hex'])) for v in result]
    except (OSError,subprocess.TimeoutExpired,ValueError,KeyError) as exc:
        raise ParseError('告示牌协议助手未返回有效数据。') from exc


async def write_block_entity(operation, world, session, dimension, observed=None):
    guard=getattr(world,'assert_local_repair_current',None)
    if guard is not None:guard()
    if operation.kind == 'container':
        for command in operation.commands(observed):
            if guard is not None:guard()
            # Relative aiCommand anchors are not involved: coordinates are explicit.
            if hasattr(session,'_raknet'):
                from proloader.pacing import transport_pressure
                await transport_pressure(session)
            pacer=getattr(world,'nbt_pacer',None)
            if pacer is not None:await pacer.wait()
            await world.send(f'execute in {dimension} run {command}',wait=False)
    else:
        if getattr(session,'_netease_protocol',None)!=860:
            raise ValueError('告示牌写入尚未支持当前连接的协议。')
        x,y,z = operation.position
        await world.teleport(x+1,y,z)
        # Mirrors upstream sign.Make: empty the selected hand before clicking.
        await world.send('replaceitem entity @s slot.hotbar 0 minecraft:air 1 0')
        player = (x+1,y+1.62,z)
        for packet in await asyncio.to_thread(sign_packets,operation,'sign_click',player):
            if guard is not None:guard()
            await session.send_packet(packet)
        for _ in range(2):
            if guard is not None:guard()
            await world.send('testfor @s')
            tick = getattr(session,'_nbt_auth_tick',0)+1
            session._nbt_auth_tick = tick
            for packet in await asyncio.to_thread(sign_packets,operation,'auth',player,tick):
                if guard is not None:guard()
                await session.send_packet(packet)
        for packet in await asyncio.to_thread(sign_packets,operation,'sign_text',player):
            if guard is not None:guard()
            await session.send_packet(packet)
    await world.send('testfor @s')


class BlockEntityMismatch(ValueError):
    """A confirmed mismatch that can be retried within this live session."""


async def apply_block_entities(operations, session, world, dimension, job, save, move, update):
    if not operations:return
    if job.get('block_entities_next',0)>=len(operations):
        job['block_entities_state']='readback_verified'
        return
    started=time.monotonic();before=job.get('block_entities_next',0)
    try:
        return await _apply_block_entities(operations,session,world,dimension,job,save,move,update)
    finally:
        elapsed=time.monotonic()-started
        job['block_entities_seconds']=round(job.get('block_entities_seconds',0)+elapsed,3)
        completed=max(0,job.get('block_entities_next',0)-before)
        job['block_entities_last_run']=dict(completed=completed,seconds=round(elapsed,3),
            actual_rate=round(completed/elapsed,2) if elapsed else 0)
        save()


async def _apply_block_entities(operations, session, world, dimension, job, save, move, update):
    if not operations:return
    job['block_entities_state']='writing'
    cursor=job.get('block_entities_next',0)
    schedule=job.get('block_entities_schedule')
    if schedule is None:
        # Preserve an old source-order prefix when resuming; schedule only the
        # unread suffix spatially. The saved permutation keeps cursors stable.
        def order(i):
            op=operations[i];x,y,z=op.position
            return (x//144,z//144,x//16,z//16,op.kind!='container',y,x,z)
        schedule=list(range(cursor))+sorted(range(cursor,len(operations)),key=order)
        job['block_entities_schedule']=schedule
    if len(schedule)!=len(operations) or sorted(schedule)!=list(range(len(operations))):
        raise ValueError('附加数据执行顺序与当前计划不一致。')
    save()
    for index in range(cursor,len(operations)):
        operation = operations[schedule[index]]
        loader=getattr(world,'chunk_window',None)
        if loader is not None:
            await loader.ensure(('setblock',*operation.position,*operation.position,
                                 operation.block_name,operation.states))
        # A command-coordinate frame can change without moving the bot. Only
        # confirmed physical position is valid for deciding whether to move.
        physical=getattr(world,'_last_import_position',None)
        same_column=physical is not None and physical[0]//16==operation.position[0]//16 and physical[2]//16==operation.position[2]//16
        need_move=(not same_column) if operation.kind=='container' else (
            physical is None or sum((a-b)**2 for a,b in zip(physical,operation.position))>36)
        if need_move:
            await move(world,operation.position)
        update('block_entity_data',detail=f'写入并读回附加数据 {index+1}/{len(operations)}')
        tile = session._states_to_str(operation.states)
        async def check_shell():
            if hasattr(world,'check_block_entity_shell'):
                for attempt in range(3):
                    try:
                        if await world.check_block_entity_shell(operation):return False
                        # A confirmed shell mismatch is repairable. Unknown reads
                        # only reposition/retry; they never authorize replacement.
                        x,y,z=operation.position
                        await world.place(('setblock',x,y,z,x,y,z,
                                           operation.block_name,operation.states),wait=True)
                        if await world.check_block_entity_shell(operation):return True
                    except (ValueError,asyncio.TimeoutError):
                        session._raise_if_failed()
                    if attempt<2:
                        await move(world,operation.position)
                        await asyncio.sleep(.25*(attempt+1))
                raise BlockEntityMismatch('方块附加数据外壳恢复后仍未确认，断点已保存。')
            await world.send(f'execute in {dimension} run testforblock '+
                ' '.join(map(str,operation.position))+f' {operation.block_name} {tile}')
            return False
        from proloader.importer import CommandUnconfirmedError
        async def read_current():
            repaired=await check_shell()
            consume=getattr(world,'consume_entity_snapshot',None)
            snapshot=consume(operation.position) if consume is not None else None
            observed=snapshot if snapshot is not None else await session.read_block_entity(operation.position,dimension)
            return observed,repaired
        for recovery in range(3):
            try:
                old,_ = await read_current()
                if operation.differences(old):
                    if operation.kind=='sign' and old.get('IsWaxed',0):
                        raise ValueError('目标告示牌已涂蜡，停止写入并保留断点。')
                    job['block_entities_inflight']=index
                    save()
                    await write_block_entity(operation,world,session,dimension,old)
                    # Readback may lag a game tick. Retry reads only; never resend writes here.
                    for attempt in range(5):
                        observed,repaired = await read_current()
                        if repaired:
                            raise BlockEntityMismatch('外壳已恢复，重新读取后再补写附加数据。')
                        differences = operation.differences(observed)
                        if not differences:break
                        if attempt<4:await asyncio.sleep(0.25)
                    if differences:
                        job['block_entity_mismatch']=dict(index=index,position=list(operation.position),fields=differences)
                        save()
                        raise BlockEntityMismatch('方块附加数据读回不一致，断点已保存。')
                break
            except (asyncio.TimeoutError,CommandUnconfirmedError,BlockEntityMismatch) as exc:
                session._raise_if_failed()
                if recovery==2:raise
                job.setdefault('block_entities_recoveries',[]).append(
                    dict(index=index,error=type(exc).__name__,attempt=recovery+1))
                save()
                update('block_entity_data',detail=f'附加数据局部恢复 {index+1}/{len(operations)} · {recovery+1}/2')
                await move(world,operation.position)
                await asyncio.sleep(.25*(recovery+1))
        job['block_entities_next']=index+1
        job.pop('block_entities_inflight',None);job.pop('block_entity_mismatch',None)
        save()
    job['block_entities_state']='readback_verified'
    save()
