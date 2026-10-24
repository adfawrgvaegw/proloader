"""Dependency checks without importing optional runtime packages."""
import importlib.util
from pathlib import Path
import json
import shutil

def emit(value):
    print(json.dumps(value,ensure_ascii=False),flush=True)

def doctor(as_json=False):
    root=Path(__file__).resolve().parents[2]
    checks={name:importlib.util.find_spec(name) is not None for name in ('numpy','rich','requests','msgpack','cryptography')}
    checks['native_source']=(root/'third_party/fatalder/cmd/proloader-plan/main.go').exists()
    checks['native_binary_or_go']=(root/'bin/fatalder-plan').exists() or shutil.which('go') is not None
    if as_json:emit(dict(ok=all(checks.values()),checks=checks))
    else:
        for name,ok in checks.items():print(f"{'通过' if ok else '缺少'}  {name}")
        print('仅检查本机环境，未登录或请求短信。')
    return 0 if all(checks.values()) else 1

