#!/usr/bin/env python3
"""Reproducible offline fingerprinting. Never import/execute target bytecode."""
from __future__ import annotations
import hashlib
import importlib.metadata
import io
import json
from pathlib import Path
import shutil
import subprocess
import sys
import zipfile

ASSETS=Path(__file__).resolve().parent.parent/'assets'
MODULES=('8bbfa2c7_d6367fb9.pyc','94b92985_d94c8a8c.pyc','94b92985_12e0e8fc.pyc')
METHODS={'OnLoginWebServer','get_rnbundle_md5','get_mcp_md5','get_android_libminecraftpe_so_md5'}


def sha(data):return hashlib.sha256(data).hexdigest()
def stable(value):return json.dumps(value,sort_keys=True,separators=(',',':'),ensure_ascii=True).encode()


def fingerprints(directory):
    from xdis.load import load_module
    def normalize(value):
        if hasattr(value,'co_code'):
            fields=('co_code','co_consts','co_names','co_varnames','co_freevars','co_cellvars',
                    'co_argcount','co_kwonlyargcount','co_posonlyargcount','co_flags',
                    'co_nlocals','co_stacksize','co_name','co_exceptiontable')
            return {f:normalize(getattr(value,f,None)) for f in fields}
        if isinstance(value,bytes):return {'bytes_hex':value.hex()}
        if isinstance(value,(tuple,list)):return [normalize(v) for v in value]
        if isinstance(value,(set,frozenset)):return {'set':sorted([normalize(v) for v in value],key=lambda v:stable(v))}
        if isinstance(value,dict):return {str(k):normalize(v) for k,v in value.items()}
        if value is None or isinstance(value,(str,int,float,bool)):return value
        # Fail rather than use repr(object), which may contain process addresses.
        raise ValueError('unsupported_static_constant_'+type(value).__name__)
    methods={};modules={}
    def walk(code):
        if getattr(code,'co_name',None) in METHODS:
            methods[code.co_name]=sha(stable(normalize(code)))
        for child in getattr(code,'co_consts',()):
            if hasattr(child,'co_code'):walk(child)
    for name in MODULES:
        loaded=load_module(str(Path(directory)/name))
        modules[name]=sha(stable(normalize(loaded[3])))
        walk(loaded[3])
    if set(methods)!=METHODS:raise ValueError('authentication_methods_missing')
    return dict(modules=modules,methods=methods,target_execution=False)


def verified_source(source,pin):
    return all((source/f['path']).is_file() and sha((source/f['path']).read_bytes())==f['sha256'] for f in pin['files'])


def ensure_inspector(root,session):
    pin=json.loads((ASSETS/'inspector-source.json').read_text())
    folder=root/'cache/maintenance/toolbox'/pin['commit'];source=folder/'source'
    if not verified_source(source,pin):
        cached=root/'cache/rea-loader-20261018/unmcpk-reference/source'
        if verified_source(cached,pin):
            contents={f['path']:(cached/f['path']).read_bytes() for f in pin['files']}
        else:
            response=session.get(f"https://codeload.github.com/yeah-god/unmcpk/zip/{pin['commit']}",timeout=60)
            response.raise_for_status()
            with zipfile.ZipFile(io.BytesIO(response.content)) as archive:
                prefix=f"unmcpk-{pin['commit']}/"
                contents={f['path']:archive.read(prefix+f['path']) for f in pin['files']}
            if any(sha(contents[f['path']])!=f['sha256'] for f in pin['files']):
                raise ValueError('inspector_source_digest_mismatch')
        for name,data in contents.items():
            target=source/name;target.parent.mkdir(parents=True,exist_ok=True);target.write_bytes(data)
    # Only audited source files and these two local adapters are built.
    for name,target in [('offline_select.go',source/'mcpk/offline_select.go'),('offline_main.go',source/'cmd/proloader-select/main.go')]:
        target.parent.mkdir(parents=True,exist_ok=True);target.write_bytes((ASSETS/name).read_bytes())
    allowed={f['path'] for f in pin['files']}|{'mcpk/offline_select.go','cmd/proloader-select/main.go'}
    if any(str(p.relative_to(source)) not in allowed for p in source.rglob('*.go')):
        raise ValueError('unexpected_inspector_source_file')
    if not shutil.which('go'):raise ValueError('go_toolchain_missing')
    revision=sha(stable(pin)+ (ASSETS/'offline_select.go').read_bytes()+(ASSETS/'offline_main.go').read_bytes())
    binary=folder/'select';stamp=folder/'build.json'
    if not binary.exists() or not stamp.exists() or json.loads(stamp.read_text()).get('revision')!=revision or json.loads(stamp.read_text()).get('binary_sha256')!=sha(binary.read_bytes()):
        result=subprocess.run(['go','build','-o',str(binary.resolve()),'./cmd/proloader-select'],cwd=source,capture_output=True,timeout=240)
        (folder/'build.log').write_bytes(result.stdout+result.stderr)
        if result.returncode:raise ValueError('inspector_build_failed_see_toolbox_build_log')
        stamp.write_text(json.dumps(dict(revision=revision,binary_sha256=sha(binary.read_bytes()))))
    return binary


def evidence(root,artifact,report_dir,session):
    binary=ensure_inspector(root,session)
    selected=report_dir/'selected-pyc'
    result=subprocess.run([str(binary),str(artifact.resolve()),str(selected.resolve())],capture_output=True,timeout=120)
    (report_dir/'decode.log').write_bytes(result.stdout+result.stderr)
    if result.returncode:raise ValueError('static_decoder_failed_see_decode_log')
    try:available=importlib.metadata.version('xdis')=='6.1.8'
    except importlib.metadata.PackageNotFoundError:available=False
    python=Path(sys.executable)
    if not available:
        env=root/'cache/maintenance/toolbox/xdis-6.1.8'
        python=env/'bin/python'
        if not python.exists():
            subprocess.run([sys.executable,'-m','venv',str(env)],check=True,capture_output=True,timeout=120)
        subprocess.run([str(python),'-m','pip','install','xdis==6.1.8'],check=True,capture_output=True,timeout=180)
    target=report_dir/'STATIC.json'
    result=subprocess.run([str(python),str(Path(__file__).resolve()),str(selected),str(target)],capture_output=True,timeout=120)
    (report_dir/'static.log').write_bytes(result.stdout+result.stderr)
    if result.returncode:raise ValueError('static_fingerprint_failed_see_static_log')
    return json.loads(target.read_text())


if __name__=='__main__':
    Path(sys.argv[2]).write_text(json.dumps(fingerprints(sys.argv[1]),indent=2))
