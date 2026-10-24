import json
import stat
from unittest.mock import Mock
from tools.diagnostics import diag_netease_otp as diag


def test_otp_only_and_private_save(tmp_path,monkeypatch):
    raw=json.dumps({'sauth_json':json.dumps({'platform':'pc','sessionid':'SECRET'})})
    cookie=tmp_path/'cookie';cookie.write_text(raw)
    session=Mock()
    session.get.return_value.json.return_value={'CoreServerUrl':'https://test.minecraft.cn'}
    session.post.return_value.json.return_value={'code':0,'entity':{'otp_token':'SECRET','aid':123}}
    session.post.return_value.status_code=200
    monkeypatch.setattr(diag.requests,'Session',lambda:session)
    output=tmp_path/'result'
    result=diag.diagnose(cookie,output)
    assert result['ok'] and 'SECRET' not in repr(result)
    assert session.post.call_count==1
    assert session.post.call_args.args[0]=='https://test.minecraft.cn/login-otp'
    assert session.post.call_args.kwargs['data']==raw.encode()
    assert stat.S_IMODE(output.stat().st_mode)==0o600
    assert diag.diagnose(cookie,output)['error']=='output_exists'


def test_unexpected_origin_never_receives_cookie(tmp_path,monkeypatch):
    cookie=tmp_path/'cookie';cookie.write_text(json.dumps({'sauth_json':'{"platform":"pc"}'}))
    session=Mock();session.get.return_value.json.return_value={'CoreServerUrl':'https://example.test'}
    monkeypatch.setattr(diag.requests,'Session',lambda:session)
    assert not diag.diagnose(cookie,tmp_path/'out')['ok']
    session.post.assert_not_called()


def test_nested_rejection_is_reported_without_arbitrary_server_text():
    from tools.diagnostics.netease_auth_result import summarize_response
    payload={'code':32,'message':'SECRET','entity':{'code':400,'subcode':1,'status':'invalid param: sdkuid','token':'SECRET'}}
    assert summarize_response(payload)=={'code':32,'service_code':400,'service_subcode':1,'reason':'sdkuid_rejected'}
    payload['entity']['status']='SECRET'
    assert 'SECRET' not in repr(summarize_response(payload))
