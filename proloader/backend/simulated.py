"""SimulatedBackend —— numpy 模拟世界后端（MVP 灵魂）。

学习要点：
    - 用 numpy 数组模拟一个虚拟世界，零网络依赖
    - 多个 SimulatedBackend 实例共享同一个 SimulatedWorld（模拟多假人在同一服务器）
    - 事件总线（asyncio.Queue）驱动 Web UI 实时可视化
    - 可配置 place_delay 模拟真实网络延迟（用于演示并行加速比）

这是 proloader MVP 的核心：四个创新点都在此后端上完整验证，无需真实服务器。
"""

from __future__ import annotations

import asyncio
from dataclasses import dataclass, field
from typing import AsyncIterator, Optional, Tuple

import numpy as np

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
from proloader.errors import NotSupportedError
from proloader.model.block import Block
from proloader.model.palette import BlockPalette
from proloader.model.region import BoundingBox, Region


class SimulatedWorld:
    """模拟世界：numpy 数组 + 调色板。

    多个 SimulatedBackend 共享同一个 SimulatedWorld 实例（模拟多假人在同一服务器）。
    世界尺寸固定，超出范围的放置会被忽略（带告警事件）。
    """

    def __init__(
        self,
        size: Tuple[int, int, int] = (256, 256, 256),
        offset: Tuple[int, int, int] = (0, 0, 0),
    ) -> None:
        self.size = size
        self.offset = offset
        # 世界用独立 palette（air=0），所有 backend 共享
        self.palette = BlockPalette()
        # 全空气初始化
        self.blocks = np.zeros(size, dtype=np.uint32)
        # 统计
        self.total_placed = 0
        self._lock = asyncio.Lock()  # 保护并发写

    @property
    def bbox(self) -> BoundingBox:
        sx, sy, sz = self.size
        ox, oy, oz = self.offset
        return BoundingBox.from_origin_size(ox, oy, oz, sx, sy, sz)

    def _world_to_local(self, wx: int, wy: int, wz: int) -> Optional[Tuple[int, int, int]]:
        """世界坐标 → 本地坐标。越界返回 None。"""
        ox, oy, oz = self.offset
        lx, ly, lz = wx - ox, wy - oy, wz - oz
        sx, sy, sz = self.size
        if 0 <= lx < sx and 0 <= ly < sy and 0 <= lz < sz:
            return (lx, ly, lz)
        return None

    async def get_block(self, wx: int, wy: int, wz: int) -> Block:
        """读取世界坐标的方块。"""
        local = self._world_to_local(wx, wy, wz)
        if local is None:
            return Block.air()
        lx, ly, lz = local
        return self.palette.get(int(self.blocks[lx, ly, lz]))

    async def set_block(self, wx: int, wy: int, wz: int, block: Block) -> bool:
        """设置世界坐标的方块。返回是否成功（越界返回 False）。"""
        local = self._world_to_local(wx, wy, wz)
        if local is None:
            return False
        lx, ly, lz = local
        async with self._lock:
            bid = self.palette.add(block)
            self.blocks[lx, ly, lz] = bid
            self.total_placed += 1
        return True

    def read_region_sync(self, box: BoundingBox) -> Region:
        """同步读取区域（用于差异导入扫描）。"""
        ox, oy, oz = self.offset
        sx0 = max(0, box.min_x - ox)
        sy0 = max(0, box.min_y - oy)
        sz0 = max(0, box.min_z - oz)
        sx1 = min(self.size[0], box.max_x - ox)
        sy1 = min(self.size[1], box.max_y - oy)
        sz1 = min(self.size[2], box.max_z - oz)
        if sx1 <= sx0 or sy1 <= sy0 or sz1 <= sz0:
            return Region.empty((1, 1, 1), (box.min_x, box.min_y, box.min_z))
        sub = self.blocks[sx0:sx1, sy0:sy1, sz0:sz1].copy()
        return Region(
            palette=self.palette,
            blocks=sub,
            offset=(sx0 + ox, sy0 + oy, sz0 + oz),
        )


@dataclass
class SimulatedBackend:
    """模拟后端：操作 SimulatedWorld，实现 PlacementBackend 接口。

    一个实例 = 一个假人。多个实例共享同一个 world 模拟多假人并行。
    place_delay 模拟真实网络延迟（秒），用于演示并行加速比。

    Example:
        >>> world = SimulatedWorld()
        >>> bots = [SimulatedBackend(world, f"bot-{i}") for i in range(3)]
        >>> for bot in bots:
        ...     await bot.connect()
    """

    world: SimulatedWorld
    bid: str
    place_delay: float = 0.0  # 模拟放置延迟（秒），测试并行用
    _position: Tuple[float, float, float] = (0.0, 0.0, 0.0)
    _orientation: Tuple[float, float] = (0.0, 0.0)
    _placed_count: int = 0
    _failed_count: int = 0
    _total: int = 0
    _connected: bool = False
    _event_queue: "asyncio.Queue[PlacementEvent]" = field(default_factory=asyncio.Queue)
    _last_subtask_id: Optional[int] = None

    # ── PlacementBackend 接口实现 ──────────────────────────────────────────────
    @property
    def backend_id(self) -> str:
        return self.bid

    @property
    def capabilities(self) -> BackendCapabilities:
        # SimulatedBackend 支持读区域、不需要 teleport、不需要切块
        return BackendCapabilities(
            can_read_region=True,
            needs_teleport=False,
            needs_chunking=False,
            max_blocks_per_batch=1024,  # 模拟器无协议限制
            load_radius=10**9,  # 无限加载范围
        )

    async def connect(self) -> None:
        self._connected = True

    async def disconnect(self) -> None:
        self._connected = False

    async def read_region(self, box: BoundingBox) -> Region:
        if not self._connected:
            raise NotSupportedError("后端未连接")
        return self.world.read_region_sync(box)

    async def place_block(self, x: int, y: int, z: int, block: Block) -> None:
        if not self._connected:
            raise NotSupportedError("后端未连接")
        if self.place_delay > 0:
            await asyncio.sleep(self.place_delay)
        ok = await self.world.set_block(x, y, z, block)
        if ok:
            self._placed_count += 1
            # place_block 不区分 ADD/CHANGE，用 PLACE；放空气视为 REMOVE
            op_type = OpType.REMOVE if block.is_air else OpType.PLACE
            await self._event_queue.put(
                BlockPlacedEvent(
                    backend_id=self.bid, x=x, y=y, z=z, op_type=op_type
                )
            )
        else:
            self._failed_count += 1
            await self._event_queue.put(
                ErrorEvent(
                    backend_id=self.bid,
                    error=f"坐标 ({x},{y},{z}) 越界",
                    recoverable=True,
                    coord=(x, y, z),
                )
            )

    async def place_batch(self, ops: list[PlacementOp]) -> int:
        success = 0
        for op in ops:
            try:
                if self.place_delay > 0:
                    await asyncio.sleep(self.place_delay)
                ok = await self.world.set_block(op.x, op.y, op.z, op.block)
                if ok:
                    self._placed_count += 1
                    success += 1
                    await self._event_queue.put(
                        BlockPlacedEvent(
                            backend_id=self.bid,
                            x=op.x,
                            y=op.y,
                            z=op.z,
                            op_type=op.op_type,
                        )
                    )
                else:
                    self._failed_count += 1
                    await self._event_queue.put(
                        ErrorEvent(
                            backend_id=self.bid,
                            error=f"坐标 {op.coord} 越界",
                            recoverable=True,
                            coord=op.coord,
                        )
                    )
            except Exception as e:
                self._failed_count += 1
                await self._event_queue.put(
                    ErrorEvent(backend_id=self.bid, error=str(e), recoverable=True, coord=op.coord)
                )
        # 批次结束发进度事件
        await self._event_queue.put(
            ProgressEvent(
                backend_id=self.bid,
                placed=self._placed_count,
                failed=self._failed_count,
                total=self._total,
                last_coord=ops[-1].coord if ops else None,
                last_op_type=ops[-1].op_type if ops else None,
            )
        )
        return success

    async def teleport(self, x: float, y: float, z: float) -> None:
        self._position = (x, y, z)

    async def set_orientation(self, yaw: float, pitch: float) -> None:
        self._orientation = (yaw, pitch)

    async def events(self) -> AsyncIterator[PlacementEvent]:
        """事件流异步生成器。从队列读事件直到收到 DoneEvent。"""
        while True:
            event = await self._event_queue.get()
            yield event
            if isinstance(event, DoneEvent):
                break

    async def emit_done(self) -> None:
        """通知此后端任务完成（调度器调用）。"""
        await self._event_queue.put(
            DoneEvent(
                backend_id=self.bid,
                placed=self._placed_count,
                failed=self._failed_count,
            )
        )

    def set_total(self, total: int) -> None:
        """设置该后端负责的方块总数（用于进度计算）。"""
        self._total = total

    async def checkpoint(self) -> BackendCheckpoint:
        return BackendCheckpoint(
            backend_id=self.bid,
            position=self._position,
            placed_count=self._placed_count,
            last_subtask_id=self._last_subtask_id,
        )

    async def restore(self, cp: BackendCheckpoint) -> None:
        self._position = cp.position or (0.0, 0.0, 0.0)
        self._placed_count = cp.placed_count
        self._last_subtask_id = cp.last_subtask_id

    def set_last_subtask(self, subtask_id: int) -> None:
        self._last_subtask_id = subtask_id

    @property
    def placed_count(self) -> int:
        return self._placed_count

    @property
    def failed_count(self) -> int:
        return self._failed_count

    @property
    def position(self) -> Tuple[float, float, float]:
        return self._position
