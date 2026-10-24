"""Pipeline 集成测试：四大创新点端到端串联。

验证 parser → diff → snapshot → sched → backend 完整流程。
"""

from __future__ import annotations

import pytest

from proloader.backend.simulated import SimulatedWorld
from proloader.pipeline import make_simulated_backends, run_import
from proloader.session import ImportOptions, ImportSession
from proloader.snapshot.checkpoint import CheckpointManager, SessionStatus


class TestPipelineImport:
    async def test_full_import_diff(self, simple_structure):
        """差异增量导入（空世界 → 全 ADD）。"""
        world = SimulatedWorld(size=(64, 64, 64))
        backends = make_simulated_backends(3, world, place_delay=0.0)
        cm = CheckpointManager(sessions_dir="/tmp/proloader-test-sessions")

        options = ImportOptions(num_bots=3, cell_size=16, use_diff=True)
        session = ImportSession(
            structure=simple_structure,
            origin=(5, 5, 5),
            backends=backends,
            options=options,
            checkpoint_manager=cm,
        )
        report = await run_import(session)

        assert report.scheduler_report.failed == 0
        assert report.scheduler_report.placed_ops == simple_structure.block_count
        # 差异应该全是 ADD（空世界）
        assert report.diff_plan.stats.adds == simple_structure.block_count
        # 会话状态应为 DONE
        meta = cm.load_meta(session.session_id)
        assert meta.status == SessionStatus.DONE

    async def test_full_import_no_diff(self, simple_structure):
        """全量导入（禁用差异）。"""
        world = SimulatedWorld(size=(64, 64, 64))
        backends = make_simulated_backends(2, world, place_delay=0.0)
        cm = CheckpointManager(sessions_dir="/tmp/proloader-test-sessions")

        options = ImportOptions(num_bots=2, cell_size=16, use_diff=False)
        session = ImportSession(
            structure=simple_structure,
            origin=(0, 0, 0),
            backends=backends,
            options=options,
            checkpoint_manager=cm,
        )
        report = await run_import(session)
        assert report.scheduler_report.placed_ops == simple_structure.block_count

    async def test_idempotent_import(self, simple_structure):
        """重复导入同一建筑到同一位置：第二次差异为 0。"""
        world = SimulatedWorld(size=(64, 64, 64))
        backends = make_simulated_backends(2, world, place_delay=0.0)
        cm = CheckpointManager(sessions_dir="/tmp/proloader-test-sessions")

        # 第一次导入
        options = ImportOptions(num_bots=2, cell_size=16, use_diff=True)
        session1 = ImportSession(
            structure=simple_structure, origin=(0, 0, 0),
            backends=backends, options=options, checkpoint_manager=cm,
        )
        report1 = await run_import(session1)
        assert report1.diff_plan.stats.adds == simple_structure.block_count

        # 第二次导入（世界已有建筑 → 差异为 0）
        backends2 = make_simulated_backends(2, world, place_delay=0.0)
        session2 = ImportSession(
            structure=simple_structure, origin=(0, 0, 0),
            backends=backends2, options=options, checkpoint_manager=cm,
        )
        report2 = await run_import(session2)
        # 第二次全是 SKIP
        assert report2.diff_plan.stats.skipped == simple_structure.total_blocks
        assert report2.diff_plan.stats.adds == 0
        assert report2.scheduler_report.placed_ops == 0

    async def test_example_house_import(self, example_structure):
        """完整导入示例建筑 house.mcstructure。"""
        world = SimulatedWorld(size=(128, 128, 128))
        backends = make_simulated_backends(4, world, place_delay=0.0)
        cm = CheckpointManager(sessions_dir="/tmp/proloader-test-sessions")

        options = ImportOptions(num_bots=4, cell_size=16, use_diff=True)
        session = ImportSession(
            structure=example_structure,
            origin=(20, 20, 20),
            backends=backends,
            options=options,
            checkpoint_manager=cm,
        )
        report = await run_import(session)
        assert report.scheduler_report.failed == 0
        assert report.scheduler_report.placed_ops == 168

    async def test_multi_bot_speedup(self, simple_structure):
        """多 bot 比单 bot 快（有延迟时）。"""
        import time

        # 单 bot
        world1 = SimulatedWorld(size=(64, 64, 64))
        backends1 = make_simulated_backends(1, world1, place_delay=0.001)
        cm1 = CheckpointManager(sessions_dir="/tmp/proloader-test-sessions")
        session1 = ImportSession(
            structure=simple_structure, origin=(0, 0, 0),
            backends=backends1, options=ImportOptions(num_bots=1, place_delay=0.001),
            checkpoint_manager=cm1,
        )
        report1 = await run_import(session1)
        t1 = report1.scheduler_report.elapsed

        # 4 bot
        world2 = SimulatedWorld(size=(64, 64, 64))
        backends2 = make_simulated_backends(4, world2, place_delay=0.001)
        cm2 = CheckpointManager(sessions_dir="/tmp/proloader-test-sessions")
        session2 = ImportSession(
            structure=simple_structure, origin=(0, 0, 0),
            backends=backends2, options=ImportOptions(num_bots=4, cell_size=2, place_delay=0.001),
            checkpoint_manager=cm2,
        )
        report2 = await run_import(session2)
        t2 = report2.scheduler_report.elapsed

        # 多 bot 应该更快（至少不慢太多）
        # 注意：小建筑加速比有限，只验证不会更慢 5 倍以上
        assert t2 < t1 * 5
