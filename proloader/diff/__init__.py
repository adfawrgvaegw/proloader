"""创新点1：差异增量导入。

导入前扫描目标区域，只放"需要改变"的方块，跳过相同方块。减少 50-90% 放置量。

流程：
    1. RegionScanner.scan(backend, bbox) → 现状 Region（或 None 退化全量）
    2. DiffEngine.compute(target, structure) → DiffPlan（adds/removes/changes）
    3. DiffPlanner.plan(diff_plan, options) → PlacementOp 列表（排序、过滤）
"""

from __future__ import annotations

from proloader.diff.differ import DiffEngine, DiffPlan, DiffStats
from proloader.diff.planner import DiffPlanner, PlannerOptions
from proloader.diff.scanner import RegionScanner

__all__ = [
    "DiffEngine",
    "DiffPlan",
    "DiffStats",
    "DiffPlanner",
    "PlannerOptions",
    "RegionScanner",
]
