"""Pinned Fatalder/WaterStructure palette conversion, isolated behind a Go bridge.

No unknown block is silently replaced with air. Mapper source/version and AGPL
notice are retained in tools/blockmapper and THIRD_PARTY_NOTICES.md.
"""
from __future__ import annotations
import json
import os
from pathlib import Path
import shutil
import subprocess

from proloader.errors import ParseError

REVISION = '54ab0b294dfe+iron-chain-v1'
ROOT = Path(__file__).resolve().parents[2]
BRIDGE = ROOT / 'tools' / 'blockmapper'


def _map_request(request) -> dict:
    binary = ROOT / 'cache' / 'blockmapper'
    sources = [BRIDGE / 'main.go', BRIDGE / 'go.mod']
    if not binary.exists() or any(p.stat().st_mtime > binary.stat().st_mtime for p in sources):
        go = shutil.which('go')
        if not go:
            raise ParseError('首次转换 Java 建筑需要安装 Go，或使用已转换的 .mcstructure 文件。')
        binary.parent.mkdir(parents=True, exist_ok=True)
        env = dict(os.environ)
        proc = subprocess.run([go, 'build', '-mod=readonly', '-o', str(binary), '.'],
                              cwd=BRIDGE, env=env, capture_output=True, timeout=180)
        if proc.returncode:
            raise ParseError('方块转换器未能构建；请在 tools/blockmapper 运行 go mod download 后重试。')
    proc = subprocess.run([str(binary)], input=json.dumps(request), text=True,
                          capture_output=True, timeout=60)
    if proc.returncode:
        raise ParseError('方块状态转换失败，未生成导入计划。')
    try:
        result = json.loads(proc.stdout)
        if not isinstance(result, dict):raise ValueError
        return result
    except (ValueError, TypeError):
        raise ParseError('方块转换器返回了无效数据。') from None


def map_schematic_palette(pairs: list[tuple[int, int]]) -> dict:
    result = _map_request({'schematic': pairs})
    missing = [f'{bid}:{data}' for bid, data in pairs
               if not result.get(f'{bid}:{data}', {}).get('found')]
    if missing:
        raise ParseError('无法转换的旧版方块 ID:Data（含模组/扩展ID不支持）：' + '、'.join(missing[:12]))
    return result


def map_java_palette(entries: list[str]) -> dict:
    result = _map_request(entries)
    try:
        missing = [entry for entry in entries if not result.get(entry, {}).get('found')]
        if missing:
            raise ParseError('无法转换的方块：' + '、'.join(missing[:12]))
        if any(result[e]['name'] == 'minecraft:air' and not e.startswith(('minecraft:air', 'minecraft:cave_air', 'minecraft:void_air')) for e in entries):
            raise ParseError('转换器将非空气方块映为空气，已停止导入。')
        return result
    except (ValueError, KeyError, TypeError):
        raise ParseError('方块转换器返回了无效数据。') from None
