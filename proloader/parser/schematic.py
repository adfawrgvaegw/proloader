"""MCEdit Alpha .schematic: Java numeric ID/data -> pinned Bedrock states."""
import gzip

import numpy as np

from proloader.errors import ParseError
from proloader.model.block import Block, BlockState
from proloader.model.palette import BlockPalette
from proloader.model.region import Region
from proloader.model.structure import Structure
from .base import register_parser
from .java_mapper import map_schematic_palette, REVISION
from .nbt import NBTReader


@register_parser('.schematic')
def parse_schematic(data: bytes, name: str = 'structure') -> Structure:
    raw=data
    if data[:2]==b'\x1f\x8b':
        try:data=gzip.decompress(data)
        except (OSError, EOFError) as exc:raise ParseError('建筑 gzip 数据损坏。') from exc
    root=NBTReader(data,endian='big').read_root()
    root=root.get('Schematic',root)
    # Some exporters use the old extension for a modern Sponge payload.
    if ('Version' in root and 'Blocks' not in root) or isinstance(root.get('Blocks'),dict):
        from .sponge import parse_sponge
        return parse_sponge(raw,name)
    try:
        if root.get('Materials','Alpha')!='Alpha':
            raise ParseError('仅支持 MCEdit Alpha .schematic，非 Alpha/模组格式不能直接转换。')
        size=tuple(root[k] for k in ('Width','Height','Length'))
        if any(not isinstance(v,int) or isinstance(v,bool) or v<=0 for v in size) or np.prod(size,dtype=object)>64_000_000:
            raise ParseError('Schematic 尺寸无效或超过6400万格。')
        sx,sy,sz=map(int,size);count=sx*sy*sz
        for field in ('Blocks','Data'):
            if not isinstance(root[field],bytes) or len(root[field])!=count:
                raise ParseError(f'Schematic {field} 长度不符：期望{count}。')
        ids=np.frombuffer(root['Blocks'],dtype=np.uint8).astype(np.uint16)
        meta=np.frombuffer(root['Data'],dtype=np.uint8)
        high_metadata_count=int(np.count_nonzero(meta>15))
        # Match Fatalder's import path: blockData & 0xF. High metadata bits
        # must never bleed into the packed block ID used below.
        meta=meta & 0x0F
        if 'AddBlocks' in root:
            extra=root['AddBlocks']
            if not isinstance(extra,bytes) or len(extra)!=(count+1)//2:
                raise ParseError('Schematic AddBlocks 长度不符。')
            extra=np.frombuffer(extra,dtype=np.uint8)
            ids[0::2] |= (extra[:len(ids[0::2])].astype(np.uint16)&15)<<8
            ids[1::2] |= (extra[:len(ids[1::2])].astype(np.uint16)>>4)<<8
        codes=(ids<<4)|meta
        unique=np.unique(codes)
        pairs=[(int(v)>>4,int(v)&15) for v in unique]
        mapped=map_schematic_palette(pairs)
        palette=BlockPalette();remap=np.zeros(65536,dtype=np.uint32)
        for code,(bid,value) in zip(unique,pairs):
            item=mapped[f'{bid}:{value}']
            remap[code]=palette.add(Block(BlockState(item['name'],item.get('states') or {})))
        blocks=remap[codes].reshape((sy,sz,sx)).transpose(2,0,1).copy()
        entities={}
        for entity in root.get('TileEntities',[]):
            pos=tuple(entity[k] for k in ('x','y','z'))
            if any(not isinstance(v,int) or isinstance(v,bool) or not 0<=v<s for v,s in zip(pos,(sx,sy,sz))) or pos in entities:
                raise ParseError('Schematic 方块实体坐标无效或重复。')
            entities[pos]=entity
        warnings=[]
        if high_metadata_count:
            warnings.append(f'{high_metadata_count}处Data包含高位，已按MCEdit附加值低4位规则兼容读取。')
        if entities:warnings.append(f'{len(entities)}个Java方块实体附加数据保留，但未转换为基岩NBT。')
        if root.get('Entities'):warnings.append(f'{len(root["Entities"])}个Java实体未导入。')
        structure=Structure(name,(sx,sy,sz),palette,Region(palette,blocks),block_entities=entities,format_version='Alpha')
        structure.source_info=dict(source_format='MCEdit Alpha',source_edition='java',
            mapper=f'Yeah114/blocks@{REVISION}/schematic-v2-low-nibble',source_palette_count=len(pairs),
            normalized_metadata_count=high_metadata_count,
            unknown_blocks=0,block_entity_count=len(entities),warnings=warnings,
            worldedit_offset=[root.get(k,0) for k in ('WEOffsetX','WEOffsetY','WEOffsetZ')])
        return structure
    except (KeyError,TypeError,ValueError,IndexError,OverflowError) as exc:
        raise ParseError('MCEdit Schematic 结构无效或数据不完整。') from exc
