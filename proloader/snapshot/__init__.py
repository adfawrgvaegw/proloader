"""创新点4：断点续传 + 快照回滚。

导入前快照目标区域，中断从断点续传，出错一键回滚。

组件：
    - RegionSnapshot: 导入前快照（npz 存储）
    - PlacementJournal: WAL 放置日志（JSONL，fsync 持久化）
    - CheckpointManager: 会话状态机 + resume 支持
    - RollbackEngine: 用快照作"目标结构"再跑差异导入
"""

from __future__ import annotations

from proloader.snapshot.checkpoint import CheckpointManager, SessionMeta, SessionStatus
from proloader.snapshot.journal import PlacementJournal
from proloader.snapshot.rollback import RollbackEngine, RollbackReport
from proloader.snapshot.snapshot import RegionSnapshot

__all__ = [
    "RegionSnapshot",
    "PlacementJournal",
    "CheckpointManager",
    "SessionMeta",
    "SessionStatus",
    "RollbackEngine",
    "RollbackReport",
]
