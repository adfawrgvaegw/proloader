"""模型层测试：Block / BlockState / Palette / Region / Structure。"""

from __future__ import annotations

import numpy as np
import pytest

from proloader.model.block import Block, BlockState
from proloader.model.palette import BlockPalette
from proloader.model.region import BoundingBox, Region
from proloader.model.structure import Structure


# ── Block / BlockState ────────────────────────────────────────────────────────
class TestBlock:
    def test_air_block(self):
        air = Block.air()
        assert air.is_air
        assert air.state.name == "minecraft:air"

    def test_block_equality(self):
        b1 = Block(BlockState("minecraft:stone"))
        b2 = Block(BlockState("minecraft:stone"))
        assert b1 == b2

    def test_block_inequality(self):
        b1 = Block(BlockState("minecraft:stone"))
        b2 = Block(BlockState("minecraft:dirt"))
        assert b1 != b2

    def test_block_with_states(self):
        b = Block(BlockState("minecraft:oak_stairs", {"facing": "north", "half": "top"}))
        assert b.state.name == "minecraft:oak_stairs"
        assert b.state.states.get("facing") == "north"
        assert b.state.states.get("half") == "top"

    def test_block_hashable(self):
        """frozen=True 使 Block 可哈希（用于 palette 去重）。"""
        b = Block(BlockState("minecraft:stone"))
        assert hash(b) == hash(Block(BlockState("minecraft:stone")))


# ── BlockPalette ──────────────────────────────────────────────────────────────
class TestPalette:
    def test_air_at_index_zero(self):
        p = BlockPalette()
        assert p.get(0) == Block.air()

    def test_add_and_get(self):
        p = BlockPalette()
        stone = Block(BlockState("minecraft:stone"))
        idx = p.add(stone)
        assert idx == 1
        assert p.get(idx) == stone

    def test_dedup(self):
        p = BlockPalette()
        stone = Block(BlockState("minecraft:stone"))
        idx1 = p.add(stone)
        idx2 = p.add(stone)
        assert idx1 == idx2

    def test_different_blocks_different_index(self):
        p = BlockPalette()
        stone = Block(BlockState("minecraft:stone"))
        dirt = Block(BlockState("minecraft:dirt"))
        assert p.add(stone) != p.add(dirt)

    def test_len(self):
        p = BlockPalette()
        p.add(Block(BlockState("minecraft:stone")))
        p.add(Block(BlockState("minecraft:dirt")))
        assert len(p) == 3  # air + stone + dirt

    def test_id_of(self):
        p = BlockPalette()
        stone = Block(BlockState("minecraft:stone"))
        idx = p.add(stone)
        assert p.id_of(stone) == idx
        assert p.id_of_or(stone) == idx
        assert p.id_of_or(Block(BlockState("minecraft:dirt")), -1) == -1


# ── BoundingBox ───────────────────────────────────────────────────────────────
class TestBoundingBox:
    def test_from_origin_size(self):
        bb = BoundingBox.from_origin_size(10, 20, 30, 5, 6, 7)
        assert bb.min_x == 10 and bb.min_y == 20 and bb.min_z == 30
        assert bb.max_x == 15 and bb.max_y == 26 and bb.max_z == 37

    def test_size(self):
        bb = BoundingBox.from_origin_size(0, 0, 0, 5, 6, 7)
        assert bb.size == (5, 6, 7)
        assert bb.volume == 210

    def test_contains(self):
        bb = BoundingBox.from_origin_size(0, 0, 0, 5, 5, 5)
        assert bb.contains(0, 0, 0)
        assert bb.contains(4, 4, 4)
        assert not bb.contains(5, 0, 0)  # 半开区间
        assert not bb.contains(-1, 0, 0)

    def test_no_overlap(self):
        a = BoundingBox.from_origin_size(0, 0, 0, 5, 5, 5)
        b = BoundingBox.from_origin_size(10, 0, 0, 5, 5, 5)
        assert not a.overlaps(b)

    def test_overlap(self):
        a = BoundingBox.from_origin_size(0, 0, 0, 5, 5, 5)
        b = BoundingBox.from_origin_size(3, 0, 0, 5, 5, 5)
        assert a.overlaps(b)


# ── Region ────────────────────────────────────────────────────────────────────
class TestRegion:
    def test_empty_region(self):
        r = Region.empty((4, 4, 4), (0, 0, 0))
        assert r.size == (4, 4, 4)
        assert r.non_air_count == 0

    def test_set_and_get(self):
        r = Region.empty((4, 4, 4), (0, 0, 0))
        stone = Block(BlockState("minecraft:stone"))
        r.set(0, 0, 0, stone)
        assert r.get(0, 0, 0) == stone
        assert r.non_air_count == 1

    def test_world_coords(self):
        r = Region.empty((4, 4, 4), (10, 20, 30))
        stone = Block(BlockState("minecraft:stone"))
        r.set_world(12, 22, 32, stone)
        assert r.get_world(12, 22, 32) == stone

    def test_iter_non_air(self, simple_structure):
        blocks = list(simple_structure.region.iter_non_air())
        assert len(blocks) == 18  # 3*2*3 全非空气

    def test_non_air_count(self, simple_structure):
        assert simple_structure.region.non_air_count == 18

    def test_slice(self, simple_structure):
        sub_bb = BoundingBox.from_origin_size(0, 0, 0, 2, 2, 2)
        sub = simple_structure.region.slice(sub_bb)
        assert sub.size == (2, 2, 2)

    def test_matches(self):
        """两个内容相同的 Region 应 matches。"""
        r1 = Region.empty((2, 2, 2))
        r2 = Region.empty((2, 2, 2))
        stone = Block(BlockState("minecraft:stone"))
        r1.set(0, 0, 0, stone)
        r2.set(0, 0, 0, stone)
        assert r1.matches(r2)

    def test_from_blocks(self):
        stone = Block(BlockState("minecraft:stone"))
        r = Region.from_blocks([(0, 0, 0, stone), (1, 0, 0, stone)])
        assert r.non_air_count == 2


# ── Structure ─────────────────────────────────────────────────────────────────
class TestStructure:
    def test_basic_properties(self, simple_structure):
        assert simple_structure.name == "simple"
        assert simple_structure.size == (3, 2, 3)
        assert simple_structure.block_count == 18
        assert simple_structure.total_blocks == 18

    def test_to_region(self, simple_structure):
        r = simple_structure.to_region((10, 20, 30))
        assert r.offset == (10, 20, 30)
        assert r.size == (3, 2, 3)
        assert r.non_air_count == 18

    def test_bbox_at(self, simple_structure):
        bb = simple_structure.bbox_at((10, 20, 30))
        assert bb.min_x == 10 and bb.min_y == 20 and bb.min_z == 30
        assert bb.max_x == 13 and bb.max_y == 22 and bb.max_z == 33

    def test_summary(self, simple_structure):
        s = simple_structure.summary()
        assert s["name"] == "simple"
        assert s["block_count"] == 18
        assert "palette_size" in s

    def test_example_structure(self, example_structure):
        """验证真实 .mcstructure 文件加载。"""
        assert example_structure.name == "house"
        assert example_structure.size == (7, 5, 7)
        assert example_structure.block_count == 168
