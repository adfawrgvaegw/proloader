"""BlockPalette：方块调色板，Block ↔ uint32 id 双向映射。

学习要点：
    - 建筑内大量重复方块（空气、石头），用整数索引存储比存 Block 对象省 10x+ 内存
    - 这与基岩版协议本身的 palette 存储方式天然对齐
    - Region 用 numpy uint32 数组存 palette id，差异计算时向量化比较

参考：
    - 基岩版 sub-chunk palette: wiki.bedrock.dev
    - .mcstructure 文件格式同样用 palette + index
"""

from __future__ import annotations

from typing import Dict, Iterator, List

from proloader.model.block import Block
from proloader.model.constants import AIR_NAME


class BlockPalette:
    """方块调色板：Block ↔ 整数 id 双向映射。

    - add(block): 返回 id（已存在则返回旧 id，否则新增）
    - get(id): 返回 Block
    - id_of(block): 返回 id 或 None

    约定：id=0 永远是空气（air），这样 Region 的零初始化数组天然全是空气。
    """

    def __init__(self) -> None:
        # id=0 固定为空气
        self._id_to_block: List[Block] = [Block.air()]
        self._block_to_id: Dict[Block, int] = {Block.air(): 0}

    @property
    def air_id(self) -> int:
        """空气方块的 id，永远是 0。"""
        return 0

    def add(self, block: Block) -> int:
        """添加方块到调色板，返回 id。已存在则返回旧 id（去重）。"""
        existing = self._block_to_id.get(block)
        if existing is not None:
            return existing
        new_id = len(self._id_to_block)
        self._id_to_block.append(block)
        self._block_to_id[block] = new_id
        return new_id

    def get(self, block_id: int) -> Block:
        """根据 id 获取方块。"""
        return self._id_to_block[block_id]

    def id_of(self, block: Block) -> int:
        """获取方块的 id。不存在则抛 KeyError。"""
        return self._block_to_id[block]

    def id_of_or(self, block: Block, default: int = -1) -> int:
        """获取方块的 id，不存在返回 default。"""
        return self._block_to_id.get(block, default)

    def __len__(self) -> int:
        return len(self._id_to_block)

    def __iter__(self) -> Iterator[Block]:
        return iter(self._id_to_block)

    def __contains__(self, block: Block) -> bool:
        return block in self._block_to_id

    def to_list(self) -> List[Block]:
        """序列化为 Block 列表（index 即 id）。"""
        return list(self._id_to_block)

    @classmethod
    def from_list(cls, blocks: List[Block]) -> "BlockPalette":
        """从 Block 列表构造（index 即 id）。

        约定：blocks[0] 必须是空气，否则会调整。
        """
        palette = cls.__new__(cls)
        palette._id_to_block = list(blocks)
        palette._block_to_id = {b: i for i, b in enumerate(blocks)}
        # 保证 air 在 id=0
        air = Block.air()
        if palette._id_to_block[0] != air:
            if air in palette._block_to_id:
                # 交换 air 到 0 位
                air_id = palette._block_to_id[air]
                other = palette._id_to_block[0]
                palette._id_to_block[0], palette._id_to_block[air_id] = air, other
                palette._block_to_id[air] = 0
                palette._block_to_id[other] = air_id
            else:
                # 没有 air，强制把 air 放到 0
                palette._id_to_block.insert(0, air)
                palette._block_to_id = {b: i for i, b in enumerate(palette._id_to_block)}
        return palette

    def to_dict_list(self) -> List[dict]:
        """序列化为可 JSON 化的列表（用于 Web API / 持久化）。"""
        result = []
        for block in self._id_to_block:
            result.append(
                {
                    "name": block.state.name,
                    "states": dict(block.state.states),
                    "layer": block.layer,
                }
            )
        return result

    @classmethod
    def from_dict_list(cls, data: List[dict]) -> "BlockPalette":
        """从可 JSON 化的列表反序列化。"""
        from proloader.model.block import BlockState

        blocks = []
        for item in data:
            blocks.append(Block(BlockState(item["name"], item.get("states", {})), item.get("layer", 0)))
        return cls.from_list(blocks)

    def __repr__(self) -> str:
        return f"BlockPalette(size={len(self._id_to_block)})"
