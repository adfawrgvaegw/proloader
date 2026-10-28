"""Fatalder-style fresh subchunk comparison with durable, bounded fix rounds."""
import time
import numpy as np
from proloader.game_ui import ProgressMeter
from proloader.verification import ordered_coords,check_subchunk
from proloader.pacing import transport_pressure
from proloader.fatalder import skipped_names
from proloader.block_names import cell_states


async def repair_building(plan,session,world,job,save,move,update,pacer,*,chunk=None):
    excluded=[i for i,b in enumerate(plan.structure.palette) if b.state.name in skipped_names(plan.structure)]
    region=plan.structure.region.blocks
    offset=np.zeros(3,dtype=int)
    if chunk is not None:
        cx,cz=chunk
        lx,lz=max(0,cx*16-plan.origin[0]),max(0,cz*16-plan.origin[2])
        ex,ez=min(plan.structure.size[0],(cx+1)*16-plan.origin[0]),min(plan.structure.size[2],(cz+1)*16-plan.origin[2])
        if lx>=ex or lz>=ez:
            job['checked']=job['passed']=job['verification_total']=0
            job['local_repair_empty']=True
            update('manual_repair_check',detail='当前位置块与本建筑没有重合，无需修复')
            return True
        job['local_repair_empty']=False
        region=region[lx:ex,:,lz:ez];offset=np.asarray((lx,0,lz))
    coords=ordered_coords(region,plan.structure.palette.air_id,tuple(np.asarray(plan.origin)+offset),excluded)+offset
    world._scan_subchunks={}
    all_pending=job.setdefault('repair_pending',{})
    if chunk is None:
        pending=all_pending;pending.clear()
    else:
        # A local scan cannot clear an unverified range outside its column.
        scoped={key:c for key,c in all_pending.items() if
                c[1]//16==c[4]//16==chunk[0] and c[3]//16==c[6]//16==chunk[1]}
        pending=dict(scoped);local_keys=set(scoped);persist=save
        def save():
            for key in local_keys:all_pending.pop(key,None)
            all_pending.update(pending);local_keys.update(pending)
            persist()
    first_round=len(job.get('repair_rounds',[]))
    # A fresh scan on each repair discovers damage since the last import too.
    meter=ProgressMeter(len(coords));current=None
    job['checked']=job['passed']=0;job['verification_total']=len(coords)
    async def near(point):
        nonlocal current
        guard=getattr(world,'assert_local_repair_current',None)
        if guard is not None:guard()
        tile=(point[0]//16,point[2]//16)
        physical=getattr(world,'_last_import_position',None)
        if chunk is not None and physical is not None and (physical[0]//16,physical[2]//16)==tile:
            current=tile;world._verification_chunk=tile
            return
        if current!=tile:
            await move(world,(tile[0]*16+8,point[1]+1,tile[1]*16+8));current=tile
            # move() already confirmed this column. Do not teleport a second
            # time inside check_subchunk, which otherwise sees a cleared marker.
            world._verification_chunk=tile
    cursor=0;saved_at=time.monotonic()
    while cursor<len(coords):
        absolute=coords[cursor]+np.asarray(plan.origin);tile=absolute//16
        end=cursor+1
        while end<len(coords) and np.array_equal((coords[end]+np.asarray(plan.origin))//16,tile):end+=1
        points=[]
        for local in coords[cursor:end]:
            lx,ly,lz=map(int,local);b=plan.structure.palette.get(int(plan.structure.region.blocks[lx,ly,lz]))
            points.append((*tuple(int(a+b) for a,b in zip(local,plan.origin)),b.state.name,cell_states(plan.structure,(lx,ly,lz))))
        await near(points[0])
        update('manual_repair_check',detail=('只扫描当前位置块的建筑方块' if chunk is not None else '按子区块扫描实际差异，尚未重发建筑'))
        values=await check_subchunk(world,points)
        for point,good in zip(points,values):
            x,y,z,name,states=point;key=f'block:{x},{y},{z}'
            if not good:pending[key]=['setblock',x,y,z,x,y,z,name,states]
            else:pending.pop(key,None)
        cursor=end;job['checked']=cursor;job['passed']+=sum(values)
        job['phase_telemetry']=meter.read(cursor)
        if time.monotonic()-saved_at>=2:save();saved_at=time.monotonic()
    if chunk is not None:
        # The complete local scan now supersedes old range suspicions. Keep
        # them durable until here: an unreadable scan must not erase evidence.
        for key in scoped:
            if not key.startswith('block:'):pending.pop(key,None)
    save()
    for round_number in range(1,4):
        if not pending:break
        work=list(pending.items());writes=coalesce_repairs(plan,work,chunk=chunk)
        r=dict(round=round_number,commands=len(writes),differences=len(work),sent=0,kind='differential')
        job.setdefault('repair_rounds',[]).append(r);meter=ProgressMeter(len(writes))
        update('repairing',detail=f'只补实际差异 · 第{round_number}/3轮 · {len(work)}处')
        for c in writes:
            guard=getattr(world,'assert_local_repair_current',None)
            if guard is not None:guard()
            if not getattr(getattr(world,'chunk_window',None),'stationary',False):
                await near(c[1:4])
            await transport_pressure(session);await pacer.wait()
            await world.place(c)
            r['sent']+=1;job['phase_telemetry']=meter.read(r['sent'])
            if r['sent']%256==0:await world.send('testfor @s');save()
        await world.send('testfor @s');save()
        current=None  # Placement's virtual command frame is not bot location.
        world._scan_subchunks={}
        # Recheck only affected cubes; every pending coordinate must be observed.
        grouped={}
        for key,c in work:grouped.setdefault(tuple(v//16 for v in c[1:4]),[]).append((key,c))
        job['repair_checked']=0;job['repair_check_total']=len(work)
        update('repair_verifying',detail='读回补全区块，逐项确认差异已消失')
        for group in grouped.values():
            await near(group[0][1][1:4]);points=[(*c[1:4],c[7],c[8]) for _,c in group]
            good=await check_subchunk(world,points)
            for (key,_),passed in zip(group,good):
                if passed:pending.pop(key,None)
            job['repair_checked']+=len(group)
            update('repair_verifying',detail='读回补全区块，逐项确认差异已消失')
            save()
        r['remaining']=len(pending)
    job['verification']=('local_chunk_verified' if chunk is not None else 'all_block_names') if not pending else 'differences_remaining'
    job['checked']=len(coords);job['passed']=len(coords)-len(pending)
    job.setdefault('manual_repairs',[]).append(dict(kind='local_chunk' if chunk is not None else 'differential',chunk=list(chunk) if chunk is not None else None,commands=sum(
        r['sent'] for r in job.get('repair_rounds',[])[first_round:] if r.get('kind')=='differential'),remaining=len(pending)))
    save()
    return not pending


def coalesce_repairs(plan, work,*,chunk=None):
    """Reuse disjoint upstream fill commands that cover actual differences.

    Rewrites stay inside the validated source footprint; no bounding-box fill
    crosses air holes or mixes states. Native order places supports first.
    """
    buckets={}
    for key,c in work:
        buckets.setdefault((c[1]//48,c[3]//48),[]).append((key,c))
    writes=[];covered=set()
    for c in plan.commands:
        if chunk is not None:
            c=list(c)
            c[1]=max(c[1],chunk[0]*16);c[4]=min(c[4],chunk[0]*16+15)
            c[3]=max(c[3],chunk[1]*16);c[6]=min(c[6],chunk[1]*16+15)
            if c[1]>c[4] or c[3]>c[6]:continue
            c=tuple(c)
        hit=[]
        for bx in range(c[1]//48,c[4]//48+1):
            for bz in range(c[3]//48,c[6]//48+1):
                for key,p in buckets.get((bx,bz),[]):
                    if key not in covered and all(c[i]<=p[i]<=c[i+3] for i in (1,2,3)):
                        hit.append(key)
        if hit:
            covered.update(hit);writes.append(c)
    writes.extend(c for key,c in work if key not in covered and (chunk is None or
        (c[1]//16==c[4]//16==chunk[0] and c[3]//16==c[6]//16==chunk[1])))
    return writes
