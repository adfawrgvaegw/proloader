from __future__ import annotations
import os
import shutil
import time
import struct
import hashlib
from concurrent.futures import ProcessPoolExecutor
from mcpk import unpack_mcpk
from anti_confuser import restore_data

# Core script filenames that need content-based MD5 renaming
CORE_SCRIPTS = {'modMain.pyc', '__init__.pyc'}

WRAPPER_CONTENT_TEMPLATE = """# -*- coding: utf-8 -*-
import os, marshal, sys
if __name__ != '__main__': sys.modules[__name__] = sys.modules.get(__name__)
try:
    base_dir = os.path.dirname(os.path.normpath(os.path.join(os.getcwd(), __file__)))
    with open(os.path.join(base_dir, '{hex_name}'), 'rb') as f:
        f.read(8)
        code_obj = marshal.load(f)
    exec(code_obj, globals())
except Exception: raise
"""

def decrypt_task(mcs_file_path, target_dir, pyc_name):
    """Worker task: Decrypts .mcs to .pyc with dynamic MD5 naming"""
    try:
        with open(mcs_file_path, 'rb') as f:
            data = f.read()
        pyc_data = restore_data(data)
        
        if pyc_name in CORE_SCRIPTS:
            # Calculate dynamic MD5 from content
            hex_name = hashlib.md5(pyc_data).hexdigest() + ".pyc"
            with open(os.path.join(target_dir, hex_name), 'wb') as f_out:
                f_out.write(pyc_data)
            
            # Generate wrapper pointing to this unique hash
            with open(os.path.join(target_dir, pyc_name.replace(".pyc", ".py")), 'w', encoding='utf-8') as f_wrap:
                f_wrap.write(WRAPPER_CONTENT_TEMPLATE.format(hex_name=hex_name))
        else:
            with open(os.path.join(target_dir, pyc_name), 'wb') as f_out:
                f_out.write(pyc_data)
    except Exception:
        pass

def process_behavior_pack(bp_path: str, max_workers: int = None):
    start_total = time.time()
    bp_path = os.path.abspath(bp_path)
    dev_mods_path = os.path.join(bp_path, "developer_mods")
    os.makedirs(dev_mods_path, exist_ok=True)

    # 1. Collect all MCPs
    mcp_files = []
    for root, dirs, files in os.walk(bp_path):
        if "developer_mods" in dirs: dirs.remove("developer_mods")
        for f in files:
            if f.lower().endswith(".mcp"): mcp_files.append(os.path.join(root, f))

    if not mcp_files: return

    # 2. Use a single ProcessPool for all tasks
    with ProcessPoolExecutor(max_workers=max_workers) as executor:
        for mcp_path in mcp_files:
            mcp_name = os.path.splitext(os.path.basename(mcp_path))[0]
            temp_unpack = os.path.join(bp_path, f"tmp_{mcp_name}")
            if os.path.exists(temp_unpack): shutil.rmtree(temp_unpack)
            os.makedirs(temp_unpack)

            try:
                # Unpack MCPK (Sequential, but usually fast)
                unpack_mcpk(mcp_path, temp_unpack)

                # Prepare decryption tasks
                for code_root in [d for d in os.listdir(temp_unpack) if os.path.isdir(os.path.join(temp_unpack, d))]:
                    source_root = os.path.join(temp_unpack, code_root)
                    target_root = os.path.join(dev_mods_path, code_root)
                    if os.path.exists(target_root): shutil.rmtree(target_root)
                    os.makedirs(target_root)

                    for r, _, files in os.walk(source_root):
                        rel = os.path.relpath(r, source_root)
                        t_dir = os.path.join(target_root, rel) if rel != "." else target_root
                        os.makedirs(t_dir, exist_ok=True)
                        for f in files:
                            if f.endswith(".mcs"):
                                executor.submit(decrypt_task, os.path.join(r, f), t_dir, f.replace(".mcs", ".pyc"))

                # 3. Quick cleanup of temp folder
                # (Optional: can be done at the very end to save time during processing)
                # shutil.rmtree(temp_unpack) 
            except Exception:
                pass

    # Final Cleanup
    for mcp_path in mcp_files:
        temp_unpack = os.path.join(bp_path, f"tmp_{os.path.splitext(os.path.basename(mcp_path))[0]}")
        if os.path.exists(temp_unpack): shutil.rmtree(temp_unpack, ignore_errors=True)

    print(f"[+] Total time: {time.time() - start_total:.2f}s")

if __name__ == "__main__":
    import sys
    if len(sys.argv) < 2:
        print("Usage: python auto_mcp_decryptor.py <folder>")
    else:
        process_behavior_pack(sys.argv[1])
