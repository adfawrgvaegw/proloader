#!/usr/bin/env python3
"""Export a clean source tree without deleting/moving the owner's private data."""
from __future__ import annotations
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import tempfile

ROOT_FILES={'README.md','LICENSE','LICENSE-MIT','THIRD_PARTY_NOTICES.md','pyproject.toml',
            'proloader启动器.command','proloader维护.command','MAINTAINERS.md','.gitignore','SECURITY.md','RELEASE_REVIEW.md','CHANGELOG.md'}
SOURCE_DIRS={'proloader','tests','examples','third_party','tools','templates'}
SKILL=('.agents','skills','proloader-maintenance')
DENIED_DIRS={'cache','bin','build','dist','.git','.venv','venv','__pycache__','.pytest_cache',
             '.proloader','bots','imports','node_modules','proloader_sessions'}
DENIED_SUFFIXES={'.pyc','.pyo','.log','.pcap','.pcapng','.db','.sqlite','.sqlite3','.token',
                 '.pem','.key','.exe','.dll','.dylib','.so'}
DENIED_NAMES={'cookie.json','pe.json','credentials.json','.env','profile.json','.lease'}


def allowed(relative):
    parts=relative.parts
    if not parts or any(part in DENIED_DIRS for part in parts):return False
    if relative.suffix.lower() in DENIED_SUFFIXES or relative.name in DENIED_NAMES:return False
    if len(parts)==1:return relative.name in ROOT_FILES
    if relative.as_posix()=='.github/workflows/ci.yml':return True
    if parts[0]=='docs':return relative.as_posix() in {'docs/AI_CLI.md','docs/SNOW_MENU_INSTALL.md'}
    if relative.as_posix()=='tools/gophertunnel_test/gophertunnel_test':return False
    return parts[0] in SOURCE_DIRS or parts[:3]==SKILL


def secret_sentinels(private_root):
    """Read only local identity fields; never print the values or copy their files."""
    values=set()
    for directory in (private_root/'bots').glob('*'):
        for name in ('cookie.json','pe.json'):
            path=directory/name
            if not path.is_file() or path.is_symlink():continue
            try:data=json.loads(path.read_text())
            except (OSError,ValueError):continue
            def visit(value):
                if isinstance(value,dict):
                    for key,item in value.items():
                        if key=='sauth_json' and isinstance(item,str):
                            try:visit(json.loads(item))
                            except ValueError:pass
                        if key in ('sessionid','access_token','user_token','refresh_token') and isinstance(item,str) and len(item)>=12:
                            values.add(item.encode())
                        visit(item)
                elif isinstance(value,list):
                    for item in value:visit(item)
            visit(data)
    # A developer's literal home directory does not belong in public source/docs.
    home=str(Path.home())
    if len(home)>6:values.add(home.encode())
    return values


def export(source,destination,private_root=None):
    source=Path(source).resolve()
    destination=Path(destination).absolute()
    private_root=Path(private_root).resolve() if private_root else Path.home()/'.proloader'
    resolved=destination.resolve()
    if not source.is_dir():raise ValueError('Source directory does not exist')
    if destination.exists() or destination.is_symlink():raise ValueError('Destination must not exist')
    if resolved==source or source in resolved.parents:raise ValueError('Destination must be outside the working tree')
    if resolved==private_root or private_root in resolved.parents:raise ValueError('Destination must not be inside private data')
    destination.parent.mkdir(parents=True,exist_ok=True)
    sentinels=secret_sentinels(private_root)
    staging=Path(tempfile.mkdtemp(prefix='.proloader-export-',dir=destination.parent))
    manifest={}
    try:
        for path in sorted(source.rglob('*')):
            relative=path.relative_to(source)
            if not allowed(relative):continue
            if path.is_symlink():raise ValueError(f'Symlink requires manual review: {relative}')
            if not path.is_file():continue
            data=path.read_bytes()
            if any(value in data for value in sentinels):
                raise ValueError(f'Private material or owner path detected; inspect locally: {relative}')
            target=staging/relative
            target.parent.mkdir(parents=True,exist_ok=True)
            shutil.copy2(path,target)
            manifest[relative.as_posix()]=hashlib.sha256(data).hexdigest()
        if not manifest:raise ValueError('No allowed source files found')
        (staging/'RELEASE_MANIFEST.json').write_text(json.dumps(dict(files=manifest),ensure_ascii=False,indent=2))
        # Same filesystem, new destination only. Never writes to source/private roots.
        staging.rename(destination)
    except BaseException:
        shutil.rmtree(staging,ignore_errors=True)
        raise
    return len(manifest)


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source',type=Path,required=True)
    parser.add_argument('--destination',type=Path,required=True)
    parser.add_argument('--private-root',type=Path,help='Read-only identity directory, defaults to ~/.proloader')
    args=parser.parse_args()
    try:count=export(args.source,args.destination,args.private_root)
    except (ValueError,OSError) as exc:
        parser.exit(1,str(exc)+'\n')
    print(f'Exported {count} source files to the new release directory; local private data untouched.')


if __name__=='__main__':main()
