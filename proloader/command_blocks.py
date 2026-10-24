"""Command block configuration planning and strict offline Fatalder NBT decoding.

Source activation settings are kept in the plan. The first integration only
supports staging: needs redstone, no first-tick execution. Java commands require
an explicit conversion workflow and cannot be staged as Bedrock commands.
"""
from dataclasses import dataclass, asdict
import base64
import hashlib
import json
import subprocess

from proloader.errors import ParseError
from proloader.fatalder import native_binary
from proloader.net.bedrock.command_block import build_command_block_update
from proloader.net.bedrock.packets import unwrap_packet

REVISION = 'command-block-stage-v2-860-filtered-name'
MODES = {'minecraft:command_block': 0, 'minecraft:repeating_command_block': 1,
         'minecraft:chain_command_block': 2}
DIMENSIONS = {'overworld': 0, 'nether': 1, 'the_end': 2}


def _bit(data, name, default=0):
    value = data.get(name, default)
    if not isinstance(value, (bool, int)) or value not in (0, 1):
        raise ParseError(f'Invalid command block flag: {name}')
    return bool(value)


@dataclass(frozen=True)
class CommandBlockData:
    position: tuple
    block_name: str
    states: dict
    command: str
    name: str
    needs_redstone: bool
    conditional: bool
    track_output: bool
    tick_delay: int
    execute_on_first_tick: bool
    source_edition: str = 'bedrock'

    def packet(self):
        if self.source_edition != 'bedrock':
            raise ValueError('Java命令方块指令尚未转换，不能直接写入基岩版。')
        return build_command_block_update(self.position, mode=MODES[self.block_name],
            needs_redstone=True, conditional=self.conditional, command=self.command,
            name=self.name, track_output=self.track_output, tick_delay=self.tick_delay,
            execute_on_first_tick=False)

    def differences(self, nbt):
        # Presence matters: a missing field is not an acceptable false/empty value.
        expected = dict(Command=self.command, CustomName=self.name, auto=0,
                        TrackOutput=int(self.track_output), TickDelay=self.tick_delay,
                        ExecuteOnFirstTick=0)
        differences = [k for k, v in expected.items() if k not in nbt or nbt[k] != v]
        if nbt.get('id') not in ('CommandBlock', 'minecraft:command_block'):
            differences.append('id')
        # NEMC LP* fields are stale UI caches (observed 0 even for a repeating
        # shell). Mode/condition/facing are verified through exact block states,
        # never inferred from these NBT caches.
        return differences


def plan_command_blocks(structure, origin):
    import numpy as np
    ids = [i for i, b in enumerate(structure.palette) if b.state.name in MODES]
    if not ids:return []
    java = (structure.source_info.get('source_edition') == 'java' or
            structure.source_info.get('source_format', '').startswith('Sponge'))
    coords = np.argwhere(np.isin(structure.region.blocks, ids))
    result = []
    for local in coords:
        local = tuple(map(int, local))
        block = structure.palette.get(int(structure.region.blocks[local]))
        raw = structure.block_entities.get(local)
        if raw is None:
            raise ParseError('命令方块缺少内部数据，已停止规划，避免只导入外壳。')
        nbt = raw.get('Data', raw) if java else raw
        if not isinstance(nbt, dict):raise ParseError('命令方块内部数据格式无效。')
        command, name, delay = nbt.get('Command', ''), nbt.get('CustomName', ''), nbt.get('TickDelay', 0)
        if not isinstance(command, str) or not isinstance(name, str):raise ParseError('命令方块文本格式无效。')
        if type(delay) is not int or not 0 <= delay < 1<<31:raise ParseError('命令方块延迟无效。')
        states = dict(block.state.states)
        if 'conditional_bit' in states:states['conditional_bit'] = _bit(states, 'conditional_bit')
        conditional = _bit(nbt, 'conditionalMode') or _bit(states, 'conditional_bit')
        if 'conditional_bit' in states and conditional != states['conditional_bit']:
            raise ParseError('命令方块NBT条件模式与方块状态矛盾，不能直接导入。')
        result.append(CommandBlockData(tuple(a+b for a,b in zip(origin, local)),
            block.state.name, states, command, name, not _bit(nbt, 'auto'),
            conditional, _bit(nbt, 'TrackOutput', 1), delay, _bit(nbt, 'ExecuteOnFirstTick'),
            'java' if java else 'bedrock'))
    return result


def operation_hash(operations):
    return hashlib.sha256(json.dumps([REVISION, [asdict(x) for x in operations]],
        ensure_ascii=False, sort_keys=True, separators=(',', ':')).encode()).hexdigest()


def decode_subchunk(packet):
    _, body = unwrap_packet(packet)
    result = subprocess.run([str(native_binary('nbt'))], input=json.dumps(dict(
        action='decode_subchunk', data=base64.b64encode(body).decode())),
        capture_output=True, text=True, timeout=15)
    if result.returncode:
        raise ValueError('网易子区块NBT解码失败，未确认内部数据。')
    return json.loads(result.stdout)
