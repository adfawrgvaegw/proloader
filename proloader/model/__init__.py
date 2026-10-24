"""核心数据结构层：Block / BlockState / BlockPalette / Region / Structure / Transform。

协议无关、零 IO 依赖、可单测。所有上层模块（diff/sched/snapshot/backend）依赖此层。
"""

from __future__ import annotations

from proloader.model.block import Block, BlockState, NetEaseBlock
from proloader.model.constants import (
    AIR_NAME,
    BEDROCK_PROTOCOL_VERSION,
    BOT_LOAD_RADIUS,
    CHUNK_SIZE,
    MAX_BLOCKS_PER_PACKET,
    NETEASE_GAME_VERSION,
    RAKNET_DEFAULT_PORT,
    RAKNET_MAGIC,
    STONE_NAME,
    SUBCHUNK_SIZE,
    WORLD_MAX_Y,
    WORLD_MIN_Y,
)
from proloader.model.palette import BlockPalette
from proloader.model.region import BoundingBox, Region
from proloader.model.structure import Structure
from proloader.model.transform import Transform

__all__ = [
    # block
    "Block",
    "BlockState",
    "NetEaseBlock",
    # palette
    "BlockPalette",
    # region
    "Region",
    "BoundingBox",
    # structure
    "Structure",
    # transform
    "Transform",
    # constants
    "AIR_NAME",
    "STONE_NAME",
    "RAKNET_MAGIC",
    "RAKNET_DEFAULT_PORT",
    "NETEASE_GAME_VERSION",
    "BEDROCK_PROTOCOL_VERSION",
    "CHUNK_SIZE",
    "SUBCHUNK_SIZE",
    "WORLD_MIN_Y",
    "WORLD_MAX_Y",
    "MAX_BLOCKS_PER_PACKET",
    "BOT_LOAD_RADIUS",
]
