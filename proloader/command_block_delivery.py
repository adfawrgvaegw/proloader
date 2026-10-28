"""Reach-bounded staging with fresh readback and recoverable, durable batches."""
import asyncio
import time

from proloader.pacing import transport_pressure
from proloader.net.bedrock.session import BedrockLoginError

ATTEMPTS = 3
BATCH = 64


def cube(position):
    return tuple(v // 16 for v in position)


def reachable(anchor, position):
    # Same conservative radius as upstream CanReachOrMove (squared distance 36).
    return sum((a-b)**2 for a,b in zip(anchor,position)) <= 36


async def read_batch(session, blocks, dimension):
    if not hasattr(session, 'read_subchunk'):
        return [await session.read_block_entity(b.position,dimension) for b in blocks]
    entry = await session.read_subchunk(blocks[0].position,dimension)
    result=[]
    for block in blocks:
        matches=[n for n in entry['nbt'] if tuple(n.get(k) for k in ('x','y','z'))==block.position]
        if len(matches)>1 or (matches and '_decode_error' in matches[0]):
            raise ValueError('目标命令方块NBT缺失、重复或解码失败。')
        observed=dict(matches[0]) if matches else {}
        observed.pop('_shell_match',None)  # Only decoded palette evidence may set this marker.
        ids=entry.get('blocks',[])
        if len(ids)==4096:
            x,y,z=block.position
            state=entry.get('palette',{}).get(str(ids[((x%16)*16+y%16)*16+z%16]))
            if state:
                observed['_shell_match']=(state['name']==block.block_name and all(
                    state.get('states',{}).get(k)==v for k,v in block.states.items()))
        result.append(observed)
    return result


async def stage_command_blocks(blocks, session, world, dimension, job, save, move, update):
    index=job.get('command_blocks_next',0)
    started=time.monotonic()
    metrics=job.setdefault('command_blocks_metrics',dict(moves=0,move_seconds=0.,reads=0,read_seconds=0.,updates=0))
    original_move=move
    async def move(w,position):
        tick=time.monotonic()
        try:return await original_move(w,position)
        finally:
            metrics['moves']+=1;metrics['move_seconds']+=time.monotonic()-tick
    async def read(group):
        tick=time.monotonic()
        try:return await read_batch(session,group,dimension)
        finally:
            metrics['reads']+=1;metrics['read_seconds']+=time.monotonic()-tick
    while index<len(blocks):
        guard=getattr(world,'assert_local_repair_current',None)
        if guard is not None:guard()
        anchor=blocks[index].position
        player_anchor=(anchor[0],anchor[1]+1,anchor[2])
        end=index+1
        while end<len(blocks) and end-index<BATCH and cube(blocks[end].position)==cube(anchor):
            positions=[b.position for b in blocks[index:end+1]]
            centre=tuple((min(p[i] for p in positions)+max(p[i] for p in positions))/2 for i in range(3))
            candidate=(centre[0],centre[1]+1,centre[2])
            eye=(candidate[0],candidate[1]+1.62,candidate[2])
            if not all(reachable(candidate,p) and reachable(eye,p) for p in positions):break
            player_anchor=candidate
            end+=1
        group=blocks[index:end]
        loader=getattr(world,'chunk_window',None)
        if loader is not None:
            await loader.ensure(('setblock',*anchor,*anchor,group[0].block_name,group[0].states))
        await move(world,player_anchor)
        for attempt in range(ATTEMPTS):
            try:
                old=await read(group)
                repaired_shell=False
                # Shell repair is deliberate and limited to the source footprint.
                for offset,block in enumerate(group):
                    shell=old[offset].get('_shell_match')
                    if shell is None:shell=await world.check_command_block(block)
                    if not shell:
                        update('command_block_repair',detail=f'修复命令方块外壳 {index+1}/{len(blocks)} · {attempt+1}/{ATTEMPTS}')
                        job['command_blocks_inflight']=index;save()
                        x,y,z=block.position
                        await world.place(('setblock',x,y,z,x,y,z,block.block_name,block.states),wait=True)
                        if not await world.check_command_block(block):
                            raise ValueError('命令方块外壳修复后仍未确认。')
                        old[offset]={}  # Replaced shells require their source NBT.
                        repaired_shell=True
                if repaired_shell:
                    # A construction window may reposition the bot to its
                    # centre. Packet-based edits must return within reach.
                    await move(world,player_anchor)
                changed=[b for b,n in zip(group,old) if b.differences(n)]
                if changed:
                    job['command_blocks_inflight']=index;save()
                    for block in changed:
                        if guard is not None:guard()
                        await transport_pressure(session)
                        await session.send_packet(block.packet())
                        metrics['updates']+=1
                    await world.send('testfor @s')
                    observed=await read(group)
                else:
                    observed=old
                differences=[]
                for offset,(block,nbt) in enumerate(zip(group,observed)):
                    fields=block.differences(nbt)
                    shell=nbt.get('_shell_match')
                    if shell is None:shell=await world.check_command_block(block)
                    if not shell:fields.append('block_states')
                    if fields:differences.append(dict(index=index+offset,position=list(block.position),fields=fields))
                if differences:
                    job['command_block_mismatch']=differences[0];save()
                    raise ValueError('命令方块内部数据读回不一致。')
                break
            except (ValueError,asyncio.TimeoutError,BedrockLoginError) as exc:
                # Disconnect is terminal to this connection; never turn it into a
                # mismatch or blindly replay. Lost reads get a fresh read first.
                session._raise_if_failed()
                if isinstance(exc,BedrockLoginError) and not str(exc).startswith(('aiCommand 执行失败','服务器命令处理确认未收到')):
                    raise
                job.setdefault('command_blocks_recoveries',[]).append(dict(index=index,attempt=attempt+1,error_type=type(exc).__name__))
                save()
                if attempt+1==ATTEMPTS:raise
                update('command_block_repair',detail=f'命令方块重新定位、读回并补写 {index+1}/{len(blocks)} · {attempt+1}/{ATTEMPTS}')
                await move(world,player_anchor)
                await asyncio.sleep(.15*(attempt+1))
        index=end
        job['command_blocks_next']=index
        job.pop('command_blocks_inflight',None);job.pop('command_block_mismatch',None)
        update('command_block_data',detail=f'内部数据已严格读回 {index}/{len(blocks)}')
        save()
    job['command_blocks_seconds']=round(job.get('command_blocks_seconds',0)+time.monotonic()-started,3)
