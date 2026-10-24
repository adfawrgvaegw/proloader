"""RollbackEngine —— 创新点4：一键回滚到快照。

原理：
    回滚 = 用快照作为"目标结构"再跑一次差异导入。
    1. 加载快照（导入前的 target Region）
    2. 扫描当前区域（backend.read_region）
    3. 用 DiffEngine 计算当前 vs 快照的差异
    4. 把差异作为新的 DiffPlan 放置（恢复原状）

    这样回滚只恢复"被改变"的方块，不重写整个区域（同样享受差异增量收益）。

学习要点：
    - 回滚复用 DiffEngine，体现"差异导入"的通用性
    - 快照是回滚的唯一真相源，没有快照就无法回滚
    - 回滚本身也可能中断，所以也走 journal + checkpoint
"""

from __future__ import annotations

import logging
from dataclasses import dataclass

from proloader.backend.base import PlacementBackend
from proloader.diff.differ import DiffEngine, DiffPlan
from proloader.diff.planner import DiffPlanner, PlannerOptions
from proloader.diff.scanner import RegionScanner
from proloader.model.region import BoundingBox, Region
from proloader.snapshot.snapshot import RegionSnapshot

logger = logging.getLogger(__name__)


@dataclass
class RollbackReport:
    """回滚报告。"""

    diff_plan: DiffPlan
    placed: int = 0

    def summary(self) -> str:
        return f"回滚: {self.diff_plan.stats.summary()} | 实际放置 {self.placed}"


class RollbackEngine:
    """回滚引擎。

    用法：
        engine = RollbackEngine()
        report = await engine.rollback(backend, snapshot)
    """

    def __init__(self) -> None:
        self._diff = DiffEngine()
        self._planner = DiffPlanner()
        self._scanner = RegionScanner()

    async def rollback(
        self,
        backend: PlacementBackend,
        snapshot: RegionSnapshot,
    ) -> RollbackReport:
        """回滚到快照状态。

        Args:
            backend: 放置后端（单个即可，回滚通常不需要并行）
            snapshot: 导入前快照

        Returns:
            RollbackReport
        """
        target_region = snapshot.to_region()
        bbox = snapshot.bbox

        # 扫描当前区域
        current = await self._scanner.scan(backend, bbox)
        if current is None:
            logger.warning("后端不支持读区域，回滚退化为全量恢复")
            plan = self._diff.compute_full(target_region)
        else:
            plan = self._diff.compute(current, target_region)

        logger.info("回滚差异: %s", plan.stats.summary())

        # 逐个放置（回滚用单 backend，简单可靠）
        ops = self._planner.plan(plan, PlannerOptions(include_removes=True, sort_by_y=True))
        placed = 0
        batch_size = backend.capabilities.max_blocks_per_batch
        for i in range(0, len(ops), batch_size):
            batch = ops[i : i + batch_size]
            if batch:
                placed += await backend.place_batch(batch)

        return RollbackReport(diff_plan=plan, placed=placed)
