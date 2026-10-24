"""ParallelScheduler —— 创新点2 核心：多假人并行调度。

原理：
    把 SubTask 列表放入共享 asyncio.Queue，N 个 worker 协程（每个绑定一个 backend）
    并行拉取执行。快的 worker 拉得多（work-stealing 风格，自然负载均衡）。

    每个 worker 执行 SubTask 的流程：
        1. acquire region lock（bbox 不重叠则并行，重叠则等待）
        2. teleport 到 bbox 中心（真实后端需要，模拟器跳过）
        3. 分批 place_batch（按 max_blocks_per_batch 切批）
        4. release region lock
        5. 回调 on_subtask_done（用于 journal 断点续传）

特性：
    - 负载均衡：共享队列，work-stealing
    - 失败隔离：单个 SubTask 失败不中断其他
    - 区域锁：bbox 重叠的 SubTask 串行执行
    - 并行加速比：N 个 backend 理论加速 ~N（受锁竞争影响）

学习要点：
    - asyncio 共享队列实现 work-stealing 比手动分配更均衡
    - 区域锁保证正确性，但切块时应尽量不重叠（chunker 已保证）
"""

from __future__ import annotations

import asyncio
import logging
import time
from dataclasses import dataclass, field
from typing import Callable, Dict, List, Optional

from proloader.backend.base import PlacementBackend
from proloader.backend.events import PlacementOp
from proloader.errors import BackendError
from proloader.sched.region_lock import RegionLockManager
from proloader.sched.strategy import SchedStrategy
from proloader.sched.task import SubTask, SubTaskStatus

logger = logging.getLogger(__name__)


@dataclass
class SchedulerReport:
    """调度器执行报告。"""

    total_subtasks: int = 0
    done: int = 0
    failed: int = 0
    skipped: int = 0  # 空块跳过
    total_ops: int = 0
    placed_ops: int = 0
    failed_ops: int = 0
    elapsed: float = 0.0
    per_backend: Dict[str, Dict] = field(default_factory=dict)

    @property
    def speedup_hint(self) -> str:
        """加速比提示（与单 backend 串行比）。"""
        if self.elapsed <= 0:
            return "N/A"
        ops_per_sec = self.placed_ops / self.elapsed if self.elapsed > 0 else 0
        return f"{ops_per_sec:.0f} ops/s"

    def summary(self) -> str:
        return (
            f"子任务 {self.done}/{self.total_subtasks} 完成 "
            f"(失败 {self.failed}, 跳过 {self.skipped}) | "
            f"放置 {self.placed_ops}/{self.total_ops} 方块 | "
            f"耗时 {self.elapsed:.3f}s | {self.speedup_hint}"
        )


class ParallelScheduler:
    """多假人并行调度器。

    用法：
        scheduler = ParallelScheduler(backends, strategy="least_loaded")
        report = await scheduler.run(subtasks, on_subtask_done=journal.record)
    """

    def __init__(
        self,
        backends: List[PlacementBackend],
        strategy: str | SchedStrategy = SchedStrategy.LEAST_LOADED,
    ) -> None:
        if not backends:
            raise ValueError("至少需要一个 backend")
        self._backends = backends
        self._strategy = SchedStrategy(strategy) if isinstance(strategy, str) else strategy
        self._lock_mgr = RegionLockManager()
        self._cancelled = False

    async def run(
        self,
        subtasks: List[SubTask],
        on_subtask_done: Optional[Callable[[SubTask], None]] = None,
        on_progress: Optional[Callable] = None,
    ) -> SchedulerReport:
        """执行所有 SubTask。

        Args:
            subtasks: 子任务列表
            on_subtask_done: 子任务完成回调（同步，用于 journal 记录）
            on_progress: 进度回调（可选）

        Returns:
            SchedulerReport 执行报告
        """
        start = time.monotonic()
        report = SchedulerReport(total_subtasks=len(subtasks))

        # 过滤空块
        non_empty = []
        for st in subtasks:
            if st.is_empty:
                st.status = SubTaskStatus.SKIPPED
                report.skipped += 1
            else:
                non_empty.append(st)
                report.total_ops += st.ops_count

        if not non_empty:
            report.elapsed = time.monotonic() - start
            return report

        # 共享任务队列
        queue: asyncio.Queue = asyncio.Queue()
        for st in non_empty:
            await queue.put(st)

        # 启动 N 个 worker
        workers = [
            asyncio.create_task(self._worker(backend, queue, report, on_subtask_done, on_progress))
            for backend in self._backends
        ]

        try:
            await asyncio.gather(*workers)
        except asyncio.CancelledError:
            self._cancelled = True
            for w in workers:
                w.cancel()
            await asyncio.gather(*workers, return_exceptions=True)

        # 通知所有 backend 完成
        for backend in self._backends:
            if hasattr(backend, "emit_done"):
                await backend.emit_done()

        report.elapsed = time.monotonic() - start
        return report

    async def _worker(
        self,
        backend: PlacementBackend,
        queue: asyncio.Queue,
        report: SchedulerReport,
        on_subtask_done: Optional[Callable[[SubTask], None]],
        on_progress: Optional[Callable],
    ) -> None:
        """单个 worker 协程：循环拉取 SubTask 执行。"""
        bid = backend.backend_id
        report.per_backend.setdefault(bid, {"subtasks": 0, "placed": 0, "failed": 0})

        while True:
            try:
                # get_nowait 是同步方法（asyncio 单线程无竞争），队列为空则退出
                subtask: SubTask = queue.get_nowait()
            except asyncio.QueueEmpty:
                break

            subtask.assigned_backend = bid
            subtask.status = SubTaskStatus.RUNNING
            try:
                placed, failed = await self._execute_subtask(backend, subtask)
                subtask.placed = placed
                subtask.failed = failed
                subtask.status = SubTaskStatus.DONE
                report.done += 1
                report.placed_ops += placed
                report.failed_ops += failed
                report.per_backend[bid]["subtasks"] += 1
                report.per_backend[bid]["placed"] += placed
                report.per_backend[bid]["failed"] += failed
            except BackendError as e:
                logger.warning("SubTask %d 在 %s 失败: %s", subtask.id, bid, e)
                subtask.status = SubTaskStatus.FAILED
                report.failed += 1
            except Exception as e:
                logger.exception("SubTask %d 在 %s 异常", subtask.id, bid)
                subtask.status = SubTaskStatus.FAILED
                report.failed += 1

            # 回调（journal 记录）
            if on_subtask_done is not None:
                try:
                    on_subtask_done(subtask)
                except Exception:
                    logger.exception("on_subtask_done 回调失败")

            # 进度回调
            if on_progress is not None:
                try:
                    on_progress(report)
                except Exception:
                    pass

            queue.task_done()

            # 公平让步：让出事件循环，使其他 worker 有机会拉取任务。
            # 真实后端的 place_batch 会在网络 I/O 上挂起，天然产生让步点；
            # 模拟后端（place_delay=0）无 I/O 挂起，需显式让步才能体现
            # work-stealing 的负载均衡（否则首个 worker 会抽干整个队列）。
            await asyncio.sleep(0)

    async def _execute_subtask(
        self, backend: PlacementBackend, subtask: SubTask
    ) -> tuple:
        """执行单个 SubTask：锁 → teleport → 分批放置 → 解锁。

        Returns:
            (placed_count, failed_count)
        """
        bid = backend.backend_id
        # 1. 获取区域锁
        token = await self._lock_mgr.acquire(subtask.bbox, bid)
        try:
            # 2. teleport（真实后端需要）
            if backend.capabilities.needs_teleport:
                cx = (subtask.bbox.min_x + subtask.bbox.max_x) / 2
                cy = subtask.bbox.max_y + 2  # 站在上方
                cz = (subtask.bbox.min_z + subtask.bbox.max_z) / 2
                await backend.teleport(cx, cy, cz)

            # 3. 分批放置
            batch_size = backend.capabilities.max_blocks_per_batch
            placed = 0
            failed = 0
            for i in range(0, len(subtask.ops), batch_size):
                batch = subtask.ops[i : i + batch_size]
                if batch:
                    success = await backend.place_batch(batch)
                    placed += success
                    failed += len(batch) - success
            return (placed, failed)
        finally:
            await self._lock_mgr.release(token)

    def cancel(self) -> None:
        """取消调度（用于断点续传中断）。"""
        self._cancelled = True
