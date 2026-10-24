"""Region：三维方块矩阵 + BoundingBox。

学习要点：
    - Region 用 numpy uint32 三维数组存 palette id（不是 Block 对象）
    - shape = (size_x, size_y, size_z) 严格按基岩版 (x,y,z) 顺序
    - offset 是 Region 在世界坐标系中的原点（放置建筑时的左下后角）
    - 用 numpy 是为了向量化差异计算（创新点1性能关键）

内存对比：
    - 存 Block 对象：每方块 ~200 字节（Python 对象开销），10w 方块 = 20MB
    - 存 uint32：每方块 4 字节，10w 方块 = 400KB，省 50x
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Iterator, List, Tuple

import numpy as np

from proloader.model.block import Block
from proloader.model.palette import BlockPalette


# ── BoundingBox：轴对齐包围盒 ──────────────────────────────────────────────────
@dataclass(frozen=True)
class BoundingBox:
    """轴对齐包围盒（整数坐标，包含 min 不包含 max）。

    约定：[min_x, max_x) 半开区间，即 max 点不在框内。
    这样 size = max - min，与 Region.shape 对齐。
    """

    min_x: int
    min_y: int
    min_z: int
    max_x: int
    max_y: int
    max_z: int

    @property
    def size(self) -> Tuple[int, int, int]:
        return (self.max_x - self.min_x, self.max_y - self.min_y, self.max_z - self.min_z)

    @property
    def volume(self) -> int:
        sx, sy, sz = self.size
        return sx * sy * sz

    def contains(self, x: int, y: int, z: int) -> bool:
        return self.min_x <= x < self.max_x and self.min_y <= y < self.max_y and self.min_z <= z < self.max_z

    def overlaps(self, other: "BoundingBox") -> bool:
        """是否与另一个 bbox 相交（共享任意体素）。"""
        return (
            self.min_x < other.max_x
            and self.max_x > other.min_x
            and self.min_y < other.max_y
            and self.max_y > other.min_y
            and self.min_z < other.max_z
            and self.max_z > other.min_z
        )

    @classmethod
    def from_origin_size(cls, ox: int, oy: int, oz: int, sx: int, sy: int, sz: int) -> "BoundingBox":
        return cls(ox, oy, oz, ox + sx, oy + sy, oz + sz)

    def __iter__(self) -> Iterator[int]:
        return iter((self.min_x, self.min_y, self.min_z, self.max_x, self.max_y, self.max_z))


# ── Region：三维方块矩阵 ──────────────────────────────────────────────────────
@dataclass
class Region:
    """三维方块矩阵。

    blocks: numpy uint32 数组，shape=(sx, sy, sz)，存 palette id
    palette: BlockPalette，id→Block 映射
    offset: 在世界坐标系中的原点 (ox, oy, oz)
    """

    palette: BlockPalette
    blocks: np.ndarray  # dtype=uint32, shape=(sx, sy, sz)
    offset: Tuple[int, int, int] = (0, 0, 0)

    def __post_init__(self) -> None:
        if self.blocks.dtype != np.uint32:
            self.blocks = self.blocks.astype(np.uint32)

    @property
    def size(self) -> Tuple[int, int, int]:
        sx, sy, sz = self.blocks.shape
        return (sx, sy, sz)

    @property
    def bbox(self) -> BoundingBox:
        sx, sy, sz = self.size
        ox, oy, oz = self.offset
        return BoundingBox.from_origin_size(ox, oy, oz, sx, sy, sz)

    @property
    def volume(self) -> int:
        return int(self.blocks.size)

    @classmethod
    def empty(
        cls, size: Tuple[int, int, int], offset: Tuple[int, int, int] = (0, 0, 0)
    ) -> "Region":
        """全空气区域。palette 仅含 AIR（id=0），数组全零。"""
        palette = BlockPalette()
        blocks = np.zeros(size, dtype=np.uint32)  # 全 0 = 全 air
        return cls(palette=palette, blocks=blocks, offset=offset)

    @classmethod
    def from_blocks(
        cls,
        blocks_data: List[Tuple[int, int, int, Block]],
        offset: Tuple[int, int, int] = (0, 0, 0),
    ) -> "Region":
        """从 (x, y, z, Block) 列表构建。

        坐标是相对于 offset 的本地坐标（从 0 开始）。
        """
        if not blocks_data:
            return cls.empty((1, 1, 1), offset)
        max_x = max(b[0] for b in blocks_data) + 1
        max_y = max(b[1] for b in blocks_data) + 1
        max_z = max(b[2] for b in blocks_data) + 1
        palette = BlockPalette()
        arr = np.zeros((max_x, max_y, max_z), dtype=np.uint32)
        for x, y, z, block in blocks_data:
            arr[x, y, z] = palette.add(block)
        return cls(palette=palette, blocks=arr, offset=offset)

    def get(self, x: int, y: int, z: int) -> Block:
        """获取本地坐标 (x,y,z) 的方块。"""
        return self.palette.get(int(self.blocks[x, y, z]))

    def set(self, x: int, y: int, z: int, block: Block) -> None:
        """设置本地坐标 (x,y,z) 的方块。"""
        self.blocks[x, y, z] = self.palette.add(block)

    def get_world(self, wx: int, wy: int, wz: int) -> Block:
        """获取世界坐标 (wx,wy,wz) 的方块（自动减去 offset）。"""
        ox, oy, oz = self.offset
        return self.get(wx - ox, wy - oy, wz - oz)

    def set_world(self, wx: int, wy: int, wz: int, block: Block) -> None:
        """设置世界坐标 (wx,wy,wz) 的方块。"""
        ox, oy, oz = self.offset
        self.set(wx - ox, wy - oy, wz - oz, block)

    def to_world(self, local_xyz: Tuple[int, int, int]) -> Tuple[int, int, int]:
        """本地坐标 → 世界坐标。"""
        lx, ly, lz = local_xyz
        ox, oy, oz = self.offset
        return (lx + ox, ly + oy, lz + oz)

    def to_local(self, world_xyz: Tuple[int, int, int]) -> Tuple[int, int, int]:
        """世界坐标 → 本地坐标。"""
        wx, wy, wz = world_xyz
        ox, oy, oz = self.offset
        return (wx - ox, wy - oy, wz - oz)

    def iter_non_air(self) -> Iterator[Tuple[int, int, int, Block]]:
        """迭代所有非空气方块，yield (local_x, local_y, local_z, Block)。

        用于放置（跳过空气）和差异比较。
        用 np.argwhere 向量化找非零位置，比 Python 三重循环快 100x。
        """
        air_id = self.palette.air_id
        non_air_indices = np.argwhere(self.blocks != air_id)
        for idx in non_air_indices:
            x, y, z = int(idx[0]), int(idx[1]), int(idx[2])
            yield (x, y, z, self.palette.get(int(self.blocks[x, y, z])))

    def iter_non_air_world(self) -> Iterator[Tuple[int, int, int, Block]]:
        """迭代所有非空气方块，yield 世界坐标 (wx, wy, wz, Block)。"""
        ox, oy, oz = self.offset
        for x, y, z, block in self.iter_non_air():
            yield (x + ox, y + oy, z + oz, block)

    @property
    def non_air_count(self) -> int:
        """非空气方块数量。"""
        return int(np.count_nonzero(self.blocks != self.palette.air_id))

    def fill(self, block: Block) -> None:
        """整个区域填充为指定方块。"""
        bid = self.palette.add(block)
        self.blocks[...] = bid

    def slice(self, bbox: BoundingBox) -> "Region":
        """切出子区域（bbox 是世界坐标），返回新 Region（offset=bbox.min）。

        用于切块调度（创新点2）。
        """
        ox, oy, oz = self.offset
        sx0 = bbox.min_x - ox
        sy0 = bbox.min_y - oy
        sz0 = bbox.min_z - oz
        sx1 = bbox.max_x - ox
        sy1 = bbox.max_y - oy
        sz1 = bbox.max_z - oz
        sub_blocks = self.blocks[sx0:sx1, sy0:sy1, sz0:sz1].copy()
        return Region(
            palette=self.palette,  # 共享 palette（不复制，省内存）
            blocks=sub_blocks,
            offset=(bbox.min_x, bbox.min_y, bbox.min_z),
        )

    def copy(self) -> "Region":
        """深拷贝（palette 共享，blocks 复制）。"""
        return Region(palette=self.palette, blocks=self.blocks.copy(), offset=self.offset)

    def matches(self, other: "Region") -> bool:
        """两个 Region 的方块是否完全一致（忽略 offset，比较本地内容）。

        要求 palette 能互相解释 id。简单实现：逐方块比较 Block 对象。
        用于验证导入结果正确性。
        """
        if self.size != other.size:
            return False
        # 向量化：把两边的 id 都映射到统一的"内容指纹"再比较
        # 简单做法：把每个 id 映射为 (name, states, layer) 的哈希
        self_sig = self._signature_array()
        other_sig = other._signature_array()
        return np.array_equal(self_sig, other_sig)

    def _signature_array(self) -> np.ndarray:
        """把每个 palette id 映射为内容哈希（int64），返回同 shape 数组。

        用于 matches() 的向量化比较——避免逐方块比较 Block 对象。
        """
        sig_map = np.zeros(len(self.palette), dtype=np.int64)
        for i, block in enumerate(self.palette):
            sig_map[i] = hash(block)
        return sig_map[self.blocks]

    def __repr__(self) -> str:
        sx, sy, sz = self.size
        return f"Region(size={sx}x{sy}x{sz}, offset={self.offset}, non_air={self.non_air_count})"
