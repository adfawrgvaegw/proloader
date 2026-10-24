"""方块名 → 颜色映射表（Web 3D 渲染用）。

学习要点：
    - Three.js InstancedMesh 渲染需要每种方块一个颜色
    - 真实方块纹理加载复杂（需要 texture atlas），MVP 用纯色近似
    - 颜色按方块"视觉印象"选取，非精确纹理采样

完整版（Phase 11）可加载真实材质贴图。
"""

from __future__ import annotations

from typing import Tuple

# 常用方块 → RGB 颜色 (0-255)
_BLOCK_COLORS: dict[str, Tuple[int, int, int]] = {
    "minecraft:air": (0, 0, 0),
    "minecraft:stone": (128, 128, 128),
    "minecraft:cobblestone": (120, 120, 120),
    "minecraft:granite": (157, 122, 95),
    "minecraft:diorite": (200, 200, 200),
    "minecraft:andesite": (140, 140, 140),
    "minecraft:grass": (124, 189, 107),
    "minecraft:grass_block": (124, 189, 107),
    "minecraft:dirt": (134, 96, 67),
    "minecraft:coarse_dirt": (119, 85, 60),
    "minecraft:podzol": (105, 80, 50),
    "minecraft:oak_log": (96, 76, 50),
    "minecraft:oak_planks": (160, 130, 80),
    "minecraft:oak_leaves": (60, 110, 50),
    "minecraft:birch_log": (210, 210, 200),
    "minecraft:birch_planks": (200, 195, 170),
    "minecraft:spruce_log": (70, 50, 35),
    "minecraft:spruce_planks": (110, 80, 55),
    "minecraft:jungle_log": (90, 65, 45),
    "minecraft:jungle_planks": (155, 120, 75),
    "minecraft:acacia_log": (110, 75, 55),
    "minecraft:acacia_planks": (170, 90, 60),
    "minecraft:dark_oak_log": (55, 40, 30),
    "minecraft:dark_oak_planks": (75, 55, 40),
    "minecraft:sand": (219, 207, 163),
    "minecraft:red_sand": (180, 110, 60),
    "minecraft:gravel": (135, 130, 130),
    "minecraft:clay": (160, 165, 175),
    "minecraft:snow": (245, 250, 255),
    "minecraft:ice": (155, 190, 230),
    "minecraft:packed_ice": (140, 180, 220),
    "minecraft:blue_ice": (110, 160, 210),
    "minecraft:water": (60, 110, 200),
    "minecraft:flowing_water": (60, 110, 200),
    "minecraft:lava": (230, 90, 20),
    "minecraft:flowing_lava": (230, 90, 20),
    "minecraft:obsidian": (30, 25, 40),
    "minecraft:bedrock": (60, 60, 60),
    "minecraft:gold_block": (255, 215, 50),
    "minecraft:iron_block": (220, 220, 220),
    "minecraft:diamond_block": (100, 235, 220),
    "minecraft:emerald_block": (60, 220, 130),
    "minecraft:redstone_block": (200, 30, 30),
    "minecraft:lapis_block": (40, 60, 180),
    "minecraft:netherrack": (110, 50, 50),
    "minecraft:nether_bricks": (70, 40, 40),
    "minecraft:glowstone": (220, 180, 80),
    "minecraft:end_stone": (220, 220, 170),
    "minecraft:purpur_block": (160, 130, 170),
    "minecraft:quartz_block": (235, 235, 230),
    "minecraft:bricks": (150, 75, 65),
    "minecraft:tnt": (190, 80, 60),
    "minecraft:bookshelf": (135, 105, 65),
    "minecraft:crafting_table": (150, 110, 70),
    "minecraft:furnace": (90, 90, 90),
    "minecraft:glass": (200, 220, 230),
    "minecraft:glass_pane": (200, 220, 230),
    "minecraft:wool": (220, 220, 220),
    "minecraft:white_wool": (235, 235, 235),
    "minecraft:black_wool": (30, 30, 30),
    "minecraft:red_wool": (170, 50, 50),
    "minecraft:blue_wool": (60, 80, 180),
    "minecraft:green_wool": (80, 160, 70),
    "minecraft:yellow_wool": (220, 200, 60),
    "minecraft:concrete": (200, 200, 200),
    "minecraft:white_concrete": (235, 235, 235),
    "minecraft:terracotta": (150, 100, 80),
    # 网易方块
    "netease:farmland": (120, 85, 60),
}

# 默认颜色（未知方块）
_DEFAULT_COLOR: Tuple[int, int, int] = (180, 100, 220)  # 紫色，醒目标记未知方块


def get_block_color(name: str) -> Tuple[int, int, int]:
    """获取方块的渲染颜色 (R, G, B)，0-255。未知方块返回紫色。"""
    return _BLOCK_COLORS.get(name, _DEFAULT_COLOR)


def color_to_hex(color: Tuple[int, int, int]) -> int:
    """RGB tuple → 0xRRGGBB 整数（Three.js Color 用）。"""
    r, g, b = color
    return (r << 16) | (g << 8) | b


def color_to_hex_str(color: Tuple[int, int, int]) -> str:
    """RGB tuple → '#RRGGBB' 字符串。"""
    r, g, b = color
    return f"#{r:02x}{g:02x}{b:02x}"
