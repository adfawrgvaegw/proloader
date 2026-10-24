from __future__ import annotations
import struct

from typing import Any
from io import TextIOBase

from mcs_marshal import McsMarshal
from opcode_map import get_mcs2std_op_map

# Python 2.7 HAVE_ARGUMENT threshold
HAVE_ARGUMENT = 90

class FakeFileObject(TextIOBase):
    def __init__(self):
        self.data = bytearray()

    def write(self, b: bytes) -> None:
        self.data.extend(b)

    def getvalue(self) -> bytes:
        return bytes(self.data)


def transform_code(mcs_obj: dict) -> bytes:
    magic = mcs_obj['magic']
    version = mcs_obj.get('version', 1)
    op_map = get_mcs2std_op_map(version)
    mcs_code = bytearray(mcs_obj['code'])
    new_code = bytearray()
    i = 0

    obj_name = mcs_obj.get('name')
    if isinstance(obj_name, bytes):
        obj_name = obj_name.decode('utf-8', 'ignore')

    while i < len(mcs_code):
        mcs_op = mcs_code[i]

        # ✅ Bug 1 Fix:
        # 先将 mcs_op 映射为 std_op，再用 std_op 判断是否有参数
        # 原代码用 mcs_op >= 93 判断，是完全错误的
        std_op = op_map.get(mcs_op, mcs_op)

        if std_op >= HAVE_ARGUMENT:
            # 带参数指令：读后续 2 字节作为参数
            if i + 2 < len(mcs_code):
                arg = mcs_code[i + 1] | (mcs_code[i + 2] << 8)
                step = 3
            else:
                arg = 0
                step = len(mcs_code) - i
        else:
            # 不带参数指令
            arg = None
            step = 1

        new_code.append(std_op)
        if std_op >= HAVE_ARGUMENT:
            a = arg if arg is not None else 0
            new_code.extend([a & 0xFF, (a >> 8) & 0xFF])

        i += step

    return bytes(new_code)


def w_long(val: int, f: TextIOBase) -> None:
    f.write(b'l')
    if val == 0:
        f.write(struct.pack('<i', 0))
        return
    sign = 1 if val >= 0 else -1
    v = abs(val)
    digits = []
    while v:
        digits.append(v & 0x7FFF)
        v >>= 15
    f.write(struct.pack('<i', sign * len(digits)))
    for d in digits:
        f.write(struct.pack('<H', d))


def w_object(obj: Any, f: TextIOBase, interned: bool = False) -> None:
    if obj is None:
        f.write(b'N')
    elif obj is True:
        f.write(b'T')
    elif obj is False:
        f.write(b'F')
    elif obj is Ellipsis:
        f.write(b'.')
    elif isinstance(obj, int):
        if -2147483648 <= obj <= 2147483647:
            f.write(b'i')
            f.write(struct.pack('<i', obj))
        else:
            w_long(obj, f)
    elif isinstance(obj, float):
        s = repr(obj).encode()
        f.write(b'f')
        f.write(struct.pack('B', len(s)))
        f.write(s)
    elif isinstance(obj, bytes):
        f.write(b's')
        f.write(struct.pack('<i', len(obj)))
        f.write(obj)
    elif isinstance(obj, str):
        b = obj.encode('utf-8')
        # ✅ Bug 3 Fix:
        # 函数名、变量名等字符串在 Python 2.7 marshal 中是 interned string，tag 为 't'
        # 普通字符串 tag 为 's'
        # 这里根据 interned 参数决定，names/varnames 等表中的字符串应传 interned=True
        if interned:
            f.write(b't')
        else:
            f.write(b's')
        f.write(struct.pack('<i', len(b)))
        f.write(b)
    elif isinstance(obj, (tuple, list, set, frozenset)):
        if isinstance(obj, tuple):
            f.write(b'(')
        elif isinstance(obj, list):
            f.write(b'[')
        elif isinstance(obj, frozenset):
            f.write(b'>')
        else:
            f.write(b'<')
        f.write(struct.pack('<i', len(obj)))
        for item in obj:
            w_object(item, f)
    elif isinstance(obj, dict) and 'magic' in obj:
        f.write(b'c')
        f.write(struct.pack('<i', obj['argcount']))
        f.write(struct.pack('<i', obj['nlocals']))
        f.write(struct.pack('<i', obj['stacksize']))
        f.write(struct.pack('<i', obj['flags']))
        
        # ✅ Fix co_code
        w_object(transform_code(obj), f)
        
        # ✅ Fix co_consts
        w_object(tuple(obj['consts']), f)

        # ✅ Bug 3 Fix: names, varnames, freevars, cellvars 中的字符串都是 interned string
        # 写出时用 't' tag
        _write_name_tuple(tuple(obj['names']), f)
        _write_name_tuple(tuple(obj['varnames']), f)
        _write_name_tuple(tuple(obj['freevars']), f)
        _write_name_tuple(tuple(obj['cellvars']), f)

        # ✅ co_filename and co_name (interned)
        w_object(obj.get('filename') or '', f)
        w_object(obj.get('name') or '', f, interned=True)

        # ✅ Bug 2 Fix: firstlineno 在 CodeObject 中必须是裸写的 4 字节 int，不能带 'i' Tag
        f.write(struct.pack('<i', obj.get('firstlineno', 0)))

        # ✅ Bug 4 Fix: co_lnotab 必须是 bytes，不能是 None
        lnotab = obj.get('lnotab')
        if lnotab is None:
            lnotab = b''
        w_object(lnotab, f)
    elif isinstance(obj, dict):
        f.write(b'{')
        for k, v in obj.items():
            w_object(k, f)
            w_object(v, f)
        f.write(b'0')
    else:
        f.write(b'N')


def _write_name_tuple(names: tuple, f: TextIOBase) -> None:
    """写出 names/varnames 等 interned 字符串元组"""
    f.write(b'(')
    f.write(struct.pack('<i', len(names)))
    for name in names:
        w_object(name, f, interned=True)


def restore_data(data: bytes) -> bytes:
    from crypto import decrypt_data

    decrypted_data = decrypt_data(data)
    parser = McsMarshal(decrypted_data)
    root = parser.r_object()
    f = FakeFileObject()
    # Python 2.7 pyc header: magic(4) + timestamp(4)
    f.write(b"\x03\xf3\x0d\x0a\x00\x00\x00\x00")
    w_object(root, f)
    return f.getvalue()


def main():
    import sys
    if len(sys.argv) < 2:
        print("Usage: python anti_confuser.py <input_file> [output_file]")
        return

    in_name = sys.argv[1]
    out_name = sys.argv[2] if len(sys.argv) > 2 else in_name + ".pyc"

    with open(in_name, 'rb') as f:
        data = f.read()

    restored_data = restore_data(data)
    with open(out_name, 'wb') as out_f:
        out_f.write(restored_data)
    print(f"[+] Restored to {out_name}")


if __name__ == "__main__":
    main()