"""建筑文件解析器抽象基类与自动识别。

所有解析器实现 StructureParser 接口：parse(data) -> Structure。
通过文件扩展名自动选择解析器。
"""

from __future__ import annotations

from pathlib import Path
from typing import Callable, Dict, Optional

from proloader.errors import UnsupportedFormatError
from proloader.model.structure import Structure

# 解析器注册表：扩展名 -> 解析函数
_PARSERS: Dict[str, Callable[[bytes, str], Structure]] = {}


def register_parser(*extensions: str) -> Callable:
    """装饰器：注册解析器。

    Example:
        @register_parser(".mcstructure")
        def parse_mcstructure(data, name): ...
    """

    def decorator(func: Callable[[bytes, str], Structure]) -> Callable[[bytes, str], Structure]:
        for ext in extensions:
            _PARSERS[ext.lower()] = func
        return func

    return decorator


def load_structure(path: str | Path, name: Optional[str] = None) -> Structure:
    """根据文件扩展名自动选择解析器，加载建筑文件。

    Args:
        path: 文件路径
        name: 建筑名（默认用文件名，不含扩展名）

    Raises:
        UnsupportedFormatError: 不支持的扩展名
    """
    p = Path(path)
    ext = p.suffix.lower()
    if ext == '.mcworld':
        from proloader.parser.mcworld import load_mcworld
        return load_mcworld(p, name)
    if ext not in _PARSERS:
        raise UnsupportedFormatError(
            f"不支持的文件格式 {ext}，支持的格式: {sorted(_PARSERS.keys())}"
        )
    if name is None:
        name = p.stem
    data = p.read_bytes()
    return _PARSERS[ext](data, name)


def load_structure_from_bytes(data: bytes, ext: str, name: str = "structure") -> Structure:
    """从字节加载建筑（用于 Web API 上传）。"""
    ext = ext.lower()
    if ext not in _PARSERS:
        raise UnsupportedFormatError(f"不支持的文件格式 {ext}")
    return _PARSERS[ext](data, name)
