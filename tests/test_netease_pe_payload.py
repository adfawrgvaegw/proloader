"""Verify decrypted PE wire shape, platform consistency and guest identity."""
import json
from unittest.mock import Mock
import pytest
from proloader.net.netease.client import NeteaseClient
from proloader.net.netease.crypto import g79_http_encrypt, g79_http_decrypt, get_valid_json

@pytest.mark.parametrize('platform,os_name,step', [('android','android',True),('ios','ios',False)])
@pytest.mark.parametrize('guest', [0,1])
def test_pe_wire_payload(platform, os_name, step, guest, capsys):
    c=NeteaseClient(platform=platform)
    c.g79_latest_version='test-version'
    c.patch_resources_hash='test-hash'
    captured={}
    def post(url, *, data, **kwargs):
        captured.update(json.loads(get_valid_json(g79_http_decrypt(bytes.fromhex(data)))))
        return Mock(text=g79_http_encrypt(b'{"code":32,"message":"SECRET","entity":{"token":"SECRET"}}').hex())
    c._session.post=post
    sauth=dict(sdkuid='U',sessionid='TOKEN',udid='D',deviceid='DEVICE',platform='pc',is_unisdk_guest=guest)
    with pytest.raises(RuntimeError,match='code: 32') as err:
        c.authenticate_with_cookie(json.dumps({'sauth_json':json.dumps(sauth)}))
    assert isinstance(captured['sauth_json'],dict)
    assert isinstance(captured['sa_data'],str)
    assert captured['sauth_json']['platform']=='pc'
    assert captured['sauth_json']['is_unisdk_guest']==guest
    assert ('step' in captured['sauth_json'])==step
    devices=json.loads(captured['sa_data'])
    assert devices['os_name']==os_name and devices['is_guest']==guest
    assert 'SECRET' not in str(err.value) + capsys.readouterr().out
    c._session.close()


def test_bootstrap_rejects_unmatched_patch_instead_of_cross_version_fallback():
    c=NeteaseClient()
    c.engine_version='3.10.0.300878'
    c._fetch_engine_version=Mock()
    response=Mock()
    response.json.return_value={'android':['3.1.9.123','3.9.41.300665'],'urlNew':'https://example.test'}
    c._session.get=Mock(return_value=response)
    with pytest.raises(ValueError,match='3.10'):
        c.bootstrap()
    assert c.g79_latest_version=='' and c.patch_resources_hash==''
    c._session.get.assert_called_once()
    c._session.close()


def test_official_download_version_is_independent_of_gray_pack():
    c=NeteaseClient()
    c._session.get=Mock(return_value=Mock(text='var android_link = android_type ? "https://g79.gdl.netease.com/mcpe-release2_3.9-3.9.15.297907-Publish-arm64.apk?key=public"'))
    c._fetch_official_download_version()
    assert c.engine_version=='3.9.15.297907'
    c._session.close()


def test_official_download_does_not_trust_other_origin():
    c=NeteaseClient()
    c._session.get=Mock(return_value=Mock(text='var android_link = android_type ? "https://evil.test/a-3.9.15.297907-Publish.apk"'))
    with pytest.raises(ValueError,match='origin'):c._fetch_official_download_version()
    c._session.close()


def test_manifest_and_rn_failure_cannot_be_success_metadata():
    c=NeteaseClient();c.engine_version='3.9.15.297907'
    c._fetch_engine_version=Mock()
    c._session.get=Mock(side_effect=[Mock(json=Mock(return_value={'android':['3.9.41.300665'],'urlNew':'https://example.test'})), Mock(content=b'not a zip')])
    with pytest.raises(ValueError,match='manifest'):c.bootstrap()
    assert c.g79_latest_version=='' and c.patch_resources_hash==''
    c._session.close()


@pytest.mark.parametrize('status,declared,passes',[(404,False,True),(404,True,False),(503,False,False)])
def test_rn_optional_only_when_manifest_and_explicit_404_agree(status,declared,passes):
    import io,zipfile
    import requests
    assets={'vanilla.mcp':{'md5':'a'*32}}
    if declared:assets['rn/index.bundle.backup']={'md5':'b'*32}
    buf=io.BytesIO()
    with zipfile.ZipFile(buf,'w') as z:z.writestr('manifest.json',json.dumps({'assets':assets}))
    rn=Mock(status_code=status)
    rn.raise_for_status.side_effect=requests.HTTPError()
    c=NeteaseClient();c.engine_version='3.9.15.297907';c._fetch_engine_version=Mock()
    c._session.get=Mock(side_effect=[Mock(json=Mock(return_value={'android':['3.9.41.300665'],'urlNew':'https://example.test'})),Mock(content=buf.getvalue()),rn])
    if passes:
        c.bootstrap();assert len(c.patch_resources_hash)==32 and c.rn_bundle_present is False
    else:
        with pytest.raises(requests.HTTPError):c.bootstrap()
        assert c.g79_latest_version=='' and c.patch_resources_hash==''
    c._session.close()


@pytest.mark.parametrize('entity',[{'entity_id':1,'token':''},{'entity_id':True,'token':'T'},{'entity_id':'0','token':'T'},{}])
def test_game_auth_requires_valid_credentials_and_clears_old_state(entity):
    c=NeteaseClient();c.user_id='OLD';c.user_token='OLD_SECRET'
    c._session.post=Mock(return_value=Mock(text=g79_http_encrypt(json.dumps({'code':0,'entity':entity}).encode()).hex()))
    sauth=dict(sdkuid='U',sessionid='T',udid='D',deviceid='DEVICE')
    with pytest.raises(RuntimeError,match='凭据'):c.authenticate_with_cookie(json.dumps({'sauth_json':json.dumps(sauth)}))
    assert c.user_id==c.user_token==''
    c._session.close()
