"""CommandRequest 逐字节对比：Python build_command_request vs Gophertunnel v1.58.1。

用与 tools/gophertunnel_test/cmd_marshal/main.go 完全相同的固定字段，
marshal CommandRequest 并打印 hex，供人工 / 脚本对比。

字段（与 Go 端 cmd_marshal/main.go 一一对应）：
    CommandLine      = "setblock 0 10 0 gold_block"
    Origin           = 0 (Player → "player")
    UUID             = 01234567-89ab-cdef-0123-456789abcdef
    RequestID        = "proloader_1"
    PlayerUniqueID   = 0
    Internal         = false
    Version          = ""  (v1.58.1: string)
"""
from __future__ import annotations

from pathlib import Path
import subprocess
import sys
import uuid as _uuid

from proloader.net.bedrock.login import build_command_request
from proloader.net.raknet.wire import ByteReader, ByteWriter

# 固定 UUID（与 Go 端完全一致）
FIXED_UUID = _uuid.UUID("01234567-89ab-cdef-0123-456789abcdef").bytes


def python_marshal() -> bytes:
    """复刻 build_command_request，但用固定 UUID（原函数内部随机 uuid4）。"""
    from proloader.net.bedrock.packets import PacketID

    origin = 0
    ORIGIN_NAMES = {0: "player"}
    origin_name = ORIGIN_NAMES[origin]

    w = ByteWriter()
    w.write_unsigned_varint(PacketID.COMMAND_REQUEST)  # 77
    w.write_string("setblock 0 10 0 gold_block")
    w.write_string(origin_name)          # "player"
    w.write_uuid(FIXED_UUID)             # 16 bytes（Bedrock 格式）
    w.write_string("proloader_1")        # request_id
    w.write_i64(0)                       # PlayerUniqueID int64 LE
    w.write_bool(False)                  # internal
    w.write_string("")                   # version (string)
    return w.to_bytes()


def annotate(b: bytes) -> str:
    """把 CommandRequest 字节流按字段分段，便于人工核对。"""
    r = ByteReader(b)
    pid = r.read_unsigned_varint()
    cmd_len = r.read_unsigned_varint()
    cmd = r.read(cmd_len).decode()
    origin_len = r.read_unsigned_varint()
    origin = r.read(origin_len).decode()
    uid = r.read(16)
    rid_len = r.read_unsigned_varint()
    rid = r.read(rid_len).decode()
    import struct
    puid = struct.unpack("<q", r.read(8))[0]
    internal = r.read(1)[0]
    ver_len = r.read_unsigned_varint()
    ver = r.read(ver_len).decode()
    return (
        f"  pid={pid} cmd={cmd!r} origin={origin!r} uuid={uid.hex()} "
        f"reqid={rid!r} player_unique_id={puid} internal={internal} version={ver!r}\n"
        f"  remaining={r.remaining}"
    )


def main() -> int:
    py_bytes = python_marshal()
    print(f"PY [{len(py_bytes)} B]: {py_bytes.hex()}")
    print(annotate(py_bytes))

    # 运行 Go 端对比程序
    go_dir = str(Path(__file__).resolve().parents[1] / "gophertunnel_test/cmd_marshal")
    res = subprocess.run(
        ["go", "run", "."], cwd=go_dir, capture_output=True, text=True, timeout=120
    )
    if res.returncode != 0:
        print("Go 运行失败:")
        print(res.stdout)
        print(res.stderr)
        return 1
    # 从输出提取 GO_BYTES=...
    go_bytes_hex = None
    for line in (res.stdout + res.stderr).splitlines():
        if "GO_BYTES=" in line:
            go_bytes_hex = line.split("GO_BYTES=")[1].strip()
    if go_bytes_hex is None:
        print("未能从 Go 输出提取 GO_BYTES")
        print(res.stdout)
        print(res.stderr)
        return 1
    print(f"\nGO [{len(go_bytes_hex)//2} B]: {go_bytes_hex}")

    py_hex = py_bytes.hex()
    if py_hex == go_bytes_hex:
        print("\n✓✓✓ Python 与 Gophertunnel v1.58.1 完全一致！CommandRequest 格式正确。")
        return 0
    else:
        print("\n✗✗✗ 不一致！定位第一个差异字节:")
        py_b = bytes.fromhex(py_hex)
        go_b = bytes.fromhex(go_bytes_hex)
        n = min(len(py_b), len(go_b))
        for i in range(n):
            if py_b[i] != go_b[i]:
                print(f"  offset {i}: PY=0x{py_b[i]:02x} GO=0x{go_b[i]:02x}")
                print(f"  PY 上下文: ...{py_hex[max(0,i-4)*2:(i+5)*2]}")
                print(f"  GO 上下文: ...{go_bytes_hex[max(0,i-4)*2:(i+5)*2]}")
                break
        if len(py_b) != len(go_b):
            print(f"  长度不同: PY={len(py_b)} GO={len(go_b)}")
        return 2


if __name__ == "__main__":
    sys.exit(main())
