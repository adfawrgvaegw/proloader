"""Bounded all-name verification: spatial locality and conjunction probes.

A positive conjunction proves every member; negative/unknown groups are split
down to the original single-block check. No sampling replaces full coverage.
"""
import asyncio
import numpy as np

REVISION = 'spatial-conjunction-v1'
BATCH = 32
WINDOW = 8


def ordered_coords(region, air_id):
    coords = np.argwhere(region != air_id)
    if len(coords):
        order = np.lexsort((coords[:,2], coords[:,1], coords[:,0],
                            coords[:,2]//16, coords[:,0]//16))
        coords = coords[order]
    return coords


async def check_group(world, points):
    """Return ordered booleans, retaining single-probe failure semantics."""
    if len(points) == 1:
        x,y,z,name,states = points[0]
        return [await world.check(x,y,z,name,states)]
    from proloader.block_names import command_state
    clauses = []
    for x,y,z,name,states in points:
        name, _ = command_state(name, states)
        clauses.append(f'if block {world.relative(x,y,z)} {name} []')
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
