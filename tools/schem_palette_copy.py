"""Create an explicit palette-substituted Sponge copy without reserializing NBT.

Palette IDs and all other bytes (block data, entities, offsets, tag widths) stay
unchanged. Colliding destination keys are rejected rather than losing IDs.
"""
import gzip
import struct
from pathlib import Path

from proloader.errors import ParseError
from proloader.parser.nbt import NBTReader


def substituted_bytes(data, replacements):
    compressed=data[:2]==b'\x1f\x8b'
    raw=gzip.decompress(data) if compressed else data
    root=NBTReader(raw,endian='big').read_root()
    root=root.get('Schematic',root)
    if root.get('Version') not in (2,3):raise ParseError('Only Sponge v2/v3 is supported.')
    section=root['Blocks'] if root['Version']==3 else root
    palette=section['Palette']
    if not replacements or set(replacements)-set(palette):raise ParseError('Missing source palette key.')
    targets=[replacements.get(key,key) for key in palette]
    if len(set(targets))!=len(targets):raise ParseError('Destination palette keys collide; remapping would be required.')
    def token(name,index):
        encoded=name.encode('utf-8')
        return b'\x03'+struct.pack('>H',len(encoded))+encoded+struct.pack('>i',index)
    edits=[]
    for source,target in replacements.items():
        old=token(source,palette[source])
        if raw.count(old)!=1:raise ParseError('Palette token is not unique; refusing to change unrelated NBT.')
        start=raw.index(old)
        edits.append((start,start+len(old),token(target,palette[source])))
    # Slice the original stream once, so a replacement never matches a later edit.
    parts=[];previous=0
    for start,end,new in sorted(edits):
        if start<previous:raise ParseError('Overlapping palette edits.')
        parts.extend((raw[previous:start],new));previous=end
    parts.append(raw[previous:])
    result=b''.join(parts)
    return gzip.compress(result,mtime=0) if compressed else result


def write_copy(source, destination, replacements):
    source=Path(source);destination=Path(destination)
    if source.resolve()==destination.resolve():raise ValueError('The original file must be preserved.')
    data=substituted_bytes(source.read_bytes(),replacements)
    # Never overwrite an existing user file.
    with destination.open('xb') as stream:stream.write(data)
    return destination
