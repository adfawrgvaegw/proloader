import json
import stat
from unittest.mock import Mock
from tools.diagnostics import diag_netease_pe as diag


def test_profile_is_explicit_and_source_unchanged(tmp_path, monkeypatch):
    cookie=tmp_path/'cookie.json'
    original=json.dumps({'sauth_json':json.dumps({'platform':'pc','source_platform':'pc','sessionid':'SECRET'})})
    cookie.write_text(original)
    c=Mock()
    c._session.hooks={'response':[]}
    c.user_id='U';c.user_token='SECRET';c.seed='S';c.user_detail={}
    c.engine_version='V';c.g79_latest_version='P';c.patch_resources_hash='H';c.core_server_url='https://example.test'
    monkeypatch.setattr(diag,'NeteaseClient',lambda:c)
    output=tmp_path/'result.json'
    report=diag.diagnose(cookie,output,'reference-android')
    sent=json.loads(json.loads(c.authenticate_with_cookie.call_args.args[0])['sauth_json'])
    assert sent['platform']=='ad' and sent['source_platform']=='ad'
    assert cookie.read_text()==original and 'SECRET' not in repr(report)
    assert stat.S_IMODE(output.stat().st_mode)==0o600
    assert report['ok']
    assert diag.diagnose(cookie,output)['error']=='output_exists'
    assert c.authenticate_with_cookie.call_count==1
