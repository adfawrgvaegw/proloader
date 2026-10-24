"""后端层：PlacementBackend 抽象接口与实现。

后端实例代表"一个可放置方块的执行体"：
    - SimulatedBackend: numpy 模拟世界的一个 slot（MVP 必跑）
    - BedrockProtocolBackend: 一个已登录的基岩版假人连接（Phase 9）
    - NeteaseBackend: 一个网易租赁服内的假人（Phase 10）

上层调度器/差异算法/Web UI 只依赖 PlacementBackend 接口（backend/base.py），
通过依赖注入传入具体后端，实现协议无关。
"""

from __future__ import annotations

from proloader.backend.base import PlacementBackend
from proloader.backend.events import (
    BackendCapabilities,
    BackendCheckpoint,
    BlockPlacedEvent,
    DoneEvent,
    ErrorEvent,
    OpType,
    PlacementEvent,
    PlacementOp,
    ProgressEvent,
)
from proloader.backend.simulated import SimulatedBackend, SimulatedWorld

__all__ = [
    # 接口
    "PlacementBackend",
    # 事件
    "BackendCapabilities",
    "BackendCheckpoint",
    "PlacementEvent",
    "PlacementOp",
    "OpType",
    "ProgressEvent",
    "ErrorEvent",
    "DoneEvent",
    "BlockPlacedEvent",
    # 实现
    "SimulatedBackend",
    "SimulatedWorld",
]
