"""Direct native Fatalder planner adapter; no game connection ownership."""
from pathlib import Path
import hashlib
import json
import re
import subprocess
from proloader.native_build import go_build_environment

FRAGILE = ('torch','ladder','button','lever','banner','flower','sapling','carpet',
           'tripwire','coral','lantern','end_rod','bell','bed','door','vine','bamboo')
PROPERTY = re.compile(r'("(?:[^"\\]|\\.)*")=("(?:[^"\\]|\\.)*"|true|false|-?\d+)')


def parse_command(text):
    parts=text.split(' ',8 if text.startswith('fill ') else 5)
    fill=parts[0]=='fill'
    if parts[0] not in ('fill','setblock'):
        raise ValueError('原生规划器返回非放置命令')
    count=6 if fill else 3
    xyz=list(map(int,parts[1:1+count]))
    if not fill:xyz+=xyz
    state_text=parts[2+count]
    if not state_text.startswith('[') or not state_text.endswith(']'):
        raise ValueError('原生方块状态格式无效')
    properties=state_text[1:-1]
    matches=list(PROPERTY.finditer(properties))
    if ','.join(m.group(0) for m in matches)!=properties:
        raise ValueError('原生方块状态无法无损读取')
    states={json.loads(m[1]):json.loads(m[2]) for m in matches}
    return (parts[0],*xyz,parts[1+count],states)


def native_plan(structure, origin, binary):
    palette=[dict(name=b.state.name,states=b.state.states) for b in structure.palette]
    request=dict(size=structure.size,origin=origin,palette=palette)
    if structure.source_info.get('blocks_file'):
        request['blocks_file']=structure.source_info['blocks_file']
        request['exact_states']=bool(structure.source_info.get('exact_states'))
    else:
        request['blocks']=structure.region.blocks.flatten().tolist()
    result=subprocess.run([str(binary)],input=json.dumps(request),text=True,capture_output=True,timeout=600)
    if result.returncode:
        raise ValueError('Fatalder 原生规划失败：'+result.stderr[:300])
    payload=json.loads(result.stdout)
    batches=payload['batches']
    commands=[]
    for fragile in (False,True):
        for batch in batches:
            selected=[parse_command(c) for c in batch['commands']]
            selected=[c for c in selected if any(s in c[7] for s in FRAGILE)==fragile]
            selected.sort(key=lambda c:(c[2],c[1],c[3]))
            commands.extend(selected)
    # Reject any native widening, air overwrite, duplicate, or invalid volume before connecting.
    validate_coverage(structure,origin,commands,payload['palette'])
    return commands


def validate_coverage(structure,origin,commands,canonical=None):
    import numpy as np
    seen=np.zeros(structure.size,dtype=bool)
    skipped={'minecraft:air','minecraft:structure_void','minecraft:structure_block'}
    for c in commands:
        _,x,y,z,ex,ey,ez,name,states=c
        lo=[x-origin[0],y-origin[1],z-origin[2]]
        hi=[ex-origin[0],ey-origin[1],ez-origin[2]]
        if any(a<0 or b<a or b>=s for a,b,s in zip(lo,hi,structure.size)):
            raise ValueError('Fatalder 规划超出建筑范围')
        if (ex-x+1)*(ey-y+1)*(ez-z+1)>32768:
            raise ValueError('Fatalder fill 超过32768')
        region=tuple(slice(a,b+1) for a,b in zip(lo,hi))
        if seen[region].any():raise ValueError('Fatalder 规划重复覆盖')
        seen[region]=True
        for bid in np.unique(structure.region.blocks[region]):
            block=structure.palette.get(int(bid))
            state=canonical[int(bid)] if canonical else dict(name=block.state.name,states=block.state.states)
            if state['name']!=name or state['states']!=states:
                raise ValueError(f'Fatalder 规划改变目标方块：{block.state.name} → {name}')
    skipped_ids=[bid for bid,block in enumerate(structure.palette) if block.state.name in skipped]
    expected=np.isin(structure.region.blocks,skipped_ids,invert=True)
    if not np.array_equal(seen,expected):raise ValueError('Fatalder 规划遗漏目标方块')


def native_binary(target='plan'):
    import os
    if target not in ('plan', 'nbt'):
        raise ValueError('未知原生助手')
    root=Path(__file__).resolve().parents[1]
    source=root/'third_party/fatalder'
    binary=root/f'bin/fatalder-{target}'
    stamp=root/f'bin/fatalder-{target}.build.json'
    digest=hashlib.sha256()
    for path in sorted(source.rglob('*')):
        if path.is_file() and (path.suffix=='.go' or path.name in ('go.mod','go.sum')):
            digest.update(str(path.relative_to(source)).encode())
            digest.update(path.read_bytes())
    version=digest.hexdigest()
    if not binary.exists() or not stamp.exists() or json.loads(stamp.read_text()).get('source_sha256')!=version:
        binary.parent.mkdir(exist_ok=True)
        result=subprocess.run(['go','build','-o',str(binary),f'./cmd/proloader-{target}'],cwd=source,
                              text=True,capture_output=True,timeout=240,env=go_build_environment())
        if result.returncode:raise RuntimeError('Fatalder原生引擎编译失败：'+result.stderr[:500])
        stamp.write_text(json.dumps(dict(source_sha256=version)))
    return binary
