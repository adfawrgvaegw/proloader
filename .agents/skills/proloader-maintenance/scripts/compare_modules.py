#!/usr/bin/env python3
"""Compare already repaired pyc modules by qualified function name; no execution.

Use xdis 6.1.8. Reports all code-object metadata used by mcp_evidence and writes
focused disassembly files. Never treat this summary as automatic approval.
"""
import argparse
import hashlib
import json
from pathlib import Path
from xdis.load import load_module
from xdis.bytecode import Bytecode
from xdis import get_opcode_module
from mcp_evidence import MODULES, stable

FIELDS = ('co_code', 'co_consts', 'co_names', 'co_varnames', 'co_freevars', 'co_cellvars',
          'co_argcount', 'co_kwonlyargcount', 'co_posonlyargcount', 'co_flags',
          'co_nlocals', 'co_stacksize', 'co_name', 'co_exceptiontable')


def canonical(value):
    if hasattr(value, 'co_code'):
        # Nested functions are compared independently, avoiding shifted const indices.
        return {'function': value.co_name}
    if isinstance(value, bytes):
        return {'bytes_hex': value.hex()}
    if isinstance(value, (tuple, list)):
        return [canonical(x) for x in value]
    if isinstance(value, (set, frozenset)):
        return {'set': sorted([canonical(x) for x in value], key=stable)}
    if value is None or isinstance(value, (str, int, float, bool)):
        return value
    raise ValueError('unsupported_static_constant')


def functions(code, prefix='', label=None):
    path = prefix + '/' + (label or code.co_name)
    result = {path: code}
    children = [c for c in code.co_consts if hasattr(c, 'co_code')]
    counts = {}
    for child in children:
        counts[child.co_name] = counts.get(child.co_name, 0) + 1
    seen = {}
    for child in children:
        name = child.co_name
        seen[name] = seen.get(name, 0) + 1
        # Lambdas/comprehensions can legitimately share a qualified name.
        # Ordinal suffixes preserve every object; insertion requires manual review.
        label = name + ('#'+str(seen[name]) if counts[name] > 1 else '')
        result.update(functions(child, path, label))
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--before', type=Path, required=True)
    parser.add_argument('--after', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=False)
    rows = []
    sources = []
    for name in MODULES:
        before, after = args.before/name, args.after/name
        a, b = load_module(str(before)), load_module(str(after))
        sources.append(dict(module=name, before_sha256=hashlib.sha256(before.read_bytes()).hexdigest(),
                            after_sha256=hashlib.sha256(after.read_bytes()).hexdigest()))
        if a[0] != b[0]:
            raise ValueError('python_bytecode_version_changed')
        old, new = functions(a[3]), functions(b[3])
        opcode = get_opcode_module(b[0])
        for path in sorted(set(old) | set(new)):
            if path not in old or path not in new:
                fields = ['added' if path in new else 'removed']
            else:
                fields = [field for field in FIELDS if
                          canonical(getattr(old[path], field, None)) !=
                          canonical(getattr(new[path], field, None))]
            if not fields:
                continue
            row = dict(module=name, function=path, fields=fields)
            for label, mapping in [('before', old), ('after', new)]:
                if path in mapping:
                    file = hashlib.sha256((name+path).encode()).hexdigest()[:20]+'-'+label+'.txt'
                    (args.output/file).write_text(Bytecode(mapping[path], opcode).dis())
                    row[label+'_disassembly'] = file
            rows.append(row)
    report = dict(target_execution=False, sources=sources, changes=rows)
    (args.output/'DIFF.json').write_text(json.dumps(report, indent=2))
    print(json.dumps(dict(changed_functions=len(rows), report=str(args.output/'DIFF.json'))))


if __name__ == '__main__':
    main()
