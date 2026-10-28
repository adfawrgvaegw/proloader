import base64
from pathlib import Path
from unittest.mock import Mock
import zlib

import pytest

from proloader.net.netease import mcp_check


async def test_session_uses_unique_id_and_sends_full_reply(monkeypatch):
    import asyncio
    from types import SimpleNamespace
    from unittest.mock import AsyncMock
    from proloader.net.bedrock.session import BedrockSession
    from proloader.net.bedrock.netease_packets import parse_py_rpc
    session=BedrockSession(None)
    session._start_game=SimpleNamespace(entity_unique_id=-12345,entity_runtime_id=98765)
    session._netease_engine_version='engine';session._netease_patch_version='patch'
    session._send_packet=AsyncMock()
    expected=['a','b',False,[],'','',3,'c']
    solve=Mock(return_value=(expected,'profile'))
    monkeypatch.setattr(mcp_check,'solve',solve)
    session._handle_mcp_check_num([b'module',[b'salt',[],[]]])
    tasks=list(session._mcp_challenge_tasks)
    await asyncio.gather(*tasks)
    solve.assert_called_once_with('module','salt',-12345,'engine','patch')
    rpc=parse_py_rpc(session._send_packet.call_args.args[0])
    assert rpc['name'] in ('SetMCPCheckNum',b'SetMCPCheckNum')
    assert len(rpc['payload'])==8
    assert session._terminal_error is None


async def test_unknown_profile_terminates_without_sending_legacy_answer(monkeypatch):
    import asyncio
    from types import SimpleNamespace
    from unittest.mock import AsyncMock
    from proloader.net.bedrock.session import BedrockSession, BedrockLoginError
    session=BedrockSession(None)
    session._start_game=SimpleNamespace(entity_unique_id=1)
    session._send_packet=AsyncMock()
    monkeypatch.setattr(mcp_check,'solve',Mock(side_effect=ValueError('PRIVATE')))
    session._handle_mcp_check_num(['module',['salt',[],[]]])
    await asyncio.gather(*list(session._mcp_challenge_tasks))
    session._send_packet.assert_not_called()
    with pytest.raises(BedrockLoginError) as error:session._raise_if_failed()
    assert 'PRIVATE' not in str(error.value)


def test_captured_reviewed_module_matches_independent_upstream_vector():
    source = mcp_check.ROOT/'cache/local_repair_20261027/server_dynamic.hex'
    if not source.exists():pytest.skip('local official delivered module not distributed with tests')
    response,profile=mcp_check.solve(source.read_text(),'offline-test-salt',12345,
                                   '3.9.15.297907','3.9.43.301387')
    assert profile=='574eef18e0deb853d397c1344b805f513565623c9393c07511450b288bf8912a'
    assert response==['3f064d39231a49a6b33be21cb8a9b332','20f8ad8d784aeb0988beb16a68eabc16',
                      False,[],'','',3,'5f99aa611c6dfa8d8145c98cd372b893']


def test_second_delivered_module_with_empty_prefix_has_same_reviewed_structure():
    source = mcp_check.ROOT/'cache/local_repair_20261027/server_dynamic_current.hex'
    if not source.exists():pytest.skip('local official delivered module not distributed with tests')
    response,profile=mcp_check.solve(source.read_text(),'offline-test-salt',12345,
                                   '3.9.15.297907','3.9.43.301387')
    assert profile=='574eef18e0deb853d397c1344b805f513565623c9393c07511450b288bf8912a'
    assert len(response)==8
    # Independent formula from the reviewed disassembly, with this capture's suffix.
    import hashlib
    calc=lambda text:hashlib.md5((text+'h5+ijo4gs8?').encode()).hexdigest()
    val=calc('offline-test-salt0')
    num=calc(''.join(str((ord(c)*2+5)^255) for c in val)+
             '3.9.15.297907android3.9.43.301387android21212345')
    assert response==[val,calc(val[16:]+'False[]3'+num+val[:16]),False,[],'','',3,num]


@pytest.mark.parametrize('module,salt,uid', [('', 'salt',1),('aa'*65537,'salt',1),
                                          ('aa','',1),('aa','salt','1')])
def test_invalid_challenge_is_rejected_before_worker(module,salt,uid,monkeypatch):
    worker=Mock()
    monkeypatch.setattr('proloader.world_codec.request',worker)
    with pytest.raises(ValueError):mcp_check.solve(module,salt,uid,'engine','patch')
    worker.assert_not_called()


@pytest.mark.parametrize('payload', [zlib.compress(b'x'*131073),zlib.compress(b'x')[:-1],
                                   zlib.compress(b'x')+b'extra'])
def test_decompression_is_bounded_and_requires_complete_single_stream(payload,monkeypatch):
    monkeypatch.setattr(mcp_check,'NlsCipher',lambda:Mock(decrypt=lambda raw:raw))
    worker=Mock();monkeypatch.setattr('proloader.world_codec.request',worker)
    with pytest.raises(ValueError):mcp_check.solve(payload.hex(),'salt',1,'engine','patch')
    worker.assert_not_called()


def test_unknown_code_profile_is_rejected_without_executing_it():
    from proloader.world_codec import request
    with pytest.raises(ValueError):
        request(mcp_check.binary(),dict(marshal=base64.b64encode(b'unknown').decode(),salt='salt',
                                       unique_id=1,engine='3.9.15.297907',patch='3.9.43.301387'))
