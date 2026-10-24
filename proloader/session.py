"""ImportSession —— 一次导入会话的上下文。

持有会话 id、建筑、原点、后端、检查点管理器等，贯穿导入全流程。
pipeline 用它来协调快照、差异、调度、日志。
"""

from __future__ import annotations

import uuid
from dataclasses import dataclass, field
from typing import List, Optional, Tuple

from proloader.backend.base import PlacementBackend
from proloader.model.structure import Structure
from proloader.snapshot.checkpoint import CheckpointManager, SessionMeta, SessionStatus


@dataclass
class ImportOptions:
    """导入选项。"""

    num_bots: int = 1  # 假人数量
    cell_size: int = 16  # 切块大小
    use_diff: bool = True  # 是否差异增量导入（False 则全量）
    include_removes: bool = True  # 是否包含破坏操作
    place_delay: float = 0.0  # 模拟放置延迟（SimulatedBackend 用）
    session_id: Optional[str] = None  # 指定 session id（resume 时）


@dataclass
class ImportSession:
    """一次导入会话上下文。"""

    structure: Structure
    origin: Tuple[int, int, int]
    backends: List[PlacementBackend]
    options: ImportOptions
    checkpoint_manager: CheckpointManager
    session_id: str = field(default_factory=lambda: uuid.uuid4().hex[:12])
    meta: Optional[SessionMeta] = None

    @property
    def bbox(self):
        """建筑放置后的世界 bbox。"""
        return self.structure.bbox_at(self.origin)

    def init_meta(self) -> SessionMeta:
        """创建会话元数据。"""
        self.meta = SessionMeta(
            session_id=self.session_id,
            structure_name=self.structure.name,
            origin=list(self.origin),
            size=list(self.structure.size),
            total_ops=self.structure.block_count,
            backend_type=self.backends[0].backend_id.split("-")[0] if self.backends else "unknown",
            num_bots=len(self.backends),
        )
        return self.meta

    def to_region(self):
        """建筑转为放置 Region（offset=origin）。"""
        return self.structure.to_region(self.origin)
