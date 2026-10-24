"""创新点2：多假人并行调度。

建筑切块后多个假人并行放置，区域锁避免冲突，共享队列负载均衡。

组件：
    - StructureChunker: 把 PlacementOp 按空间切块为 SubTask
    - RegionLockManager: bbox 区域锁，防止假人间冲突
    - ParallelScheduler: asyncio 编排多 worker 并行执行 SubTask
"""

from __future__ import annotations

from proloader.sched.chunker import StructureChunker
from proloader.sched.region_lock import LockToken, RegionLockManager
from proloader.sched.scheduler import ParallelScheduler, SchedulerReport
from proloader.sched.strategy import SchedStrategy, get_strategy
from proloader.sched.task import SubTask, SubTaskStatus

__all__ = [
    "StructureChunker",
    "RegionLockManager",
    "LockToken",
    "ParallelScheduler",
    "SchedulerReport",
    "SubTask",
    "SubTaskStatus",
    "SchedStrategy",
    "get_strategy",
]
