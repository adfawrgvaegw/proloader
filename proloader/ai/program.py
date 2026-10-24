"""Strict, model-independent Bedrock command-block source format."""
from pathlib import Path
import hashlib
import json

import numpy as np

from proloader.model.block import Block, BlockState
from proloader.model.palette import BlockPalette
from proloader.model.region import Region
from proloader.model.structure import Structure
from proloader.parser.mcstructure import write_mcstructure
from proloader.parser.nbt import NBTByte

TYPES = {'impulse': 'minecraft:command_block', 'repeat': 'minecraft:repeating_command_block',
         'chain': 'minecraft:chain_command_block'}
FACING = {'down': 0, 'up': 1, 'north': 2, 'south': 3, 'west': 4, 'east': 5}


def position(value):
    if not isinstance(value, (list, tuple)) or len(value) != 3 or any(type(x) is not int for x in value):
        raise ValueError('坐标必须是三个整数')
    if any(not -(1 << 31) <= x < (1 << 31) for x in value):
        raise ValueError('坐标越界')
    return tuple(value)


def command(value):
    if not isinstance(value, str) or not value.strip() or any(x in value for x in '\n\r\0'):
        raise ValueError('命令必须是非空单行字符串')
    value = value.strip().removeprefix('/')
    if not value or len(value.encode()) > 30000:
        raise ValueError('命令长度无效')
    from proloader.ai.spec import lint
    errors=[e for e in lint(value) if e['severity']=='error']
    if errors:raise ValueError('; '.join(e['message'] for e in errors))
    return value


def validate(data):
    if not isinstance(data, dict) or data.get('format') != 'proloader-ai-v1' or data.get('edition') != 'bedrock':
        raise ValueError('需要 format=proloader-ai-v1、edition=bedrock；不自动转换Java指令')
    if set(data)-{'format','edition','origin','dimension','blocks'}:
        raise ValueError('程序顶层字段无效或拼写错误')
    origin = position(data['origin'])
    dimension = data.get('dimension', 'overworld')
    if dimension not in ('overworld', 'nether', 'the_end'):
        raise ValueError('维度无效')
    raw = data['blocks']
    if not isinstance(raw, list) or not 1 <= len(raw) <= 100000:
        raise ValueError('方块数量范围1～100000')
    blocks, seen = [], set()
    for entry in raw:
        if not isinstance(entry, dict) or set(entry) - {'at','type','facing','command','name','conditional','auto','delay','first_tick','track_output'}:
            raise ValueError('方块字段无效或拼写错误')
        at = position(entry['at'])
        if min(at) < 0 or at in seen:
            raise ValueError('本地坐标必须非负且不能重复')
        seen.add(at)
        world = position([a+b for a,b in zip(origin, at)])
        low, high = (-64, 319) if dimension == 'overworld' else (0, 127 if dimension == 'nether' else 255)
        if not low <= world[1] <= high:
            raise ValueError('命令方块超过维度高度')
        kind, facing = entry.get('type', 'impulse'), entry.get('facing', 'east')
        if kind not in TYPES or facing not in FACING:
            raise ValueError('类型或朝向无效')
        e = dict(at=list(at), type=kind, facing=facing, command=command(entry['command']), name=entry.get('name','AI'))
        if not isinstance(e['name'], str) or len(e['name'].encode()) > 1024:
            raise ValueError('名称无效')
        for key, default in [('conditional',False),('auto',False),('first_tick',False),('track_output',True)]:
            e[key] = entry.get(key,default)
            if type(e[key]) is not bool:
                raise ValueError(key+'必须为布尔值')
        e['delay'] = entry.get('delay',0)
        if type(e['delay']) is not int or not 0 <= e['delay'] < 1 << 31:
            raise ValueError('延迟无效')
        blocks.append(e)
    size = tuple(max(e['at'][i] for e in blocks)+1 for i in range(3))
    if np.prod(size, dtype=object) > 2000000:
        raise ValueError('包围盒过大（最多200万格），请拆分程序')
    return dict(format='proloader-ai-v1',edition='bedrock',origin=list(origin),dimension=dimension,blocks=blocks)


def layout(text, origin, width=12, head='impulse', delay=0, dimension='overworld'):
    """Lay a .mcfunction into a connected serpentine command chain."""
    if type(width) is not int or not 1 <= width <= 256 or head not in ('impulse','repeat'):
        raise ValueError('宽度范围1～256，首块类型impulse/repeat')
    commands = [command(line) for line in text.splitlines() if line.strip() and not line.lstrip().startswith('#')]
    entries = []
    for i,c in enumerate(commands):
        row,col = divmod(i,width)
        x = col if row%2==0 else width-1-col
        facing = 'south' if col==width-1 and i+1<len(commands) else ('east' if row%2==0 else 'west')
        entries.append(dict(at=[x,0,row],type=head if i==0 else 'chain',facing=facing,
                            auto=head=='repeat' if i==0 else True,command=c,delay=delay if i==0 else 0))
    return validate(dict(format='proloader-ai-v1',edition='bedrock',origin=list(origin),dimension=dimension,blocks=entries))


def load(path):
    return validate(json.loads(Path(path).read_text()))


def fingerprint(program):
    return hashlib.sha256(json.dumps(program,sort_keys=True,ensure_ascii=False).encode()).hexdigest()


def compile_program(program, output):
    program = validate(program)
    size = tuple(max(e['at'][i] for e in program['blocks'])+1 for i in range(3))
    palette = BlockPalette()
    array = np.zeros(size, dtype=np.uint32)
    entities = {}
    for e in program['blocks']:
        at = tuple(e['at'])
        array[at] = palette.add(Block(BlockState(TYPES[e['type']], dict(
            facing_direction=FACING[e['facing']],conditional_bit=e['conditional']))))
        entities[at] = dict(id='CommandBlock',Command=e['command'],CustomName=e['name'],
            auto=NBTByte(e['auto']),TickDelay=e['delay'],ExecuteOnFirstTick=NBTByte(e['first_tick']),
            TrackOutput=NBTByte(e['track_output']),conditionalMode=NBTByte(e['conditional']))
    structure = Structure('AI command program',size,palette,Region(palette,array,offset=tuple(program['origin'])),entities)
    Path(output).write_bytes(write_mcstructure(structure))
    from proloader.ai.spec import lint_program,PROFILE
    return dict(file=str(Path(output).resolve()),blocks=len(entities),size=size,sha256=fingerprint(program),
                origin=program['origin'],dimension=program['dimension'],profile=PROFILE['id'],issues=lint_program(program))
