import gzip

import pytest

from proloader.errors import ParseError
from proloader.parser.nbt import NBTReader
from tests.test_schematic import JavaNBT
from tools.schem_palette_copy import substituted_bytes,write_copy


def fixture():
    # Include the same name as an entity string to prove only the palette changes.
    return JavaNBT().write_root({'Version':3,'Blocks':{
        'Palette':{'minecraft:air':0,'minecraft:bush':1},'Data':b'\0\1',
        'BlockEntities':[{'CustomName':'minecraft:bush','Pos':[1,0,0]}]},'Offset':[12,34,56]})


@pytest.mark.parametrize('compressed',[True,False])
def test_only_palette_keys_change(compressed):
    raw=fixture();data=gzip.compress(raw) if compressed else raw
    result=substituted_bytes(data,{'minecraft:bush':'minecraft:fern'})
    decoded=gzip.decompress(result) if compressed else result
    root=NBTReader(decoded,endian='big').read_root()
    expected=NBTReader(raw,endian='big').read_root()
    expected['Blocks']['Palette']={'minecraft:air':0,'minecraft:fern':1}
    assert root==expected
    # Restoring the name restores the original byte stream, including tag widths.
    back=substituted_bytes(result,{'minecraft:fern':'minecraft:bush'})
    assert (gzip.decompress(back) if compressed else back)==raw


def test_collision_or_missing_key_rejected():
    for replacements in ({'minecraft:bush':'minecraft:air'},{'minecraft:unknown':'minecraft:fern'}):
        with pytest.raises(ParseError):substituted_bytes(fixture(),replacements)


def test_copy_does_not_overwrite_original_or_existing_files(tmp_path):
    source=tmp_path/'source.schem';source.write_bytes(fixture())
    replacements={'minecraft:bush':'minecraft:fern'}
    with pytest.raises(ValueError):write_copy(source,source,replacements)
    target=tmp_path/'compatible.schem'
    write_copy(source,target,replacements)
    with pytest.raises(FileExistsError):write_copy(source,target,replacements)
    assert source.read_bytes()==fixture()
