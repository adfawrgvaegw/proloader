"""NeOmegaBackend —— 真实网易租赁服放置后端（Remote 模式）。

实现 PlacementBackend 接口，通过 NeOmega 原生库连接【已运行的】接入点，
控制真实假人在网易租赁服里放方块。

放置机制：通过 /setblock /fill 命令（假人需 OP 权限）。
    - /setblock x y z <方块>   单个方块
    - /fill x1 y1 z1 x2 y2 z2 <方块>  矩形区域批量（一次命令填一整个长方体）
    一次 /fill 比 N 次 /setblock 快得多，是真实导入器的核心加速手段。

差异导入（创新点1）与此后端的关系：
    差异算法算出"哪些方块要改"，本后端把这些改动翻译成 /setblock /fill 命令发出。
    can_read_region=False（无法用命令高效读取区域），所以差异扫描退化为"全量放置"，
    但 ADD/REMOVE 语义仍有效（REMOVE 发 setblock air 破坏多余方块）。

前置条件（缺失会清晰报错）：
    - 用户已运行 omega_launcher（fastbuilder）完成登录，接入点在监听
    - NeOmega 原生库（cfg.lib_path）
    - 假人在服内被授予 OP
"""

from __future__ import annotations

import asyncio
import logging
from dataclasses import dataclass, field
from typing import AsyncIterator, Optional, Tuple

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
from proloader.backend.neomega.config import NeOmegaConfig
from proloader.backend.neomega.core import CommandOutput, NeOmegaCore, NeOmegaCoreError
from proloader.errors import NotSupportedError
from proloader.model.block import Block
from proloader.model.region import BoundingBox, Region
from proloader.net.bedrock.tiles import (
    build_fill_command,
    build_setblock_command,
    build_tp_command,
)

logger = logging.getLogger(__name__)


@dataclass
class NeOmegaBackend:
    """网易租赁服后端（Remote 模式）。一个实例 = 一个假人连接。

    通过 NeOmega 原生库连接入点发命令放方块，实现 PlacementBackend 接口。

    Example:
        >>> cfg = NeOmegaConfig(lib_path="/path/to/neomega_macos_arm64.dylib")
        >>> bot = NeOmegaBackend(cfg, "netease-bot-0")
        >>> await bot.connect()  # 连接 omega_launcher 开的接入点
        >>> await bot.place_block(10, 64, 20, Block(BlockState("minecraft:stone")))
    """

    config: NeOmegaConfig
    bid: str
    _core: Optional[NeOmegaCore] = field(default=None, init=False, repr=False)
    _position: Tuple[float, float, float] = (0.0, 0.0, 0.0)
    _orientation: Tuple[float, float] = (0.0, 0.0)
    _placed_count: int = 0
    _failed_count: int = 0
    _total: int = 0
    _connected: bool = False
    _last_subtask_id: Optional[int] = None
    _event_queue: "asyncio.Queue[PlacementEvent]" = field(
        default_factory=asyncio.Queue, init=False, repr=False
    )

    # ── PlacementBackend 接口 ──────────────────────────────────────────────────
    @property
    def backend_id(self) -> str:
        return self.bid

    @property
    def capabilities(self) -> BackendCapabilities:
        # 真实后端：不能读区域（退化为全量放置差异）、需要 teleport、需要切块（加载范围限制）
        # 网易租赁服命令频率受限，单批最多 32 个 /setblock（防风控）
        return BackendCapabilities(
            can_read_region=False,
            needs_teleport=True,
            needs_chunking=True,
            max_blocks_per_batch=32,
            load_radius=64,  # 基岩版加载半径 ~4 chunk
        )

    async def connect(self) -> None:
        """加载原生库 → 连接已运行的接入点。

        前提：用户已运行 omega_launcher（fastbuilder）完成登录。
        """
        core = NeOmegaCore(self.config)
        self._core = core
        # 原生库操作是阻塞的 C 调用，放线程池避免卡事件循环
        loop = asyncio.get_event_loop()
        await loop.run_in_executor(None, core.load)
        await loop.run_in_executor(None, core.connect)
        self._connected = True
        logger.info(
            "NeOmegaBackend %s 已连接接入点 %s（假人已在租赁服内）",
            self.bid,
            self.config.access_point_addr,
        )

    async def disconnect(self) -> None:
        if self._core is not None:
            loop = asyncio.get_event_loop()
            await loop.run_in_executor(None, self._core.disconnect)
        self._connected = False

    async def read_region(self, box: BoundingBox) -> Region:
        """真实后端无法用命令高效读取区域 → 抛 NotSupportedError，调度器退化为全量放置。

        理论上可用 /testforblock 逐个查方块（ToolDelta 的 getBlockTile 就这么做），
        但极慢（一个方块一次命令），工程上不值得用于差异扫描。
        """
        raise NotSupportedError(
            "NeOmegaBackend 不支持读区域（网易租赁服用命令读方块不现实），差异扫描退化为全量放置"
        )

    async def place_block(self, x: int, y: int, z: int, block: Block) -> None:
        """单个方块放置：发 /setblock 命令。"""
        if not self._connected or self._core is None:
            raise NotSupportedError("后端未连接")
        cmd = build_setblock_command(x, y, z, block)
        try:
            ok = await self._send_command(cmd)
            if ok:
                self._placed_count += 1
                op_type = OpType.REMOVE if block.is_air else OpType.PLACE
                await self._event_queue.put(
                    BlockPlacedEvent(backend_id=self.bid, x=x, y=y, z=z, op_type=op_type)
                )
            else:
                self._failed_count += 1
                await self._event_queue.put(
                    ErrorEvent(
                        backend_id=self.bid,
                        error=f"/setblock 命令失败: {cmd}",
                        recoverable=True,
                        coord=(x, y, z),
                    )
                )
        except Exception as e:
            self._failed_count += 1
            await self._event_queue.put(
                ErrorEvent(backend_id=self.bid, error=str(e), recoverable=True, coord=(x, y, z))
            )
        await self._throttle()

    async def place_batch(self, ops: list[PlacementOp]) -> int:
        """批量放置：逐个 /setblock，发完即忘（omit_response）+ 节流，吞吐够用。

        合并成 /fill 的优化留给后续（需要空间聚类同种方块的连续矩形）。
        """
        if not self._connected or self._core is None:
            raise NotSupportedError("后端未连接")
        success = 0
        for op in ops:
            cmd = build_setblock_command(op.x, op.y, op.z, op.block)
            try:
                ok = await self._send_command(cmd)
                if ok:
                    self._placed_count += 1
                    success += 1
                    await self._event_queue.put(
                        BlockPlacedEvent(
                            backend_id=self.bid, x=op.x, y=op.y, z=op.z, op_type=op.op_type
                        )
                    )
                else:
                    self._failed_count += 1
                    await self._event_queue.put(
                        ErrorEvent(
                            backend_id=self.bid,
                            error=f"/setblock 失败: {cmd}",
                            recoverable=True,
                            coord=op.coord,
                        )
                    )
            except Exception as e:
                self._failed_count += 1
                await self._event_queue.put(
                    ErrorEvent(backend_id=self.bid, error=str(e), recoverable=True, coord=op.coord)
                )
            await self._throttle()
        # 批次进度
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
        """传送假人到放置区域（必须在加载范围内方块才能放）。"""
        if self._core is None:
            return
        try:
            await self._send_command(build_tp_command(x, y, z))
            self._position = (x, y, z)
        except Exception as e:
            logger.warning("%s 传送失败: %s", self.bid, e)
        await self._throttle()

    async def set_orientation(self, yaw: float, pitch: float) -> None:
        """朝向（影响方向敏感方块如楼梯）。Bedrock: tp @s ~ ~ ~ yaw pitch。"""
        if self._core is None:
            return
        try:
            await self._send_command(f"tp @s ~ ~ ~ {yaw:.2f} {pitch:.2f}")
            self._orientation = (yaw, pitch)
        except Exception:
            pass

    async def events(self) -> AsyncIterator[PlacementEvent]:
        while True:
            event = await self._event_queue.get()
            yield event
            if isinstance(event, DoneEvent):
                break

    async def emit_done(self) -> None:
        await self._event_queue.put(
            DoneEvent(backend_id=self.bid, placed=self._placed_count, failed=self._failed_count)
        )

    def set_total(self, total: int) -> None:
        self._total = total

    def set_last_subtask(self, subtask_id: int) -> None:
        self._last_subtask_id = subtask_id

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

    @property
    def placed_count(self) -> int:
        return self._placed_count

    @property
    def failed_count(self) -> int:
        return self._failed_count

    @property
    def position(self) -> Tuple[float, float, float]:
        return self._position

    # ── 内部 ───────────────────────────────────────────────────────────────
    async def _send_command(self, cmd: str) -> bool:
        """通过 NeOmega 接入点发命令（阻塞 C 调用放线程池）。

        Returns:
            True 表示命令成功（need_response 时看回执，omit_response 时假设成功）
        """
        assert self._core is not None
        loop = asyncio.get_event_loop()
        via = self.config.place_via
        need_resp = self.config.need_response

        if via == "player":
            result = await loop.run_in_executor(
                None, self._core.send_player_command, cmd, need_resp
            )
        elif via == "settings":
            # settings 命令无回执，永远假设成功
            await loop.run_in_executor(None, self._core.send_settings_command, cmd)
            return True
        else:  # websocket（默认，/setblock 走这个，需 OP）
            result = await loop.run_in_executor(
                None, self._core.send_websocket_command, cmd, need_resp
            )

        if isinstance(result, CommandOutput):
            return result.success
        # omit_response：假设成功（网易命令失败通常因权限/坐标，节流可缓解）
        return True

    async def _throttle(self) -> None:
        """命令节流（防网易风控限频）。"""
        if self.config.command_interval > 0:
            await asyncio.sleep(self.config.command_interval)
