"""Exercise real Brotli/BDX bytes through the bundled upstream decoder."""
from pathlib import Path

import pytest

from proloader.errors import ParseError
from proloader.parser import load_structure, load_structure_from_bytes
from proloader.command_blocks import plan_command_blocks

FIXTURES = Path(__file__).parent / 'fixtures/bdx'


def test_states_negative_coordinates_and_last_write_wins(tmp_path):
    source = FIXTURES/'states-negative-overwrite.bdx'
    path = tmp_path/'example.BDX'; path.write_bytes(source.read_bytes())
    s = load_structure(path)
    assert s.size == (2, 1, 1) and s.block_count == 2
    assert s.source_info['source_min'] == [-2, 0, 0]
    assert s.source_info['author'] == 'test-author'
    assert s.palette.get(int(s.region.blocks[0,0,0])).state.name == 'minecraft:dirt'
    assert s.palette.get(int(s.region.blocks[1,0,0])).state.name == 'minecraft:stone'
    from proloader.control import load_any_structure
    assert load_any_structure(str(path)).block_count == 2


@pytest.mark.parametrize('name,match',[
    ('unknown', 'unknown BDX block'), ('undefined', 'undefined'),
    ('truncated', 'BDX command'), ('unterminated', 'BDX command'),
    ('unknown-opcode', 'Unknown Command'), ('bad-pool', 'runtime pool'),
    ('oversize', '6400万'), ('debug-overflow', 'debug payload'),
])
def test_bad_files_fail_before_planning(name, match):
    with pytest.raises(ParseError, match=match):
        load_structure(FIXTURES/(name+'.bdx'))


def test_invalid_header():
    with pytest.raises(ParseError, match='文件头'):
        load_structure_from_bytes(b'not BDX', '.bdx')


def test_runtime_pool_117():
    import json
    from proloader.parser.bdx import UPSTREAM
    expected = json.loads((UPSTREAM/'structure/bdx_runtimeIds_117.json').read_text())[1][0]
    s = load_structure(FIXTURES/'runtime.bdx')
    assert s.block_count == 1
    assert s.palette.get(int(s.region.blocks[0,0,0])).state.name.removeprefix('minecraft:') == expected.removeprefix('minecraft:')


def test_command_data_reaches_existing_staging_flow():
    from proloader.net.raknet.wire import ByteReader
    from proloader.net.bedrock.packets import unwrap_packet
    s = load_structure(FIXTURES/'command-block.bdx')
    c, = plan_command_blocks(s, (100, 80, -100))
    assert c.position == (100, 80, -100)
    assert c.command == 'say BDX' and c.name == 'test' and c.tick_delay == 4
    assert not c.needs_redstone and c.execute_on_first_tick
    # Existing policy stages disabled even if the source requests activation.
    _, body = unwrap_packet(c.packet())
    packet = ByteReader(body)
    assert packet.read_bool()
    assert (packet.read_varint(), packet.read_unsigned_varint(), packet.read_varint()) == c.position
    assert packet.read_unsigned_varint() == 2
    assert packet.read_bool()  # needs redstone
    assert body[-1] == 0  # execute_on_first_tick


def test_chest_contents_retained_with_explicit_warning():
    s = load_structure(FIXTURES/'chest.bdx')
    assert s.block_entities[(0,0,0)]['Items'][0] == dict(Name='minecraft:stone',Count=3,Damage=0,Slot=5)
    assert '实际写入范围' in s.source_info['warnings'][0]


def test_native_plan_covers_bdx_without_widening():
    from proloader.importer import prepare_import
    plan = prepare_import(FIXTURES/'states-negative-overwrite.bdx', (-85, 78, 351))
    assert plan.block_count == 2
    assert {c[7] for c in plan.commands} == {'minecraft:stone', 'minecraft:dirt'}


def test_command_bdx_full_preflight():
    from proloader.importer import prepare_import
    plan = prepare_import(FIXTURES/'command-block.bdx', (100, 80, -100))
    assert len(plan.command_blocks) == 1
    assert plan.summary()['command_blocks'] == 1


def test_all_movement_widths_and_deprecated_z_commands():
    s = load_structure(FIXTURES/'movement-variants.bdx')
    assert s.size == (2, 3, 5)
    assert s.block_count == 2
    assert s.region.blocks[0,0,0] and s.region.blocks[1,2,4]


@pytest.mark.parametrize('file,command', [('raw-nbt','say raw nbt'), ('set-command-data','say update')])
def test_raw_nbt_and_separate_command_data(file, command):
    s = load_structure(FIXTURES/(file+'.bdx'))
    c, = plan_command_blocks(s, (0,80,0))
    assert c.command == command and c.tick_delay == 2


def test_plain_overwrite_removes_old_command_data():
    s = load_structure(FIXTURES/'clear-stale-nbt.bdx')
    assert s.block_count == 1 and not s.block_entities
    assert not plan_command_blocks(s, (0,80,0))
