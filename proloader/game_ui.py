"""In-game import HUD, exact chat controls, and measured command throughput."""
import asyncio
from collections import deque
from contextlib import suppress
import json
import time

WELCOME='proloader_bot：欢迎使用proloader开源导入器'
READY='proloader_bot：输入repair开始修复，输入done彻底完成操作'


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


def hud(plan,job):
    repair=job.get('manual_repair_in_progress',False)
    total=len(plan.commands);sent=job.get('manual_repair_submitted',0) if repair else job.get('submitted',0)
    meter=job.get('repair_telemetry',{}) if repair else job.get('telemetry',{})
    name=plan.structure.name.replace('§','')[:80]
    return (f'§bPROLOADER §f{name}\n'
            f'§e{"修复" if repair else "发送"}：§a{sent:,} / {total:,} 条命令 §7({sent/max(1,total):.1%})\n'
            f'§e速度：§f{meter.get("rate",0):,.1f} 条/秒  §e预计：§f{duration(meter.get("eta_seconds"))}\n'
            f'§7{job.get("detail","")}')


class GameConsole:
    def __init__(self,world,plan,job,enabled=False):
        self.world=world;self.plan=plan;self.job=job;self.enabled=enabled
        self.events=asyncio.Queue(maxsize=16);self.tasks=[];self.chat=None;self.results=None

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
        if self.results is not None:self.world.session.unsubscribe_command_results(self.results)
        if self.enabled:
            with suppress(Exception):
                await self.world.send('titleraw @a actionbar {"rawtext":[{"text":""}]}',wait=False)

    async def _panel(self):
        while True:
            try:
                self.job['result_tracking_dropped']=getattr(self.world.session,'_command_result_dropped',0)
                text=json.dumps({'rawtext':[{'text':hud(self.plan,self.job)}]},ensure_ascii=False)
                # HUD traffic must never bypass command backpressure.
                queue=self.world.session._raknet._send_queue
                if queue.qsize()<128:await self.world.send('titleraw @a actionbar '+text,wait=False)
            except Exception:
                self.anomaly('hud_unconfirmed')
            await asyncio.sleep(2)

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
            if not self.job.get('delivery_complete') or self.job.get('state')=='finished':continue
            if not self.events.full():self.events.put_nowait((action,message.source))

    async def next_action(self):
        while True:
            self.world.session._raise_if_failed()
            try:return await asyncio.wait_for(self.events.get(),timeout=1)
            except asyncio.TimeoutError:pass
