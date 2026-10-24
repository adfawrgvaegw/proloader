"""RegionScanner —— 扫描目标区域现有方块（创新点1 入口）。

从后端读取目标区域现状，返回 Region 供 DiffEngine 比较。
封装了"后端不支持读区域时退化为全量放置"的逻辑。
"""

from __future__ import annotations

import logging
from typing import Optional

from proloader.backend.base import PlacementBackend
from proloader.errors import NotSupportedError
from proloader.model.region import BoundingBox, Region

logger = logging.getLogger(__name__)


class RegionScanner:
    """目标区域扫描器。

    用法：
        scanner = RegionScanner()
        target = await scanner.scan(backend, bbox)
        if target is None:
            # 后端不支持读，退化为全量放置
    """

    async def scan(
        self,
        backend: PlacementBackend,
        bbox: BoundingBox,
    ) -> Optional[Region]:
        """从后端读取目标区域现状。

        Args:
            backend: 放置后端
            bbox: 要扫描的世界坐标包围盒

        Returns:
            现状 Region；若后端不支持读则返回 None（调用方退化为全量放置）
        """
        if not backend.capabilities.can_read_region:
            logger.info(
                "后端 %s 不支持读区域，退化为全量放置", backend.backend_id
            )
            return None
        try:
            region = await backend.read_region(bbox)
            logger.debug(
                "扫描 %s: %s, 非空气 %d 方块",
                bbox,
                region.size,
                region.non_air_count,
            )
            return region
        except NotSupportedError:
            logger.info("后端 %s read_region 抛 NotSupportedError，退化为全量放置",
                       backend.backend_id)
            return None
