"""RegionLock —— 基于 bbox 的区域锁（创新点2 冲突避免）。

防止多个假人同时放置同一区域的方块导致冲突。

实现：
    用 asyncio.Condition + 持有锁的 bbox 列表。
    acquire(bbox): 等待直到没有已锁 bbox 与之重叠，然后加入持有列表。
    release(token): 从持有列表移除，唤醒所有等待者。

    这保证：
      - 不重叠的 bbox 可并行获取（并行放置）
      - 重叠的 bbox 串行获取（避免冲突）
      - 无死锁（每次 release 唤醒所有等待者重新竞争）

学习要点：
    - 区域锁是多假人并行的关键：不锁会让方块互相覆盖
    - 按空间切块（chunker）保证块通常不重叠，锁几乎不阻塞
    - 真实冲突场景：两个假人被分到相邻区域且 bbox 边界重叠
"""

from __future__ import annotations

import asyncio
from dataclasses import dataclass
from typing import List, Tuple

from proloader.model.region import BoundingBox


@dataclass
class LockToken:
    """锁令牌，release 时传入。"""

    bbox: BoundingBox
    holder: str  # 持有者 id（backend_id）


class RegionLockManager:
    """区域锁管理器。

    用法：
        manager = RegionLockManager()
        token = await manager.acquire(bbox, "bot-0")
        try:
            # 放置方块
        finally:
            await manager.release(token)
    """

    def __init__(self) -> None:
        self._held: List[Tuple[BoundingBox, str]] = []
        self._cond = asyncio.Condition()

    async def acquire(self, bbox: BoundingBox, holder: str) -> LockToken:
        """获取区域锁。等待直到没有重叠的已锁 bbox。"""
        async with self._cond:
            while self._has_overlap(bbox, holder):
                await self._cond.wait()
            self._held.append((bbox, holder))
            return LockToken(bbox=bbox, holder=holder)

    async def release(self, token: LockToken) -> None:
        """释放区域锁。"""
        async with self._cond:
            self._held = [
                (b, h) for b, h in self._held if not (b == token.bbox and h == token.holder)
            ]
            self._cond.notify_all()

    def _has_overlap(self, bbox: BoundingBox, holder: str) -> bool:
        """检查 bbox 是否与已锁区域重叠（排除自己持有的）。"""
        for held, h in self._held:
            if h == holder:
                continue  # 同一持有者不与自己冲突
            if held.overlaps(bbox):
                return True
        return False

    @property
    def held_count(self) -> int:
        return len(self._held)
