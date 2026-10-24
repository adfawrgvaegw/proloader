"""Transform：建筑变换（旋转 / 镜像 / 平移）。

学习要点：
    - 旋转按 Y 轴（0/90/180/270 度），不改变 Y 坐标
    - 镜像分 X 轴和 Z 轴
    - 平移由放置原点决定（不在 Transform 里，在 to_region(offset) 里）
    - 方向敏感方块（楼梯/活塞/活板门）的 states 需要同步调整

MVP 实现坐标变换 + 简单方向调整；完整方向敏感方块映射在 Phase 11。
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Dict, Tuple

from proloader.model.block import Block, BlockState


# ── 方向敏感方块的 state 旋转映射 ──────────────────────────────────────────────
# 旋转 90 度（顺时针，俯视）时 direction/ Facing 等状态的新值
# 完整映射表很大，这里只列常见方块，其余保持原样（MVP 足够）
_FACING_ROTATE_CW: Dict[str, str] = {
    "north": "east",
    "east": "south",
    "south": "west",
    "west": "north",
}

_DIRECTION_ROTATE_CW: Dict[int, int] = {
    0: 1,  # south -> west
    1: 2,  # west -> north
    2: 3,  # north -> east
    3: 0,  # east -> south
}


def _rotate_state_cw(name: str, states: dict) -> dict:
    """旋转方块状态 90 度顺时针（简化版）。"""
    new_states = dict(states)
    # minecraft:stairs / piston 等用 "direction" int (0-3)
    if "direction" in new_states and isinstance(new_states["direction"], int):
        d = new_states["direction"]
        if d in _DIRECTION_ROTATE_CW:
            new_states["direction"] = _DIRECTION_ROTATE_CW[d]
    # facing 字符串
    if "facing" in new_states and isinstance(new_states["facing"], str):
        f = new_states["facing"]
        if f in _FACING_ROTATE_CW:
            new_states["facing"] = _FACING_ROTATE_CW[f]
    return new_states


@dataclass(frozen=True)
class Transform:
    """建筑变换：旋转 + 镜像 + 平移。

    旋转按 Y 轴顺时针（俯视），rotation ∈ {0, 90, 180, 270}。
    镜像先于旋转应用。

    Note:
        平移 translation 这里仅用于坐标计算，实际放置位置由 Region.offset 决定。
        多数场景 translation=(0,0,0)，由上层 to_region(origin) 处理。
    """

    rotation: int = 0  # 0, 90, 180, 270
    mirror_x: bool = False
    mirror_z: bool = False
    translation: Tuple[int, int, int] = (0, 0, 0)

    def __post_init__(self) -> None:
        if self.rotation not in (0, 90, 180, 270):
            raise ValueError(f"rotation 必须是 0/90/180/270，得到 {self.rotation}")

    def apply_to_coord(
        self, x: int, y: int, z: int, size: Tuple[int, int, int]
    ) -> Tuple[int, int, int]:
        """把本地坐标变换到新本地坐标。

        size 是变换前的建筑尺寸（用于镜像翻转）。
        """
        sx, _, sz = size
        nx, ny, nz = x, y, z
        # 镜像
        if self.mirror_x:
            nx = sx - 1 - nx
        if self.mirror_z:
            nz = sz - 1 - nz
        # 旋转（顺时针，俯视。size 的 x/z 会交换）
        if self.rotation == 90:
            nx, nz = nz, sx - 1 - nx
        elif self.rotation == 180:
            nx, nz = sx - 1 - nx, sz - 1 - nz
        elif self.rotation == 270:
            nx, nz = sz - 1 - nz, nx
        # 平移
        tx, ty, tz = self.translation
        return (nx + tx, ny + ty, nz + tz)

    def apply_to_block(self, block: Block) -> Block:
        """变换方块（旋转方向敏感方块的 states）。"""
        if self.rotation == 0 and not self.mirror_x and not self.mirror_z:
            return block
        states = block.state.states
        # 镜像翻转 direction
        if self.mirror_x or self.mirror_z:
            states = self._mirror_states(states)
        # 旋转
        for _ in range(self.rotation // 90):
            states = _rotate_state_cw(block.state.name, states)
        return Block(BlockState(block.state.name, states), block.layer)

    def _mirror_states(self, states: dict) -> dict:
        """镜像翻转方向状态（简化）。"""
        new_states = dict(states)
        if self.mirror_x and "direction" in new_states and isinstance(new_states["direction"], int):
            # 0<->2, 1<->3 (X 镜像)
            new_states["direction"] = {0: 2, 1: 1, 2: 0, 3: 3}.get(new_states["direction"], new_states["direction"])
        return new_states

    def transformed_size(self, size: Tuple[int, int, int]) -> Tuple[int, int, int]:
        """变换后的建筑尺寸（90/270 旋转会交换 x/z）。"""
        sx, sy, sz = size
        if self.rotation in (90, 270):
            return (sz, sy, sx)
        return (sx, sy, sz)

    @property
    def is_identity(self) -> bool:
        """是否为恒等变换。"""
        return self.rotation == 0 and not self.mirror_x and not self.mirror_z and self.translation == (0, 0, 0)
