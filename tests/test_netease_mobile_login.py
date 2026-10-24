import json
from unittest.mock import Mock
import pytest
from proloader.net.netease.mobile_login import MobileCookieProvider, MobileLoginError, official_verification_url

PHONE='13800000000'
DEVICE=dict(id='DEVICE',udid='UDID',transid='TRANS',mcount_transaction_id='COUNT')
TEMPLATE=json.dumps({'sauth_json':json.dumps(dict(gameid='x19',deviceid='DEVICE',sdkuid='OLD',sessionid='OLD_TOKEN',is_unisdk_guest=1))})

def provider(*payloads, info=None):
    if payloads:
        payloads = ((200, info if info is not None else {'registered': True, 'allow_methods': [1, 2]}),) + payloads
    s=Mock()
    s.request.side_effect=[Mock(status_code=status,json=Mock(return_value=body)) for status,body in payloads]
    return MobileCookieProvider(DEVICE,TEMPLATE,s)


def test_sms_flow_preserves_identity_and_separates_game_auth():
    p=provider((200,{}),(200,{'user':{'id':'EXACT-UID','token':'SECRET'}}))
    with pytest.raises(ValueError):p.fetch_cookie()
    p.send_sms(PHONE)
    assert p.session.request.call_args.args[0]=='GET'
    assert p.session.request.call_args.kwargs['params']['method']=='1'
    assert not p.session.request.call_args.kwargs['allow_redirects']
    with pytest.raises(ValueError):p.send_sms(PHONE)
    with pytest.raises(ValueError):p.fetch_cookie()
    p.verify_sms('123456')
    call=p.session.request.call_args
    assert call.args[0]=='POST' and call.kwargs['data']['sms_code']=='123456'
    s=json.loads(json.loads(p.fetch_cookie().cookie)['sauth_json'])
    assert s['sdkuid']=='EXACT-UID' and s['sessionid']=='SECRET'
    assert s['platform']=='ad' and s['sdk_version']=='5.16.0' and s['is_unisdk_guest']==0
    assert 'SECRET' not in repr(p)
    with pytest.raises(ValueError):p.verify_sms('123456')
    p.close()
    with pytest.raises(ValueError):p.fetch_cookie()


@pytest.mark.parametrize('status,body',[(400,{'code':1351,'reason':'SECRET','verify_url':'https://service.mkey.163.com/check?secret=1'}),(302,{}),(200,{'code':0}),(200,[])])
def test_failures_do_not_retry_or_leak(status,body):
    p=provider((status,body))
    with pytest.raises(MobileLoginError) as e:p.send_sms(PHONE)
    assert 'SECRET' not in str(e.value)+json.dumps(e.value.report())
    assert p.session.request.call_count==2
    with pytest.raises(ValueError):p.send_sms(PHONE)
    with pytest.raises(ValueError):p.verify_sms('123456')


@pytest.mark.parametrize('body',[{}, {'user':{}},{'user':{'id':123,'token':'T'}},{'user':{'id':'U','token':''}},{'user':{'id':'U','token':'T','login_channel':'other'}}])
def test_login_success_requires_real_identity(body):
    p=provider((200,{}),(200,body));p.send_sms(PHONE)
    with pytest.raises(MobileLoginError):p.verify_sms('123456')
    with pytest.raises(ValueError):p.fetch_cookie()


@pytest.mark.parametrize('phone',['','123','+8613800000000','1380000000x'])
def test_bad_phone_has_no_network(phone):
    p=provider()
    with pytest.raises(ValueError):p.send_sms(phone)
    p.session.request.assert_not_called()


@pytest.mark.parametrize('url',['http://service.mkey.163.com/x','https://163.com.evil.test/x','https://user@service.mkey.163.com/x'])
def test_verification_url_origin(url):
    assert official_verification_url(url) is None


def test_cli_default_offline(monkeypatch,capsys):
    from tools.diagnostics import login_netease_mobile as cli
    monkeypatch.setattr('sys.argv',['mobile'])
    monkeypatch.setattr(cli,'MobileCookieProvider',Mock(side_effect=AssertionError))
    assert cli.main()==0
    assert json.loads(capsys.readouterr().out)['mode']=='offline'


def test_cli_cancel_removes_reserved_file(tmp_path,monkeypatch):
    from tools.diagnostics import login_netease_mobile as cli
    recovery=tmp_path/'recovery';recovery.write_text(json.dumps({'recovery':DEVICE}))
    template=tmp_path/'template';template.write_text(TEMPLATE)
    output=tmp_path/'cookie'
    monkeypatch.setattr('sys.argv',['mobile','--login','--recovery',str(recovery),'--template-cookie',str(template),'--output',str(output)])
    monkeypatch.setattr('sys.stdin.isatty',lambda:True)
    monkeypatch.setattr(cli.getpass,'getpass',lambda _:PHONE)
    monkeypatch.setattr('builtins.input',lambda _:'CANCEL')
    assert cli.main()==1 and not output.exists()


@pytest.mark.parametrize('failure', [False, True])
def test_cli_sms_steps_and_private_failure(tmp_path, monkeypatch, capsys, failure):
    from tools.diagnostics import login_netease_mobile as cli
    recovery = tmp_path / 'recovery'
    recovery.write_text(json.dumps({'recovery': DEVICE}))
    template = tmp_path / 'template'
    template.write_text(TEMPLATE)
    output, diagnostic = tmp_path / 'cookie', tmp_path / 'failure'
    monkeypatch.setattr('sys.argv', ['mobile', '--login', '--recovery', str(recovery),
        '--template-cookie', str(template), '--output', str(output), '--diagnostic', str(diagnostic)])
    monkeypatch.setattr('sys.stdin.isatty', lambda: True)
    private = {'code': 1344, 'reason': 'PRIVATE_SERVER_DETAIL'}
    p = provider((400, private)) if failure else provider((200, {}), (200, {'user': {'id': 'UID', 'token': 'TOKEN'}}))
    monkeypatch.setattr(cli, 'MobileCookieProvider', lambda *args: p)
    prompts = []
    def hidden(prompt):
        prompts.append(prompt)
        return PHONE if len(prompts) == 1 else '123456'
    monkeypatch.setattr(cli.getpass, 'getpass', hidden)
    monkeypatch.setattr('builtins.input', lambda _: 'SEND')
    assert cli.main() == (1 if failure else 0)
    printed = capsys.readouterr().out
    assert 'PRIVATE_SERVER_DETAIL' not in printed
    if failure:
        assert len(prompts) == 1
        assert '尚未进入验证码输入步骤' in printed
        assert not output.exists()
        assert json.loads(diagnostic.read_text())['response'] == private
        assert diagnostic.stat().st_mode & 0o777 == 0o600
        assert p.session.request.call_count == 2
    else:
        assert len(prompts) == 2 and '[3/3]' in prompts[1]
        assert output.exists() and not diagnostic.exists()
        assert p.session.request.call_count == 3


@pytest.mark.parametrize('info,stage', [
    ({'registered': False, 'allow_methods': [1]}, 'sms_existing_account_required'),
    ({'allow_methods': [1]}, 'sms_existing_account_required'),
    ({'registered': True, 'allow_methods': [2]}, 'sms_method_unavailable'),
    ({'registered': True, 'allow_methods': [0]}, 'sms_method_unavailable'),
    ({'registered': True, 'allow_methods': ['1']}, 'sms_method_unavailable'),
    ({'registered': True}, 'sms_method_unavailable'),
])
def test_preflight_prevents_unsupported_send(info, stage):
    p = provider((200, {}), info=info)
    with pytest.raises(MobileLoginError) as error:
        p.send_sms(PHONE)
    assert error.value.stage == stage
    assert p.session.request.call_count == 1
    call = p.session.request.call_args
    assert call.args[1].endswith('/mobile2/info')
    assert call.kwargs['params']['login_for'] == '1'
    with pytest.raises(ValueError):
        p.send_sms(PHONE)


def test_preflight_challenge_stops_without_sms():
    p = provider()
    body = {'code': 1351, 'verify_url': 'https://service.mkey.163.com/private'}
    p.session.request.side_effect = None
    p.session.request.return_value = Mock(status_code=400, json=Mock(return_value=body))
    with pytest.raises(MobileLoginError) as error:
        p.send_sms(PHONE)
    assert error.value.stage == 'sms_info'
    assert error.value.verify_url
    assert p.session.request.call_count == 1
