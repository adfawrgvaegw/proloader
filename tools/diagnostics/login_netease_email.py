"""无需游戏客户端的网易邮箱登录入口；终端输入密码，cookie 保存到指定新文件。

python3 -m tools.diagnostics.login_netease_email --output /absolute/path/cookie.json
需要本机 Go 与 cache/g79client 源码。首次运行可能下载 Go 依赖。
"""
from __future__ import annotations

import argparse
import getpass
import json
import os
from pathlib import Path
import shutil
import subprocess

BRIDGE = Path(__file__).resolve().parents[1] / 'mpay_login'


def login_and_save(email: str, password: str, output: Path) -> dict:
    if not email.strip() or not password:
        return {'ok': False, 'error': 'empty_credentials'}
    if output.exists():
        return {'ok': False, 'error': 'output_exists'}
    go = shutil.which('go')
    if go is None:
        return {'ok': False, 'error': 'go_not_found'}
    try:
        # 密码仅通过 stdin 管道传递，不进入进程参数或环境变量。
        proc = subprocess.run([go, 'run', '-mod=mod', '.'], cwd=BRIDGE,
                              input=json.dumps({'email': email.strip(), 'password': password}),
                              text=True, capture_output=True, timeout=180)
        if proc.returncode:
            return {'ok': False, 'error': 'bridge_failed'}
        result = json.loads(proc.stdout)
        if not result.get('ok'):
            return {'ok': False, 'error': result.get('error', 'mpay_login_failed'),
                    'code': result.get('code')}
        cookie = result['cookie']
        if not isinstance(json.loads(cookie).get('sauth_json'), str):
            return {'ok': False, 'error': 'invalid_cookie'}
        with os.fdopen(os.open(output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), 'w') as stream:
            stream.write(cookie)
        return {'ok': True, 'saved': str(output)}
    except (OSError, ValueError, KeyError, TypeError, subprocess.TimeoutExpired) as exc:
        return {'ok': False, 'error': type(exc).__name__}


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--output', type=Path, required=True, help='新的 cookie 文件；不会覆盖已有文件')
    args = p.parse_args()
    if args.output.exists():
        print(json.dumps({'ok': False, 'error': 'output_exists'}))
        return 1
    email = input('网易邮箱：')
    password = getpass.getpass('密码（隐藏输入）：')
    result = login_and_save(email, password, args.output)
    print(json.dumps(result, ensure_ascii=False))
    return 0 if result['ok'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
