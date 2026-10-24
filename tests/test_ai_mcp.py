import json
from pathlib import Path
import subprocess
import sys

from proloader.ai.mcp import dispatch,TOOLS


async def test_mcp_discovery_and_java_lint_error():
    result=await dispatch(dict(method='initialize',params=dict(protocolVersion='2025-06-18')))
    assert result['protocolVersion']=='2025-06-18'
    assert result['capabilities']['tools']==dict(listChanged=False)
    tools=(await dispatch(dict(method='tools/list')))['tools']
    assert len(tools)==10 and len({t['name'] for t in tools})==10
    result=await dispatch(dict(method='tools/call',params=dict(name='proloader_lint',arguments=dict(
        command='execute as @e[limit=1] run say hi'))))
    assert result['isError']
    assert json.loads(result['content'][0]['text'])['issues'][0]['code']=='java-selector'


async def test_mcp_compilation_and_invalid_params(tmp_path):
    file=Path('examples/ai/counter.json').resolve()
    output=tmp_path/'result.mcstructure'
    result=await dispatch(dict(method='tools/call',params=dict(name='proloader_compile',arguments=dict(
        file=str(file),output=str(output)))))
    assert not result['isError'] and output.exists()
    result=await dispatch(dict(method='tools/call',params=dict(name='proloader_command',arguments=dict(
        command='say hi',unexpected=True))))
    assert result['isError']


def test_stdio_handshake_from_other_directory():
    launcher=Path('tools/proloader_mcp.py').resolve()
    requests=[dict(jsonrpc='2.0',id=1,method='initialize',params=dict(protocolVersion='2024-11-05')),
              dict(jsonrpc='2.0',method='notifications/initialized'),
              dict(jsonrpc='2.0',id=2,method='tools/list'),
              dict(jsonrpc='2.0',id=3,method='tools/call',params=dict(name='proloader_spec',arguments=dict(topic='execute')))]
    p=subprocess.run([sys.executable,str(launcher)],cwd='/tmp',input=''.join(json.dumps(r)+'\n' for r in requests),
                     capture_output=True,text=True,timeout=10)
    assert p.returncode==0
    responses={r['id']:r for r in map(json.loads,p.stdout.splitlines())}
    assert responses[1]['result']['serverInfo']['name']=='proloader-minecraft'
    assert len(responses[2]['result']['tools'])==len(TOOLS)
    assert responses[3]['result']['isError'] is False
