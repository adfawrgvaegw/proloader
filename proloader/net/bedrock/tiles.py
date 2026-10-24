"""proloader Block ↔ Bedrock /setblock 方块名映射。

Bedrock /setblock 命令语法：
    setblock <x> <y> <z> <tileName> [tileData] [replaceMode]

tileName 带方块状态时用方括号语法：
    minecraft:stairs["direction"=0,"upside_down_bit"=false]

学习要点：
    - 基岩版方块状态 key 与 Java 版不同（如 direction 而非 facing）
    - 布尔状态用 true/false，整数状态直接写数字，字符串状态加引号
    - air 方块名 minecraft:air，用于"破坏"（REMOVE 操作）
"""

from __future__ import annotations

from proloader.model.block import Block


def block_to_setblock_tile(block: Block) -> str:
    """把 proloader Block 转成 /setblock 的 tileName 片段。

    Example:
        Block("minecraft:stone") → "minecraft:stone"
        Block("minecraft:stairs", {"direction": 0, "upside_down_bit": False})
            → 'minecraft:stairs["direction"=0,"upside_down_bit"=false]'
    """
    name = block.state.name
    states = block.state.states
    if not states:
        return name
    parts = []
    for k, v in states.items():
        if isinstance(v, bool):
            parts.append(f'"{k}"={"true" if v else "false"}')
        elif isinstance(v, (int, float)):
            parts.append(f'"{k}"={v}')
        else:
            # 字符串值：外加引号
            parts.append(f'"{k}"="{v}"')
    return f'{name}[{",".join(parts)}]'


def build_setblock_command(x: int, y: int, z: int, block: Block, mode: str = "replace") -> str:
    """构造完整 /setblock 命令。

    Args:
        x, y, z: 世界坐标
        block: 目标方块
        mode: replace / keep / destroy（默认 replace，覆盖现有方块）
    """
    tile = block_to_setblock_tile(block)
    return f"setblock {x} {y} {z} {tile} {mode}"


def build_fill_command(
    x1: int, y1: int, z1: int, x2: int, y2: int, z2: int, block: Block, mode: str = "replace"
) -> str:
    """构造 /fill 命令（批量填充，矩形区域）。

    /fill 比 N 次 /setblock 快得多——一次命令填一整个长方体。
    调度器切块后，一个 subtask 内同种方块的连续区域可合并成少数 /fill。
    """
    tile = block_to_setblock_tile(block)
    return f"fill {x1} {y1} {z1} {x2} {y2} {z2} {tile} {mode}"


def build_tp_command(x: float, y: float, z: float) -> str:
    """构造传送命令（移动假人到放置区域）。"""
    return f"tp @s {x:.2f} {y:.2f} {z:.2f}"
