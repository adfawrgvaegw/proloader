"""proloader — 网易我的世界租赁服建筑导入器（学习用途）。

四大创新点：
    1. 差异增量导入（diff/）—— 只放需要改变的方块
    2. 多假人并行调度（sched/）—— 多 bot 并行 + 区域锁
    3. 3D Web 可视化（web/）—— Three.js 预览 + 实时进度
    4. 断点续传 + 快照回滚（snapshot/）—— WAL 日志 + 一键回滚
"""

from __future__ import annotations

__version__ = "0.1.1"

__all__ = ["__version__"]
