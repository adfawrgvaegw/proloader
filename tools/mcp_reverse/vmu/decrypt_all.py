from __future__ import annotations
import sys, os
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from anti_confuser import restore_data

SRC = "/tmp/vanilla_unpack"
DST = "/tmp/vanilla_pyc"
os.makedirs(DST, exist_ok=True)

mcs_files = []
for root, dirs, files in os.walk(SRC):
    for f in files:
        if f.endswith(".mcs"):
            mcs_files.append(os.path.join(root, f))

print(f"待解密: {len(mcs_files)} 个 .mcs")
ok = 0
fail = 0
fail_examples = []
for i, src_path in enumerate(mcs_files):
    try:
        with open(src_path, "rb") as f:
            data = f.read()
        pyc = restore_data(data)
        rel = os.path.relpath(src_path, SRC)
        out = os.path.join(DST, rel.replace(".mcs", ".pyc"))
        os.makedirs(os.path.dirname(out), exist_ok=True)
        with open(out, "wb") as f:
            f.write(pyc)
        ok += 1
    except Exception as e:
        fail += 1
        if len(fail_examples) < 8:
            fail_examples.append((os.path.basename(src_path), str(e)[:150]))
    if (i + 1) % 500 == 0:
        print(f"  进度 {i+1}/{len(mcs_files)} (ok={ok} fail={fail})")

print(f"\n完成: 成功 {ok}, 失败 {fail}")
for name, err in fail_examples:
    print(f"  失败示例 {name}: {err}")
