import json
import stat
from subprocess import CompletedProcess
from unittest.mock import Mock
from tools.diagnostics import login_netease_guest as login


def test_guest_cookie_private_and_single_request(tmp_path, monkeypatch):
    cookie = json.dumps({'sauth_json': json.dumps(dict(deviceid='D', sdkuid='U', sessionid='SECRET', is_unisdk_guest=1, platform='pc'))})
    run = Mock(return_value=CompletedProcess([], 0, json.dumps(dict(ok=True, cookie=cookie, stage='guest', http_status=201)), ''))
    monkeypatch.setattr(login.shutil, 'which', lambda _: '/go')
    monkeypatch.setattr(login.subprocess, 'run', run)
    path = tmp_path / 'cookie.json'
    result = login.create_and_save(path)
    assert result['ok'] and 'SECRET' not in repr(result)
    assert stat.S_IMODE(path.stat().st_mode) == 0o600
    assert json.loads(run.call_args.kwargs['input']) == {'mode': 'guest'}
    assert login.create_and_save(path)['error'] == 'output_exists'
    assert run.call_count == 1


def test_failed_guest_never_saved(tmp_path, monkeypatch):
    monkeypatch.setattr(login.shutil, 'which', lambda _: '/go')
    monkeypatch.setattr(login.subprocess, 'run', lambda *a, **kw: CompletedProcess([], 0, json.dumps(dict(ok=False, stage='guest', code=123)), 'SECRET'))
    path = tmp_path / 'cookie.json'
    assert not login.create_and_save(path)['ok']
    assert not path.exists()


def test_diagnostic_stays_private(tmp_path, monkeypatch):
    monkeypatch.setattr(login.shutil, 'which', lambda _: '/go')
    payload = dict(ok=False, code=1351, reason='PRIVATE', verify_url='https://example.test/?token=SECRET', recovery={'key': 'KEY'})
    run = Mock(return_value=CompletedProcess([], 0, json.dumps(payload), ''))
    monkeypatch.setattr(login.subprocess, 'run', run)
    diagnostic = tmp_path / 'diag.json'
    result = login.create_and_save(tmp_path / 'cookie.json', diagnostic)
    assert 'SECRET' not in repr(result) and 'KEY' not in repr(result)
    assert stat.S_IMODE(diagnostic.stat().st_mode) == 0o600
    assert json.loads(diagnostic.read_text()) == payload
    assert login.create_and_save(tmp_path / 'cookie.json', diagnostic)['error'] == 'diagnostic_exists_or_conflicts'
    assert run.call_count == 1


def test_pe_preserves_guest_identity():
    from proloader.net.netease.client import NeteaseClient
    c = NeteaseClient()
    for flag in (0, 1):
        s = c._parse_sauth(json.dumps({'is_unisdk_guest': flag, 'platform': 'pc'}))
        payload = c._build_sauth_payload(s, 'SN')
        assert payload['is_unisdk_guest'] == flag
        assert payload['platform'] == 'pc'
    import pytest
    with pytest.raises(ValueError):
        c._parse_sauth('{"is_unisdk_guest": 2}')
    c._session.close()
