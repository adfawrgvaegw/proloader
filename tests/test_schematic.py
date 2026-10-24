"""Legacy file fixtures exercise ID/data conversion and the real import entry."""
import gzip
import struct

import numpy as np
import pytest

from proloader.errors import ParseError
from proloader.parser import load_structure, load_structure_from_bytes
from proloader.parser.nbt import NBTWriter, NBTShort
from proloader.importer import prepare_import


class JavaNBT(NBTWriter):
    def write_short(self,v):self._buf+=struct.pack('>h',v)
    def write_ushort(self,v):self._buf+=struct.pack('>H',v)
    def write_int(self,v):self._buf+=struct.pack('>i',v)
    def write_root(self,value):
        self.write_byte(10);self.write_string('Schematic');self._write_compound(value)
        return bytes(self._buf)


def fixture(**changes):
    root=dict(Width=NBTShort(2),Height=NBTShort(2),Length=NBTShort(3),Materials='Alpha',
              Blocks=bytes([1,5,35,53,17,0,1,1,1,1,1,1]),
              Data=bytes([0,1,14,4,4,0,0,0,0,0,0,0]),TileEntities=[])
    root.update(changes)
    return JavaNBT().write_root(root)


@pytest.mark.parametrize('compressed',[False,True])
def test_legacy_axes_and_metadata(compressed):
    raw=fixture();s=load_structure_from_bytes(gzip.compress(raw) if compressed else raw,'.schematic')
    def state(x,y,z):return s.palette.get(int(s.region.blocks[x,y,z])).state
    assert s.size==(2,2,3)
    assert state(1,0,0).name=='minecraft:spruce_planks'
    assert state(0,0,1).name=='minecraft:red_wool'
    assert state(1,0,1).states=={'upside_down_bit':True,'weirdo_direction':0}
    assert state(0,0,2).states=={'pillar_axis':'x'}
    assert state(1,0,2).name=='minecraft:air'
    assert state(0,1,0).name=='minecraft:stone'


@pytest.mark.parametrize('changes',[
    {'Width':NBTShort(0)}, {'Blocks':b'\x01'}, {'Data':b''},
    {'AddBlocks':b'\x00'}, {'Materials':'Classic'},
    {'TileEntities':[{'x':2,'y':0,'z':0,'id':'Chest'}]},
    {'TileEntities':[{'x':0,'y':0,'z':0},{'x':0,'y':0,'z':0}]},
])
def test_malformed_data_rejected(changes):
    with pytest.raises(ParseError):load_structure_from_bytes(fixture(**changes),'.schematic')


@pytest.mark.parametrize('compressed',[False,True])
def test_high_metadata_bits_preserve_low_bits_and_block_ids(compressed):
    original=fixture()
    extended=fixture(Data=bytes([16,33,46,52,20,240,128,32,64,80,96,112]))
    if compressed:extended=gzip.compress(extended)
    expected=load_structure_from_bytes(original,'.schematic')
    actual=load_structure_from_bytes(extended,'.schematic')
    assert np.array_equal(actual.region.blocks,expected.region.blocks)
    assert [b.state for b in actual.palette]==[b.state for b in expected.palette]
    assert actual.source_info['normalized_metadata_count']==12
    assert len(actual.source_info['warnings'])==1


@pytest.mark.parametrize('extra',[b'\x01'+b'\x00'*5,b'\x10'+b'\x00'*5])
def test_addblocks_high_nibbles_never_truncate_to_vanilla(extra):
    with pytest.raises(ParseError,match='ID:Data'):
        load_structure_from_bytes(fixture(AddBlocks=extra),'.schematic')


def test_zero_addblocks_and_odd_volume():
    s=load_structure_from_bytes(fixture(Width=NBTShort(3),Height=NBTShort(1),Length=NBTShort(1),
        Blocks=bytes([1,35,5]),Data=bytes([0,14,1]),AddBlocks=b'\x00\x00'),'.schematic')
    assert s.size==(3,1,1)
    assert len(s.palette)==4


def test_file_control_and_import_plan_use_same_strict_parser(tmp_path):
    from proloader.control import load_any_structure
    p=tmp_path/'example.SCHEMATIC';p.write_bytes(gzip.compress(fixture()))
    s=load_structure(p);other=load_any_structure(str(p))
    assert np.array_equal(s.region.blocks,other.region.blocks)
    plan=prepare_import(p,(100,80,200),engine='legacy')
    assert plan.block_count==11
    assert any(c[7]=='minecraft:red_wool' for c in plan.commands)
    assert plan.summary()['warnings']==[]


def test_sponge_payload_with_old_extension():
    raw=JavaNBT().write_root(dict(Version=2,Width=NBTShort(2),Height=NBTShort(1),Length=NBTShort(1),
        Palette={'minecraft:air':0,'minecraft:stone':1},BlockData=b'\x00\x01'))
    s=load_structure_from_bytes(gzip.compress(raw),'.schematic')
    assert s.source_info['source_format']=='Sponge v2'
    assert s.palette.get(int(s.region.blocks[1,0,0])).state.name=='minecraft:stone'


def test_menu_lists_schematic_files(tmp_path,monkeypatch):
    from proloader.cli import wizard
    buildings=tmp_path/'buildings';buildings.mkdir()
    p=buildings/'palace.schematic';p.write_bytes(b'fixture')
    monkeypatch.setattr(wizard,'PROJECT',tmp_path)
    seen=[]
    def choose(title,options,**kwargs):
        seen.extend(options);return 0
    monkeypatch.setattr(wizard.ui,'choose',choose)
    assert wizard.select_file()==p
    assert 'palace.schematic' in seen


def test_legacy_java_commands_never_become_bedrock_commands():
    from proloader.command_blocks import plan_command_blocks
    s=load_structure_from_bytes(fixture(Blocks=bytes([137]+[0]*11),Data=bytes(12),
        TileEntities=[dict(x=0,y=0,z=0,id='Control',Command='say test')]),'.schematic')
    blocks=plan_command_blocks(s,(0,80,0))
    assert blocks[0].source_edition=='java'


def test_truncated_nbt_and_bad_gzip_rejected():
    for raw in (fixture()[:-3],b'\x1f\x8bgarbage'):
        with pytest.raises(ParseError):load_structure_from_bytes(raw,'.schematic')
