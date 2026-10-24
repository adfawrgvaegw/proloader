"""建筑文件解析层。

支持格式：
    - .mcstructure 基岩版原生 NBT
    - .schem Sponge v2/v3
    - .schematic MCEdit Alpha（旧版数字ID）及Sponge改后缀文件
    - .mcworld 基岩版世界包
"""

from __future__ import annotations

from proloader.parser.base import load_structure, load_structure_from_bytes, register_parser
from proloader.parser.blockmap import color_to_hex, color_to_hex_str, get_block_color
from proloader.parser.mcstructure import parse_mcstructure_bytes, write_mcstructure

# 导入 mcstructure 模块以触发 @register_parser 装饰器
import proloader.parser.mcstructure  # noqa: F401
import proloader.parser.sponge  # noqa: F401
import proloader.parser.schematic  # noqa: F401

__all__ = [
    "load_structure",
    "load_structure_from_bytes",
    "register_parser",
    "parse_mcstructure_bytes",
    "write_mcstructure",
    "get_block_color",
    "color_to_hex",
    "color_to_hex_str",
]
