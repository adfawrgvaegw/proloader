"""后端事件类型与放置操作定义。

事件流是创新点3（Web 可视化）的数据源：Web UI 订阅 events() 流，实时高亮方块。
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from typing import Optional, Tuple, Union

from proloader.model.block import Block


class OpType(str, Enum):
    """放置操作类型（创新点1 差异导入的产物）。"""

    ADD = "ADD"        # 现状空气，期望非空气 → 放置
    REMOVE = "REMOVE"  # 现状非空气，期望空气 → 放置空气（破坏）
    CHANGE = "CHANGE"  # 都非空气但不同 → 替换
    PLACE = "PLACE"    # 全量放置（未启用差异时）


@dataclass
class PlacementOp:
    """单个放置操作。

    Attributes:
        x, y, z: 世界坐标
        block: 要放置的方块（REMOVE 时是 air）
        op_type: 操作类型
    """

    x: int
    y: int
    z: int
    block: Block
    op_type: OpType = OpType.PLACE

    @property
    def coord(self) -> Tuple[int, int, int]:
        return (self.x, self.y, self.z)


# ── 事件类型 ──────────────────────────────────────────────────────────────────
@dataclass
class ProgressEvent:
    """放置进度事件。每次 place_block / place_batch 后触发。"""

    backend_id: str
    placed: int
    failed: int
    total: int
    last_coord: Optional[Tuple[int, int, int]] = None
    last_op_type: Optional[OpType] = None
    type: str = "progress"


@dataclass
class ErrorEvent:
    """错误事件。单个方块放置失败。"""

    backend_id: str
    error: str
    recoverable: bool = True
    coord: Optional[Tuple[int, int, int]] = None
    type: str = "error"


@dataclass
class DoneEvent:
    """后端完成所有任务事件。"""

    backend_id: str
    placed: int
    failed: int
    type: str = "done"


@dataclass
class BlockPlacedEvent:
    """单个方块放置完成事件（细粒度，Web 高亮用）。

    Web UI 订阅此事件实时高亮已放置方块。
    """

    backend_id: str
    x: int
    y: int
    z: int
    op_type: OpType
    type: str = "block_placed"


PlacementEvent = Union[ProgressEvent, ErrorEvent, DoneEvent, BlockPlacedEvent]


# ── 后端能力声明 ──────────────────────────────────────────────────────────────
@dataclass
class BackendCapabilities:
    """后端能力声明，调度器据此选择策略。

    Attributes:
        can_read_region: 是否支持读区域（创新点1 依赖）。不支持则退化为全量放置。
        needs_teleport: 是否需要移动假人到放置点（真实后端 True，模拟器 False）。
        needs_chunking: 是否需要按 chunk 切块（真实后端 True，受加载范围限制）。
        max_blocks_per_batch: 单次 place_batch 最大方块数。
        load_radius: 假人加载半径（真实后端 ~64）。
    """

    can_read_region: bool = True
    needs_teleport: bool = False
    needs_chunking: bool = False
    max_blocks_per_batch: int = 256
    load_radius: int = 64


# ── 后端检查点（创新点4 依赖）──────────────────────────────────────────────────
@dataclass
class BackendCheckpoint:
    """后端状态检查点，用于断点续传。

    Attributes:
        backend_id: 后端 id
        position: 假人当前位置
        placed_count: 已确认放置数
        last_subtask_id: 最后完成的 subtask id
    """

    backend_id: str
    position: Optional[Tuple[float, float, float]] = None
    placed_count: int = 0
    last_subtask_id: Optional[int] = None
    extra: dict = field(default_factory=dict)
