"""Structure：一个完整建筑结构的内存表示。

与 Region 的区别：
    - Structure 是"想要放的"建筑（来自 .mcstructure 文件）
    - Region 是"世界里的"现状方块

放置 = 把 Structure 叠加到世界 Region（受 Transform 影响，对齐到放置原点）。
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Dict, Tuple

from proloader.model.block import Block
from proloader.model.palette import BlockPalette
from proloader.model.region import BoundingBox, Region


@dataclass
class Structure:
    """完整建筑结构。

    Attributes:
        name: 建筑名（通常来自文件名）
        size: (sx, sy, sz)
        palette: 方块调色板
        region: 内部 Region，offset 通常为 (0,0,0)
        block_entities: 方块实体（箱子/告示牌等），key=本地坐标 (x,y,z)
        format_version: 来源格式版本（如 mcstructure 的 format_version）
        is_netease: 是否含网易方块
    """

    name: str
    size: Tuple[int, int, int]
    palette: BlockPalette
    region: Region
    block_entities: Dict[Tuple[int, int, int], dict] = field(default_factory=dict)
    format_version: str = ""
    is_netease: bool = False
    source_info: dict = field(default_factory=dict)

    @classmethod
    def from_region(
        cls,
        name: str,
        region: Region,
        block_entities: Dict[Tuple[int, int, int], dict] | None = None,
        format_version: str = "",
        is_netease: bool = False,
    ) -> "Structure":
        """从 Region 构造 Structure。"""
        return cls(
            name=name,
            size=region.size,
            palette=region.palette,
            region=region,
            block_entities=block_entities or {},
            format_version=format_version,
            is_netease=is_netease,
        )

    @property
    def block_count(self) -> int:
        """非空气方块总数。"""
        return self.region.non_air_count

    @property
    def total_blocks(self) -> int:
        """总体积（含空气）。"""
        return self.region.volume

    def to_region(self, offset: Tuple[int, int, int]) -> Region:
        """转换为新 Region，offset 设为放置原点（世界坐标）。

        blocks 数组复制一份，避免修改影响原 Structure。
        """
        return Region(
            palette=self.palette,
            blocks=self.region.blocks.copy(),
            offset=offset,
        )

    def bbox_at(self, origin: Tuple[int, int, int]) -> BoundingBox:
        """建筑放在 origin 时的世界 bbox。"""
        sx, sy, sz = self.size
        return BoundingBox.from_origin_size(origin[0], origin[1], origin[2], sx, sy, sz)

    def slice(self, sub_bbox: BoundingBox) -> "Structure":
        """切出子结构（用于切块调度）。返回新 Structure，offset 归零。"""
        sub_region = self.region.slice(sub_bbox)
        # 切出对应的 block_entities
        ox, oy, oz = self.region.offset
        sub_entities: Dict[Tuple[int, int, int], dict] = {}
        for (x, y, z), entity in self.block_entities.items():
            wx, wy, wz = x + ox, y + oy, z + oz
            if sub_bbox.contains(wx, wy, wz):
                # 转为子 region 本地坐标
                sub_entities[(wx - sub_bbox.min_x, wy - sub_bbox.min_y, wz - sub_bbox.min_z)] = entity
        return Structure(
            name=f"{self.name}[slice]",
            size=sub_region.size,
            palette=sub_region.palette,
            region=Region(
                palette=sub_region.palette,
                blocks=sub_region.blocks,
                offset=(0, 0, 0),
            ),
            block_entities=sub_entities,
            format_version=self.format_version,
            is_netease=self.is_netease,
        )

    def summary(self) -> dict:
        """返回摘要信息（用于 Web API / 日志）。"""
        return {
            "name": self.name,
            "size": list(self.size),
            "block_count": self.block_count,
            "total_blocks": self.total_blocks,
            "palette_size": len(self.palette),
            "block_entities": len(self.block_entities),
            "format_version": self.format_version,
            "is_netease": self.is_netease,
        }

    def __repr__(self) -> str:
        return (
            f"Structure(name={self.name!r}, size={self.size}, "
            f"blocks={self.block_count}, palette={len(self.palette)})"
        )
