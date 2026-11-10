#!/usr/bin/env python3
"""Observe certificates in the official APK's length-checked v2 signing block.

This is not APK signature verification. Never install or execute the target.
Input SOURCE.json is produced by apk_entries.py.
"""
import argparse
import hashlib
import json
from pathlib import Path
import struct
from urllib.parse import urlsplit
import zipfile
from apk_entries import RemoteAPK


def length_prefixed(data):
    if len(data) < 4:
        raise ValueError('truncated_signing_field')
    size = struct.unpack_from('<I', data)[0]
    if size > len(data)-4:
        raise ValueError('truncated_signing_field')
    return data[4:4+size], data[4+size:]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    source = json.loads(args.source.read_text())
    url = source['url']; parsed = urlsplit(url)
    if (parsed.scheme != 'https' or parsed.hostname != 'g79.gdl.netease.com' or
            parsed.username or parsed.password):
        raise ValueError('unexpected_android_origin')
    args.output.mkdir(parents=True, exist_ok=False)
    rows = []
    with RemoteAPK(url) as remote:
        with zipfile.ZipFile(remote) as archive:
            central = archive.start_dir
        remote.seek(central-24); footer = remote.read(24)
        if footer[8:] != b'APK Sig Block 42':
            raise ValueError('apk_signing_block_missing')
        size = struct.unpack_from('<Q', footer)[0]
        if not 24 <= size <= 8*1024**2 or size+8 > central:
            raise ValueError('invalid_apk_signing_block_length')
        remote.seek(central-size-8); block = remote.read(size+8)
        if struct.unpack_from('<Q', block)[0] != size or block[-24:] != footer:
            raise ValueError('inconsistent_apk_signing_block')
        (args.output/'signing-block.bin').write_bytes(block)
        position = 8
        while position < len(block)-24:
            if position+8 > len(block)-24:
                raise ValueError('truncated_signing_pair')
            length = struct.unpack_from('<Q', block, position)[0]
            if not 4 <= length <= len(block)-24-position-8:
                raise ValueError('invalid_signing_pair')
            ident = struct.unpack_from('<I', block, position+8)[0]
            value = block[position+12:position+8+length]; position += 8+length
            if ident != 0x7109871a:  # v2 only; no claims about unparsed v3 fields.
                continue
            signers, trailing = length_prefixed(value)
            if trailing:
                raise ValueError('unexpected_v2_trailing_bytes')
            signer_number = 0
            while signers:
                signer, signers = length_prefixed(signers)
                signed, _ = length_prefixed(signer)
                _, remainder = length_prefixed(signed)
                certificates, _ = length_prefixed(remainder)
                cert_number = 0
                while certificates:
                    cert, certificates = length_prefixed(certificates)
                    name = f'v2-{signer_number}-{cert_number}.der'
                    (args.output/name).write_bytes(cert)
                    rows.append(dict(file=name, bytes=len(cert), md5=hashlib.md5(cert).hexdigest(),
                                     sha256=hashlib.sha256(cert).hexdigest()))
                    cert_number += 1
                signer_number += 1
    if not rows:
        raise ValueError('v2_certificates_not_found')
    report = dict(source=str(args.source), url=url, full_apk_signature_verified=False,
                  block_sha256=hashlib.sha256(block).hexdigest(), certificates=rows)
    (args.output/'CERTIFICATES.json').write_text(json.dumps(report, indent=2))
    print(json.dumps(dict(output=str(args.output), certificates=len(rows))))


if __name__ == '__main__':
    main()
