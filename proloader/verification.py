"""Bounded all-name verification: spatial locality and conjunction probes.

A positive conjunction proves every member; negative/unknown groups are split
down to the original single-block check. No sampling replaces full coverage.
"""
import asyncio
import numpy as np
import json

REVISION = 'fresh-subchunk-v6-paired-doors'
BATCH = 32
WINDOW = 8


def exact_source(world):
    structure=getattr(getattr(world,'plan',None),'structure',None)
    return bool(getattr(structure,'source_info',{}).get('exact_states'))


def ordered_coords(region, air_id, origin=(0,0,0), excluded_ids=None):
    coords = np.argwhere(~np.isin(region,excluded_ids) if excluded_ids is not None else region != air_id)
    if len(coords):
        absolute = coords + np.asarray(origin)
        order = np.lexsort((coords[:,2], coords[:,1], coords[:,0],
                            absolute[:,1]//16, absolute[:,2]//16, absolute[:,0]//16))
        coords = coords[order]
    return coords


async def check_group(world, points):
    """Return ordered booleans, retaining single-probe failure semantics."""
    if len(points) == 1:
        x,y,z,name,states = points[0]
        return [await world.check(x,y,z,name,states)]
    from proloader.block_names import verification_state
    from proloader.net.bedrock.session import BedrockSession
    clauses = []
    for x,y,z,name,states in points:
        name, props = verification_state(name, states)
        tile=(BedrockSession._states_to_str(props) if exact_source(world) or
              (name in ('minecraft:stone_block_slab4','minecraft:double_stone_block_slab4')
              and props.get('stone_slab_type_4')=='stone') else '[]')
        clauses.append(f'if block {world.relative(x,y,z)} {name} {tile}')
    try:
        result = await world.send(world.context('execute ' + ' '.join(clauses) + ' run testfor @s'))
        if result is not None:
            return [True] * len(points)
    except Exception as exc:
        from proloader.net.bedrock.session import BedrockLoginError
        if not isinstance(exc, BedrockLoginError) or not str(exc).startswith('aiCommand 执行失败'):
            raise
    mid = len(points)//2
    return (await check_group(world, points[:mid])) + (await check_group(world, points[mid:]))


async def check_window(world, points):
    groups = [points[i:i+BATCH] for i in range(0,len(points),BATCH)]
    if len(groups) > WINDOW:
        raise ValueError('Verification window exceeds bounded concurrency')
    tasks = [asyncio.create_task(check_group(world, group)) for group in groups]
    try:
        return [v for result in await asyncio.gather(*tasks) for v in result]
    finally:
        for task in tasks:
            if not task.done():task.cancel()
        await asyncio.gather(*tasks,return_exceptions=True)


async def check_subchunk(world, points):
    """Prove known runtime names from one fresh cube; probe unknowns strictly.

    Runtime IDs are resolved by the pinned upstream block library. No unknown
    ID, rejected cube, decode error or missing name is treated as a match.
    """
    from proloader.block_names import verification_state
    if not points:return []
    cube=tuple(v//16 for v in points[0][:3])
    if any(tuple(v//16 for v in p[:3])!=cube for p in points):
        raise ValueError('核对批次必须位于同一子区块。')
    horizontal=(cube[0],cube[2])
    if hasattr(world,'teleport') and getattr(world,'_verification_chunk',None)!=horizontal:
        await world.teleport(cube[0]*16+8,points[0][1]+1,cube[2]*16+8)
        world._verification_chunk=horizontal
    entry=None
    for attempt in range(3):
        try:
            cache=getattr(world,'_scan_subchunks',None)
            if cache is not None and hasattr(world.session,'read_subchunks'):
                if cube not in cache:
                    cache.clear()  # One horizontal chunk, never a whole-world cache.
                    low=world.plan.origin[1]//16
                    high=(world.plan.origin[1]+world.plan.structure.size[1]-1)//16
                    entries=await world.session.read_subchunks(points[0][:3],world.plan.dimension,range(low,high+1),allow_partial=True)
                    for e in entries:cache[(cube[0],e['offset'][1],cube[2])]=e
                entry=cache[cube]
            else:
                entry=await world.session.read_subchunk(points[0][:3],world.plan.dimension)
            break
        except (asyncio.TimeoutError,ValueError):
            world.session._raise_if_failed()
            if attempt==2:
                # Unsupported/unavailable cube reads fall back to independent
                # server predicates, never an assumed match or a task abort.
                return [v for start in range(0,len(points),BATCH*WINDOW)
                        for v in await check_window(world,points[start:start+BATCH*WINDOW])]
            if hasattr(world,'teleport'):
                x,y,z=points[0][:3]
                await world.teleport(x,y+1,z)
            await asyncio.sleep(.15*(attempt+1))
    if entry.get('result',1)!=1:
        if entry.get('result')!=6 and hasattr(world.session,'read_subchunk'):
            for attempt in range(3):
                try:
                    if hasattr(world,'teleport'):
                        x,y,z=points[0][:3];await world.teleport(x,y+1,z)
                        world._verification_chunk=horizontal
                    entry=await world.session.read_subchunk(points[0][:3],world.plan.dimension)
                    if entry.get('result',1) in (1,6):
                        if getattr(world,'_scan_subchunks',None) is not None:world._scan_subchunks[cube]=entry
                        break
                except (ValueError,asyncio.TimeoutError):world.session._raise_if_failed()
                if attempt<2:await asyncio.sleep(.15*(attempt+1))
        if entry.get('result')==6:
            return [p[3]=='minecraft:air' for p in points]
        if entry.get('result',1)!=1:
            return [v for start in range(0,len(points),BATCH*WINDOW)
                    for v in await check_window(world,points[start:start+BATCH*WINDOW])]
    ids=entry.get('blocks',[]);palette=entry.get('palette',{})
    hashes=await world.runtime_hashes() if hasattr(world,'runtime_hashes') else {}
    results=[False]*len(points);unresolved=[]
    for i,(x,y,z,name,states) in enumerate(points):
        cell=((x%16)*16+y%16)*16+z%16
        if len(ids)==4096 and ids[cell] in hashes.get(json.dumps([name,states],sort_keys=True),set()):
            results[i]=True
            continue
        observed=palette.get(str(ids[cell])) if len(ids)==4096 else None
        if observed and observed.get('name')=='minecraft:air' and name!='minecraft:air':
            continue  # A decoded air cell is a proven gap, no command round trip.
        if observed and observed.get('name'):
            expected,props=verification_state(name,states)
            actual,actual_props=verification_state(observed['name'],observed.get('states'))
            if expected==actual and (not exact_source(world) or all(actual_props.get(k)==v for k,v in props.items())):
                results[i]=True
            continue  # A registered different name is already a proven difference.
        unresolved.append(i)
    # Confirm apparent differences through server predicates. This also handles
    # version-specific aliases without trusting unknown runtime IDs.
    for start in range(0,len(unresolved),BATCH*WINDOW):
        indices=unresolved[start:start+BATCH*WINDOW]
        values=await check_window(world,[points[i] for i in indices])
        for i,value in zip(indices,values):results[i]=value
    return results
