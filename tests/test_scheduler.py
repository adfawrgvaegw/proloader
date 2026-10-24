"""创新点2 测试：多假人并行调度。

测试 ParallelScheduler / StructureChunker / RegionLock。
核心验证：多 bot 并行执行 + 区域锁防止冲突 + work-stealing 负载均衡。
"""

from __future__ import annotations

import asyncio

import pytest

from proloader.backend.simulated import SimulatedBackend, SimulatedWorld
from proloader.diff.differ import DiffEngine
from proloader.diff.planner import DiffPlanner, PlannerOptions
from proloader.sched.chunker import StructureChunker
from proloader.sched.region_lock import RegionLockManager
from proloader.sched.scheduler import ParallelScheduler
from proloader.sched.task import SubTask, SubTaskStatus


# ── StructureChunker ──────────────────────────────────────────────────────────
class TestChunker:
    def test_chunk_count(self, simple_structure):
        """切块数量 >= 1。"""
        region = simple_structure.to_region((0, 0, 0))
        plan = DiffEngine().compute_full(region)
        ops = DiffPlanner().plan(plan, PlannerOptions(sort_by_y=True))
        subtasks = StructureChunker(cell_size=16).chunk(ops, num_bots=3)
        assert len(subtasks) >= 1
        # 所有 subtask 的 ops 总和 == ops 总数
        total = sum(st.ops_count for st in subtasks)
        assert total == len(ops)

    def test_small_cell_more_chunks(self, simple_structure):
        """更小的 cell_size → 更多 subtask。"""
        region = simple_structure.to_region((0, 0, 0))
        plan = DiffEngine().compute_full(region)
        ops = DiffPlanner().plan(plan, PlannerOptions(sort_by_y=True))
        big = StructureChunker(cell_size=16).chunk(ops, num_bots=3)
        small = StructureChunker(cell_size=2).chunk(ops, num_bots=3)
        assert len(small) >= len(big)

    def test_empty_ops(self):
        """空 ops → 0 subtask。"""
        subtasks = StructureChunker(cell_size=16).chunk([], num_bots=2)
        assert len(subtasks) == 0


# ── RegionLockManager ─────────────────────────────────────────────────────────
class TestRegionLock:
    async def test_acquire_release(self):
        """获取/释放锁。"""
        mgr = RegionLockManager()
        from proloader.model.region import BoundingBox

        bb = BoundingBox.from_origin_size(0, 0, 0, 5, 5, 5)
        token = await mgr.acquire(bb, "bot-0")
        assert token is not None
        await mgr.release(token)

    async def test_non_overlapping_parallel(self):
        """不重叠区域可同时获取。"""
        mgr = RegionLockManager()
        from proloader.model.region import BoundingBox

        bb1 = BoundingBox.from_origin_size(0, 0, 0, 5, 5, 5)
        bb2 = BoundingBox.from_origin_size(10, 0, 0, 5, 5, 5)
        t1 = await mgr.acquire(bb1, "bot-0")
        t2 = await mgr.acquire(bb2, "bot-1")
        assert t1 is not None
        assert t2 is not None
        await mgr.release(t1)
        await mgr.release(t2)

    async def test_overlapping_blocks(self):
        """重叠区域第二个 acquire 应等待。"""
        mgr = RegionLockManager()
        from proloader.model.region import BoundingBox

        bb1 = BoundingBox.from_origin_size(0, 0, 0, 5, 5, 5)
        bb2 = BoundingBox.from_origin_size(3, 0, 0, 5, 5, 5)
        t1 = await mgr.acquire(bb1, "bot-0")
        # bb2 与 bb1 重叠，应该等待
        done = asyncio.Event()

        async def try_acquire():
            t2 = await mgr.acquire(bb2, "bot-1")
            done.set()
            await mgr.release(t2)

        task = asyncio.create_task(try_acquire())
        await asyncio.sleep(0.05)
        assert not done.is_set(), "重叠区域不应能立即获取"
        await mgr.release(t1)
        await asyncio.wait_for(task, timeout=1.0)
        assert done.is_set()


# ── ParallelScheduler ─────────────────────────────────────────────────────────
class TestParallelScheduler:
    async def test_single_bot(self, simple_structure):
        """单 bot 完成所有 subtask。"""
        world = SimulatedWorld(size=(64, 64, 64))
        bot = SimulatedBackend(world, "bot-0")
        await bot.connect()

        region = simple_structure.to_region((0, 0, 0))
        plan = DiffEngine().compute_full(region)
        ops = DiffPlanner().plan(plan, PlannerOptions(sort_by_y=True))
        subtasks = StructureChunker(cell_size=16).chunk(ops, num_bots=1)

        scheduler = ParallelScheduler([bot], strategy="least_loaded")
        report = await scheduler.run(subtasks)

        assert report.done == len(subtasks)
        assert report.failed == 0
        assert report.placed_ops == simple_structure.block_count

    async def test_multi_bot_parallel(self, simple_structure):
        """多 bot 并行执行。"""
        world = SimulatedWorld(size=(64, 64, 64))
        bots = [SimulatedBackend(world, f"bot-{i}") for i in range(4)]
        for b in bots:
            await b.connect()

        region = simple_structure.to_region((0, 0, 0))
        plan = DiffEngine().compute_full(region)
        ops = DiffPlanner().plan(plan, PlannerOptions(sort_by_y=True))
        subtasks = StructureChunker(cell_size=2).chunk(ops, num_bots=4)

        scheduler = ParallelScheduler(bots, strategy="least_loaded")
        report = await scheduler.run(subtasks)

        assert report.failed == 0
        assert report.placed_ops == simple_structure.block_count
        # 至少有 2 个 bot 参与了工作
        active_bots = sum(1 for v in report.per_backend.values() if v["subtasks"] > 0)
        assert active_bots >= 2

    async def test_empty_subtasks(self):
        """空 subtask 列表 → 立即返回。"""
        world = SimulatedWorld(size=(32, 32, 32))
        bot = SimulatedBackend(world, "bot-0")
        await bot.connect()
        scheduler = ParallelScheduler([bot])
        report = await scheduler.run([])
        assert report.total_subtasks == 0
        assert report.placed_ops == 0

    async def test_on_subtask_done_callback(self, simple_structure):
        """on_subtask_done 回调被调用。"""
        world = SimulatedWorld(size=(64, 64, 64))
        bot = SimulatedBackend(world, "bot-0")
        await bot.connect()

        region = simple_structure.to_region((0, 0, 0))
        plan = DiffEngine().compute_full(region)
        ops = DiffPlanner().plan(plan, PlannerOptions(sort_by_y=True))
        subtasks = StructureChunker(cell_size=16).chunk(ops, num_bots=1)

        done_list = []

        def on_done(st: SubTask):
            done_list.append(st)

        scheduler = ParallelScheduler([bot])
        await scheduler.run(subtasks, on_subtask_done=on_done)
        assert len(done_list) == len(subtasks)
        for st in done_list:
            assert st.status == SubTaskStatus.DONE
