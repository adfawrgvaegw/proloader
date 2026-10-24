"""原始 PC cookie 的单次 login-otp 对照；不执行 authentication-otp 或游戏操作。"""
import argparse
import json
import logging
import os
from pathlib import Path
from urllib.parse import urlsplit
import requests
from .netease_auth_result import summarize_response

RELEASE='https://x19.update.netease.com/serverlist/release.json'


def diagnose(cookie: Path, output: Path) -> dict:
    if output.exists():
        return dict(ok=False,error='output_exists')
    stage='cookie'
    record={}
    session=requests.Session()
    try:
        raw=cookie.read_text()
        sauth=json.loads(json.loads(raw)['sauth_json'])
        if sauth.get('platform')!='pc':
            raise ValueError('requires original PC cookie')
        stage='release'
        resp=session.get(RELEASE,timeout=15);resp.raise_for_status()
        origin=resp.json()['CoreServerUrl']
        parsed=urlsplit(origin)
        if parsed.scheme!='https' or not parsed.hostname or not (parsed.hostname.endswith('.minecraft.cn') or (parsed.hostname == 'x19obtcore.nie.netease.com' and parsed.port == 8443)) or parsed.username or parsed.password or parsed.path not in ('','/') or parsed.query or parsed.fragment:
            raise ValueError('unexpected official auth origin')
        record['origin']=origin
        stage='login-otp'
        resp=session.post(origin.rstrip('/')+'/login-otp',data=raw.encode(),headers={'User-Agent':'libhttpclient/1.0.0.0','Accept-Encoding':'gzip','Content-Type':'application/json'},timeout=15,allow_redirects=False)
        record['http_status']=resp.status_code
        resp.raise_for_status()
        payload=resp.json()
        record['response']=payload
        entity=payload.get('entity') or {}
        success=payload.get('code')==0 and isinstance(entity.get('otp_token'),str) and bool(entity['otp_token'])
        record.update(ok=success,stage=stage)
        report=dict(ok=success,stage=stage,http_status=resp.status_code,otp_token_present=bool(entity.get('otp_token')),**summarize_response(payload))
    except Exception as exc:
        report=dict(ok=False,stage=stage,error=type(exc).__name__)
        record.update(report)
    finally:
        session.close()
    try:
        with os.fdopen(os.open(output,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600),'w') as f:
            json.dump(record,f,ensure_ascii=False)
    except OSError as exc:
        return dict(ok=False,stage='save',error=type(exc).__name__)
    return dict(report,saved=str(output))


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--cookie',type=Path,required=True)
    p.add_argument('--output',type=Path,required=True)
    p.add_argument('--authenticate',action='store_true')
    args=p.parse_args()
    logging.disable(logging.CRITICAL)
    if not args.authenticate:
        print(json.dumps(dict(mode='offline',plan='one login-otp request; no authentication-otp')))
        return 0
    result=diagnose(args.cookie,args.output)
    print(json.dumps(result,ensure_ascii=False))
    return 0 if result['ok'] else 1

if __name__=='__main__':
    raise SystemExit(main())
