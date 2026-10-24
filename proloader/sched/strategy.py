"""调度策略（创新点2）。

MVP 实现两种策略：
    - round_robin: 轮询分配 SubTask 到 backend
    - least_loaded: worker 空闲就拉新任务（work-stealing 风格，自然负载均衡）

完整版（Phase 11）：
    - greedy: 按 bbox 距离选最近的假人（减少移动）
"""

from __future__ import annotations

from enum import Enum
from typing import List


class SchedStrategy(str, Enum):
    """调度策略。"""

    ROUND_ROBIN = "round_robin"
    LEAST_LOADED = "least_loaded"
    GREEDY = "greedy"  # Phase 11


def get_strategy(name: str) -> SchedStrategy:
    """按名称获取策略枚举。"""
    try:
        return SchedStrategy(name)
    except ValueError:
        raise ValueError(
            f"未知调度策略 {name}，可选: {[s.value for s in SchedStrategy]}"
        )
