"""Offline MCWorld layer-zero reader using Fatalder's world dependency.

Read an extracted copy and keep dense indices in a memory-mapped file. Entity
NBT and secondary liquid layers are reported explicitly, not claimed imported.
"""
import hashlib
import json
from pathlib import Path
import subprocess
from zipfile import ZipFile

import numpy as np
from proloader.native_build import go_build_environment
from proloader.model.block import Block, BlockState
from proloader.model.palette import BlockPalette
from proloader.model.region import Region
from proloader.model.structure import Structure
from proloader.errors import ParseError


def load_mcworld(path, name=None):
    path=Path(path).resolve()
    root=Path(__file__).resolve().parents[2]
    source=root/'third_party/fatalder'
    helper=source/'cmd/proloader-world/main.go'
    revision=hashlib.sha256(helper.read_bytes()+(source/'go.sum').read_bytes()).hexdigest()
    digest=hashlib.sha256(path.read_bytes()).hexdigest()
    folder=root/'cache'/'mcworld'/(digest+'-'+revision[:12])
    folder.mkdir(parents=True,exist_ok=True)
    prefix=folder/'structure'
    metadata=prefix.with_suffix('.json')
    binary=root/'bin'/'fatalder-world'
    stamp=binary.with_suffix('.build.json')
    if not metadata.exists():
        if not binary.exists() or not stamp.exists() or json.loads(stamp.read_text()).get('revision')!=revision:
            binary.parent.mkdir(exist_ok=True)
            built=subprocess.run(['go','build','-o',str(binary),'./cmd/proloader-world'],cwd=source,capture_output=True,text=True,timeout=240,env=go_build_environment())
            if built.returncode:raise ParseError('世界读取器编译失败：'+built.stderr[:300])
            stamp.write_text(json.dumps({'revision':revision}))
        extracted=folder/'world'
        extracted.mkdir(exist_ok=True)
        with ZipFile(path) as archive:
            if sum(i.file_size for i in archive.infolist())>4*1024**3:
                raise ParseError('世界解压后超过4GB，请先划定导出范围。')
            for entry in archive.infolist():
                output=(extracted/entry.filename).resolve()
                if not output.is_relative_to(extracted.resolve()) or ((entry.external_attr>>16)&0o170000)==0o120000:
                    raise ParseError('世界压缩包包含不安全路径。')
            archive.extractall(extracted)
        result=subprocess.run([str(binary),str(extracted),str(prefix)],capture_output=True,text=True,timeout=600)
        if result.returncode:raise ParseError('世界读取失败：'+result.stderr[-400:])
    info=json.loads(metadata.read_text())
    size=tuple(info['size'])
    raw=prefix.with_suffix('.bin')
    if raw.stat().st_size!=int(np.prod(size))*4:raise ParseError('世界索引文件大小不匹配。')
    palette=BlockPalette.from_list([Block(BlockState(s['name'],s['states'])) for s in info['palette']])
    blocks=np.memmap(raw,dtype='<u4',mode='r',shape=size)
    warnings=[]
    entities = {}
    for nbt in info.get('block_entity_data', []):
        local = tuple(int(nbt[k])-offset for k, offset in zip(('x','y','z'), info['source_min']))
        if all(0 <= a < b for a, b in zip(local, size)):
            if local in entities:raise ParseError('世界包含重复位置的方块实体。')
            entities[local] = nbt
    if info['block_entities']:warnings.append(f"{info['block_entities']} 个方块实体的物品/文字等NBT尚未写入。")
    if info['secondary_blocks']:warnings.append(f"{info['secondary_blocks']} 个次层液体尚未导入；当前只放置主层。")
    return Structure(name or path.stem,size,palette,Region(palette,blocks),block_entities=entities,
                     source_info=dict(warnings=warnings,blocks_file=str(raw),exact_states=True,
                                      source_min=info['source_min'],reader_revision=revision,source_sha256=digest))
