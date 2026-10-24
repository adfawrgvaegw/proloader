from __future__ import annotations
import os
import sys
import subprocess
import uncompyle6
import contextlib
import io

from concurrent.futures import ProcessPoolExecutor

from log import logging, logger
from anti_confuser import restore_data

def file_handler(input_path: str, output_path: str) -> None:
    if not os.path.exists(input_path) or not input_path.endswith(".mcs"):
        return
    if output_path is None:
        output_path = input_path.replace(".mcs", ".pyc")
    
    try:
        log_path = output_path.replace(".pyc", "_log.txt")
        with open(log_path, "w", encoding="utf-8") as f:
            with contextlib.redirect_stdout(f), contextlib.redirect_stderr(f):
                open(output_path, 'wb').write(
                    restore_data(open(input_path, 'rb').read()))
        
        try:
            subprocess.run(['pycdas', output_path, '-o', output_path.replace(".pyc", "_asm.txt")], 
                        check=True,
                        stdout=subprocess.DEVNULL, 
                        stderr=subprocess.DEVNULL)
        except Exception as e:
            pass
        
        py_path = output_path.replace(".pyc", ".py")
        
        with open(py_path, "w", encoding="utf-8") as f:
            try:
                with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
                    uncompyle6.decompile_file(output_path, f)
            except Exception as e:
                f.write(f"\n# Uncompyle6 Error: {str(e)}\n")
        
        logger.info(f"Processed: {input_path}")
    except Exception as e:
        logger.error(f"Error processing {input_path}: {e}")

def main():
    if len(sys.argv) < 2:
        logger.error("Usage: python batch_process.py <input_folder> [output_folder]")
        return
        
    input_file = os.path.abspath(sys.argv[1])
    output_folder = os.path.abspath(sys.argv[2]) if len(sys.argv) > 2 else None
    
    max_workers = 8

    futures = []
    with ProcessPoolExecutor(max_workers=max_workers) as pool:
        if os.path.isdir(input_file):
            for root, dirs, files in os.walk(input_file):
                for filename in files:
                    if not filename.endswith(".mcs"):
                        continue
                        
                    file_path = os.path.join(root, filename)
                    relative_path = os.path.relpath(file_path, input_file)
                    
                    if output_folder:
                        out_dir = os.path.join(output_folder, os.path.dirname(relative_path))
                        os.makedirs(out_dir, exist_ok=True)
                        out_path = os.path.join(out_dir, filename.replace(".mcs", ".pyc"))
                    else:
                        out_path = file_path.replace(".mcs", ".pyc")
                    
                    futures.append(pool.submit(file_handler, file_path, out_path))
        else:
            futures.append(pool.submit(file_handler, input_file, output_folder))

        # Wait for all futures with a global timeout or per-item monitoring
        from concurrent.futures import as_completed
        for future in as_completed(futures):
            try:
                future.result(timeout=60)
            except Exception as e:
                logger.error(f"Task failed or timed out: {e}")
                future.cancel()

if __name__ == "__main__":
    logger.setLevel(logging.INFO)
    main()