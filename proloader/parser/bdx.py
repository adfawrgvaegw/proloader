"""BDX adapter backed by the unmodified bundled bdump command decoder."""
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
from proloader.native_build import go_build_environment

import numpy as np

from proloader.errors import ParseError
from proloader.model.block import Block, BlockState
from proloader.model.palette import BlockPalette
from proloader.model.region import Region
from proloader.model.structure import Structure
from .base import register_parser

ROOT = Path(__file__).resolve().parents[2]
UPSTREAM = ROOT / 'third_party/fatalder/modules/WaterStructure'


def reader_binary():
    source = ROOT / 'tools/bdxreader'
    files = list(source.glob('*.go')) + [source/'go.mod', source/'go.sum']
    files += sorted((UPSTREAM/'modules/bdump').rglob('*.go'))
    files += [UPSTREAM/'modules/bdump/go.mod', UPSTREAM/'structure/bdx_runtimeIds_117.json']
    revision = hashlib.sha256(b''.join(p.read_bytes() for p in files)).hexdigest()
    binary = ROOT / 'cache' / ('bdxreader-' + revision[:16])
    if not binary.exists():
        go = shutil.which('go')
        if not go:raise ParseError('首次读取 BDX 需要 Go 编译离线读取器。')
        binary.parent.mkdir(parents=True, exist_ok=True)
        try:
            result = subprocess.run([go, 'build', '-mod=readonly', '-o', str(binary), '.'],
                cwd=source, env=go_build_environment(), capture_output=True, timeout=180)
        except (OSError, subprocess.TimeoutExpired) as exc:
            raise ParseError('BDX 离线读取器构建失败。') from exc
        if result.returncode:raise ParseError('BDX 离线读取器构建失败：' + result.stderr.decode(errors='replace')[:400])
    return binary, revision


def _nbt(command):
    if 'BlockNBT' in command:return command['BlockNBT']
    if 'CommandBlockData' in command:
        c = command['CommandBlockData']
        if c['Mode'] not in (0, 1, 2):raise ParseError('BDX 命令方块模式无效。')
        return dict(id='CommandBlock', Command=c['Command'], CustomName=c['CustomName'],
            LastOutput=c['LastOutput'], TickDelay=c['TickDelay'],
            ExecuteOnFirstTick=int(c['ExecuteOnFirstTick']), TrackOutput=int(c['TrackOutput']),
            conditionalMode=int(c['Conditional']), auto=int(not c['NeedsRedstone']))
    if 'ChestSlots' in command:
        return dict(Items=command['ChestSlots'])
    return None


@register_parser('.bdx')
def parse_bdx(data: bytes, name: str = 'structure') -> Structure:
    if data[:3] != b'BD@':raise ParseError('BDX 文件头无效，应为 BD@。')
    binary, revision = reader_binary()
    try:
        result = subprocess.run([str(binary), str(UPSTREAM/'structure/bdx_runtimeIds_117.json')],
            input=data, capture_output=True, timeout=120)
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise ParseError('BDX 读取器失败或超时，未生成导入计划。') from exc
    if result.returncode:
        raise ParseError('BDX 读取失败：' + result.stderr.decode(errors='replace')[:400])
    palette = BlockPalette()
    placed = {}; entities = {}; pos = [0, 0, 0]; terminated = False
    try:
        lines = iter(result.stdout.splitlines())
        author = json.loads(next(lines))['author']
        for line in lines:
            c = json.loads(line); opcode = c['id']
            if opcode == 88:terminated = True; break
            pos = c['position']
            if 'block' in c:
                b = c['block']; local = tuple(pos)
                placed[local] = palette.add(Block(BlockState(b['name'], b['states'] or {})))
                entities.pop(local, None)
                nbt = _nbt(c)
                if nbt is not None:entities[local] = nbt
            elif opcode == 26:
                local = tuple(pos)
                if local not in placed or palette.get(placed[local]).state.name not in (
                    'minecraft:command_block', 'minecraft:chain_command_block', 'minecraft:repeating_command_block'):
                    raise ParseError('BDX 设置命令数据的位置没有命令方块。')
                entities[local] = _nbt(c)
            else:raise ParseError(f'BDX 尚不支持操作码 {opcode}。')
        if not terminated or not placed:raise ParseError('BDX 缺少结束标记或没有方块。')
        low = tuple(min(p[i] for p in placed) for i in range(3))
        high = tuple(max(p[i] for p in placed) for i in range(3))
        size = tuple(b-a+1 for a,b in zip(low,high))
        if np.prod(size, dtype=object) > 64_000_000:raise ParseError('BDX 包围盒超过6400万格。')
        blocks = np.zeros(size, dtype=np.uint32)
        for p, bid in placed.items():blocks[tuple(a-b for a,b in zip(p,low))] = bid
        entities = {tuple(a-b for a,b in zip(p,low)): nbt for p,nbt in entities.items()}
        other_nbt = sum(v.get('id') != 'CommandBlock' for v in entities.values())
        warnings = []
        if other_nbt:warnings.append(f'{other_nbt}个非命令方块附加数据已保留；实际写入范围以预检NBT清单为准。')
        return Structure(name, size, palette, Region(palette, blocks), block_entities=entities,
            format_version='BDX', source_info=dict(source_format='BDX', source_edition='bedrock',
                author=author, source_min=list(low), reader_revision=revision,
                source_sha256=hashlib.sha256(data).hexdigest(), unknown_blocks=0,
                block_entity_count=len(entities), warnings=warnings, exact_states=True))
    except (KeyError, TypeError, ValueError, IndexError, StopIteration) as exc:
        raise ParseError('BDX 读取器返回了无效结构。') from exc
