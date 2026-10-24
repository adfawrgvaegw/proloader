"""离线解码服务端下发的 dynamicMCPLoader 模块（GetMCPCheckNum 的 mcp_hex 参数）。

用途：proloader 连服时会收到 GetMCPCheckNum(mcp_hex, salt) 挑战，日志里记录了 mcp_hex 全文。
若 proloader 内置的 dynamicMCP.py 算法（MD5('l00g<ks'+salt+'tsa') + 旋转签名）被服务端拒绝，
把日志里的 mcp_hex 传给本脚本，解码出服务端真正下发的 dynamicMCPLoader.GetCheckNum 算法。

解码管线（对齐 clientrpchandler.py + redirect.py McpImporter.read_module）：
    data = unhexlify(mcp_hex)
    data = NlsCipher().decrypt(data)      # rotor 流密码的 Py3 重实现（固定 seed）
    data = zlib.decompress(data)
    data = _reverse_data(data)            # 前130字节 XOR 0x9C + 整体反转
    code = marshal.loads(data)            # Py2.7 code 对象

用法：
    python3 decode_delivered_mcp.py <mcp_hex>
    python3 decode_delivered_mcp.py @file_with_hex.txt
"""
from __future__ import annotations

import sys
import os
import zlib
import binascii

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "vmu"))
from nls_cipher import NlsCipher  # noqa: E402

# Py2.7 pyc magic (62211 = 0x03f3 + 0x0d0a)
PY27_MAGIC = b"\x03\xf3\x0d\x0a"


def _reverse_data(data: bytes) -> bytes:
    """对齐 redirect.py McpImporter._reverse_data：前130字节 XOR 156(0x9C) + 整体反转。"""
    b = bytearray(data)
    for i in range(min(130, len(b))):
        b[i] ^= 0x9C
    b.reverse()
    return bytes(b)


def decode_mcp_hex(mcp_hex: str) -> bytes:
    """完整解码 mcp_hex → Py2.7 marshaled code object（raw marshal，无 pyc 头）。"""
    data = binascii.unhexlify(mcp_hex)
    data = NlsCipher().decrypt(data)
    data = zlib.decompress(data)
    data = _reverse_data(data)
    # redirect.py: if data[0] == 'o': return  → 'o' 表示无效，正常 code 对象首字节是 'c'(0x63)
    return data


def main() -> None:
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(1)

    arg = sys.argv[1]
    if arg.startswith("@"):
        mcp_hex = open(arg[1:]).read().strip()
    else:
        mcp_hex = arg.strip()
    # 容错：去掉可能的空白/换行/0x 前缀
    mcp_hex = mcp_hex.replace(" ", "").replace("\n", "").replace("\r", "")
    if mcp_hex.startswith("0x"):
        mcp_hex = mcp_hex[2:]

    print(f"[+] mcp_hex 长度: {len(mcp_hex)} 字符 ({len(mcp_hex)//2} 字节)")
    try:
        raw_marshal = decode_mcp_hex(mcp_hex)
    except Exception as e:
        print(f"[!] 解码失败: {e}")
        import traceback; traceback.print_exc()
        sys.exit(1)

    print(f"[+] 解码后 raw marshal: {len(raw_marshal)} 字节, 首字节={chr(raw_marshal[0])!r}(0x{raw_marshal[0]:02x})")
    if raw_marshal[0] == ord("o"):
        print("[!] 首字节是 'o'（redirect.py 视为无效模块），但仍保存以供分析")

    # 加 pyc 头，写成 .pyc 供 uncompyle6 反编译
    out_pyc = "/tmp/delivered_dynamicMCPLoader.pyc"
    with open(out_pyc, "wb") as f:
        f.write(PY27_MAGIC + b"\x00\x00\x00\x00" + raw_marshal)
    print(f"[+] 已写出 pyc: {out_pyc}")

    # 尝试反编译
    out_py = "/tmp/delivered_dynamicMCPLoader.py"
    import subprocess
    u = os.path.expanduser("~/Library/Python/3.9/bin/uncompyle6")
    if not os.path.exists(u):
        u = "uncompyle6"
    r = subprocess.run([u, out_pyc, "-o", "/tmp/delivered_src"],
                       capture_output=True, text=True)
    if os.path.exists(os.path.join("/tmp/delivered_src", os.path.basename(out_py))):
        src = open(os.path.join("/tmp/delivered_src", os.path.basename(out_py))).read()
        print(f"[+] 反编译成功，源码已写到 /tmp/delivered_src/")
        print("=" * 60)
        print(src)
        print("=" * 60)
        # 重点找 GetCheckNum
        if "GetCheckNum" in src:
            print("\n[★] 找到 GetCheckNum 函数，请对比 proloader 内置算法是否一致")
        else:
            print("\n[!] 源码中未找到 GetCheckNum，可能反编译不完整或算法名不同")
    else:
        print(f"[!] uncompyle6 反编译失败:")
        print(r.stderr[:500] if r.stderr else r.stdout[:500])
        print("[.] 可手动用 xdis 查看字节码: python3 -c \"import xdis; ...\"")
        # 备用：用 xdis 加载 code 对象并打印常量/字节码
        try:
            sys.path.insert(0, os.path.expanduser("~/Library/Python/3.9/lib/python/site-packages"))
            import xdis.marsh
            co = xdis.marsh.loads(raw_marshal, (2, 7))
            print(f"[+] xdis 加载 code 对象成功: {co}")
            print(f"[+] co_names: {co.co_names}")
            print(f"[+] co_consts (前10): {co.co_consts[:10]}")
        except Exception as e:
            print(f"[.] xdis 备用分析也失败: {e}")


if __name__ == "__main__":
    main()
