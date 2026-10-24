"""创新点1 测试：差异增量导入。

测试 DiffEngine / RegionScanner / DiffPlanner。
核心验证：只计算出需要改变的方块操作（ADD/CHANGE/REMOVE/SKIP）。
"""

from __future__ import annotations

import numpy as np
import pytest

from proloader.backend.simulated import SimulatedBackend, SimulatedWorld
from proloader.diff.differ import DiffEngine
from proloader.diff.planner import DiffPlanner, PlannerOptions
from proloader.diff.scanner import RegionScanner
from proloader.model.block import Block, BlockState
from proloader.model.palette import BlockPalette
from proloader.model.region import BoundingBox, Region


def _make_region(blocks_3d, offset=(0, 0, 0)):
    """从嵌套列表构造 Region。blocks_3d[x][y][z] = palette_idx (0=air)。

    自动注册 palette 并把数组值重映射为真实 palette id。
    注意：palette.add() 返回的是顺序 id（0=air, 1=第一个新增...），
    不一定等于数组里的原始值，必须建立 idx→palette_id 映射后重写数组。
    """
    arr = np.array(blocks_3d, dtype=np.uint32)
    palette = BlockPalette()
    block_names = {1: "minecraft:stone", 2: "minecraft:dirt", 3: "minecraft:glass"}
    remap = {0: palette.air_id}
    for idx in sorted(set(int(i) for i in np.unique(arr)) - {0}):
        name = block_names.get(idx, f"minecraft:block_{idx}")
        remap[idx] = palette.add(Block(BlockState(name)))
    mapped = np.vectorize(lambda v: remap[int(v)])(arr).astype(np.uint32)
    return Region(palette=palette, blocks=mapped, offset=offset)


# ── DiffEngine ────────────────────────────────────────────────────────────────
class TestDiffEngine:
    def test_all_add(self):
        """空目标 → 全部 ADD。"""
        target = Region.empty((1, 3, 3), (0, 0, 0))
        desired = _make_region([[[1, 1, 1], [1, 1, 1], [1, 1, 1]]])
        plan = DiffEngine().compute(target, desired)
        assert plan.stats.adds == 9
        assert plan.stats.changes == 0
        assert plan.stats.removes == 0
        assert plan.stats.skipped == 0

    def test_all_skip(self):
        """目标 == 期望 → 全部 SKIP（零差异）。"""
        region = _make_region([[[1, 1, 1], [1, 1, 1], [1, 1, 1]]])
        plan = DiffEngine().compute(region, region)
        assert plan.stats.skipped == 9
        assert plan.stats.adds == 0
        assert plan.stats.saved_ratio == 1.0

    def test_change(self):
        """目标非空但不同 → CHANGE。"""
        target = _make_region([[[1, 1, 1], [1, 1, 1], [1, 1, 1]]])
        desired = _make_region([[[2, 2, 2], [2, 2, 2], [2, 2, 2]]])
        plan = DiffEngine().compute(target, desired)
        assert plan.stats.changes == 9
        assert plan.stats.adds == 0

    def test_remove(self):
        """目标有方块，期望空气 → REMOVE。"""
        target = _make_region([[[1, 1, 1], [1, 1, 1], [1, 1, 1]]])
        desired = Region.empty((1, 3, 3), (0, 0, 0))
        plan = DiffEngine().compute(target, desired)
        assert plan.stats.removes == 9

    def test_mixed(self):
        """混合：ADD + CHANGE + REMOVE + SKIP。"""
        # 目标: [1, 1, 0]  (2个stone)
        # 期望: [1, 2, 1]  (stone不变=SKIP, stone→dirt=CHANGE, air→stone=ADD)
        target = _make_region([[[1, 1, 0]]])
        desired = _make_region([[[1, 2, 1]]])
        plan = DiffEngine().compute(target, desired)
        assert plan.stats.skipped == 1
        assert plan.stats.changes == 1
        assert plan.stats.adds == 1

    def test_compute_full(self, simple_structure):
        """全量放置（无差异对比）。"""
        region = simple_structure.to_region((0, 0, 0))
        plan = DiffEngine().compute_full(region)
        assert plan.stats.adds == simple_structure.block_count
        assert plan.stats.skipped == 0

    def test_saved_ratio(self):
        """节省比例 = skipped / total。"""
        target = _make_region([[[1, 1, 1], [1, 1, 1], [1, 1, 1]]])
        desired = _make_region([[[1, 1, 1], [1, 1, 1], [1, 1, 1]]])
        plan = DiffEngine().compute(target, desired)
        assert plan.stats.saved_ratio == 1.0
        assert plan.stats.total == 9

    def test_diff_with_example(self, example_structure):
        """真实建筑在空世界上差异 = 全 ADD。"""
        region = example_structure.to_region((0, 0, 0))
        empty = Region.empty(region.size, region.offset)
        plan = DiffEngine().compute(empty, region)
        assert plan.stats.adds == example_structure.block_count
        assert plan.stats.saved_ratio < 1.0


# ── RegionScanner ─────────────────────────────────────────────────────────────
class TestRegionScanner:
    async def test_scan_empty_world(self, empty_world):
        """扫描空世界 → 全空气。"""
        bot = SimulatedBackend(empty_world, "scanner")
        await bot.connect()
        scanner = RegionScanner()
        bbox = BoundingBox.from_origin_size(0, 0, 0, 5, 5, 5)
        region = await scanner.scan(bot, bbox)
        assert region is not None
        assert region.non_air_count == 0

    async def test_scan_after_placement(self, empty_world):
        """放置后方块后扫描 → 能读到。"""
        bot = SimulatedBackend(empty_world, "scanner")
        await bot.connect()
        stone = Block(BlockState("minecraft:stone"))
        await bot.place_block(2, 3, 4, stone)
        scanner = RegionScanner()
        bbox = BoundingBox.from_origin_size(0, 0, 0, 5, 5, 5)
        region = await scanner.scan(bot, bbox)
        assert region.non_air_count == 1
        assert region.get_world(2, 3, 4) == stone


# ── DiffPlanner ───────────────────────────────────────────────────────────────
class TestDiffPlanner:
    def test_plan_generates_ops(self, simple_structure):
        """planner 把 DiffPlan 转为 PlacementOp 列表。"""
        region = simple_structure.to_region((0, 0, 0))
        plan = DiffEngine().compute_full(region)
        ops = DiffPlanner().plan(plan, PlannerOptions(sort_by_y=True))
        assert len(ops) == simple_structure.block_count
        for op in ops:
            assert op.x >= 0 and op.y >= 0 and op.z >= 0

    def test_plan_include_removes(self):
        """include_removes=False 时不包含 REMOVE 操作。"""
        target = _make_region([[[1, 1, 1]]])
        desired = Region.empty((1, 1, 3), (0, 0, 0))
        plan = DiffEngine().compute(target, desired)
        ops_with = DiffPlanner().plan(plan, PlannerOptions(include_removes=True))
        ops_without = DiffPlanner().plan(plan, PlannerOptions(include_removes=False))
        assert len(ops_with) == 3
        assert len(ops_without) == 0

    def test_sort_by_y(self, simple_structure):
        """sort_by_y=True 时 ops 按 y 坐标升序。"""
        region = simple_structure.to_region((0, 0, 0))
        plan = DiffEngine().compute_full(region)
        ops = DiffPlanner().plan(plan, PlannerOptions(sort_by_y=True))
        ys = [op.y for op in ops]
        assert ys == sorted(ys)
