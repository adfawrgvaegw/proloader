"""SubTask —— 一个切块的放置任务（创新点2 基本单元）。"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from typing import List, Optional

from proloader.backend.events import PlacementOp
from proloader.model.region import BoundingBox


class SubTaskStatus(str, Enum):
    """子任务状态。"""

    PENDING = "pending"  # 待执行
    RUNNING = "running"  # 执行中
    DONE = "done"  # 成功完成
    FAILED = "failed"  # 失败
    SKIPPED = "skipped"  # 跳过（断点续传时已完成）


@dataclass
class SubTask:
    """一个切块的放置任务。

    多假人并行调度的基本单元：每个 SubTask 由一个假人完成。
    含 bbox（用于区域锁）、ops（放置操作列表）。

    Attributes:
        id: 子任务 id（唯一，用于 journal 断点续传）
        bbox: 子任务包围盒（世界坐标，用于 RegionLock）
        ops: 放置操作列表
        status: 当前状态
        assigned_backend: 被分配到的后端 id
    """

    id: int
    bbox: BoundingBox
    ops: List[PlacementOp] = field(default_factory=list)
    status: SubTaskStatus = SubTaskStatus.PENDING
    assigned_backend: Optional[str] = None
    placed: int = 0  # 该子任务已放置数
    failed: int = 0  # 该子任务失败数

    @property
    def ops_count(self) -> int:
        return len(self.ops)

    @property
    def is_empty(self) -> bool:
        """无放置操作（纯空气切块）。"""
        return len(self.ops) == 0

    def summary(self) -> dict:
        return {
            "id": self.id,
            "bbox": list(self.bbox),
            "ops_count": self.ops_count,
            "status": self.status.value,
            "placed": self.placed,
            "failed": self.failed,
            "assigned_backend": self.assigned_backend,
        }
