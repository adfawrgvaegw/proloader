#!/usr/bin/env python3
"""One-entry maintenance: fresh public metadata → proof → tests → 10 read-only rounds.

Stdout is one compact JSON result; bulk evidence and logs stay in cache. No world
writes, SMS, global network changes, GitHub publication, or target pyc execution.
"""
from __future__ import annotations
import argparse
import ast
from datetime import datetime,timezone
import fcntl
import hashlib
import io
import json
import logging
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import uuid
import zipfile

from mcp_evidence import evidence,sha,stable,ASSETS


def atomic(path,data):
    path=Path(path);path.parent.mkdir(parents=True,exist_ok=True)
    fd,tmp=tempfile.mkstemp(dir=path.parent,prefix='.maintenance-')
    try:
        os.fchmod(fd,0o600)
        with os.fdopen(fd,'wb') as stream:stream.write(data);stream.flush();os.fsync(stream.fileno())
        os.replace(tmp,path)
    finally:
        if os.path.exists(tmp):os.unlink(tmp)


def write_json(path,value):atomic(path,json.dumps(value,ensure_ascii=False,indent=2).encode())


def profile(path):
    values={}
    for node in ast.parse(path.read_text()).body:
        if isinstance(node,ast.Assign):
            for target in node.targets:
                if isinstance(target,ast.Name) and target.id in ('OFFICIAL_ENGINE','OFFICIAL_PATCHES'):
                    values[target.id]=ast.literal_eval(node.value)
    return values['OFFICIAL_ENGINE'],values['OFFICIAL_PATCHES']


def discover(root,folder):
    from proloader.net.netease.client import NeteaseClient,PACK_LIST_URL,PATCH_LIST_URL
    client=NeteaseClient(pe_auth_profile='official-android-3915')
    records=[]
    def observe(response,*a,**kw):
        index=len(records);body=response.content;file=f'public-{index}.bin'
        (folder/file).write_bytes(body)
        records.append(dict(url=response.url,status=response.status_code,file=file,sha256=sha(body),
                            last_modified=response.headers.get('Last-Modified')))
        return response
    client._session.hooks['response']=[observe]
    try:
        client.bootstrap(engine_source='official-download')
        response=client._session.get(PACK_LIST_URL,timeout=15);response.raise_for_status();pack=response.json()
        response=client._session.get(PATCH_LIST_URL,timeout=15);response.raise_for_status();patches=response.json()
        manifest_record=next(r for r in records if r['url'].endswith('/manifest.zip') and r['status']==200)
        with zipfile.ZipFile(io.BytesIO((folder/manifest_record['file']).read_bytes())) as z:
            manifest=json.loads(z.read(z.namelist()[0]))
        assets=manifest['assets'];mcp=assets['vanilla.mcp']
        base=manifest_record['url'].rsplit('/',1)[0]
        result=dict(engine=client.engine_version,patch=client.g79_latest_version,
            resources_hash=client.patch_resources_hash,rn_present=client.rn_bundle_present,
            mcp=dict(url=base+'/'+mcp.get('pathUrl','vanilla.mcp'),md5=mcp['md5'],bytes=mcp['size']),
            overlay='vanilla_patch.mcp' in assets,
            native_assets={n:v.get('md5') for n,v in assets.items() if n.endswith(('.so','.dll','.dylib'))},
            gray=dict(engine=pack.get('netease',{}).get('version'),ratio=pack.get('netease',{}).get('gray_ratio')),
            latest_android=patches.get('android',[])[-3:],requests=records)
        write_json(folder/'METADATA.json',result)
        return result
    finally:client._session.close()


def download(root,metadata,session):
    item=metadata['mcp']
    if not re.fullmatch('[a-fA-F0-9]{32}',item['md5']) or not 0<item['bytes']<=128*1024**2:
        raise ValueError('invalid_official_artifact_metadata')
    # Even cache hits are checked against the freshly fetched official manifest.
    path=root/'cache/maintenance/artifacts'/(item['md5']+'.mcp')
    if path.is_file():
        body=path.read_bytes()
        if len(body)==item['bytes'] and hashlib.md5(body).hexdigest()==item['md5']:return path
    response=session.get(item['url'],timeout=60);response.raise_for_status();body=response.content
    if len(body)!=item['bytes'] or hashlib.md5(body).hexdigest()!=item['md5']:
        raise ValueError('official_artifact_digest_mismatch')
    atomic(path,body)
    return path


def decide(metadata,baseline,proof):
    if metadata['engine']!=baseline['engine']:return 'engine_changed'
    if metadata['rn_present']!=baseline['rn_present']:return 'rn_changed'
    if metadata.get('overlay'):return 'patch_overlay_requires_analysis'
    if metadata.get('native_assets'):return 'native_assets_require_analysis'
    if proof.get('target_execution') is not False:return 'invalid_static_proof'
    if proof.get('modules')!=baseline['modules']:return 'selected_module_code_changed'
    if proof.get('methods')!=baseline['methods']:return 'authentication_method_changed'
    return 'same_engine_static_proof_passed'


def candidate_source(before,metadata):
    # Require the expected set assignment and append a literal exact pair.
    tree=ast.parse(before.decode());node=next(n for n in tree.body if isinstance(n,ast.Assign)
        and any(isinstance(t,ast.Name) and t.id=='OFFICIAL_PATCHES' for t in n.targets))
    if not isinstance(node.value,ast.Set):raise ValueError('profile_set_layout_changed')
    pair=(metadata['patch'],metadata['resources_hash'])
    if not re.fullmatch(r'\d+\.\d+\.\d+\.\d+',pair[0]) or not re.fullmatch('[0-9a-f]{32}',pair[1]):
        raise ValueError('invalid_candidate_pair')
    if pair in ast.literal_eval(node.value):return before
    lines=before.decode().splitlines(keepends=True)
    row=node.value.end_lineno-1;column=node.value.end_col_offset-1
    lines[row]=lines[row][:column]+f"    {pair!r},\n"+lines[row][column:]
    after=''.join(lines).encode();ast.parse(after)
    return after


def idle(root):
    from proloader.bots import BotStore
    store=BotStore()
    return not any(store.is_busy(b) for b in store.list())


def tests(root,folder):
    result=subprocess.run([sys.executable,'-m','pytest','-q'],cwd=root,capture_output=True,timeout=600)
    (folder/'tests.log').write_bytes(result.stdout+result.stderr)
    return result.returncode==0


def network_digest(root):
    # Compare the identical file set and path convention as the online report.
    from tools.diagnostics.verify_rental_stability import source_digest
    return source_digest(root)


def acceptance(root,folder,settings,password):
    from proloader.bots import BotStore
    store=BotStore();bot=next(b for b in store.list() if b.id==settings['bot'])
    old=store.pe(bot)
    if old:write_json(folder/'PE-before-private.json',old)
    result=subprocess.run([sys.executable,'-m','tools.diagnostics.verify_rental_stability',
        '--bot',settings['bot'],'--server',settings['server'],'--rounds','10','--hold','30',
        '--interval','5','--password-stdin','--output',str(folder/'ONLINE.json')],
        cwd=root,input=(password+'\n').encode(),capture_output=True,timeout=1800)
    atomic(folder/'online.log',result.stdout+result.stderr)
    if result.returncode or not (folder/'ONLINE.json').exists():return False
    report=json.loads((folder/'ONLINE.json').read_text())
    rounds=report.get('rounds',[])
    return (report.get('passed') is True and len(rounds)==10 and all(r.get('passed') for r in rounds)
            and report.get('source_sha256')==network_digest(root))


def configure(root,args):
    from proloader.bots import BotStore
    if not args.bot or not args.server or not re.fullmatch(r'\d{5,12}',args.server):
        raise ValueError('configure_requires_bot_and_server')
    if args.bot not in {b.id for b in BotStore().list()}:raise ValueError('unknown_local_bot')
    settings=dict(bot=args.bot,server=args.server,password_source='empty' if args.no_password else 'stdin')
    write_json(root/'cache/maintenance/settings.json',settings)
    return dict(status='configured',password_saved=False)


def operate(root,args,folder):
    metadata=discover(root,folder)
    path=root/'proloader/net/netease/pe_profiles.py'
    engine,patches=profile(path)
    known=metadata['engine']==engine and (metadata['patch'],metadata['resources_hash']) in patches
    result=dict(status='current' if known else 'update_detected',engine=metadata['engine'],
                patch=metadata['patch'],gray=metadata['gray'],source_changed=False)
    if known and not args.audit:return result
    if args.check_only:return result
    baseline=json.loads((ASSETS/'auth-baseline.json').read_text())
    import requests
    with requests.Session() as session:
        artifact=download(root,metadata,session)
        result['artifact_sha256']=sha(artifact.read_bytes())
        if metadata['engine']!=baseline['engine']:
            result.update(status='needs_analysis',reason='engine_changed',next_files=[
                'proloader/net/netease/constants.py','proloader/net/netease/client.py',
                'proloader/net/netease/pe_profiles.py','proloader/net/bedrock/session.py'])
            return result
        proof=evidence(root,artifact,folder,session)
    write_json(folder/'STATIC.json',proof)
    reason=decide(metadata,baseline,proof)
    if reason!='same_engine_static_proof_passed':
        changed=[n for n in baseline['modules'] if proof.get('modules',{}).get(n)!=baseline['modules'][n]]
        result.update(status='needs_analysis',reason=reason,changed_modules=changed,
                      next_files=['proloader/net/netease/client.py','proloader/net/netease/pe_profiles.py'])
        return result
    result['proof']=reason
    if known:return dict(result,status='current_verified')
    before=path.read_bytes();after=candidate_source(before,metadata)
    atomic(folder/'candidate-pe_profiles.py',after)
    if not args.online:return dict(result,status='ready_to_apply',next_command='rerun --online for apply, tests, and 10-round acceptance')
    settings_path=root/'cache/maintenance/settings.json'
    if not settings_path.exists():return dict(result,status='needs_configuration',next_command='--configure --bot BOT_ID --server SERVER_ID [--no-password]')
    settings=json.loads(settings_path.read_text())
    if not idle(root):return dict(result,status='busy',reason='bot_in_use_source_not_changed')
    if settings['password_source']=='empty':password=''
    elif args.password_stdin:password=sys.stdin.readline().rstrip('\r\n')
    else:return dict(result,status='needs_password',next_command='rerun --online --password-stdin; never pass a password argument')
    atomic(folder/'pe_profiles.before.py',before)
    if path.read_bytes()!=before:raise ValueError('source_changed_concurrently')
    atomic(path,after)
    passed=False
    try:
        result['source_changed']=True
        if not tests(root,folder):
            result.update(status='rolled_back',reason='offline_tests_failed');return result
        result['offline_tests_passed']=True
        if not acceptance(root,folder,settings,password):
            result.update(status='rolled_back',reason='online_acceptance_failed');return result
        passed=True
        return dict(result,status='accepted',online_rounds=10,source_sha256=sha(after))
    finally:
        password=''
        if not passed:
            if path.read_bytes()==after:atomic(path,before);result['source_changed']=False
            else:raise ValueError('concurrent_change_prevents_rollback_see_source_backup')


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root',type=Path)
    parser.add_argument('--audit',action='store_true',help='Also verify the current MCP static fingerprints')
    parser.add_argument('--online',action='store_true',help='Apply proven updates, test, and run 10 read-only connection rounds')
    parser.add_argument('--check-only',action='store_true',help='Metadata only; never edit runtime source')
    parser.add_argument('--configure',action='store_true')
    parser.add_argument('--bot');parser.add_argument('--server')
    parser.add_argument('--no-password',action='store_true');parser.add_argument('--password-stdin',action='store_true')
    args=parser.parse_args()
    root=args.root.resolve() if args.root else next((p for p in [Path.cwd(),*Path(__file__).resolve().parents]
                 if (p/'pyproject.toml').exists() and (p/'proloader').is_dir()),None)
    if root is None:parser.error('run inside a Proloader source checkout or provide --root')
    sys.path.insert(0,str(root));logging.disable(logging.CRITICAL)
    base=root/'cache/maintenance';base.mkdir(parents=True,exist_ok=True)
    folder=base/'runs'/(datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')+'-'+uuid.uuid4().hex[:6]);folder.mkdir(parents=True)
    with (base/'maintenance.lock').open('a') as lock:
        try:
            fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
            result=configure(root,args) if args.configure else operate(root,args,folder)
        except BlockingIOError:result=dict(status='busy',reason='another_maintenance_run')
        except Exception as exc:
            # Exception messages and arbitrary response bodies may contain secrets.
            result=dict(status='error',error_type=type(exc).__name__)
            if isinstance(exc,ModuleNotFoundError):
                result.update(missing_module=exc.name,next_command="create project venv, install -e '.[dev]', then rerun with that Python")
            if isinstance(exc,ValueError) and re.fullmatch('[a-z0-9_]+',str(exc)):result['reason']=str(exc)
        result['report']=str(folder.relative_to(root)/'REPORT.json')
        write_json(folder/'REPORT.json',result)
        write_json(base/'LATEST.json',result)
        next_note='''Read REPORT.json first, then only the changed modules/files listed there.
Do not re-read the full handoff or dump manifests/source. Never auto-accept a new
engine or changed authentication modules. Preserve private identities. If analysis
is required, use references/version-updates.md and references/automation.md.
'''
        atomic(folder/'NEXT.md',next_note.encode())
        print(json.dumps(result,ensure_ascii=False,separators=(',',':')))
    return 0 if result['status'] in ('current','current_verified','accepted','configured') else 2


if __name__=='__main__':raise SystemExit(main())
