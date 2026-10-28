"""In-game import HUD, exact chat controls, and measured command throughput."""
import asyncio
from collections import deque
from contextlib import suppress
import json
import time

WELCOME='proloader_bot：欢迎使用proloader开源导入器'
READY='proloader_bot：把机器人传送到你身边，自动修复当前区块；repair重试，done结束'


class ProgressMeter:
    def __init__(self,total,submitted=0):
        self.total=total;self.started=time.monotonic();self.samples=deque([(self.started,submitted)])
        self.last={}

    def read(self,submitted,force=False):
        now=time.monotonic()
        if submitted<self.samples[-1][1]:self.samples.clear();self.samples.append((now,submitted));self.last={}
        if not force and now-self.samples[-1][0]<0.5 and self.last:return self.last
        if force or now-self.samples[-1][0]>=0.5:
            self.samples.append((now,submitted))
        while len(self.samples)>2 and now-self.samples[0][0]>15:self.samples.popleft()
        elapsed=now-self.samples[0][0]
        rate=max(0,(submitted-self.samples[0][1])/elapsed) if elapsed>0 else 0
        remaining=max(0,self.total-submitted)
        self.last=dict(rate=round(rate,1),eta_seconds=round(remaining/rate) if rate>0 else None,
                       remaining=remaining,elapsed_seconds=round(now-self.started,1))
        return self.last


def duration(seconds):
    if seconds is None:return '计算中'
    seconds=max(0,int(seconds));hours,remainder=divmod(seconds,3600);minutes,seconds=divmod(remainder,60)
    return (f'{hours}小时' if hours else '')+f'{minutes}分{seconds}秒'


def phase_progress(plan,job):
    """Shared phase counters for the CLI and game HUD; no placement-only NBT bar."""
    stage=job.get('active_phase',job.get('state','placing')) if job.get('state')=='loading' else job.get('state','placing')
    local=job.get('local_repair_progress',{}) if job.get('local_repair_active') else {}
    if local and stage in ('command_block_data','command_block_repair','block_entity_data'):
        prefix='block_entities' if stage=='block_entity_data' else 'command_blocks'
        return dict(key='local_'+prefix,label='当前区块NBT',done=local.get(prefix+'_next',0),
                    total=local.get(prefix+'_total',0),unit='个')
    if stage=='cleanup':return dict(key=stage,label='清理临时施工区',done=0,total=1,unit='项')
    if stage in ('command_block_data','command_block_repair'):
        return dict(key='command_blocks',label='命令方块'+('修复' if stage.endswith('repair') else 'NBT'),
                    done=job.get('command_blocks_next',0),total=job.get('plan',{}).get('command_blocks',0),unit='个')
    if stage=='block_entity_data':
        return dict(key='block_entities',label='容器/告示牌NBT',done=job.get('block_entities_next',0),
                    total=job.get('plan',{}).get('block_entities',0),unit='个')
    if stage in ('verifying','manual_repair_check'):
        total=job.get('verification_total',getattr(plan,'block_count',0) if job.get('verify_mode')=='complete' else 64)
        return dict(key=stage,label=('当前区块扫描' if job.get('manual_repair_chunk') is not None else '差异扫描') if stage=='manual_repair_check' else '世界校验',
                    done=job.get('checked',0),total=total,unit='格')
    if stage=='repairing':
        r=job.get('repair_rounds',[{}])[-1]
        return dict(key='repairing',label='差异补全',done=r.get('sent',0),total=r.get('commands',0),unit='条命令')
    if stage=='repair_verifying':
        return dict(key=stage,label='补全复核',done=job.get('repair_checked',0),total=job.get('repair_check_total',0),unit='格')
    if job.get('manual_repair_in_progress'):
        return dict(key='manual_repair',label='差异补全',done=job.get('manual_repair_submitted',0),
                    total=job.get('manual_repair_total',len(plan.commands)),unit='条命令')
    return dict(key=stage,label={'awaiting_action':'等待玩家操作','completed':'已完成','sent':'已发送',
                'checking_permission':'检查权限','connecting':'正在连接','paused':'已暂停','failed':'已暂停'}.get(stage,'发送'),
                done=job.get('submitted',0),total=len(plan.commands),unit='条命令')


def hud(plan,job):
    phase=phase_progress(plan,job)
    total,sent=phase['total'],phase['done']
    meter=job.get('phase_telemetry',job.get('telemetry',{}))
    name=plan.structure.name.replace('§','')[:80]
    unsupported=job.get('plan',{}).get('unsupported_block_entities',0)
    unsupported_text=f' §c| 未支持NBT：{unsupported}' if unsupported else ''
    return (f'§bPROLOADER §f{name}\n'
            f'§e{phase["label"]}：§a{sent:,} / {total:,} {phase["unit"]} §7({sent/max(1,total):.1%})\n'
            f'§e速度：§f{meter.get("rate",0):,.1f} {phase["unit"]}/秒  §e预计：§f{duration(meter.get("eta_seconds"))}\n'
            f'§e待补全：§f{len(job.get("repair_pending",{})):,} §7| §eNBT：§f{job.get("command_blocks_next",0)}/{job.get("plan",{}).get("command_blocks",0)} + {job.get("block_entities_next",0)}/{job.get("plan",{}).get("block_entities",0)}{unsupported_text}\n'
            f'§7{job.get("detail","")}')


class GameConsole:
    def __init__(self,world,plan,job,enabled=False):
        self.world=world;self.plan=plan;self.job=job;self.enabled=enabled
        self.events=asyncio.Queue(maxsize=16);self.tasks=[];self.chat=None;self.results=None
        self.teleports=None;self.pending_teleport=None;self.teleport_queued=False

    def anomaly(self,kind):
        stats=self.job.setdefault('ui_anomalies',{})
        stats[kind]=stats.get(kind,0)+1

    async def notice(self,message):
        if self.enabled:
            text=json.dumps({'rawtext':[{'text':message}]},ensure_ascii=False)
            try:await self.world.send('tellraw @a '+text,wait=False)
            except Exception:
                self.world.session._raise_if_failed()
                self.anomaly('chat_notice_unconfirmed')

    async def __aenter__(self):
        if self.enabled:
            self.chat=self.world.session.subscribe_chat()
            self.tasks=[asyncio.create_task(self._panel()),asyncio.create_task(self._listen())]
            if hasattr(self.world.session,'subscribe_player_teleports'):
                self.teleports=self.world.session.subscribe_player_teleports()
                self.tasks.append(asyncio.create_task(self._listen_teleports()))
            if hasattr(self.world.session,'subscribe_command_results'):
                self.results=self.world.session.subscribe_command_results()
                self.tasks.append(asyncio.create_task(self._results()))
            try:await self.notice(WELCOME)
            except BaseException:
                await self.__aexit__(None,None,None)
                raise
        return self

    async def __aexit__(self,*exc):
        for task in self.tasks:task.cancel()
        for task in self.tasks:
            with suppress(asyncio.CancelledError,Exception):await task
        if self.chat is not None:self.world.session.unsubscribe_chat(self.chat)
        if self.teleports is not None:self.world.session.unsubscribe_player_teleports(self.teleports)
        if self.results is not None:self.world.session.unsubscribe_command_results(self.results)
        if self.enabled:
            with suppress(Exception):
                await self.world.send('titleraw @a actionbar {"rawtext":[{"text":""}]}',wait=False)

    async def _panel(self):
        meter=None;key=None
        while True:
            tick=time.monotonic()
            try:
                phase=phase_progress(self.plan,self.job)
                token=(phase['key'],phase['total'])
                if key!=token:
                    key=token;meter=ProgressMeter(phase['total'],phase['done'])
                self.job['phase_telemetry']=meter.read(phase['done'])
                self.job['result_tracking_dropped']=getattr(self.world.session,'_command_result_dropped',0)
                text=json.dumps({'rawtext':[{'text':hud(self.plan,self.job)}]},ensure_ascii=False)
                # HUD traffic must never bypass command backpressure.
                queue=self.world.session._raknet._send_queue
                if queue.qsize()<128:await self.world.send('titleraw @a actionbar '+text,wait=False)
            except Exception:
                self.anomaly('hud_unconfirmed')
            await asyncio.sleep(max(0.01,0.5-(time.monotonic()-tick)))

    async def _results(self):
        while True:
            context=await self.results.get()
            phase=context.get('phase','initial');index=context['index']
            ranges=self.job.setdefault('command_result_anomalies',[])
            self.job['negative_result_count']=self.job.get('negative_result_count',0)+1
            if ranges and ranges[-1]['phase']==phase and ranges[-1]['start']<=index<=ranges[-1]['end']+1:
                ranges[-1]['end']=max(index,ranges[-1]['end'])
            elif len(ranges)<10000:
                ranges.append(dict(phase=phase,start=index,end=index,kind='server_negative_unverified',
                                   position=context.get('position'),block=context.get('block')))
            else:self.anomaly('result_range_overflow')
            self.job['result_tracking_dropped']=getattr(self.world.session,'_command_result_dropped',0)

    async def _listen(self):
        while True:
            message=await self.chat.get()
            action=message.message.strip().casefold()
            if message.source in (self.world.name,'proloader_bot') or action not in ('repair','done'):continue
            # Early done cannot cancel the initial send; old commands are not
            # replayed when the bot transitions to awaiting_action.
            if (not self.job.get('delivery_complete') and self.job.get('state')!='awaiting_action') or self.job.get('state')=='finished':continue
            if not self.events.full():self.events.put_nowait((action,message.source))

    async def next_action(self):
        while True:
            self.world.session._raise_if_failed()
            try:
                action,sender=await asyncio.wait_for(self.events.get(),timeout=1)
                if action=='repair_here':
                    self.teleport_queued=False
                    sender=self.pending_teleport;self.pending_teleport=None
                return action,sender
            except asyncio.TimeoutError:pass

    async def _listen_teleports(self):
        import math
        while True:
            event=await self.teleports.get()
            if event.get('importer_move'):continue
            if self.job.get('state')!='awaiting_action' and not self.job.get('local_repair_active'):continue
            position=event.get('position',())
            if len(position)!=3 or not all(math.isfinite(v) for v in position):continue
            self.world._verification_chunk=None
            self.world._last_import_position=tuple(position)
            self.world._external_teleport_revision=getattr(self.world,'_external_teleport_revision',0)+1
            if hasattr(self.world,'_scan_subchunks'):self.world._scan_subchunks.clear()
            self.pending_teleport=tuple(position)
            if not self.teleport_queued and not self.events.full():
                self.events.put_nowait(('repair_here',None));self.teleport_queued=True
