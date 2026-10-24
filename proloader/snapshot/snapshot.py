"""RegionSnapshot —— 创新点4：导入前快照目标区域。

原理：
    导入建筑前，先扫描目标区域现状并存盘。这样：
      - 出错时可一键回滚到快照状态
      - 中断后 resume 时可对比已放置部分

存储格式：
    numpy npz（原生压缩格式），含：
      - blocks: uint32 数组
      - palette: 序列化的 Block 列表（dict list）
      - offset: (ox, oy, oz)
      - bbox: 包围盒
      - timestamp: 快照时间

学习要点：
    - 快照是回滚的前提：没有快照就无法恢复
    - npz 比 pickle 安全（无代码执行风险）且压缩
    - 快照只存目标区域，不存整个世界（省空间）
"""

from __future__ import annotations

import json
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import List, Tuple

import numpy as np

from proloader.backend.base import PlacementBackend
from proloader.model.palette import BlockPalette
from proloader.model.region import BoundingBox, Region


@dataclass
class RegionSnapshot:
    """目标区域快照。"""

    blocks: np.ndarray  # uint32
    palette_data: List[dict]  # 序列化的 palette
    offset: Tuple[int, int, int]
    bbox: BoundingBox
    timestamp: float = field(default_factory=time.time)

    @classmethod
    async def capture(
        cls, backend: PlacementBackend, bbox: BoundingBox
    ) -> "RegionSnapshot":
        """从后端读取目标区域并存为快照。"""
        region = await backend.read_region(bbox)
        return cls.from_region(region)

    @classmethod
    def from_region(cls, region: Region) -> "RegionSnapshot":
        """从 Region 构造快照。"""
        return cls(
            blocks=region.blocks.copy(),
            palette_data=region.palette.to_dict_list(),
            offset=region.offset,
            bbox=region.bbox,
        )

    def to_region(self) -> Region:
        """快照转回 Region（用于回滚时作为"目标结构"）。"""
        palette = BlockPalette.from_dict_list(self.palette_data)
        return Region(palette=palette, blocks=self.blocks.copy(), offset=self.offset)

    def save(self, path: str | Path) -> None:
        """保存为 npz 文件。"""
        path = Path(path)
        path.parent.mkdir(parents=True, exist_ok=True)
        np.savez_compressed(
            str(path),
            blocks=self.blocks,
            palette_json=json.dumps(self.palette_data),
            offset=np.array(self.offset),
            bbox=np.array(list(self.bbox)),
            timestamp=np.array([self.timestamp]),
        )

    @classmethod
    def load(cls, path: str | Path) -> "RegionSnapshot":
        """从 npz 文件加载。"""
        path = Path(path)
        data = np.load(str(path), allow_pickle=False)
        bbox_arr = data["bbox"]
        bbox = BoundingBox(
            int(bbox_arr[0]), int(bbox_arr[1]), int(bbox_arr[2]),
            int(bbox_arr[3]), int(bbox_arr[4]), int(bbox_arr[5]),
        )
        return cls(
            blocks=data["blocks"],
            palette_data=json.loads(str(data["palette_json"])),
            offset=tuple(int(x) for x in data["offset"]),
            bbox=bbox,
            timestamp=float(data["timestamp"][0]),
        )
