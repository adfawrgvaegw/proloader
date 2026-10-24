""".mcstructure 解析器（网易/基岩版原生结构文件格式）。

.mcstructure 格式（little-endian NBT）：
    TAG_Compound {
        format_version: Int (通常 1)
        size: [Int, Int, Int]              # (sx, sy, sz)
        structure_world_origin: [Int, Int, Int]  # 世界原点
        structure: Compound {
            block_indices: [                # 两个 layer 的列表
                [Int, Int, ...],            # layer 0: palette id 数组，-1 = 空气
                [Int, Int, ...]             # layer 1: palette id 数组（液体层，可能为空）
            ]
            palette: Compound {
                default: Compound {
                    block_palette: [        # 方块调色板
                        Compound { name: String, states: Compound, version: Int }
                    ]
                    block_position_data: Compound  # 方块实体（箱子等）
                }
            }
        }
    }

block_indices[0] 的索引为 x*sy*sz + y*sz + z，z 最内层。
结构文件的顺序不能套用 sub-chunk 存储顺序。

参考：wiki.bedrock.dev .mcstructure 格式
"""

from __future__ import annotations

from typing import Any, Dict, List, Tuple

import numpy as np

from proloader.errors import ParseError
from proloader.model.block import Block, BlockState
from proloader.model.palette import BlockPalette
from proloader.model.region import Region
from proloader.model.structure import Structure
from proloader.parser.base import register_parser
from proloader.parser.nbt import parse_nbt_auto, NBTByte


def _index_to_coord(index: int, size: Tuple[int, int, int]) -> Tuple[int, int, int]:
    """结构文件索引 → 本地坐标；对齐 WaterStructure.GetChunks/GetNBT。"""
    sx, sy, sz = size
    x, rem = divmod(index, sy * sz)
    y, z = divmod(rem, sz)
    return (x, y, z)


def _coord_to_index(x: int, y: int, z: int, size: Tuple[int, int, int]) -> int:
    """三维坐标 → 一维索引。"""
    _, sy, sz = size
    return x * sy * sz + y * sz + z


def _parse_block_palette(palette_data: List[Dict[str, Any]]) -> BlockPalette:
    """解析 block_palette 列表为 BlockPalette。

    .mcstructure 的 palette 第一项通常是空气，但我们仍以 BlockPalette 约定（air=0）对齐。
    """
    palette = BlockPalette()
    for item in palette_data:
        name = item.get("name", "minecraft:air")
        states = item.get("states", {})
        if not isinstance(states, dict):
            states = {}
        # version 字段暂不使用
        block = Block(BlockState(name, states), 0)
        palette.add(block)
    # 保证 air 在 id=0
    air = Block.air()
    if palette.get(0) != air:
        # .mcstructure 的 palette[0] 不是 air，强制插入 air 到 0
        # 但这会打乱 block_indices 的索引！需要处理映射
        # 实际上 .mcstructure 的 -1 表示空气，0 是 palette 第一个方块
        # 所以我们的 BlockPalette 的 id 和 .mcstructure 的 palette index 不一定一致
        pass
    return palette


def parse_mcstructure_bytes(data: bytes, name: str = "structure") -> Structure:
    """解析 .mcstructure 字节为 Structure。"""
    try:
        root = parse_nbt_auto(data)
    except ParseError:
        raise
    except Exception as e:
        raise ParseError(f"解析 .mcstructure 失败: {e}") from e

    if "format_version" not in root:
        raise ParseError(".mcstructure 缺少 format_version 字段")
    format_version = str(root["format_version"])

    if "size" not in root:
        raise ParseError(".mcstructure 缺少 size 字段")
    size_list = root["size"]
    if not isinstance(size_list, list) or len(size_list) != 3:
        raise ParseError(f".mcstructure size 格式错误: {size_list}")
    size: Tuple[int, int, int] = (int(size_list[0]), int(size_list[1]), int(size_list[2]))
    sx, sy, sz = size
    if sx <= 0 or sy <= 0 or sz <= 0:
        raise ParseError(f".mcstructure size 非法: {size}")

    # 世界原点
    origin = (0, 0, 0)
    if "structure_world_origin" in root:
        o = root["structure_world_origin"]
        if isinstance(o, list) and len(o) == 3:
            origin = (int(o[0]), int(o[1]), int(o[2]))

    # structure 字段
    struct_data = root.get("structure", {})
    if not isinstance(struct_data, dict):
        raise ParseError(".mcstructure structure 字段格式错误")

    # block_indices: 两个 layer
    block_indices = struct_data.get("block_indices", [])
    if not isinstance(block_indices, list):
        raise ParseError(".mcstructure block_indices 格式错误")
    layer0 = block_indices[0] if len(block_indices) >= 1 else []
    layer1 = block_indices[1] if len(block_indices) >= 2 else []
    if not isinstance(layer0, list):
        layer0 = []

    # palette
    palette_data = struct_data.get("palette", {})
    default_palette = palette_data.get("default", {}) if isinstance(palette_data, dict) else {}
    block_palette_list = default_palette.get("block_palette", [])
    if not isinstance(block_palette_list, list):
        block_palette_list = []
    # block_position_data（方块实体）
    block_position_data = default_palette.get("block_position_data", {})
    if not isinstance(block_position_data, dict):
        block_position_data = {}

    # 构建 BlockPalette（.mcstructure palette index 直接映射到我们的 palette）
    # .mcstructure: index=-1 表示空气，index>=0 是 palette 中的方块
    # 我们：air 固定 id=0，其他方块顺序加入
    our_palette = BlockPalette()  # id=0 是 air
    # 建立 .mcstructure index -> our palette id 的映射
    index_map: Dict[int, int] = {-1: our_palette.air_id}
    for i, item in enumerate(block_palette_list):
        if not isinstance(item, dict):
            continue
        bname = item.get("name", "minecraft:air")
        bstates = item.get("states", {})
        if not isinstance(bstates, dict):
            bstates = {}
        bstates = {k: bool(v) if isinstance(v, NBTByte) else v for k, v in bstates.items()}
        block = Block(BlockState(bname, bstates), 0)
        our_id = our_palette.add(block)
        index_map[i] = our_id

    # 构建 Region
    blocks = np.full((sx, sy, sz), our_palette.air_id, dtype=np.uint32)
    total = sx * sy * sz
    if len(layer0) != total:
        raise ParseError(f".mcstructure 主层长度错误: 预期 {total}，实际 {len(layer0)}")
    for i in range(min(len(layer0), total)):
        mc_index = int(layer0[i])
        if mc_index not in index_map:
            raise ParseError(f".mcstructure 调色板索引无效: {mc_index}")
        our_id = index_map[mc_index]
        x, y, z = _index_to_coord(i, size)
        blocks[x, y, z] = our_id

    # layer1（液体层）：暂存到 block_entities 或忽略（MVP 不处理双层）
    # TODO Phase 11: 处理 layer1

    # 方块实体
    block_entities: Dict[Tuple[int, int, int], dict] = {}
    for pos_key, entity in block_position_data.items():
        if not isinstance(entity, dict):
            continue
        try:
            if "block_entity_data" in entity:
                index = int(pos_key)
                if not 0 <= index < total or not isinstance(entity['block_entity_data'], dict):
                    raise ValueError
                coord = _index_to_coord(index, size)
                if coord in block_entities:
                    raise ParseError('方块实体位置重复。')
                block_entities[coord] = entity['block_entity_data']
            else:
                # Compatibility with older Proloader-generated data.
                pos = entity.get("pos")
                if isinstance(pos, list) and len(pos) == 3:
                    coord = tuple(map(int, pos))
                    if not all(0 <= a < b for a, b in zip(coord, size)):
                        raise ValueError
                    if coord in block_entities:
                        raise ParseError('方块实体位置重复。')
                    block_entities[coord] = entity.get("block_linked_entity", entity)
        except (ValueError, TypeError):
            raise ParseError('方块实体位置索引或数据无效。') from None

    region = Region(palette=our_palette, blocks=blocks, offset=origin)
    return Structure(
        name=name,
        size=size,
        palette=our_palette,
        region=region,
        block_entities=block_entities,
        format_version=format_version,
        is_netease=False,
    )


@register_parser(".mcstructure")
def _parse(data: bytes, name: str) -> Structure:
    return parse_mcstructure_bytes(data, name)


def write_mcstructure(structure: Structure) -> bytes:
    """把 Structure 序列化为 .mcstructure 字节（little-endian NBT）。

    用于生成测试用文件 / Web 导出。
    """
    from proloader.parser.nbt import dump_nbt

    sx, sy, sz = structure.size
    # 构建 palette list（.mcstructure index 顺序）
    # 我们保证 air=0，.mcstructure 用 -1 表示空气
    # 简化：palette list 就用 our_palette 的顺序（air 在 0），block_indices 用 -1 表示空气
    block_palette_list: List[Dict[str, Any]] = []
    # .mcstructure index 0 我们也放空气，方便映射
    for block in structure.palette:
        block_palette_list.append(
            {"name": block.state.name, "states": dict(block.state.states), "version": 17959425}
        )

    # block_indices layer0：z 最内层，与独立上游读取器一致。
    layer0: List[int] = []
    air_id = structure.palette.air_id
    for x in range(sx):
        for y in range(sy):
            for z in range(sz):
                bid = int(structure.region.blocks[x, y, z])
                # .mcstructure 用 -1 表示空气，但我们 palette[0] 就是 air
                # 为兼容标准格式，把 air 映射为 -1（但这样 palette[0] 就没被引用）
                # 简化：直接用 our id（air=0），不转 -1，这样能 round-trip
                layer0.append(bid if bid != air_id else -1)

    position_data = {}
    for coord, entity in structure.block_entities.items():
        if len(coord) != 3 or not all(type(a) is int and 0 <= a < b for a, b in zip(coord, structure.size)):
            raise ParseError('方块实体位置超出建筑范围。')
        nbt = dict(entity)
        nbt.update(zip(('x', 'y', 'z'), (a+b for a, b in zip(coord, structure.region.offset))))
        position_data[str(_coord_to_index(*coord, structure.size))] = {'block_entity_data': nbt}
    root: Dict[str, Any] = {
        "format_version": 1,
        "size": [sx, sy, sz],
        "structure_world_origin": list(structure.region.offset),
        "structure": {
            "block_indices": [layer0, []],  # layer1 空
            "palette": {
                "default": {
                    "block_palette": block_palette_list,
                    "block_position_data": position_data,
                }
            },
        },
    }
    data = dump_nbt(root)
    # Standard little-endian root compound includes an empty UTF-8 name.
    return data[:1] + b'\x00\x00' + data[1:]
