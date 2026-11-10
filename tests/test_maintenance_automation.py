"""Behavior tests for the one-entry updater; no production writes or logins."""
import hashlib
import importlib.util
import json
from pathlib import Path
from types import SimpleNamespace
import sys
from unittest.mock import Mock

import pytest

SCRIPTS=Path(__file__).resolve().parents[1]/'.agents/skills/proloader-maintenance/scripts'
sys.path.insert(0,str(SCRIPTS))
spec=importlib.util.spec_from_file_location('maintenance_entry',SCRIPTS/'maintain.py')
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)


@pytest.fixture
def scenario(tmp_path,monkeypatch):
    baseline=json.loads((m.ASSETS/'auth-baseline.json').read_text())
    profile=tmp_path/'proloader/net/netease/pe_profiles.py';profile.parent.mkdir(parents=True)
    profile.write_text(f"OFFICIAL_ENGINE={baseline['engine']!r}\nOFFICIAL_PATCHES={{\n    {(baseline['patch'],baseline['resources_hash'])!r},\n}}\n")
    metadata=dict(engine=baseline['engine'],patch='3.9.43.999999',resources_hash='a'*32,
                  gray={'engine':'3.10.0.300878','ratio':5},rn_present=False,overlay=False,native_assets={})
    folder=tmp_path/'report';folder.mkdir()
    helper=tmp_path/'tools/mcpcheck/main.go';helper.parent.mkdir(parents=True)
    helper.write_text('r.Engine == "'+metadata['engine']+'" && r.Patch == "'+metadata['patch']+'"')
    settings=tmp_path/'cache/maintenance/settings.json';settings.parent.mkdir(parents=True)
    settings.write_text(json.dumps(dict(bot='fixture',server='12345678',password_source='empty')))
    artifact=tmp_path/'source.mcp';artifact.write_bytes(b'fixture')
    monkeypatch.setattr(m,'discover',lambda *a:metadata)
    monkeypatch.setattr(m,'download',lambda *a:artifact)
    monkeypatch.setattr(m,'evidence',lambda *a:json.loads(json.dumps(baseline)))
    monkeypatch.setattr(m,'idle',lambda *a:True)
    monkeypatch.setattr(m,'tests',lambda *a:True)
    monkeypatch.setattr(m,'acceptance',lambda *a:True)
    args=SimpleNamespace(audit=False,online=True,check_only=False,password_stdin=False)
    return SimpleNamespace(root=tmp_path,path=profile,before=profile.read_bytes(),metadata=metadata,
                           baseline=baseline,folder=folder,args=args)


def test_no_update_does_not_download_or_inspect(scenario,monkeypatch):
    s=scenario;s.metadata.update(patch=s.baseline['patch'],resources_hash=s.baseline['resources_hash'])
    def forbidden(*a):raise AssertionError('unneeded work')
    monkeypatch.setattr(m,'download',forbidden)
    result=m.operate(s.root,s.args,s.folder)
    assert result['status']=='current' and s.path.read_bytes()==s.before


def test_proven_update_is_exact_and_accepted_only_after_validation(scenario):
    s=scenario;result=m.operate(s.root,s.args,s.folder)
    assert result['status']=='accepted' and result['online_rounds']==10
    engine,pairs=m.profile(s.path)
    assert engine==s.baseline['engine']
    assert (s.metadata['patch'],s.metadata['resources_hash']) in pairs
    assert len(pairs)==2
    assert (s.folder/'pe_profiles.before.py').read_bytes()==s.before


@pytest.mark.parametrize('failure',['offline','online','exception'])
def test_failed_validation_rolls_source_back(scenario,monkeypatch,failure):
    s=scenario
    if failure=='exception':
        def raised(*a):raise RuntimeError('simulated failure')
        monkeypatch.setattr(m,'tests',raised)
        with pytest.raises(RuntimeError):m.operate(s.root,s.args,s.folder)
    else:
        monkeypatch.setattr(m,'tests' if failure=='offline' else 'acceptance',lambda *a:False)
        result=m.operate(s.root,s.args,s.folder)
        assert result['status']=='rolled_back' and result['source_changed'] is False
    assert s.path.read_bytes()==s.before


def test_changed_auth_module_cannot_auto_accept(scenario,monkeypatch):
    s=scenario;proof=json.loads(json.dumps(s.baseline));proof['modules'][next(iter(proof['modules']))]='changed'
    monkeypatch.setattr(m,'evidence',lambda *a:proof)
    result=m.operate(s.root,s.args,s.folder)
    assert result['status']=='needs_analysis' and result['changed_modules']
    assert s.path.read_bytes()==s.before


def test_engine_change_cannot_reuse_old_native_recipe(scenario,monkeypatch):
    s=scenario;s.metadata['engine']='3.10.0.300878'
    inspector=Mock(return_value=s.baseline)
    monkeypatch.setattr(m,'evidence',inspector)
    result=m.operate(s.root,s.args,s.folder)
    assert result['reason']=='engine_changed' and s.path.read_bytes()==s.before
    inspector.assert_called_once()
    assert (s.folder/'STATIC.json').exists()


@pytest.mark.parametrize('reason',['busy','no_online'])
def test_no_source_change_without_available_online_validation(scenario,monkeypatch,reason):
    s=scenario
    if reason=='busy':monkeypatch.setattr(m,'idle',lambda *a:False)
    else:s.args.online=False
    result=m.operate(s.root,s.args,s.folder)
    assert result['status']==('busy' if reason=='busy' else 'ready_to_apply')
    assert s.path.read_bytes()==s.before


def test_rollback_does_not_overwrite_concurrent_source_change(scenario,monkeypatch):
    s=scenario
    def tests(*a):s.path.write_bytes(b'concurrent maintainer change');return False
    monkeypatch.setattr(m,'tests',tests)
    with pytest.raises(ValueError,match='concurrent_change_prevents_rollback'):
        m.operate(s.root,s.args,s.folder)
    assert s.path.read_bytes()==b'concurrent maintainer change'
    assert (s.folder/'pe_profiles.before.py').read_bytes()==s.before


def test_cached_artifact_is_rechecked_not_blindly_trusted(tmp_path):
    body=b'verified public artifact';md5=hashlib.md5(body).hexdigest()
    metadata={'mcp':dict(md5=md5,bytes=len(body),url='https://example.invalid/vanilla.mcp')}
    cached=tmp_path/'cache/maintenance/artifacts'/(md5+'.mcp');cached.parent.mkdir(parents=True);cached.write_bytes(b'tampered')
    response=Mock(content=body);session=Mock();session.get.return_value=response
    assert m.download(tmp_path,metadata,session).read_bytes()==body
    session.get.assert_called_once()
    session.get.reset_mock()
    assert m.download(tmp_path,metadata,session).read_bytes()==body
    session.get.assert_not_called()


def test_bad_download_never_enters_verified_cache(tmp_path):
    body=b'expected';metadata={'mcp':dict(md5=hashlib.md5(body).hexdigest(),bytes=len(body),url='https://example.invalid/mcp')}
    session=Mock();session.get.return_value=Mock(content=b'corrupt!')
    with pytest.raises(ValueError,match='digest_mismatch'):m.download(tmp_path,metadata,session)
    assert not list(tmp_path.rglob('*.mcp'))


def test_native_and_rn_changes_require_analysis(scenario):
    s=scenario
    for field,value,reason in [('rn_present',True,'rn_changed'),('overlay',True,'patch_overlay_requires_analysis'),('native_assets',{'a.so':'digest'},'native_assets_require_analysis')]:
        metadata=dict(s.metadata);metadata[field]=value
        assert m.decide(metadata,s.baseline,s.baseline)==reason


@pytest.mark.parametrize('round_count,digest_ok,expected',[(9,True,False),(10,False,False),(10,True,True)])
def test_online_acceptance_requires_ten_rounds_and_unchanged_source(tmp_path,monkeypatch,round_count,digest_ok,expected):
    from proloader import bots
    store=Mock();store.list.return_value=[SimpleNamespace(id='fixture')];store.pe.return_value={'private':'preserved'}
    monkeypatch.setattr(bots,'BotStore',lambda:store)
    monkeypatch.setattr(m,'network_digest',lambda *a:'CURRENT')
    def invoked(*a,**kw):
        assert kw['input']==b'\n'
        (tmp_path/'ONLINE.json').write_text(json.dumps(dict(passed=True,
            rounds=[{'passed':True}]*round_count,source_sha256='CURRENT' if digest_ok else 'OLD')))
        return SimpleNamespace(returncode=0,stdout=b'',stderr=b'')
    monkeypatch.setattr(m.subprocess,'run',invoked)
    assert m.acceptance(tmp_path,tmp_path,dict(bot='fixture',server='12345678'),'') is expected
    backup=tmp_path/'PE-before-private.json'
    assert backup.stat().st_mode&0o777==0o600
    assert json.loads(backup.read_text())=={'private':'preserved'}


def test_timeout_preserves_partial_stage_log(tmp_path, monkeypatch):
    def timeout(*a, **kw):
        raise m.subprocess.TimeoutExpired(a[0], kw['timeout'], output=b'partial stdout\n', stderr=b'partial stderr\n')
    monkeypatch.setattr(m.subprocess, 'run', timeout)
    with pytest.raises(m.subprocess.TimeoutExpired):
        m.tests(tmp_path, tmp_path)
    assert (tmp_path/'tests.log').read_bytes() == b'partial stdout\npartial stderr\n'


def test_missing_dynamic_version_binding_stops_before_source_or_online(scenario, monkeypatch):
    s = scenario
    (s.root/'tools/mcpcheck/main.go').write_text('r.Engine == "3.9.15.297907" && r.Patch == "3.9.43.301387"')
    online = Mock()
    monkeypatch.setattr(m, 'acceptance', online)
    result = m.operate(s.root, s.args, s.folder)
    assert result['reason'] == 'dynamic_profile_requires_review'
    assert s.path.read_bytes() == s.before
    online.assert_not_called()



def test_discovery_uses_manifest_from_successful_public_retry(tmp_path, monkeypatch):
    import io, zipfile
    from proloader.net.netease import client
    data=io.BytesIO()
    with zipfile.ZipFile(data, 'w') as archive:
        archive.writestr('manifest.json',json.dumps({'assets':{'vanilla.mcp':{'md5':'a'*32,'size':3}}}))
    class FakeClient:
        def __init__(self, **kw):
            self._session=Mock();self._session.hooks={}
            self._session.get.side_effect=[Mock(json=lambda:{'netease':{'version':'3.10.5.301327','gray_ratio':100}}),
                                          Mock(json=lambda:{'android':['3.10.15.301976']})]
            self.engine_version='3.10.5.301327';self.g79_latest_version='3.10.15.301976'
            self.patch_resources_hash='b'*32;self.rn_bundle_present=False
        def bootstrap(self, **kw):
            for body in (b'CDN not a zip',data.getvalue()):
                response=Mock(url='https://cdn.example/manifest.zip',status_code=200,content=body,headers={})
                for hook in self._session.hooks['response']:hook(response)
    monkeypatch.setattr(client,'NeteaseClient',FakeClient)
    result=m.discover(tmp_path,tmp_path)
    assert result['mcp']['md5']=='a'*32 and result['mcp']['bytes']==3
    assert len(result['requests'])==2
