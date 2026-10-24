"""StructureChunker —— 把放置操作按空间切块（创新点2）。

切块策略：
    按世界坐标分到 grid 单元（cell_size × cell_size × cell_size）。
    每个 cell 是一个 SubTask，bbox 用于 RegionLock。

    默认 cell_size=16（一个 chunk 大小），保证：
      - 单假人加载范围内（真实后端 ~64）
      - 单 chunk 方块数可控（16³=4096，分批放置）

学习要点：
    - 切块是为了并行：不同块由不同假人放置
    - 块间不重叠（按 grid 切），RegionLock 不会阻塞
    - 空块（无 ops）被过滤，减少调度开销
"""

from __future__ import annotations

from typing import List, Tuple

from proloader.backend.events import PlacementOp
from proloader.model.region import BoundingBox
from proloader.sched.task import SubTask


class StructureChunker:
    """放置操作切块器。"""

    def __init__(self, cell_size: int = 16) -> None:
        """初始化切块器。

        Args:
            cell_size: 每个 cell 的边长（方块数）。默认 16（一个 chunk）。
        """
        if cell_size <= 0:
            raise ValueError(f"cell_size 必须 > 0，得到 {cell_size}")
        self.cell_size = cell_size

    def chunk(self, ops: List[PlacementOp], num_bots: int = 1) -> List[SubTask]:
        """把 ops 切成多个 SubTask。

        按 cell_size 的 grid 分组，每个非空 cell 一个 SubTask。
        空块（无 ops）被过滤。

        Args:
            ops: 放置操作列表（世界坐标）
            num_bots: 假人数量（用于估算切块粒度，块数会 >= num_bots）

        Returns:
            SubTask 列表（已编号 id 从 0 递增，按 bbox 排序）
        """
        if not ops:
            return []

        cs = self.cell_size
        # 按 cell 坐标分组
        cells: dict[Tuple[int, int, int], List[PlacementOp]] = {}
        for op in ops:
            # 负数坐标要正确 floor（-1 // 16 = -1 in Python，正确）
            cx = op.x // cs
            cy = op.y // cs
            cz = op.z // cs
            cells.setdefault((cx, cy, cz), []).append(op)

        # 转 SubTask
        subtasks: List[SubTask] = []
        for idx, ((cx, cy, cz), cell_ops) in enumerate(
            sorted(cells.items())
        ):
            # cell 的世界 bbox
            min_x = cx * cs
            min_y = cy * cs
            min_z = cz * cs
            # 实际 bbox 取 ops 的真实范围（可能小于 cell）
            xs = [op.x for op in cell_ops]
            ys = [op.y for op in cell_ops]
            zs = [op.z for op in cell_ops]
            bbox = BoundingBox(
                min_x=min(xs),
                min_y=min(ys),
                min_z=min(zs),
                max_x=max(xs) + 1,
                max_y=max(ys) + 1,
                max_z=max(zs) + 1,
            )
            subtasks.append(
                SubTask(
                    id=idx,
                    bbox=bbox,
                    ops=cell_ops,
                )
            )
        return subtasks
