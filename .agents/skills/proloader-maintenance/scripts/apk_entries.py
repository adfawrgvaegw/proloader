#!/usr/bin/env python3
"""Read selected official APK ZIP entries over Range. Never install/run the APK.

Writes provenance including CRC32/MD5/SHA256. This does not verify the APK's
whole-file signing scheme. Pass the landing page retained by maintain.py.
"""
import argparse
import hashlib
import io
import json
from pathlib import Path
import re
from urllib.parse import urlsplit
import zipfile

import requests


class RemoteAPK(io.RawIOBase):
    def __init__(self, url):
        self.session = requests.Session()
        self.url, self.position = url, 0
        body, total = self.fetch(0, 0)
        self.size = total
        if not body or not 0 < total <= 8 * 1024**3:
            raise ValueError('invalid_apk_size')

    def fetch(self, start, end):
        response = self.session.get(self.url, headers={'Range': f'bytes={start}-{end}',
                                   'Accept-Encoding': 'identity'}, timeout=(20, 60))
        response.raise_for_status()
        match = re.fullmatch(r'bytes (\d+)-(\d+)/(\d+)', response.headers.get('Content-Range', ''))
        if (response.status_code != 206 or not match or
                (int(match[1]), int(match[2])) != (start, end) or
                len(response.content) != end - start + 1):
            raise ValueError('invalid_http_range_response')
        total = int(match[3])
        if hasattr(self, 'size') and total != self.size:
            raise ValueError('apk_changed_during_read')
        return response.content, total

    def tell(self):
        return self.position

    def seek(self, offset, whence=0):
        self.position = offset if whence == 0 else (self.position if whence == 1 else self.size) + offset
        if self.position < 0:
            raise ValueError('negative_seek')
        return self.position

    def seekable(self):
        return True

    def read(self, size=-1):
        size = self.size - self.position if size < 0 else min(size, self.size - self.position)
        if size <= 0:
            return b''
        if size > 256 * 1024**2:
            raise ValueError('range_read_too_large')
        body, _ = self.fetch(self.position, self.position + size - 1)
        self.position += len(body)
        return body

    def close(self):
        self.session.close()
        super().close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--landing-file', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True, help='New evidence directory')
    parser.add_argument('--entry', action='append', default=[])
    args = parser.parse_args()
    match = re.search(r'var android_link = android_type \? "([^"\n]+)"', args.landing_file.read_text())
    if not match:
        raise ValueError('official_android_url_missing')
    url = match[1]
    parsed = urlsplit(url)
    if (parsed.scheme != 'https' or parsed.hostname != 'g79.gdl.netease.com' or
            parsed.username or parsed.password):
        raise ValueError('unexpected_android_origin')
    args.output.mkdir(parents=True, exist_ok=False)
    rows = []
    source = dict(url=url, landing_sha256=hashlib.sha256(args.landing_file.read_bytes()).hexdigest(),
                  target_executed=False, full_apk_signature_verified=False, entries=rows)
    with RemoteAPK(url) as remote, zipfile.ZipFile(remote) as archive:
        source['apk_bytes'] = remote.size
        index = [dict(name=i.filename, offset=i.header_offset, compressed_bytes=i.compress_size,
                      bytes=i.file_size, crc32=f'{i.CRC:08x}', method=i.compress_type)
                 for i in archive.infolist()]
        (args.output / 'INDEX.json').write_text(json.dumps(index, indent=2))
        for name in args.entry:
            # Explicit nested paths are allowed; traversal and absolute names are not.
            if Path(name).is_absolute() or '..' in Path(name).parts or '\\' in name:
                raise ValueError('unsafe_entry_path')
            info = archive.getinfo(name)
            if not 0 <= info.file_size <= 512 * 1024**2:
                raise ValueError('entry_too_large')
            target = args.output / name
            target.parent.mkdir(parents=True, exist_ok=True)
            md5, sha = hashlib.md5(), hashlib.sha256()
            count = 0
            # zipfile verifies the uncompressed CRC at EOF.
            with archive.open(info) as entry, target.open('xb') as out:
                while True:
                    block = entry.read(1024 * 1024)
                    if not block:
                        break
                    count += len(block)
                    out.write(block); md5.update(block); sha.update(block)
            if count != info.file_size:
                raise ValueError('entry_length_mismatch')
            rows.append(dict(name=name, bytes=count, md5=md5.hexdigest(), sha256=sha.hexdigest()))
        (args.output / 'SOURCE.json').write_text(json.dumps(source, indent=2))
    print(json.dumps(dict(output=str(args.output), extracted=len(rows))))


if __name__ == '__main__':
    main()
