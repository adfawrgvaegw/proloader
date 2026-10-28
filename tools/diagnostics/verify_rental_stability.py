"""Sequential first-attempt acceptance against the production rental lifecycle.

Password is read from stdin when --password-stdin is set; never stored in reports.
Stops on the first failed round. No automatic reconnection or world writes.
"""
import argparse
import asyncio
from datetime import datetime, timezone
import hashlib
import json
import logging
from pathlib import Path
import sys
import time

from proloader.bots import BotStore, private_json
from proloader.net.netease.runtime import rental_session


def source_digest(root=None):
    root = Path(root) if root is not None else Path.cwd()
    digest = hashlib.sha256()
    paths = sorted((root/'proloader').rglob('*.py')) + [root/'tools/diagnostics/verify_rental_stability.py']
    native = root/'third_party/fatalder'
    paths += sorted((native/'cmd/proloader-nbt').glob('*.go'))
    paths += [native/'go.mod', native/'go.sum']
    paths += sorted((root/'tools/mcpcheck').rglob('*.go'))
    paths += [root/'tools/mcpcheck/go.mod',root/'tools/mcpcheck/UPSTREAM.json',
              root/'tools/mcp_reverse/vmu/nls_cipher.py']
    for path in sorted(set(paths)):
        digest.update(str(path.relative_to(root)).encode())
        digest.update(path.read_bytes())
    return digest.hexdigest()


async def run(args, password):
    store = BotStore()
    bot = next(b for b in store.list() if b.id == args.bot)
    report = dict(started_at=datetime.now(timezone.utc).isoformat(), source_sha256=source_digest(),
                  server=args.server, bot=args.bot, protocol=860, profile='official-android-3915',
                  hold_seconds=args.hold, interval_seconds=args.interval,
                  reconnect_retries=0, rounds=[], passed=False)
    def save():
        private_json(args.output, report)
    save()
    for number in range(1, args.rounds+1):
        row = dict(number=number, started_at=datetime.now(timezone.utc).isoformat(),
                   events=[], passed=False, reconnect_retries=0)
        report['rounds'].append(row)
        def event(data):
            row['events'].append(data)
            save()
            if data['event'] != 'received':
                print(json.dumps(dict(round=number, **data), ensure_ascii=False), flush=True)
        started = time.monotonic()
        try:
            async with rental_session(store, bot, args.server, password, on_event=event) as (session, name):
                # Permission/online probe: privileged read-only command, no world mutation.
                output = await session.run_command_ai('testfor @s', timeout=10, dimensional=True, bot_name=name)
                if output is None:
                    raise RuntimeError('readonly_probe_no_response')
                event(dict(event='probe', command='testfor @s', response_received=True,
                           elapsed_s=round(time.monotonic()-started, 3)))
                until = time.monotonic()+args.hold
                while time.monotonic() < until:
                    await asyncio.sleep(min(1, until-time.monotonic()))
                    session._raise_if_failed()
                    if session._raknet._closed:
                        raise RuntimeError('transport_closed')
                output = await session.run_command_ai('testfor @s', timeout=10, dimensional=True, bot_name=name)
                if output is None:
                    raise RuntimeError('final_probe_no_response')
                event(dict(event='hold', seconds=args.hold, final_probe=True,
                           elapsed_s=round(time.monotonic()-started, 3)))
            cleanup = next((e for e in reversed(row['events']) if e['event']=='cleanup'), {})
            if not cleanup.get('socket_closed') or not cleanup.get('tasks_finished'):
                raise RuntimeError('incomplete_cleanup')
            row['passed'] = True
        except Exception as exc:
            # Store exception type only: arbitrary server messages may contain secrets.
            row['error_type'] = type(exc).__name__
            if str(exc) in ('readonly_probe_no_response', 'final_probe_no_response', 'transport_closed', 'incomplete_cleanup'):
                row['error_code'] = str(exc)
            print(f'Round {number} failed: {type(exc).__name__}', flush=True)
        row['elapsed_s'] = round(time.monotonic()-started, 3)
        save()
        if not row['passed']:
            return 1
        print(f'PASS {number}/{args.rounds}', flush=True)
        if number < args.rounds:
            await asyncio.sleep(args.interval)
    report['final_source_sha256'] = source_digest()
    if report['final_source_sha256'] != report['source_sha256']:
        report['error_code'] = 'source_changed_during_acceptance'
        save()
        return 1
    report['passed'] = True
    report['finished_at'] = datetime.now(timezone.utc).isoformat()
    save()
    return 0


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--bot', required=True)
    p.add_argument('--server', required=True)
    p.add_argument('--output', type=Path, required=True)
    p.add_argument('--rounds', type=int, default=10)
    p.add_argument('--hold', type=int, default=30)
    p.add_argument('--interval', type=int, default=10)
    p.add_argument('--password-stdin', action='store_true', required=True)
    a=p.parse_args()
    if a.output.exists() or a.rounds < 1 or a.hold < 30 or a.interval < 5:
        p.error('需要新输出文件、至少30秒在线、至少5秒间隔')
    password=sys.stdin.readline().rstrip('\r\n')
    logging.disable(logging.CRITICAL)
    return asyncio.run(run(a,password))

if __name__=='__main__':
    raise SystemExit(main())
