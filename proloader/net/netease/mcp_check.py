"""Reviewed dynamic check profile; only static bytecode decoding, never execution."""
import base64
from functools import lru_cache
import hashlib
from pathlib import Path
import subprocess
from proloader.native_build import go_build_environment
import zlib

from tools.mcp_reverse.vmu.nls_cipher import NlsCipher

ROOT = Path(__file__).resolve().parents[3]


@lru_cache(maxsize=1)
def binary():
    folder = ROOT / 'tools/mcpcheck'
    digest = hashlib.sha256()
    for path in sorted(folder.rglob('*')):
        if path.is_file():
            digest.update(str(path.relative_to(folder)).encode());digest.update(path.read_bytes())
    target = ROOT / 'cache' / ('mcpcheck-' + digest.hexdigest()[:16])
    if not target.exists():
        result = subprocess.run(['go','build','-mod=readonly','-o',str(target),'.'],
                                cwd=folder,env=go_build_environment(),capture_output=True,timeout=120)
        if result.returncode:raise ValueError('MCP静态解码工具构建失败')
    return target


def solve(module_hex, salt, unique_id, engine, patch):
    if not isinstance(module_hex,str) or not 0 < len(module_hex) <= 128*1024:
        raise ValueError('MCP校验模块长度无效')
    if not isinstance(salt,str) or not salt or len(salt)>4096 or type(unique_id) is not int:
        raise ValueError('MCP校验参数无效')
    data = NlsCipher().decrypt(bytes.fromhex(module_hex))
    decoder = zlib.decompressobj()
    data = decoder.decompress(data,128*1024+1)
    if len(data)>128*1024 or not decoder.eof or decoder.unconsumed_tail or decoder.unused_data:
        raise ValueError('MCP校验模块压缩格式无效')
    data = bytearray(data)
    for i in range(min(130,len(data))):data[i]^=156
    data.reverse()
    from proloader.world_codec import request
    result = request(binary(),dict(marshal=base64.b64encode(data).decode(),salt=salt,
                                  unique_id=unique_id,engine=engine,patch=patch))
    answer=result.get('response')
    if not isinstance(answer,list) or len(answer)!=8:
        raise ValueError('MCP校验返回格式无效')
    return answer,result['template_sha256']
