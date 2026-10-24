"""Smooth command pacing and bounded, asynchronous processing probes."""
import asyncio
import time
from collections import deque

DEFAULT_SPEED=4000
DEFAULT_BURST=10
PROBE_INTERVAL=256
PROCESSING_LIMIT=768

class CommandPacer:
    def __init__(self, speed=DEFAULT_SPEED, burst=DEFAULT_BURST):
        if not 1<=speed<=5000:raise ValueError('命令速率必须在1～5000之间')
        self.speed=speed
        self.burst=min(speed,burst)
        self.tokens=float(self.burst)
        self.updated=time.monotonic()
    async def wait(self):
        now=time.monotonic()
        self.tokens=min(self.burst,self.tokens+(now-self.updated)*self.speed)
        self.updated=now
        if self.tokens<1:
            await asyncio.sleep((1-self.tokens)/self.speed)
            now=time.monotonic()
            self.tokens=min(self.burst,self.tokens+(now-self.updated)*self.speed)
            self.updated=now
        self.tokens-=1


class ProcessingWindow:
    """Send a probe every interval; wait only when the bounded window fills.

    A completed probe releases a scheduling window, not proof of block placement.
    The callback owns negative/timeout recording and terminal-error propagation.
    Drain before teleporting, NBT writes, verification, or ending a pass.
    """
    def __init__(self,probe,metrics,interval=PROBE_INTERVAL,limit=PROCESSING_LIMIT):
        if interval<1 or limit<interval:raise ValueError('无效处理窗口')
        self.probe=probe;self.metrics=metrics;self.interval=interval;self.limit=limit
        self.pending=deque();self.sent=0;self.released=0;self.requested=0
        self.begin=None;self.end=0;self.phase='initial'
        for key in ('processing_probe_seconds','processing_wait_seconds','processing_probes','max_processing_window'):
            metrics.setdefault(key,0)

    async def __aenter__(self):return self

    async def __aexit__(self,*args):
        for _,task in self.pending:task.cancel()
        await asyncio.gather(*(t for _,t in self.pending),return_exceptions=True)
        self.pending.clear()

    async def _probe(self,begin,end,phase):
        started=time.monotonic()
        try:await self.probe(begin,end,phase)
        finally:
            self.metrics['processing_probes']+=1
            self.metrics['processing_probe_seconds']+=time.monotonic()-started

    async def _release(self):
        end,task=self.pending[0]
        started=time.monotonic()
        try:await task
        finally:self.metrics['processing_wait_seconds']+=time.monotonic()-started
        self.pending.popleft();self.released=end

    async def before(self):
        while self.pending and self.pending[0][1].done():await self._release()
        if self.sent-self.released>=self.limit:await self._release()

    async def _launch(self):
        self.pending.append((self.sent,asyncio.create_task(self._probe(self.begin,self.end,self.phase))))
        self.requested=self.sent;self.begin=None
        # Register the ordered probe before callers enqueue more writes.
        await asyncio.sleep(0)

    async def after(self,index,phase='initial'):
        if self.begin is None:self.begin=index
        self.end=index+1;self.phase=phase;self.sent+=1
        self.metrics['max_processing_window']=max(self.metrics['max_processing_window'],self.sent-self.released)
        if self.sent-self.requested>=self.interval:await self._launch()

    async def drain(self):
        if self.sent>self.requested:await self._launch()
        while self.pending:await self._release()

async def transport_pressure(session, limit=512, timeout=20):
    """Bound outstanding writes while retaining the configured token-bucket rate."""
    until=time.monotonic()+timeout
    while session._raknet._send_queue.qsize()+len(session._raknet._pending_acks)>=limit:
        session._raise_if_failed()
        if session._raknet._closed:raise RuntimeError('施工传输已断开')
        if time.monotonic()>=until:raise RuntimeError('施工传输确认积压超时')
        await asyncio.sleep(0.005)
