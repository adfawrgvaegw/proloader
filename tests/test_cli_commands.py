import io
import asyncio
import json
from types import SimpleNamespace
from unittest.mock import AsyncMock

import pytest

from proloader.bots import BotStore,private_json
from proloader.cli import commands
from proloader.cli.main import build_parser


def identity(tmp_path):
    store=BotStore(tmp_path/'bots')
    cookie=json.dumps({'sauth_json':json.dumps(dict(gameid='x19',sdkuid='fixture',sessionid='PRIVATE-SESSION',deviceid='d',udid='u'))})
    return store,store.add('fixture-bot',cookie)


def test_import_is_real_and_default_rate_matches_fatalder():
    args=build_parser().parse_args(['import','house.mcstructure','--to=-85,78,351','--server','12345678'])
    assert args.command=='import' and args.rate==4000 and args.engine=='fatalder'
    assert not hasattr(args,'world_size')
    with pytest.raises(SystemExit):build_parser().parse_args(['import','x','--to=1,2,3','--server','12345678','--rate','6000'])


def test_real_import_dispatch_passes_password_in_memory_and_does_not_echo(tmp_path,monkeypatch,capsys):
    store,bot=identity(tmp_path)
    monkeypatch.setattr(commands,'BotStore',lambda:store)
    plan=SimpleNamespace(summary=lambda:dict(name='fixture'),commands=[1,2])
    monkeypatch.setattr(commands,'prepare_import',lambda *a,**k:plan)
    executed=AsyncMock(return_value=dict(id='a'*32,state='completed',submitted=2,checked=1,passed=1))
    monkeypatch.setattr(commands,'execute_import',executed)
    monkeypatch.setattr(commands.sys,'stdin',io.StringIO('PRIVATE-PASSWORD\n'))
    args=build_parser().parse_args(['import','house.mcstructure','--to=1,2,3','--server','12345678','--password-stdin','--yes','--json'])
    # CLI owns a fresh loop. Restore pytest's previous default for Python 3.9
    # synchronous fixtures that construct asyncio.Queue/Lock before their runner.
    previous=asyncio.get_event_loop()
    try:
        assert commands.dispatch(args)==0
    finally:
        asyncio.set_event_loop(previous)
    assert executed.call_args.args[4]=='PRIVATE-PASSWORD'
    assert executed.call_args.kwargs['commands_per_second']==4000
    output=capsys.readouterr().out
    assert 'PRIVATE-PASSWORD' not in output and 'PRIVATE-SESSION' not in output
    assert json.loads(output.splitlines()[-1])['event']=='result'


def test_noninteractive_does_not_connect_without_confirmation(tmp_path,monkeypatch):
    store,bot=identity(tmp_path)
    monkeypatch.setattr(commands,'BotStore',lambda:store)
    monkeypatch.setattr(commands,'prepare_import',lambda *a,**k:SimpleNamespace(summary=lambda:{},commands=[]))
    monkeypatch.setattr(commands.sys,'stdin',io.StringIO('private\n'))
    executed=AsyncMock();monkeypatch.setattr(commands,'execute_import',executed)
    args=build_parser().parse_args(['import','x','--to=1,2,3','--server','12345678','--password-stdin','--json'])
    with pytest.raises(ValueError,match='--yes'):commands.dispatch(args)
    executed.assert_not_awaited()


def test_resume_uses_saved_target_and_rejects_changed_fingerprint(tmp_path,monkeypatch):
    job=dict(state='failed',plan=dict(file='fixture',origin=[1,2,3],dimension='overworld',engine='fatalder',plan_sha256='a',wire_revision='v'))
    monkeypatch.setattr(commands,'prepare_import',lambda *a,**k:SimpleNamespace(summary=lambda:dict(job['plan'])))
    assert commands.resume_plan(job).summary()==job['plan']
    monkeypatch.setattr(commands,'prepare_import',lambda *a,**k:SimpleNamespace(summary=lambda:dict(job['plan'],plan_sha256='different')))
    with pytest.raises(ValueError,match='已变化'):commands.resume_plan(job)
    with pytest.raises(ValueError,match='旧版'):commands.resume_plan(dict(state='failed',plan={}))


def test_task_path_traversal_and_completed_resume_rejected():
    with pytest.raises(ValueError):commands.read_job('../cookie.json')
    with pytest.raises(ValueError,match='已经结束'):commands.resume_plan(dict(state='completed'))


def test_duplicate_bot_label_requires_id(tmp_path):
    store,bot=identity(tmp_path)
    fake=SimpleNamespace(list=lambda:[bot,bot])
    with pytest.raises(ValueError,match='名称重复'):commands.select_bot(fake,bot.label)


def test_pending_loading_jobs_are_visible(tmp_path,monkeypatch):
    from proloader.cli import wizard
    monkeypatch.setattr(wizard,'JOBS',tmp_path)
    private_json(tmp_path/'task.json',dict(id='a'*32,state='loading',server='12345678',bot_id='bot',plan=dict(sha256='hash',origin=[1,2,3],dimension='overworld')))
    assert wizard.pending_jobs()[0]['state']=='loading'
