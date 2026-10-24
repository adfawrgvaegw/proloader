"""Offline Python↔Go DEFLATE interop using synthetic data; no credentials/network."""
import base64
import hashlib
import json
import os
from pathlib import Path
import random
import subprocess

from proloader.net.bedrock.batch import _netease_compress, NeteaseDecompressor
from proloader.net.bedrock.crypto import generate_keypair, serialize_public_key_b64
from proloader.net.bedrock.login import ClientData, build_login_from_jwt_chain, build_login_packet, sign_jwt
from proloader.net.bedrock.login_audit import audit_connection_request
from proloader.net.raknet.wire import ByteWriter


def main():
    ca, key = generate_keypair(), generate_keypair()
    token = sign_jwt(ca, {'alg': 'ES384', 'x5u': serialize_public_key_b64(ca)},
                     {'identityPublicKey': serialize_public_key_b64(key), 'nbf': 0, 'exp': 2**40})
    request = build_login_from_jwt_chain(json.dumps([token]), ClientData(), key)
    packet = build_login_packet(request, protocol=860)
    writer = ByteWriter()
    writer.write_unsigned_varint(len(packet))
    writer.write(packet)
    cases = {'synthetic_login_860': writer.to_bytes(),
             'incompressible_64k': random.Random(0).randbytes(65536),
             'repeated_128k': b'abcd' * 32768, 'tiny': b'\x01\x04'}
    encoded = [dict(plain=base64.b64encode(data).decode(),
                    compressed=base64.b64encode(_netease_compress(data)).decode())
               for data in cases.values()]
    env = {**os.environ, 'GO111MODULE': 'off', 'GOTOOLCHAIN': 'local', 'GOPROXY': 'off'}
    proc = subprocess.run(['go', 'run', str(Path(__file__).with_name('netease_flate_oracle.go'))],
                          input=json.dumps(encoded), text=True, capture_output=True, env=env, timeout=60)
    if proc.returncode:
        raise RuntimeError('Go offline oracle failed')
    results = json.loads(proc.stdout)
    assert len(results) == len(cases)
    report = {}
    for (name, data), result in zip(cases.items(), results):
        go_bytes = base64.b64decode(result['compressed'], validate=True)
        report[name] = {
            'python_to_go': result['decoded_hash'] == hashlib.sha256(data).hexdigest(),
            'go_to_python': NeteaseDecompressor().decompress(go_bytes) == data,
            'plain_bytes': len(data), 'python_compressed_bytes': len(_netease_compress(data)),
            'go_compressed_bytes': len(go_bytes),
        }
    print(json.dumps(dict(oracle='Go standard library RFC1951; not server acceptance',
                          cases=report, synthetic_login=audit_connection_request(request)), indent=2))
    return 0 if all(r['python_to_go'] and r['go_to_python'] for r in report.values()) else 1


if __name__ == '__main__':
    raise SystemExit(main())
