"""生成示例建筑文件（用于演示和测试）。

generate_house(): 生成一个小房子（地基+墙壁+屋顶+门+窗）
generate_tower(): 生成一个塔
"""

from __future__ import annotations

from pathlib import Path

from proloader.model.block import Block, BlockState
from proloader.model.palette import BlockPalette
from proloader.model.region import Region
from proloader.model.structure import Structure
from proloader.parser.mcstructure import write_mcstructure


def _block(name: str, **states) -> Block:
    return Block(BlockState(name, dict(states)), 0)


# 常用方块
AIR = Block.air()
STONE = _block("minecraft:stone")
DIRT = _block("minecraft:dirt")
OAK_PLANKS = _block("minecraft:oak_planks")
OAK_LOG = _block("minecraft:oak_log")
GLASS = _block("minecraft:glass")
COBBLESTONE = _block("minecraft:cobblestone")
BRICKS = _block("minecraft:bricks")
WOOL_WHITE = _block("minecraft:white_wool")
WOOL_RED = _block("minecraft:red_wool")
GOLD_BLOCK = _block("minecraft:gold_block")
DIAMOND_BLOCK = _block("minecraft:diamond_block")


def generate_house(path: str | Path) -> Structure:
    """生成 7×5×7 的小房子。

    结构：
        - 地基（Y=0）: 石头
        - 墙壁（Y=1-3）: 橡木板，正面有门，侧面有窗
        - 屋顶（Y=4）: 红色羊毛
    """
    sx, sy, sz = 7, 5, 7
    palette = BlockPalette()
    blocks = []
    for x in range(sx):
        for y in range(sy):
            for z in range(sz):
                b = AIR
                if y == 0:
                    b = STONE  # 地基
                elif 1 <= y <= 3:
                    # 墙壁：四周边缘
                    on_wall = (x == 0 or x == sx - 1 or z == 0 or z == sz - 1)
                    if on_wall:
                        b = OAK_PLANKS
                        # 正面门（x=3, z=0, y=1-2）
                        if x == 3 and z == 0 and y in (1, 2):
                            b = AIR
                        # 窗户（玻璃）
                        if x == 3 and z == sz - 1 and y == 2:
                            b = GLASS
                        if (x == 0 or x == sx - 1) and z == 3 and y == 2:
                            b = GLASS
                elif y == 4:
                    # 屋顶
                    b = WOOL_RED
                blocks.append((x, y, z, b))

    region = Region.from_blocks(blocks, offset=(0, 0, 0))
    struct = Structure.from_region("house", region)
    data = write_mcstructure(struct)
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(data)
    return struct


def generate_tower(path: str | Path) -> Structure:
    """生成 5×8×5 的塔（适合演示多假人并行切块）。"""
    sx, sy, sz = 5, 8, 5
    blocks = []
    for x in range(sx):
        for y in range(sy):
            for z in range(sz):
                b = AIR
                on_edge = (x == 0 or x == sx - 1 or z == 0 or z == sz - 1)
                if on_edge:
                    if y == sy - 1:
                        b = GOLD_BLOCK  # 塔顶
                    else:
                        b = COBBLESTONE  # 墙
                elif y == 0:
                    b = STONE  # 地基
                # 中间楼层（y=4）
                if y == 4 and not on_edge:
                    b = OAK_PLANKS
                blocks.append((x, y, z, b))

    region = Region.from_blocks(blocks, offset=(0, 0, 0))
    struct = Structure.from_region("tower", region)
    data = write_mcstructure(struct)
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(data)
    return struct


if __name__ == "__main__":
    import sys
    out = sys.argv[1] if len(sys.argv) > 1 else "house.mcstructure"
    struct = generate_house(out)
    print(f"已生成 {out}: {struct}")
