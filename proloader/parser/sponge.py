"""Strict Sponge .schem v2/v3 reader with explicit Java→Bedrock palette conversion."""
from __future__ import annotations
import gzip
import numpy as np

from proloader.errors import ParseError
from proloader.model.block import Block, BlockState
from proloader.model.palette import BlockPalette
from proloader.model.region import Region
from proloader.model.structure import Structure
from .base import register_parser
from .nbt import NBTReader
from .java_mapper import map_java_palette, REVISION


def decode_indices(data: bytes, count: int) -> np.ndarray:
    values = []
    value = shift = 0
    for byte in data:
        value |= (byte & 127) << shift
        if byte & 128:
            shift += 7
            if shift >= 35:
                raise ParseError('BlockData 中的 VarInt 超长。')
        else:
            values.append(value)
            value = shift = 0
    if shift or len(values) != count:
        raise ParseError(f'建筑数据不完整：期望 {count} 个方块，实际 {len(values)}。')
    return np.asarray(values, dtype=np.uint32)


@register_parser('.schem')
def parse_sponge(data: bytes, name: str = 'structure') -> Structure:
    if data[:2] == b'\x1f\x8b':
        try:
            data = gzip.decompress(data)
        except (OSError, EOFError):
            raise ParseError('建筑 gzip 数据损坏。') from None
    root = NBTReader(data, endian='big').read_root()
    root = root.get('Schematic', root)
    version = root.get('Version')
    if version not in (2, 3):
        raise ParseError(f'目前支持 Sponge .schem v2/v3，实际版本 {version}。')
    try:
        sx, sy, sz = (int(root[k]) for k in ('Width', 'Height', 'Length'))
        if min(sx, sy, sz) <= 0 or sx * sy * sz > 64_000_000:
            raise ValueError
        section = root['Blocks'] if version == 3 else root
        palette_raw = section['Palette']
        indices = decode_indices(section['Data' if version == 3 else 'BlockData'], sx * sy * sz)
        if set(map(int, np.unique(indices))) - set(palette_raw.values()):
            raise ParseError('建筑数据引用了调色板中不存在的方块。')
        mapped = map_java_palette(list(palette_raw))
        palette = BlockPalette()
        remap = np.zeros(max(palette_raw.values()) + 1, dtype=np.uint32)
        for source, index in palette_raw.items():
            item = mapped[source]
            remap[index] = palette.add(Block(BlockState(item['name'], item.get('states') or {})))
        blocks = remap[indices].reshape((sy, sz, sx)).transpose(2, 0, 1).copy()
        entities = {}
        for entity in section.get('BlockEntities', []):
            pos = entity.get('Pos')
            if isinstance(pos, list) and len(pos) == 3:
                entities[tuple(map(int, pos))] = entity
        metadata = {
            'source_format': f'Sponge v{version}', 'java_data_version': root.get('DataVersion'),
            'mapper': f'Yeah114/blocks@{REVISION}', 'source_palette_count': len(mapped),
            'unknown_blocks': 0, 'block_entity_count': len(entities),
            'warnings': ([f'{len(entities)} 个方块实体的附加数据尚未写入（包括旗帜图案）；基础方块仍会放置。'] if entities else []),
        }
        structure = Structure(name, (sx, sy, sz), palette, Region(palette, blocks),
                              block_entities=entities, format_version=str(version))
        structure.source_info = metadata
        return structure
    except (KeyError, TypeError, ValueError, IndexError) as exc:
        raise ParseError('Sponge 建筑结构或调色板无效。') from exc
