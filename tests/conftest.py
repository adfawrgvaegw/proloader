"""共享测试 fixtures。"""

from __future__ import annotations

import pytest

from proloader.backend.simulated import SimulatedBackend, SimulatedWorld
from proloader.model.block import Block, BlockState
from proloader.model.palette import BlockPalette
from proloader.model.region import BoundingBox, Region
from proloader.model.structure import Structure
from proloader.parser import load_structure


@pytest.fixture
def example_structure() -> Structure:
    """加载示例建筑（examples/house.mcstructure）。"""
    return load_structure("examples/house.mcstructure")


@pytest.fixture
def simple_structure() -> Structure:
    """构造一个 3×2×3 的简单建筑（用于精确断言）。

    底层(y=0): 9 个 stone
    顶层(y=1): 8 个 oak_planks + 1 个 glass(中心)
    非空气总数 = 18
    """
    import numpy as np

    palette = BlockPalette()
    # 0=air(自动), 1=stone, 2=oak_planks, 3=glass
    palette.add(Block(BlockState("minecraft:stone")))       # idx 1
    palette.add(Block(BlockState("minecraft:oak_planks")))  # idx 2
    palette.add(Block(BlockState("minecraft:glass")))       # idx 3

    blocks = np.zeros((3, 2, 3), dtype=np.uint32)
    blocks[:, 0, :] = 1  # 底层全石砖
    blocks[:, 1, :] = 2  # 顶层全木板
    blocks[1, 1, 1] = 3  # 顶层中心玻璃

    region = Region(palette=palette, blocks=blocks, offset=(0, 0, 0))
    return Structure.from_region(
        name="simple",
        region=region,
        format_version="1",
    )


@pytest.fixture
def empty_world() -> SimulatedWorld:
    """空模拟世界。"""
    return SimulatedWorld(size=(64, 64, 64))


@pytest.fixture
def tmp_sessions_dir(tmp_path):
    """临时会话目录。"""
    return tmp_path / "sessions"
