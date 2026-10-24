"""单个 MPay 游客实验。默认只显示计划，--create 才创建一个身份。"""
from __future__ import annotations
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
from .login_netease_email import BRIDGE


def create_and_save(output: Path, diagnostic: Path = None, resume: Path = None) -> dict:
    if output.exists():
        return {'ok': False, 'error': 'output_exists'}
    if diagnostic is not None and (diagnostic.exists() or diagnostic.absolute() == output.absolute()):
        return {'ok': False, 'error': 'diagnostic_exists_or_conflicts'}
    go = shutil.which('go')
    if not go:
        return {'ok': False, 'error': 'go_not_found'}
    try:
        request = dict(mode='guest', **({'diagnostic': True} if diagnostic is not None else {}))
        if resume is not None:
            saved = json.loads(resume.read_text())
            recovery = saved.get('recovery')
            if not isinstance(recovery, dict) or not recovery.get('id') or not recovery.get('key'):
                return {'ok': False, 'error': 'invalid_recovery'}
            request['recovery'] = recovery
        proc = subprocess.run([go, 'run', '-mod=mod', '.'], cwd=BRIDGE,
                              input=json.dumps(request), text=True,
                              capture_output=True, timeout=180)
        if proc.returncode:
            return {'ok': False, 'error': 'bridge_failed'}
        result = json.loads(proc.stdout)
        report = {k: result[k] for k in ('ok', 'error', 'stage', 'http_status', 'code') if k in result}
        if not result.get('ok'):
            if diagnostic is not None:
                with os.fdopen(os.open(diagnostic, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), 'w') as stream:
                    json.dump(result, stream, ensure_ascii=False)
                report['diagnostic_saved'] = str(diagnostic)
            return report
        cookie = result['cookie']
        sauth = json.loads(json.loads(cookie)['sauth_json'])
        if not all(isinstance(sauth.get(k), str) and sauth[k] for k in ('deviceid', 'sdkuid', 'sessionid')) or sauth.get('is_unisdk_guest') != 1:
            return {'ok': False, 'error': 'invalid_guest_cookie'}
        with os.fdopen(os.open(output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), 'w') as stream:
            stream.write(cookie)
        return dict(report, saved=str(output), platform=sauth.get('platform'), guest=True)
    except (OSError, ValueError, KeyError, TypeError, subprocess.TimeoutExpired) as exc:
        return {'ok': False, 'error': type(exc).__name__}


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--create', action='store_true')
    p.add_argument('--output', type=Path)
    p.add_argument('--diagnostic', type=Path, help='失败诊断及设备状态，仅存本机 0600 新文件')
    p.add_argument('--resume', type=Path, help='安全验证完成后复用诊断文件中的同一设备；仍需 --create')
    args = p.parse_args()
    if not args.create:
        print(json.dumps({'mode': 'offline', 'plan': 'one guest; no identity created'}))
        return 0
    if args.output is None:
        p.error('--create requires --output')
    result = create_and_save(args.output, args.diagnostic, args.resume)
    print(json.dumps(result, ensure_ascii=False))
    return 0 if result['ok'] else 1

if __name__ == '__main__':
    raise SystemExit(main())
