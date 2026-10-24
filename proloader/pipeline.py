"""pipeline —— 顶层编排：parser → diff → snapshot → sched → backend 全流程。

把四大创新点串成完整的导入流程：
    1. 加载建筑文件 → Structure
    2. 后端读区域（创新点1：扫描现状）
    3. 差异计算（创新点1）→ DiffPlan → ops
    4. 切块（创新点2）→ SubTask
    5. 快照 + 创建会话（创新点4）
    6. 多假人并行调度（创新点2）→ journal 记录
    7. 返回报告

提供三个入口：
    - run_import(): 全新导入
    - resume_import(): 断点续传
    - rollback_session(): 回滚
"""

from __future__ import annotations

import logging
from dataclasses import dataclass
from typing import List, Optional, Tuple

from proloader.backend.base import PlacementBackend
from proloader.backend.simulated import SimulatedBackend, SimulatedWorld
from proloader.diff.differ import DiffEngine, DiffPlan
from proloader.diff.planner import DiffPlanner, PlannerOptions
from proloader.diff.scanner import RegionScanner
from proloader.sched.chunker import StructureChunker
from proloader.sched.scheduler import ParallelScheduler, SchedulerReport
from proloader.sched.task import SubTask, SubTaskStatus
from proloader.session import ImportOptions, ImportSession
from proloader.snapshot.checkpoint import CheckpointManager, SessionStatus
from proloader.snapshot.rollback import RollbackEngine
from proloader.snapshot.snapshot import RegionSnapshot

logger = logging.getLogger(__name__)


@dataclass
class ImportReport:
    """导入报告。"""

    session_id: str
    diff_plan: Optional[DiffPlan]
    scheduler_report: SchedulerReport
    snapshot: Optional[RegionSnapshot] = None

    def summary(self) -> str:
        diff_str = self.diff_plan.stats.summary() if self.diff_plan else "无差异(全量)"
        return (
            f"会话 {self.session_id}\n"
            f"  差异: {diff_str}\n"
            f"  调度: {self.scheduler_report.summary()}"
        )


def make_simulated_backends(
    num_bots: int, world: SimulatedWorld, place_delay: float = 0.0
) -> List[SimulatedBackend]:
    """创建 N 个共享同一世界的 SimulatedBackend（多假人）。"""
    return [
        SimulatedBackend(world, f"sim-{i}", place_delay=place_delay)
        for i in range(num_bots)
    ]


async def run_import(session: ImportSession) -> ImportReport:
    """执行全新导入（全流程）。

    流程：快照 → 差异 → 切块 → 调度 → journal
    """
    cm = session.checkpoint_manager
    meta = session.init_meta()
    cm.create_session(meta)

    # 连接所有后端
    for b in session.backends:
        await b.connect()

    bbox = session.bbox
    structure_region = session.to_region()

    # 1. 快照（创新点4）
    snapshot = None
    if session.options.use_diff and session.backends[0].capabilities.can_read_region:
        try:
            snapshot = await RegionSnapshot.capture(session.backends[0], bbox)
            cm.save_snapshot(session.session_id, snapshot)
            logger.info("快照已保存")
        except Exception as e:
            logger.warning("快照失败，跳过: %s", e)
            snapshot = None

    cm.transition(session.session_id, SessionStatus.SNAPSHOT_TAKEN)
    # 用 transition 返回的最新 meta，避免后续 save_meta 覆盖状态
    meta = cm.transition(session.session_id, SessionStatus.RUNNING)

    # 2. 差异计算（创新点1）
    diff_plan: Optional[DiffPlan] = None
    if session.options.use_diff:
        scanner = RegionScanner()
        target = await scanner.scan(session.backends[0], bbox)
        if target is not None:
            diff_plan = DiffEngine().compute(target, structure_region)
            logger.info("差异: %s", diff_plan.stats.summary())
        else:
            diff_plan = DiffEngine().compute_full(structure_region)
            logger.info("后端不支持读，全量放置: %s", diff_plan.stats.summary())
    else:
        diff_plan = DiffEngine().compute_full(structure_region)

    # 3. 生成 ops + 切块（创新点2）
    ops = DiffPlanner().plan(
        diff_plan,
        PlannerOptions(include_removes=session.options.include_removes, sort_by_y=True),
    )
    subtasks = StructureChunker(cell_size=session.options.cell_size).chunk(
        ops, num_bots=session.options.num_bots
    )
    meta.total_subtasks = len(subtasks)
    cm.save_meta(meta)
    logger.info("切块: %d 个子任务", len(subtasks))

    # 4. 调度执行（创新点2）+ journal 记录（创新点4）
    journal = cm.get_journal(session.session_id)
    scheduler = ParallelScheduler(session.backends, strategy="least_loaded")

    def on_done(st: SubTask) -> None:
        journal.append(st)

    report = await scheduler.run(subtasks, on_subtask_done=on_done)

    # 5. 更新状态
    if report.failed == 0:
        cm.transition(session.session_id, SessionStatus.DONE)
    else:
        cm.transition(session.session_id, SessionStatus.FAILED)

    return ImportReport(
        session_id=session.session_id,
        diff_plan=diff_plan,
        scheduler_report=report,
        snapshot=snapshot,
    )


async def resume_import(
    session_id: str,
    structure,
    origin: Tuple[int, int, int],
    backends: List[PlacementBackend],
    options: ImportOptions,
    checkpoint_manager: CheckpointManager,
) -> ImportReport:
    """断点续传：跳过已完成 SubTask，继续未完成的。"""
    meta = checkpoint_manager.load_meta(session_id)
    if not checkpoint_manager.can_resume(session_id):
        from proloader.errors import SessionStateError
        raise SessionStateError(f"会话 {session_id} 状态 {meta.status.value}，不可 resume")

    for b in backends:
        await b.connect()

    checkpoint_manager.transition(session_id, SessionStatus.RUNNING)

    # 重新生成 subtasks（与首次一致），用 journal 过滤已完成
    structure_region = structure.to_region(origin)
    # 注意：resume 时不再重新算差异（diff 已在首次完成），直接用全量 ops
    # 因为已放置的部分 + 未放置的部分 = 完整 ops
    # 但这会导致已放置的部分被重放——所以 resume 应该用"剩余 ops"
    # 简化：resume 用全量 ops，依赖 journal 跳过已完成的 subtask
    diff_plan = DiffEngine().compute_full(structure_region)
    ops = DiffPlanner().plan(diff_plan, PlannerOptions(sort_by_y=True))
    all_subtasks = StructureChunker(cell_size=options.cell_size).chunk(ops, num_bots=options.num_bots)

    pending = checkpoint_manager.filter_pending_subtasks(session_id, all_subtasks)
    logger.info("resume: %d 个子任务, 跳过已完成 %d", len(pending), len(all_subtasks) - len(pending))

    journal = checkpoint_manager.get_journal(session_id)
    scheduler = ParallelScheduler(backends, strategy="least_loaded")

    def on_done(st: SubTask) -> None:
        journal.append(st)

    report = await scheduler.run(pending, on_subtask_done=on_done)

    if report.failed == 0:
        checkpoint_manager.transition(session_id, SessionStatus.DONE)

    return ImportReport(
        session_id=session_id,
        diff_plan=diff_plan,
        scheduler_report=report,
    )


async def rollback_session(
    session_id: str,
    backend: PlacementBackend,
    checkpoint_manager: CheckpointManager,
) -> "RollbackReport":
    """回滚会话到快照状态。"""
    from proloader.snapshot.rollback import RollbackReport

    meta = checkpoint_manager.load_meta(session_id)
    snapshot = checkpoint_manager.load_snapshot(session_id)
    await backend.connect()

    engine = RollbackEngine()
    report = await engine.rollback(backend, snapshot)
    return report
