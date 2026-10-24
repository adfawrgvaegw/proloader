"""Local bot identities. Private material stays in 0700 directories / 0600 files."""
from __future__ import annotations
from dataclasses import dataclass
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import tempfile
import uuid
from contextlib import contextmanager


def private_json(path: Path, data) -> None:
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    fd, temporary = tempfile.mkstemp(dir=path.parent, prefix='.write-')
    try:
        os.fchmod(fd, 0o600)
        with os.fdopen(fd, 'w', encoding='utf-8') as stream:
            json.dump(data, stream, ensure_ascii=False, indent=2)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def validate_cookie(cookie: str) -> dict:
    try:
        outer = json.loads(cookie)
        sauth = json.loads(outer['sauth_json'])
        if sauth.get('gameid') != 'x19' or not all(isinstance(sauth.get(k), str) and sauth[k]
                                                 for k in ('sdkuid', 'sessionid', 'deviceid', 'udid')):
            raise ValueError
        return outer
    except (ValueError, TypeError, KeyError):
        raise ValueError('登录材料格式无效，请重新添加机器人。') from None


@dataclass(frozen=True)
class BotProfile:
    id: str
    label: str
    game_name: str = ''


class BotStore:
    def __init__(self, root: Path | None = None):
        self.root = Path(root) if root else Path.home() / '.proloader' / 'bots'
        self.root.mkdir(parents=True, exist_ok=True, mode=0o700)

    def _dir(self, bot_id):
        if len(bot_id) != 32 or any(c not in '0123456789abcdef' for c in bot_id):
            raise ValueError('无效机器人编号。')
        return self.root / bot_id

    def list(self) -> list[BotProfile]:
        result = []
        for path in sorted(self.root.glob('*/profile.json')):
            try:
                data = json.loads(path.read_text())
                self._dir(data['id'])
                result.append(BotProfile(data['id'], data['label'], data.get('game_name', '')))
            except (OSError, ValueError, KeyError, TypeError):
                continue
        return result

    def cookie(self, bot: BotProfile) -> str:
        text = (self._dir(bot.id) / 'cookie.json').read_text()
        validate_cookie(text)
        return text

    def pe(self, bot: BotProfile) -> dict | None:
        path = self._dir(bot.id) / 'pe.json'
        if not path.exists():
            return None
        try:
            data = json.loads(path.read_text())
            if data.get('ok') is True and data.get('auth_profile') == 'official-android-3915':
                return data
        except (OSError, ValueError):
            pass
        return None

    def save_pe(self, bot: BotProfile, data: dict):
        private_json(self._dir(bot.id) / 'pe.json', data)
        profile = json.loads((self._dir(bot.id) / 'profile.json').read_text())
        profile['game_name'] = data['credentials']['user_detail'].get('name', '')
        private_json(self._dir(bot.id) / 'profile.json', profile)

    def add(self, label: str, cookie: str, pe: dict | None = None) -> BotProfile:
        outer = validate_cookie(cookie)
        sdkuid = json.loads(outer['sauth_json'])['sdkuid']
        for existing in self.list():
            current = json.loads(json.loads(self.cookie(existing))['sauth_json'])
            if current['sdkuid'] == sdkuid:
                if pe:
                    self.save_pe(existing, pe)
                return existing
        bot = BotProfile(uuid.uuid4().hex, label.strip() or '建筑助手')
        directory = self._dir(bot.id)
        directory.mkdir(mode=0o700)
        private_json(directory / 'cookie.json', outer)
        private_json(directory / 'profile.json', dict(id=bot.id, label=bot.label,
                    created_at=datetime.now(timezone.utc).isoformat(), game_name=''))
        if pe:
            self.save_pe(bot, pe)
        return next(p for p in self.list() if p.id == bot.id)

    def remove(self, bot: BotProfile):
        # Remove only the known identity files; never follow a user-supplied path.
        directory = self._dir(bot.id)
        if self.is_busy(bot):
            raise ValueError('机器人正在执行任务，请等待任务结束后再移除。')
        for name in ('cookie.json', 'pe.json', 'profile.json', '.lease'):
            (directory / name).unlink(missing_ok=True)
        directory.rmdir()

    @contextmanager
    def lease(self, bot: BotProfile):
        import fcntl
        fd=os.open(self._dir(bot.id)/'.lease',os.O_CREAT|os.O_RDWR,0o600)
        try:
            try:
                fcntl.flock(fd,fcntl.LOCK_EX|fcntl.LOCK_NB)
            except BlockingIOError:
                raise ValueError('这个机器人正在执行另一个任务，请等它结束或选择其他机器人。') from None
            yield
        finally:
            os.close(fd)

    def is_busy(self,bot):
        try:
            with self.lease(bot):return False
        except ValueError:return True
