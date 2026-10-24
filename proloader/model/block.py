"""方块数据结构：BlockState / Block / NetEaseBlock。

学习要点：
    - 基岩版方块 = name(字符串) + states(键值对)，例如 "minecraft:stairs" + {"direction": 0, "upside_down": False}
    - 数值 runtime ID 每个版本都变，不能用数值 ID 持久化，必须用 name+states
    - frozen=True 让方块对象可哈希，用于差异比较和调色板去重
    - 网易自定义方块多出 name_hash / network_id / val 字段（参考 bedrock-world-operator/define）

参考：
    - bedrock-world-operator: github.com/TriM-Organization/bedrock-world-operator
    - 基岩版方块状态规范: wiki.bedrock.dev
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Mapping


@dataclass(frozen=True)
class BlockState:
    """方块状态：名称 + 状态键值对。

    frozen=True 使其可哈希，用于：
      - BlockPalette 去重（相同状态的方块共用一个 palette id）
      - DiffEngine 差异比较（hash 后 O(1) 比较）

    Example:
        >>> stone = BlockState("minecraft:stone", {})
        >>> wet_farmland = BlockState("minecraft:farmland", {"moisturized_amount": 7})
    """

    name: str
    states: Mapping[str, Any] = field(default_factory=dict)

    def __post_init__(self) -> None:
        # 把 states 冻结为不可变形式（frozendict 风格：转成不可变视图）
        # 用 dict 而非 frozenset 是因为值可能非哈希（如嵌套 dict），但实践中基岩版 states 都是标量
        object.__setattr__(self, "states", dict(self.states))

    def __hash__(self) -> int:
        # 按 key 排序保证相同内容哈希一致
        items = tuple(sorted(self.states.items(), key=lambda kv: kv[0]))
        return hash((self.name, items))

    def __eq__(self, other: object) -> bool:
        if not isinstance(other, BlockState):
            return NotImplemented
        return self.name == other.name and self.states == other.states

    @property
    def is_air(self) -> bool:
        """是否为空气方块。"""
        return self.name == "minecraft:air"


@dataclass(frozen=True)
class Block:
    """完整方块：状态 + 层。

    基岩版支持 0/1 两层（layer 0 主层，layer 1 次层，用于水/熔岩双层方块）。
    大多数方块只有 layer 0。

    Example:
        >>> stone = Block(BlockState("minecraft:stone", {}))
        >>> flowing_water = Block(BlockState("minecraft:flowing_water", {"liquid_depth": 7}), layer=1)
    """

    state: BlockState
    layer: int = 0

    @property
    def is_air(self) -> bool:
        """是否为空气方块。"""
        return self.state.is_air

    @classmethod
    def air(cls) -> "Block":
        """空气方块（最常用，单独提供工厂方法）。"""
        return cls(BlockState("minecraft:air", {}), 0)

    def __hash__(self) -> int:
        return hash((self.state, self.layer))

    def __eq__(self, other: object) -> bool:
        if not isinstance(other, Block):
            return NotImplemented
        return self.state == other.state and self.layer == other.layer


@dataclass(frozen=True)
class NetEaseBlock:
    """网易自定义方块结构（对照 bedrock-world-operator/define.NetEaseBlock）。

    网易方块比标准基岩版多出：
      - name_hash: 网易用 fnv1a 哈希方块名得到
      - network_id: 网易 runtime ID
      - val: 网易旧版方块 val（兼容旧协议）

    MVP 阶段不处理网易自定义方块，Phase 11 再完整支持。
    """

    name: str
    name_hash: int
    network_id: int
    states: Mapping[str, Any] = field(default_factory=dict)
    val: int = 0
    version: int = 0

    def __post_init__(self) -> None:
        object.__setattr__(self, "states", dict(self.states))

    def to_block(self) -> Block:
        """转换为标准 Block（丢失网易特有字段，用于差异比较）。"""
        return Block(BlockState(self.name, self.states), 0)
