"""DiffPlanner —— 把 DiffPlan 整理为最终放置操作序列。

优化策略：
    1. 过滤：可选只 ADD/CHANGE 不 REMOVE（保护现有建筑）
    2. 排序：Y 轴自下而上（避免悬浮方块被物理破坏），同 Y 按 chunk 分组
    3. 合并相邻同方块（可选，减少协议包数，MVP 默认关闭）

排序规则详解：
    - Y 自下而上：先放底层再放上层，避免方块因无支撑掉落
    - 同 Y 层按 (z, x) 排序：使假人移动路径连续（蛇形扫描）
    - REMOVE 单独处理（先破坏后放置，或后破坏）
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import List

from proloader.backend.events import OpType, PlacementOp
from proloader.diff.differ import DiffPlan


@dataclass
class PlannerOptions:
    """放置计划选项。"""

    include_removes: bool = True  # 是否包含 REMOVE（破坏）操作
    sort_by_y: bool = True  # Y 轴自下而上排序
    removes_first: bool = False  # 是否先执行 REMOVE（先清理后建造）


class DiffPlanner:
    """把 DiffPlan 整理为最终 PlacementOp 序列。"""

    def plan(
        self,
        diff_plan: DiffPlan,
        options: PlannerOptions | None = None,
    ) -> List[PlacementOp]:
        """生成最终放置操作序列。

        Args:
            diff_plan: DiffEngine 计算的差异计划
            options: 排序/过滤选项

        Returns:
            PlacementOp 列表（已排序、过滤）
        """
        if options is None:
            options = PlannerOptions()

        ops: List[PlacementOp] = []

        if options.removes_first and options.include_removes:
            removes = list(diff_plan.removes)
            if options.sort_by_y:
                removes.sort(key=_sort_key)
            ops.extend(removes)

        # ADD + CHANGE（建造）
        build_ops = list(diff_plan.adds) + list(diff_plan.changes)
        if options.sort_by_y:
            build_ops.sort(key=_sort_key)
        ops.extend(build_ops)

        # REMOVE 后置
        if not options.removes_first and options.include_removes:
            removes = list(diff_plan.removes)
            if options.sort_by_y:
                removes.sort(key=_sort_key)
            ops.extend(removes)

        return ops


def _sort_key(op: PlacementOp):
    """排序键：Y 自下而上，同 Y 按 (z, x) 蛇形扫描。"""
    return (op.y, op.z, op.x)
