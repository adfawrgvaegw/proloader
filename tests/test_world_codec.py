import concurrent.futures
import json
from pathlib import Path
import subprocess
import pytest
from proloader.command_blocks import world_reader_binary
from proloader.world_codec import CodecWorker


def test_worker_serializes_concurrent_decodes_and_recovers_after_invalid_input():
    binary=world_reader_binary()
    payload=dict(action='hash_states',palette=[dict(name='minecraft:stone',states={})])
    expected=json.loads(subprocess.run([str(binary)],input=json.dumps(payload),text=True,
        capture_output=True,check=True).stdout)
    worker=CodecWorker(binary)
    try:
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            values=list(pool.map(lambda _:worker.call(payload),range(12)))
        assert values==[expected]*12
        with pytest.raises(ValueError):worker.call(dict(action='invalid'))
        assert worker.call(payload)==expected
        worker.process.kill();worker.process.wait()
        assert worker.call(payload)==expected
    finally:worker.close()


def test_worker_timeout_kills_owned_process_and_rejects_oversized_request(tmp_path):
    binary=tmp_path/'hung-codec'
    binary.write_text('#!/bin/sh\nexec sleep 20\n');binary.chmod(0o700)
    worker=CodecWorker(binary)
    try:
        with pytest.raises(ValueError):worker.call({},timeout=.05)
        assert worker.process is None
        with pytest.raises(ValueError,match='上限'):worker.call({'data':'x'*(32<<20)})
    finally:worker.close()


def test_stream_protocol_retains_framing_after_bad_request():
    good=json.dumps(dict(action='hash_states',palette=[dict(name='minecraft:stone',states={})]))
    result=subprocess.run([str(world_reader_binary()),'--stream'],input=good+'\nBAD\n'+good+'\n',
        text=True,capture_output=True,check=True)
    rows=[json.loads(line) for line in result.stdout.splitlines()]
    assert rows[0]==rows[2] and 'error' in rows[1]
