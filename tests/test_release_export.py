import importlib.util
from pathlib import Path
import json
import pytest

SCRIPT=Path(__file__).resolve().parents[1]/'.agents/skills/proloader-maintenance/scripts/export_release.py'
spec=importlib.util.spec_from_file_location('release_export',SCRIPT)
release=importlib.util.module_from_spec(spec);spec.loader.exec_module(release)


def fixture(tmp_path):
    source=tmp_path/'workspace';source.mkdir()
    (source/'README.md').write_text('public instructions')
    (source/'proloader').mkdir();(source/'proloader/main.py').write_text('print("hello")')
    (source/'cache').mkdir();(source/'cache/cookie.json').write_text('private cache')
    (source/'AGENTS.md').write_text('personal memory')
    (source/'buildings').mkdir();(source/'buildings/private.schem').write_bytes(b'private design')
    private=tmp_path/'private';bot=private/'bots'/'bot';bot.mkdir(parents=True)
    (bot/'pe.json').write_text(json.dumps(dict(credentials=dict(user_token='REAL-SECRET-FOR-TEST-ONLY'))))
    return source,private


def test_export_preserves_private_files_and_excludes_them(tmp_path):
    source,private=fixture(tmp_path)
    before={p:p.read_bytes() for base in (source,private) for p in base.rglob('*') if p.is_file()}
    destination=tmp_path/'release'
    assert release.export(source,destination,private)==2
    assert (destination/'proloader/main.py').exists()
    assert not (destination/'cache').exists()
    assert not (destination/'AGENTS.md').exists()
    assert not (destination/'buildings').exists()
    assert all(path.read_bytes()==content for path,content in before.items())


def test_embedded_current_token_blocks_export_without_printing_token(tmp_path):
    source,private=fixture(tmp_path)
    (source/'proloader/main.py').write_text('token="REAL-SECRET-FOR-TEST-ONLY"')
    destination=tmp_path/'release'
    with pytest.raises(ValueError) as exc:release.export(source,destination,private)
    assert 'REAL-SECRET' not in str(exc.value)
    assert not destination.exists()
    assert (private/'bots/bot/pe.json').exists()


def test_existing_or_private_destinations_and_symlinks_rejected(tmp_path):
    source,private=fixture(tmp_path)
    with pytest.raises(ValueError):release.export(source,source/'release',private)
    with pytest.raises(ValueError):release.export(source,private/'release',private)
    existing=tmp_path/'existing';existing.mkdir();(existing/'keep').write_text('keep')
    with pytest.raises(ValueError):release.export(source,existing,private)
    assert (existing/'keep').read_text()=='keep'
    (source/'proloader/leak.py').symlink_to(private/'bots/bot/pe.json')
    with pytest.raises(ValueError,match='Symlink'):release.export(source,tmp_path/'release',private)
