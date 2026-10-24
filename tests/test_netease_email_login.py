"""本地登录入口：密码不进入 argv；成功才写 cookie 且权限为 0600。"""
import json
import stat
from subprocess import CompletedProcess
from unittest.mock import Mock

import pytest

from tools.diagnostics import login_netease_email as login


def test_password_stays_in_stdin_and_cookie_saved_privately(tmp_path, monkeypatch):
    cookie = json.dumps({'sauth_json': json.dumps({'sessionid': 'TOKEN'})})
    run = Mock(return_value=CompletedProcess([], 0, json.dumps({'ok': True, 'cookie': cookie}), ''))
    monkeypatch.setattr(login.shutil, 'which', lambda name: '/go')
    monkeypatch.setattr(login.subprocess, 'run', run)
    output = tmp_path / 'cookie.json'
    result = login.login_and_save('user@163.com', 'PASSWORD', output)
    assert result == {'ok': True, 'saved': str(output)}
    args, kwargs = run.call_args
    assert 'PASSWORD' not in repr(args)
    assert json.loads(kwargs['input'])['password'] == 'PASSWORD'
    assert output.read_text() == cookie
    assert stat.S_IMODE(output.stat().st_mode) == 0o600
    assert 'PASSWORD' not in output.read_text()


def test_existing_cookie_never_overwritten(tmp_path, monkeypatch):
    output = tmp_path / 'cookie.json'; output.write_text('old-cookie')
    run = Mock()
    monkeypatch.setattr(login.subprocess, 'run', run)
    assert login.login_and_save('user@163.com', 'PASSWORD', output)['error'] == 'output_exists'
    run.assert_not_called()
    assert output.read_text() == 'old-cookie'


@pytest.mark.parametrize('payload', [
    {'ok': False, 'error': 'mpay_verification_required', 'code': 123},
    {'ok': True, 'cookie': '{}'},
])
def test_failed_login_does_not_save_cookie(payload, tmp_path, monkeypatch):
    monkeypatch.setattr(login.shutil, 'which', lambda name: '/go')
    monkeypatch.setattr(login.subprocess, 'run', lambda *a, **kw: CompletedProcess([], 0, json.dumps(payload), ''))
    output = tmp_path / 'cookie.json'
    result = login.login_and_save('user@163.com', 'PASSWORD', output)
    assert not result['ok']
    assert not output.exists()
