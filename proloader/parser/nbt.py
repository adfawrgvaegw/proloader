"""NBT (Named Binary Tag) 读写 —— little-endian 版本（基岩版 .mcstructure 用）。

学习要点：
    - Java 版 NBT 用 big-endian，基岩版 .mcstructure 用 little-endian
    - TAG 类型：0=End 1=Byte 2=Short 3=Int 4=Long 5=Float 6=Double
                7=ByteArray 8=String 9=List 10=Compound 11=IntArray 12=LongArray
    - .mcstructure 顶层是一个无名字的 TAG_Compound（不像 Java 版根 compound 有名字）
    - 字符串：2 字节 little-endian short 长度 + UTF-8 字节

参考：
    - wiki.bedrock.dev .mcstructure 格式
    - minecraft.wiki NBT 格式
"""

from __future__ import annotations

import struct
from io import BytesIO
from typing import Any, Dict, List, Union

from proloader.errors import ParseError

# TAG 类型常量
TAG_END = 0
TAG_BYTE = 1
TAG_SHORT = 2
TAG_INT = 3
TAG_LONG = 4
TAG_FLOAT = 5
TAG_DOUBLE = 6
TAG_BYTE_ARRAY = 7
TAG_STRING = 8
TAG_LIST = 9
TAG_COMPOUND = 10
TAG_INT_ARRAY = 11
TAG_LONG_ARRAY = 12

# NBT 解析后的 Python 类型：
#   TAG_Compound -> dict[str, Any]
#   TAG_List -> list[Any]
#   TAG_String -> str
#   TAG_Byte/Short/Int/Long -> int
#   TAG_Float/Double -> float
#   TAG_Byte_Array/Int_Array/Long_Array -> bytes / list[int]
NBTValue = Union[Dict[str, Any], List[Any], str, int, float, bytes]


# Retain numeric tag widths when round-tripping entity data. JSON and arithmetic
# still see integers; the file writer can preserve byte flags and 64-bit history.
class NBTByte(int):
    pass


class NBTShort(int):
    pass


class NBTLong(int):
    pass


class NBTReader:
    """little-endian NBT 读取器。

    用法：
        reader = NBTReader(data)
        root = reader.read_root()  # 返回 dict（顶层 compound 的内容）
    """

    def __init__(self, data: bytes, endian: str = "little") -> None:
        self._buf = BytesIO(data)
        self._pos = 0
        # endian: 'little'(基岩版 .mcstructure) 或 'big'(Java 版 NBT)
        self._en = "<" if endian == "little" else ">"

    def _read(self, n: int) -> bytes:
        b = self._buf.read(n)
        if len(b) < n:
            raise ParseError(f"NBT 读取越界：需要 {n} 字节，只读到 {len(b)} 字节")
        return b

    def read_byte(self) -> int:
        return struct.unpack(self._en + "b", self._read(1))[0]

    def read_ubyte(self) -> int:
        return struct.unpack(self._en + "B", self._read(1))[0]

    def read_short(self) -> int:
        return struct.unpack(self._en + "h", self._read(2))[0]

    def read_ushort(self) -> int:
        return struct.unpack(self._en + "H", self._read(2))[0]

    def read_int(self) -> int:
        return struct.unpack(self._en + "i", self._read(4))[0]

    def read_long(self) -> int:
        return struct.unpack(self._en + "q", self._read(8))[0]

    def read_float(self) -> float:
        return struct.unpack(self._en + "f", self._read(4))[0]

    def read_double(self) -> float:
        return struct.unpack(self._en + "d", self._read(8))[0]

    def read_string(self) -> str:
        length = self.read_ushort()  # 2 字节 short 长度（endian 由 self._en 决定）
        return self._read(length).decode("utf-8", errors="replace")

    def read_payload(self, tag_type: int) -> Any:
        """根据 TAG 类型读取负载。"""
        if tag_type == TAG_BYTE:
            return NBTByte(self.read_byte())
        if tag_type == TAG_SHORT:
            return NBTShort(self.read_short())
        if tag_type == TAG_INT:
            return self.read_int()
        if tag_type == TAG_LONG:
            return NBTLong(self.read_long())
        if tag_type == TAG_FLOAT:
            return self.read_float()
        if tag_type == TAG_DOUBLE:
            return self.read_double()
        if tag_type == TAG_BYTE_ARRAY:
            length = self.read_int()
            return self._read(length)
        if tag_type == TAG_STRING:
            return self.read_string()
        if tag_type == TAG_LIST:
            return self._read_list()
        if tag_type == TAG_COMPOUND:
            return self._read_compound()
        if tag_type == TAG_INT_ARRAY:
            length = self.read_int()
            return [self.read_int() for _ in range(length)]
        if tag_type == TAG_LONG_ARRAY:
            length = self.read_int()
            return [self.read_long() for _ in range(length)]
        raise ParseError(f"未知 NBT TAG 类型: {tag_type}")

    def _read_list(self) -> List[Any]:
        element_type = self.read_byte()  # 元素类型
        length = self.read_int()  # 元素数量
        return [self.read_payload(element_type) for _ in range(length)]

    def _read_compound(self) -> Dict[str, Any]:
        result: Dict[str, Any] = {}
        while True:
            child_type = self.read_byte()
            if child_type == TAG_END:
                break
            name = self.read_string()
            result[name] = self.read_payload(child_type)
        return result

    def read_root(self) -> Dict[str, Any]:
        """读取根 compound。

        big-endian（Java NBT）：type(1) + name + compound payload（根有名字）。
        little-endian：两种变体都存在 ——
            - 基岩版 .mcstructure 标准：type(1) + compound payload（根无名字）
            - 部分 Python 工具（如 phoenixr-codes/mcstructure）：type(1) + name + payload（根有名字）
        LE 模式先试无名字，若得到空 dict 则回退到有名字。
        """
        root_type = self.read_byte()
        if root_type != TAG_COMPOUND:
            raise ParseError(f"NBT 根必须是 TAG_Compound(10)，得到 {root_type}")
        if self._en == ">":
            # big-endian Java NBT：根 compound 一定有名字
            self.read_string()  # 读取并丢弃根名字
            return self._read_compound()
        # little-endian：先试无名字（基岩版标准）
        pos = self._buf.tell()
        try:
            root = self._read_compound()
            if root:  # 非空 dict，认为是基岩版无名字格式
                return root
            # 空 dict：可能是"根有名字"变体，回退
            self._buf.seek(pos)
        except Exception:
            self._buf.seek(pos)
        # 回退：LE + 根有名字（phoenixr-codes 等工具的变体）
        self.read_string()  # 读取并丢弃根名字
        return self._read_compound()


class NBTWriter:
    """little-endian NBT 写入器（用于生成测试用 .mcstructure 文件）。"""

    def __init__(self) -> None:
        self._buf = bytearray()

    def write_byte(self, v: int) -> None:
        self._buf += struct.pack("<b", v)

    def write_short(self, v: int) -> None:
        self._buf += struct.pack("<h", v)

    def write_int(self, v: int) -> None:
        self._buf += struct.pack("<i", v)

    def write_long(self, v: int) -> None:
        self._buf += struct.pack("<q", v)

    def write_float(self, v: float) -> None:
        self._buf += struct.pack("<f", v)

    def write_double(self, v: float) -> None:
        self._buf += struct.pack("<d", v)

    def write_string(self, v: str) -> None:
        b = v.encode("utf-8")
        self.write_ushort(len(b))
        self._buf += b

    def write_ushort(self, v: int) -> None:
        self._buf += struct.pack("<H", v)

    def write_payload(self, value: Any) -> None:
        if isinstance(value, bool):
            self.write_byte(int(value))
        elif isinstance(value, int):
            self.write_int(value)
        elif isinstance(value, float):
            self.write_double(value)
        elif isinstance(value, str):
            self.write_byte(TAG_STRING)
            self.write_string(value)
        elif isinstance(value, bytes):
            self.write_byte(TAG_BYTE_ARRAY)
            self.write_int(len(value))
            self._buf += value
        elif isinstance(value, list):
            self._write_list(value)
        elif isinstance(value, dict):
            self._write_compound(value)
        else:
            raise ParseError(f"无法序列化 NBT 值: {type(value)}")

    def _write_list(self, value: List[Any]) -> None:
        # 推断元素类型
        if not value:
            self.write_byte(TAG_END)
            self.write_int(0)
            return
        first = value[0]
        if isinstance(first, (bool, NBTByte)):
            etype = TAG_BYTE
        elif isinstance(first, NBTShort):
            etype = TAG_SHORT
        elif isinstance(first, NBTLong):
            etype = TAG_LONG
        elif isinstance(first, int):
            etype = TAG_INT
        elif isinstance(first, float):
            etype = TAG_DOUBLE
        elif isinstance(first, str):
            etype = TAG_STRING
        elif isinstance(first, dict):
            etype = TAG_COMPOUND
        elif isinstance(first, list):
            etype = TAG_LIST
        else:
            raise ParseError(f"无法推断 NBT 列表元素类型: {type(first)}")
        self.write_byte(etype)
        self.write_int(len(value))
        for item in value:
            # 列表元素无名字，直接写 payload
            if etype == TAG_BYTE:
                self.write_byte(int(item))
            elif etype == TAG_SHORT:
                self.write_short(int(item))
            elif etype == TAG_LONG:
                self.write_long(int(item))
            elif etype == TAG_INT:
                self.write_int(int(item))
            elif etype == TAG_DOUBLE:
                self.write_double(float(item))
            elif etype == TAG_STRING:
                self.write_string(str(item))
            elif etype == TAG_COMPOUND:
                self._write_compound(item)
            elif etype == TAG_LIST:
                self._write_list(item)
            else:
                raise ParseError(f"未实现的列表元素类型: {etype}")

    def _write_compound(self, value: Dict[str, Any]) -> None:
        for name, val in value.items():
            # 写 type + name
            if isinstance(val, (bool, NBTByte)):
                self.write_byte(TAG_BYTE)
            elif isinstance(val, NBTShort):
                self.write_byte(TAG_SHORT)
            elif isinstance(val, NBTLong):
                self.write_byte(TAG_LONG)
            elif isinstance(val, int):
                self.write_byte(TAG_INT)
            elif isinstance(val, float):
                self.write_byte(TAG_DOUBLE)
            elif isinstance(val, str):
                self.write_byte(TAG_STRING)
            elif isinstance(val, bytes):
                self.write_byte(TAG_BYTE_ARRAY)
            elif isinstance(val, list):
                self.write_byte(TAG_LIST)
            elif isinstance(val, dict):
                self.write_byte(TAG_COMPOUND)
            else:
                raise ParseError(f"无法序列化 NBT compound 字段 {name}: {type(val)}")
            self.write_string(name)
            # 写 payload（type 已写，这里只写 payload 部分）
            if isinstance(val, (bool, NBTByte)):
                self.write_byte(int(val))
            elif isinstance(val, NBTShort):
                self.write_short(int(val))
            elif isinstance(val, NBTLong):
                self.write_long(int(val))
            elif isinstance(val, int):
                self.write_int(int(val))
            elif isinstance(val, float):
                self.write_double(float(val))
            elif isinstance(val, str):
                self.write_string(val)
            elif isinstance(val, bytes):
                self.write_int(len(val))
                self._buf += val
            elif isinstance(val, list):
                self._write_list(val)
            elif isinstance(val, dict):
                self._write_compound(val)
        self.write_byte(TAG_END)

    def write_root(self, value: Dict[str, Any]) -> bytes:
        """写入根 compound，返回字节。"""
        self.write_byte(TAG_COMPOUND)
        self._write_compound(value)
        return bytes(self._buf)


def parse_nbt(data: bytes, endian: str = "little") -> Dict[str, Any]:
    """解析 NBT，返回 dict。endian='little'(基岩版) 或 'big'(Java)。"""
    return NBTReader(data, endian=endian).read_root()


def parse_nbt_auto(data: bytes) -> Dict[str, Any]:
    """自动检测字节序解析 NBT。

    先试 little-endian（基岩版 .mcstructure 标准），若结果为空或异常则试 big-endian（Java NBT）。
    用于兼容不同工具生成的 .mcstructure / .nbt 文件（网易客户端导出是 LE，
    phoenixr-codes/mcstructure 等 Python 库的样本可能是 BE）。
    """
    # 先试 little-endian（基岩版标准）
    try:
        root = NBTReader(data, endian="little").read_root()
        if root:  # 非空 dict，认为是合法 LE
            return root
    except Exception:
        pass
    # 回退到 big-endian（Java NBT）
    return NBTReader(data, endian="big").read_root()


def dump_nbt(value: Dict[str, Any]) -> bytes:
    """序列化为 little-endian NBT 字节。"""
    return NBTWriter().write_root(value)
