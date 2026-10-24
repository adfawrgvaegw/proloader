"""使用已有 cookie 单次诊断 PE；默认离线，--authenticate 才请求线上。输出含秘密，仅存本机。"""
from __future__ import annotations
import argparse
import json
import logging
import os
from pathlib import Path
from proloader.net.netease.client import NeteaseClient
from .netease_auth_result import summarize_response
from proloader.net.netease.crypto import g79_http_decrypt, get_valid_json


def diagnose(cookie: Path, output: Path, sauth_profile: str = "original", engine_source: str = "pack-list", auth_profile: str = "reference") -> dict:
    if output.exists():
        return dict(ok=False, error='output_exists')
    client=NeteaseClient() if auth_profile == "reference" else NeteaseClient(pe_auth_profile=auth_profile)
    stage='cookie'
    captured={}
    record={}
    report={}
    def capture(resp, *args, **kwargs):
        if resp.request.url.endswith('/pe-authentication'):
            captured.update(json.loads(get_valid_json(g79_http_decrypt(bytes.fromhex(resp.text)))))
    client._session.hooks['response'].append(capture)
    try:
        cookie_text=cookie.read_text()
        if sauth_profile not in ('original', 'reference-android'):
            raise ValueError('unknown sauth profile')
        record['sauth_profile']=sauth_profile
        record['auth_profile']=auth_profile
        if sauth_profile == 'reference-android':
            # g79client/auth.go buildAndroidSauthPayload explicitly uses ad/ad.
            outer=json.loads(cookie_text)
            sauth=json.loads(outer['sauth_json'])
            sauth.update(platform='ad',source_platform='ad')
            outer['sauth_json']=json.dumps(sauth,separators=(',',':'))
            cookie_text=json.dumps(outer,separators=(',',':'))
        stage='bootstrap'
        if engine_source == 'pack-list':
            client.bootstrap()
        else:
            client.bootstrap(engine_source=engine_source)
        record['engine_source']=engine_source
        record['metadata']=dict(engine_version=client.engine_version,patch_version=client.g79_latest_version,resources_hash=client.patch_resources_hash,core_server=client.core_server_url)
        stage='PE'
        client.authenticate_with_cookie(cookie_text)
        record['credentials']=dict(user_id=client.user_id,user_token=client.user_token,seed=client.seed,user_detail=client.user_detail)
        report=dict(ok=True,stage=stage)
    except Exception as exc:
        report=dict(ok=False,stage=stage,error=type(exc).__name__)
        if captured:
            report.update(summarize_response(captured))
    finally:
        client._session.close()
    record.update(report)
    record['response']=captured
    try:
        with os.fdopen(os.open(output,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600),'w') as stream:
            json.dump(record,stream,ensure_ascii=False)
    except OSError as exc:
        return dict(ok=False,stage='save',error=type(exc).__name__)
    return dict(report,saved=str(output))


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--cookie',type=Path,required=True)
    p.add_argument('--output',type=Path,required=True)
    p.add_argument('--authenticate',action='store_true')
    p.add_argument('--sauth-profile',choices=('original','reference-android'),default='original',help='显式对照归档 Go 的 ad/ad 平台；不修改原 cookie')
    p.add_argument('--engine-source',choices=('pack-list','official-download'),default='pack-list')
    p.add_argument('--auth-profile',choices=('reference','official-android-3915'),default='reference',help='显式样本对照；严格绑定已检查的引擎、补丁与资源摘要')
    args=p.parse_args()
    logging.disable(logging.CRITICAL)
    if not args.authenticate:
        print(json.dumps(dict(mode='offline',plan='one PE attempt with existing cookie')))
        return 0
    result=diagnose(args.cookie,args.output,args.sauth_profile,args.engine_source,args.auth_profile)
    print(json.dumps(result,ensure_ascii=False))
    return 0 if result['ok'] else 1

if __name__=='__main__':
    raise SystemExit(main())
