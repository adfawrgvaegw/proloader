"""端到端演示：四个创新点完整跑通（SimulatedBackend）。

运行：python -m proloader.examples.demo_simulated_import
"""

from __future__ import annotations

import asyncio
import tempfile
from pathlib import Path

from proloader.backend.simulated import SimulatedBackend, SimulatedWorld
from proloader.backend.events import PlacementOp, OpType
from proloader.diff import DiffEngine, DiffPlanner, RegionScanner, PlannerOptions
from proloader.examples.gen_buildings import generate_house
from proloader.model import Block, BlockState, Region
from proloader.model.region import BoundingBox
from proloader.pipeline import make_simulated_backends, run_import
from proloader.session import ImportOptions, ImportSession
from proloader.sched import StructureChunker, ParallelScheduler
from proloader.snapshot import CheckpointManager, RollbackEngine, SessionStatus
from proloader.snapshot.checkpoint import SessionStatus as SS


async def main() -> None:
    print("=" * 64)
    print("proloader 端到端演示 —— 四大创新点（SimulatedBackend）")
    print("=" * 64)

    # 生成示例建筑
    tmp = Path(tempfile.mkdtemp())
    house_path = tmp / "house.mcstructure"
    structure = generate_house(house_path)
    print(f"\n建筑: {structure}")

    # ── 创新点1+2+4: 完整导入 ──────────────────────────────────────────────
    print("\n--- 导入（差异增量 + 多假人并行 + 快照）---")
    world = SimulatedWorld(size=(64, 64, 64))
    backends = make_simulated_backends(num_bots=3, world=world, place_delay=0.0005)
    cm = CheckpointManager(sessions_dir=str(tmp / "sessions"))

    # 先在世界里放一些"现状"方块，让差异导入有收益
    dirt = Block(BlockState("minecraft:dirt", {}))
    for x in range(7):
        for z in range(7):
            await backends[0].connect()
            await backends[0].place_block(x, 0, z, dirt)

    options = ImportOptions(num_bots=3, cell_size=4, use_diff=True, place_delay=0.0005)
    session = ImportSession(
        structure=structure,
        origin=(0, 0, 0),
        backends=backends,
        options=options,
        checkpoint_manager=cm,
    )

    report = await run_import(session)
    print(report.summary())

    # 验证世界正确
    region = world.read_region_sync(BoundingBox.from_origin_size(0, 0, 0, 7, 5, 7))
    print(f"\n世界非空气方块: {region.non_air_count}（建筑 {structure.block_count}）")

    # ── 创新点4: 回滚 ──────────────────────────────────────────────────────
    print("\n--- 回滚（创新点4）---")
    rollback_world = SimulatedWorld(size=(64, 64, 64))
    rb_bot = SimulatedBackend(rollback_world, "rb", place_delay=0)
    await rb_bot.connect()
    # 先放原始地面
    for x in range(7):
        for z in range(7):
            await rb_bot.place_block(x, 0, z, dirt)
    snap = report.snapshot
    # 模拟"导入改变了区域"
    glass = Block(BlockState("minecraft:glass", {}))
    await rb_bot.place_block(0, 0, 0, glass)
    await rb_bot.place_block(0, 4, 0, glass)
    print(f"回滚前: (0,0,0)={await rollback_world.get_block(0,0,0)}")
    rb_engine = RollbackEngine()
    rb_report = await rb_engine.rollback(rb_bot, snap)
    print(rb_report.summary())
    print(f"回滚后: (0,0,0)={await rollback_world.get_block(0,0,0)} (应恢复 dirt)")
    assert await rollback_world.get_block(0, 0, 0) == dirt, "回滚失败"

    # ── 创新点1: 单独演示差异统计 ───────────────────────────────────────────
    print("\n--- 差异统计（创新点1）---")
    target = world.read_region_sync(BoundingBox.from_origin_size(0, 0, 0, 7, 5, 7))
    structure_region = structure.to_region((0, 0, 0))
    # 再次比较（应该全 skip，因为已导入完成）
    plan2 = DiffEngine().compute(target, structure_region)
    print(f"已导入后再次比较: {plan2.stats.summary()}（应 100% 跳过）")
    assert plan2.total_ops == 0, "导入完成后应无差异"

    print("\n" + "=" * 64)
    print("✓ 四大创新点全部演示成功")
    print("=" * 64)


if __name__ == "__main__":
    asyncio.run(main())
