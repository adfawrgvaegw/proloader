"""创新点4 测试：断点续传 + 快照回滚。

测试 CheckpointManager / PlacementJournal / RegionSnapshot / RollbackEngine。
核心验证：WAL 日志记录 + 状态机 + 断点续传 + 快照回滚。
"""

from __future__ import annotations

import asyncio

import pytest

from proloader.backend.simulated import SimulatedBackend, SimulatedWorld
from proloader.diff.differ import DiffEngine
from proloader.diff.planner import DiffPlanner, PlannerOptions
from proloader.model.block import Block, BlockState
from proloader.model.region import BoundingBox
from proloader.sched.chunker import StructureChunker
from proloader.sched.task import SubTask, SubTaskStatus
from proloader.snapshot.checkpoint import CheckpointManager, SessionMeta, SessionStatus
from proloader.snapshot.journal import PlacementJournal
from proloader.snapshot.rollback import RollbackEngine
from proloader.snapshot.snapshot import RegionSnapshot


# ── CheckpointManager 状态机 ──────────────────────────────────────────────────
class TestCheckpointManager:
    def test_create_and_load_meta(self, tmp_sessions_dir):
        cm = CheckpointManager(sessions_dir=tmp_sessions_dir)
        meta = SessionMeta(session_id="test-001", structure_name="house")
        cm.create_session(meta)
        loaded = cm.load_meta("test-001")
        assert loaded.session_id == "test-001"
        assert loaded.status == SessionStatus.INIT

    def test_legal_transitions(self, tmp_sessions_dir):
        cm = CheckpointManager(sessions_dir=tmp_sessions_dir)
        meta = SessionMeta(session_id="test-002")
        cm.create_session(meta)
        cm.transition("test-002", SessionStatus.SNAPSHOT_TAKEN)
        cm.transition("test-002", SessionStatus.RUNNING)
        cm.transition("test-002", SessionStatus.DONE)
        assert cm.load_meta("test-002").status == SessionStatus.DONE

    def test_illegal_transition_raises(self, tmp_sessions_dir):
        from proloader.errors import SessionStateError

        cm = CheckpointManager(sessions_dir=tmp_sessions_dir)
        meta = SessionMeta(session_id="test-003")
        cm.create_session(meta)
        # INIT → DONE 是非法的
        with pytest.raises(SessionStateError):
            cm.transition("test-003", SessionStatus.DONE)

    def test_can_resume(self, tmp_sessions_dir):
        cm = CheckpointManager(sessions_dir=tmp_sessions_dir)
        meta = SessionMeta(session_id="test-004")
        cm.create_session(meta)
        cm.transition("test-004", SessionStatus.SNAPSHOT_TAKEN)
        cm.transition("test-004", SessionStatus.RUNNING)
        cm.transition("test-004", SessionStatus.FAILED)
        assert cm.can_resume("test-004")

    def test_cannot_resume_done(self, tmp_sessions_dir):
        cm = CheckpointManager(sessions_dir=tmp_sessions_dir)
        meta = SessionMeta(session_id="test-005")
        cm.create_session(meta)
        cm.transition("test-005", SessionStatus.SNAPSHOT_TAKEN)
        cm.transition("test-005", SessionStatus.RUNNING)
        cm.transition("test-005", SessionStatus.DONE)
        assert not cm.can_resume("test-005")

    def test_list_sessions(self, tmp_sessions_dir):
        cm = CheckpointManager(sessions_dir=tmp_sessions_dir)
        for i in range(3):
            cm.create_session(SessionMeta(session_id=f"list-{i}"))
        sessions = cm.list_sessions()
        assert len(sessions) == 3


# ── PlacementJournal (WAL) ────────────────────────────────────────────────────
class TestJournal:
    def test_append_and_filter(self, tmp_path):
        """journal 记录已完成 subtask，resume 时跳过。"""
        journal_path = tmp_path / "journal.jsonl"
        journal = PlacementJournal(journal_path)

        # 模拟 3 个已完成 subtask（bbox 不能为 None，journal 会序列化它）
        for i in range(3):
            bb = BoundingBox.from_origin_size(i * 10, 0, 0, 4, 4, 4)
            st = SubTask(id=i, ops=[], bbox=bb)
            st.status = SubTaskStatus.DONE
            st.placed = 10
            journal.append(st)

        # 构造 5 个 subtask，前 3 个已完成
        all_subtasks = []
        for i in range(5):
            bb = BoundingBox.from_origin_size(i * 10, 0, 0, 4, 4, 4)
            st = SubTask(id=i, ops=[], bbox=bb)
            all_subtasks.append(st)

        pending = journal.filter_pending(all_subtasks)
        assert len(pending) == 2
        assert pending[0].id == 3
        assert pending[1].id == 4


# ── RegionSnapshot ────────────────────────────────────────────────────────────
class TestSnapshot:
    async def test_capture_and_compare(self, empty_world):
        """快照捕获世界状态，回滚后恢复。"""
        bot = SimulatedBackend(empty_world, "snap-bot")
        await bot.connect()

        bbox = BoundingBox.from_origin_size(0, 0, 0, 5, 5, 5)

        # 放置方块前快照
        snap = await RegionSnapshot.capture(bot, bbox)
        assert snap is not None

        # 放置方块
        stone = Block(BlockState("minecraft:stone"))
        await bot.place_block(2, 2, 2, stone)
        assert empty_world.total_placed == 1

        # 回滚到快照
        engine = RollbackEngine()
        report = await engine.rollback(bot, snap)
        assert report.placed > 0

        # 验证方块被恢复为空气
        block = await empty_world.get_block(2, 2, 2)
        assert block.is_air

    async def test_snapshot_save_load(self, empty_world, tmp_path):
        """快照保存到文件并重新加载。"""
        bot = SimulatedBackend(empty_world, "snap-bot")
        await bot.connect()

        bbox = BoundingBox.from_origin_size(0, 0, 0, 4, 4, 4)
        snap = await RegionSnapshot.capture(bot, bbox)

        snap_path = tmp_path / "snapshot.npz"
        snap.save(snap_path)
        loaded = RegionSnapshot.load(snap_path)

        assert loaded.bbox == snap.bbox
        assert loaded.blocks.shape == snap.blocks.shape


# ── RollbackEngine ────────────────────────────────────────────────────────────
class TestRollback:
    async def test_rollback_restores_state(self, simple_structure):
        """完整流程：放置 → 快照回滚 → 恢复空气。"""
        world = SimulatedWorld(size=(64, 64, 64))
        bot = SimulatedBackend(world, "rb-bot")
        await bot.connect()

        origin = (10, 10, 10)
        bbox = simple_structure.bbox_at(origin)

        # 1. 快照
        snap = await RegionSnapshot.capture(bot, bbox)

        # 2. 放置建筑
        region = simple_structure.to_region(origin)
        plan = DiffEngine().compute_full(region)
        ops = DiffPlanner().plan(plan, PlannerOptions(sort_by_y=True))
        subtasks = StructureChunker(cell_size=16).chunk(ops, num_bots=1)

        from proloader.sched.scheduler import ParallelScheduler

        scheduler = ParallelScheduler([bot])
        await scheduler.run(subtasks)
        assert world.total_placed == simple_structure.block_count

        # 3. 回滚
        engine = RollbackEngine()
        report = await engine.rollback(bot, snap)
        assert report.placed == simple_structure.block_count

        # 4. 验证恢复
        for lx, ly, lz, _ in simple_structure.region.iter_non_air():
            block = await world.get_block(lx + origin[0], ly + origin[1], lz + origin[2])
            assert block.is_air, f"({lx+origin[0]},{ly+origin[1]},{lz+origin[2]}) 应为空气"
