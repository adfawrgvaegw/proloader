"""PlacementBackend 抽象接口 —— 整条链路的契约。

学习要点：
    - 用 typing.Protocol（鸭子类型）而非 ABC，让 SimulatedBackend 不必继承协议层，解耦更彻底
    - 一个后端实例 = 一个可放置方块的执行体（SimulatedBackend 的一个 slot / 真实后端的一个假人连接）
    - 上层调度器/差异算法/Web UI 只依赖此接口，不感知具体后端

这是 proloader 架构的核心：协议无关性就靠这个接口实现。
"""

from __future__ import annotations

from typing import AsyncIterator, Protocol, runtime_checkable

from proloader.backend.events import BackendCapabilities, BackendCheckpoint, PlacementEvent, PlacementOp
from proloader.model.block import Block
from proloader.model.region import BoundingBox, Region


@runtime_checkable
class PlacementBackend(Protocol):
    """放置后端抽象接口。

    所有后端（SimulatedBackend / BedrockProtocolBackend / NeteaseBackend）都实现此接口。
    上层通过依赖注入把后端传给调度器，调度器只调用此接口的方法。
    """

    @property
    def backend_id(self) -> str:
        """唯一标识（如 'sim-0', 'bot-1', 'netease-bot-2'）。"""
        ...

    @property
    def capabilities(self) -> BackendCapabilities:
        """后端能力声明。调度器据此选择策略。"""
        ...

    async def connect(self) -> None:
        """建立连接/初始化。

        - SimulatedBackend: 立即返回
        - BedrockProtocolBackend: RakNet 握手 + Login + StartGame
        - NeteaseBackend: 同上 + 网易认证链
        """
        ...

    async def disconnect(self) -> None:
        """优雅断开。"""
        ...

    async def read_region(self, box: BoundingBox) -> Region:
        """【创新点1 依赖】读取目标区域现有方块。

        - SimulatedBackend: 返回 numpy 内存切片
        - BedrockProtocolBackend: 请求 LevelChunkPacket, 解码 sub-chunk
        - NeteaseBackend: 同上（网易 sub-chunk 用 UseNeteaseBlockStates 编码）

        若后端不支持读，抛 NotSupportedError，调度器退化为全量放置。

        Args:
            box: 世界坐标包围盒（包含 min 不包含 max）
        """
        ...

    async def place_block(self, x: int, y: int, z: int, block: Block) -> None:
        """放置单个方块（最细粒度）。

        真实后端底层包成 UpdateBlockPacket。
        """
        ...

    async def place_batch(self, ops: list[PlacementOp]) -> int:
        """批量放置，返回成功数。

        一次协议帧发多个 UpdateBlock（合并 GamePacket）。
        真实后端每帧最多 capabilities.max_blocks_per_batch 个方块。
        """
        ...

    async def teleport(self, x: float, y: float, z: float) -> None:
        """移动假人到指定坐标（真实后端必填，模拟器可空实现）。

        放置范围必须在假人加载范围内（基岩版 ~4 chunk）。
        """
        ...

    async def set_orientation(self, yaw: float, pitch: float) -> None:
        """朝向，影响放置方向敏感方块。"""
        ...

    def events(self) -> AsyncIterator[PlacementEvent]:
        """事件流：进度、错误、完成、单方块放置。Web UI 订阅此流。

        返回异步迭代器，每次 yield 一个 PlacementEvent。
        """
        ...

    async def checkpoint(self) -> BackendCheckpoint:
        """【创新点4 依赖】导出后端状态，用于断点续传。"""
        ...

    async def restore(self, cp: BackendCheckpoint) -> None:
        """从断点恢复后端状态。"""
        ...
