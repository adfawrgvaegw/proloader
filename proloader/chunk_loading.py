"""Owned temporary ticking window, following Fatalder's waitChunkLoad lifecycle.

No air-keep writes are used to probe readiness. Native reads prove every chunk
in the current source window is available; only this job's area is removed.
"""
import asyncio
import time


class StationaryWindow:
    """One owned ticking area, up to 100 world chunks; no placement teleports.

    A source-aligned 144-block tile touches at most 10x10 world chunks even
    when the source origin is unaligned. Native 48-block fills fit these tiles.
    Smaller command footprints use one rectangle for the entire operation.
    """
    stationary = True
    readiness_attempts = 8

    def __init__(self,world,plan,job,save,drain,update):
        self.world,self.plan,self.job,self.save=world,plan,job,save
        self.drain,self.update=drain,update
        self.name='plimp_'+job['id']
        pending=job.get('loading_area_pending')
        if pending:
            saved_name=pending.get('name','')
            if saved_name!=self.name and not (saved_name.startswith(self.name+'_') and saved_name[len(self.name)+1:].isdigit()):
                raise ValueError('临时加载区名称不属于当前任务。')
            self.name=saved_name
        self.owned=bool(job.get('loading_area_pending'))
        self.key=None;self.full_window=False;self.proven_columns=set()
        self.bounds=(min(c[1] for c in plan.commands),min(c[3] for c in plan.commands),
                     max(c[4] for c in plan.commands),max(c[6] for c in plan.commands)) if plan.commands else None

    @staticmethod
    def column_count(bounds):
        x,z,ex,ez=bounds
        return (ex//16-x//16+1)*(ez//16-z//16+1)

    @staticmethod
    def columns(bounds):
        x,z,ex,ez=bounds
        return {(cx,cz) for cx in range(x//16,ex//16+1) for cz in range(z//16,ez//16+1)}

    @staticmethod
    def contains(bounds,command):
        return (bounds is not None and bounds[0]<=command[1]<=command[4]<=bounds[2]
                and bounds[1]<=command[3]<=command[6]<=bounds[3])

    async def prove(self,bounds,level=None):
        columns=sorted(self.columns(bounds));level=self.plan.origin[1] if level is None else level
        # Positive conjunctions prove every column, including source air. No
        # unavailable native render data, placement writes, or movement needed.
        queries=[]
        for begin in range(0,len(columns),16):
            clauses=[]
            for cx,cz in columns[begin:begin+16]:
                dx=max(cx*16,bounds[0])-bounds[0]
                dz=max(cz*16,bounds[1])-bounds[1]
                pos=f'~{dx} ~ ~{dz}'
                clauses.append(f'if blocks {pos} {pos} {pos} all')
            frame=f'execute in {self.plan.dimension} positioned {bounds[0]} {level} {bounds[1]} run '
            if len(clauses)==1:
                queries.append(frame+f'testforblocks {pos} {pos} {pos} all')
            else:
                queries.append(frame+'execute '+' '.join(clauses)+' run testfor @s')
        for begin in range(0,len(queries),4):
            tasks=[asyncio.create_task(self.world.send(q)) for q in queries[begin:begin+4]]
            try:
                values=await asyncio.gather(*tasks)
                if any(v is None or v is False for v in values):raise ValueError('常加载片区只读确认未收到。')
            finally:
                for task in tasks:
                    if not task.done():task.cancel()
                await asyncio.gather(*tasks,return_exceptions=True)
        self.proven_columns.update(columns)

    async def prove_footprint(self,command):
        from proloader.net.bedrock.session import BedrockLoginError
        bounds=(command[1],command[3],command[4],command[6])
        if self.columns(bounds)<=self.proven_columns:return
        for attempt in range(self.readiness_attempts):
            try:
                await self.prove(bounds,command[2])
                return
            except (ValueError,asyncio.TimeoutError,BedrockLoginError):
                self.world.session._raise_if_failed()
                if attempt+1==self.readiness_attempts:raise
                self.update('loading',detail=f'片区预加载中；等待实际施工列 {attempt+1}/{self.readiness_attempts}')
                await asyncio.sleep(min(3,.25*2**attempt))

    async def ensure(self,command):
        if self.contains(self.key,command):
            if not self.full_window:await self.prove_footprint(command)
            return
        await self.drain()
        await self.release()
        if self.contains(self.bounds,command) and self.column_count(self.bounds)<=100:
            bounds=self.bounds
        else:
            ox,_,oz=self.plan.origin
            x=ox+(command[1]-ox)//144*144;z=oz+(command[3]-oz)//144*144
            bounds=(x,z,min(x+143,ox+self.plan.structure.size[0]-1),
                         min(z+143,oz+self.plan.structure.size[2]-1))
        if not self.contains(bounds,command) or self.column_count(bounds)>100:
            raise ValueError('放置命令跨越常加载分片；施工计划需要重新分片。')
        x,z,ex,ez=bounds;level=self.plan.origin[1]
        # Give each loaded slice its own identity so persisted confirmations
        # cannot refer to an older rectangle, including after resume.
        serial=self.job.get('loading_area_serial',0)+1
        self.job['loading_area_serial']=serial
        self.name='plimp_'+self.job['id']+'_'+str(serial)
        self.job['loading_area_pending']=dict(name=self.name,from_xz=[x,z],to_xz=[ex,ez])
        self.owned=True;self.save()
        self.update('loading',detail=f'固定位置施工：加载并确认 {self.column_count(bounds)} 个区块',target=[x,level,z])
        started=time.monotonic()
        await self.world.send(f'tickingarea add {x} {level} {z} {ex} {level} {ez} {self.name}')
        await self.world.send(f'tickingarea preload {self.name} true')
        from proloader.net.bedrock.session import BedrockLoginError
        try:
            await self.prove(bounds)
            self.full_window=True
        except (ValueError,asyncio.TimeoutError,BedrockLoginError):
            self.world.session._raise_if_failed()
            # Empty neighbouring columns may not answer even under tickingarea.
            # Prove the actual source footprint, without claiming the whole tile
            # ready or moving the bot. Future commands prove their own columns.
            await self.prove_footprint(command)
            self.full_window=False
            self.job['loading_footprint_fallbacks']=self.job.get('loading_footprint_fallbacks',0)+1
        self.key=bounds
        # This is the command's coordinate frame, not a claim about bot position.
        self.world._command_origin=(x,level,z)
        self.job['loading_strategy']='stationary_tickingarea_100'
        self.job['loaded_windows']=self.job.get('loaded_windows',0)+1
        self.job['loading_seconds']=round(self.job.get('loading_seconds',0)+time.monotonic()-started,3)
        self.save()

    async def release(self):
        return await ChunkWindow.release(self)

    async def __aenter__(self):return self

    async def __aexit__(self,*exc):
        return await ChunkWindow.__aexit__(self,*exc)


class ChunkWindow:
    def __init__(self,world,plan,job,save,drain,update):
        self.world,self.plan,self.job,self.save=world,plan,job,save
        self.drain,self.update=drain,update
        self.name='plimp_'+job['id']
        self.key=None;self.owned=bool(job.get('loading_area_pending'))
        self.full_window=False;self.proven_columns=set()

    async def prove_footprint(self,command):
        """An empty neighbour must not prevent a proven target from progressing."""
        columns={(x,z) for x in range(command[1]//16,command[4]//16+1)
                 for z in range(command[3]//16,command[6]//16+1)}
        for x,z in sorted(columns-self.proven_columns):
            self.update('loading',detail='整窗口未确认，改为确认实际施工区块',target=[x*16,command[2],z*16])
            for attempt in range(3):
                try:
                    await self.world.teleport(x*16+8,self.plan.origin[1]+self.plan.structure.size[1]+1,z*16+8)
                    await self.world.session.read_subchunk((x*16,command[2],z*16),self.plan.dimension,timeout=3)
                    break
                except (ValueError,asyncio.TimeoutError):
                    self.world.session._raise_if_failed()
                    if attempt==2:
                        # New empty areas may have no native SubChunk payload
                        # until the first real source block is placed. Require
                        # a positive read-only server query at the exact target
                        # instead of waiting forever for absent render data.
                        pos=f'{max(x*16,command[1])} {command[2]} {max(z*16,command[3])}'
                        try:
                            result=await self.world.send(f'execute in {self.plan.dimension} run testforblocks {pos} {pos} {pos} all')
                        except Exception as error:
                            self.world.session._raise_if_failed()
                            raise ValueError('施工位置的服务端读取仍未确认。') from error
                        if result is None:raise ValueError('施工位置的服务端读取仍未确认。')
                        self.job['loading_command_probes']=self.job.get('loading_command_probes',0)+1
                        self.save()
                        break
                    await asyncio.sleep(.25*(attempt+1))
            self.proven_columns.add((x,z))

    async def __aenter__(self):return self

    async def release(self):
        if not self.owned:return
        removed=False
        try:
            await self.world.send(f'tickingarea remove {self.name}')
            removed=True
        except Exception:
            self.world.session._raise_if_failed()
            # Absence is also reported negative. The following named add must
            # succeed before any new window can be trusted.
        self.key=None;self.owned=not removed
        self.full_window=False;self.proven_columns.clear()
        if removed:
            self.job.pop('loading_area_pending',None)
            self.job.pop('loading_area_cleanup_error',None)
        else:self.job['loading_area_cleanup_error']='remove_unconfirmed'
        self.save()

    async def __aexit__(self,*exc):
        previous=self.job.get('state');detail=self.job.get('detail','')
        if self.owned:self.update('cleanup',detail='清理本任务临时加载区并保存结果')
        try:
            await self.drain()
            await self.release()
        except Exception as error:
            self.job['loading_area_cleanup_error']=type(error).__name__;self.save()
        finally:
            if previous is not None:
                self.update(previous,detail=detail)
                self.save()
        return False

    async def ensure(self,command):
        x,y,z=command[1:4]
        gx=(x-self.plan.origin[0])//48;gz=(z-self.plan.origin[2])//48
        key=(gx,gz)
        if self.key==key:
            if not self.full_window:await self.prove_footprint(command)
            return
        await self.drain()
        await self.release()
        lo=(self.plan.origin[0]+gx*48,self.plan.origin[2]+gz*48)
        hi=(min(lo[0]+47,self.plan.origin[0]+self.plan.structure.size[0]-1),
            min(lo[1]+47,self.plan.origin[2]+self.plan.structure.size[2]-1))
        # Include the entire actual fill even for a source command crossing a
        # window boundary. The validated source plan bounds this rectangle.
        lo=(min(lo[0],command[1]),min(lo[1],command[3]))
        hi=(max(hi[0],command[4]),max(hi[1],command[6]))
        level=self.plan.origin[1]
        self.job['loading_area_pending']=dict(name=self.name,from_xz=list(lo),to_xz=list(hi))
        self.owned=True;self.save()
        self.update('loading',detail='临时加载施工区块；确认全部可读后开始写入',target=[lo[0],level,lo[1]])
        for attempt in range(3):
            try:
                await self.world.send(f'tickingarea add {lo[0]} {level} {lo[1]} {hi[0]} {level} {hi[1]} {self.name}')
                # Fatalder submits preload without a synchronous reply. The
                # following fresh native read still proves all target columns.
                await self.world.send(f'tickingarea preload {self.name} true',wait=False)
                break
            except Exception:
                self.world.session._raise_if_failed()
                if attempt==2:raise
                await self.release()
                self.owned=True;self.job['loading_area_pending']=dict(name=self.name,from_xz=list(lo),to_xz=list(hi));self.save()
                await asyncio.sleep(.25*(attempt+1))
        for attempt in range(5):
            try:
                if attempt==0:
                    await self.world.teleport((lo[0]+hi[0])//2,level,(lo[1]+hi[1])//2)
                try:
                    await self.world.session.read_chunk_window(lo,hi,level,self.plan.dimension,timeout=3)
                except (ValueError,asyncio.TimeoutError):
                    self.world.session._raise_if_failed()
                    # Some NEMC windows reject a bulk read despite successful
                    # preload. Visit and prove every column rather than aborting
                    # or declaring the unobserved window ready.
                    columns=[(cx,cz) for cx in range(lo[0]//16,hi[0]//16+1)
                             for cz in range(lo[1]//16,hi[1]//16+1)]
                    safe_y=self.plan.origin[1]+self.plan.structure.size[1]+1
                    for number,(cx,cz) in enumerate(columns,1):
                        self.update('loading',detail=f'批量加载未确认，逐区块恢复 {number}/{len(columns)}',target=[cx*16,level,cz*16])
                        await self.world.teleport(cx*16+8,safe_y,cz*16+8)
                        await self.world.session.read_subchunk((cx*16,level,cz*16),self.plan.dimension,timeout=3)
                    await self.world.teleport((lo[0]+hi[0])//2,safe_y,(lo[1]+hi[1])//2)
                    self.job['loading_single_fallbacks']=self.job.get('loading_single_fallbacks',0)+1
                self.key=key;self.job['loaded_windows']=self.job.get('loaded_windows',0)+1;self.save()
                self.full_window=True
                return
            except (ValueError,asyncio.TimeoutError):
                self.world.session._raise_if_failed()
                try:
                    await self.prove_footprint(command)
                    self.key=key;self.full_window=False
                    self.job['loading_footprint_fallbacks']=self.job.get('loading_footprint_fallbacks',0)+1
                    self.save()
                    return
                except (ValueError,asyncio.TimeoutError):
                    self.world.session._raise_if_failed()
                if attempt==4:raise ValueError('施工窗口尚未全部加载，已保留断点；未继续发送这一窗口。')
                self.update('loading',detail=f'重新定位并加载施工区块 · {attempt+1}/5',target=[lo[0],level,lo[1]])
                await asyncio.sleep(.25*(attempt+1))
