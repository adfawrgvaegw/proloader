"""CheckpointManager —— 创新点4：断点续传状态机。

会话状态机：
    INIT → SNAPSHOT_TAKEN → RUNNING → (PAUSED | FAILED | DONE)
    任一非 DONE 状态都可通过 resume() 继续。

resume 流程：
    1. 读 snapshot + journal
    2. 已完成的 sub-task 跳过（journal.completed_subtask_ids）
    3. 未完成的 sub-task 重新入队
    4. 重新启动 scheduler.run(remaining_subtasks)

会话目录结构：
    {sessions_dir}/{session_id}/
        meta.json       # 会话元数据（状态、建筑信息、origin 等）
        snapshot.npz    # 导入前快照
        journal.jsonl   # WAL 放置日志
"""

from __future__ import annotations

import json
import time
from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path
from typing import List, Optional

from proloader.errors import SessionNotFound, SessionStateError
from proloader.sched.task import SubTask
from proloader.snapshot.journal import PlacementJournal
from proloader.snapshot.snapshot import RegionSnapshot


class SessionStatus(str, Enum):
    """会话状态。"""

    INIT = "init"
    SNAPSHOT_TAKEN = "snapshot_taken"
    RUNNING = "running"
    PAUSED = "paused"
    FAILED = "failed"
    DONE = "done"


@dataclass
class SessionMeta:
    """会话元数据（存 meta.json）。"""

    session_id: str
    status: SessionStatus = SessionStatus.INIT
    structure_name: str = ""
    origin: List[int] = field(default_factory=lambda: [0, 0, 0])
    size: List[int] = field(default_factory=lambda: [0, 0, 0])
    total_subtasks: int = 0
    total_ops: int = 0
    created_at: float = field(default_factory=time.time)
    updated_at: float = field(default_factory=time.time)
    backend_type: str = "simulated"
    num_bots: int = 1

    def to_dict(self) -> dict:
        d = self.__dict__.copy()
        d["status"] = self.status.value
        return d

    @classmethod
    def from_dict(cls, d: dict) -> "SessionMeta":
        d = dict(d)
        d["status"] = SessionStatus(d.get("status", "init"))
        return cls(**d)


class CheckpointManager:
    """会话检查点管理器。

    管理会话目录、元数据、快照、日志，提供状态转换和 resume 支持。
    """

    def __init__(self, sessions_dir: str | Path = "~/.proloader/sessions") -> None:
        self.sessions_dir = Path(sessions_dir).expanduser()
        self.sessions_dir.mkdir(parents=True, exist_ok=True)

    def session_dir(self, session_id: str) -> Path:
        return self.sessions_dir / session_id

    def meta_path(self, session_id: str) -> Path:
        return self.session_dir(session_id) / "meta.json"

    def snapshot_path(self, session_id: str) -> Path:
        return self.session_dir(session_id) / "snapshot.npz"

    def journal_path(self, session_id: str) -> Path:
        return self.session_dir(session_id) / "journal.jsonl"

    def create_session(self, meta: SessionMeta) -> None:
        """创建新会话。"""
        d = self.session_dir(meta.session_id)
        d.mkdir(parents=True, exist_ok=True)
        self.save_meta(meta)

    def save_meta(self, meta: SessionMeta) -> None:
        """保存/更新会话元数据。"""
        meta.updated_at = time.time()
        self.meta_path(meta.session_id).write_text(
            json.dumps(meta.to_dict(), ensure_ascii=False, indent=2), encoding="utf-8"
        )

    def load_meta(self, session_id: str) -> SessionMeta:
        """加载会话元数据。"""
        p = self.meta_path(session_id)
        if not p.exists():
            raise SessionNotFound(f"会话 {session_id} 不存在")
        return SessionMeta.from_dict(json.loads(p.read_text(encoding="utf-8")))

    def save_snapshot(self, session_id: str, snapshot: RegionSnapshot) -> None:
        snapshot.save(self.snapshot_path(session_id))

    def load_snapshot(self, session_id: str) -> RegionSnapshot:
        return RegionSnapshot.load(self.snapshot_path(session_id))

    def get_journal(self, session_id: str) -> PlacementJournal:
        return PlacementJournal(self.journal_path(session_id))

    def filter_pending_subtasks(self, session_id: str, subtasks: List[SubTask]) -> List[SubTask]:
        """过滤掉已完成的 SubTask（断点续传）。"""
        journal = self.get_journal(session_id)
        return journal.filter_pending(subtasks)

    def transition(self, session_id: str, new_status: SessionStatus) -> SessionMeta:
        """状态转换（带合法性检查）。"""
        meta = self.load_meta(session_id)
        legal = {
            SessionStatus.INIT: {SessionStatus.SNAPSHOT_TAKEN, SessionStatus.FAILED},
            SessionStatus.SNAPSHOT_TAKEN: {SessionStatus.RUNNING, SessionStatus.FAILED},
            SessionStatus.RUNNING: {
                SessionStatus.PAUSED,
                SessionStatus.FAILED,
                SessionStatus.DONE,
            },
            SessionStatus.PAUSED: {SessionStatus.RUNNING, SessionStatus.DONE, SessionStatus.FAILED},
            SessionStatus.FAILED: {SessionStatus.RUNNING},  # resume
            SessionStatus.DONE: set(),
        }
        if new_status not in legal.get(meta.status, set()):
            raise SessionStateError(
                f"非法状态转换: {meta.status.value} → {new_status.value}"
            )
        meta.status = new_status
        self.save_meta(meta)
        return meta

    def can_resume(self, session_id: str) -> bool:
        """会话是否可 resume。"""
        try:
            meta = self.load_meta(session_id)
        except SessionNotFound:
            return False
        return meta.status in {
            SessionStatus.PAUSED,
            SessionStatus.FAILED,
            SessionStatus.RUNNING,
        }

    def list_sessions(self) -> List[SessionMeta]:
        """列出所有会话。"""
        result = []
        for d in self.sessions_dir.iterdir():
            if d.is_dir() and self.meta_path(d.name).exists():
                try:
                    result.append(self.load_meta(d.name))
                except Exception:
                    continue
        return result

    def reset_journal(self, session_id: str) -> None:
        """清空 journal（回滚后重新导入用）。"""
        self.get_journal(session_id).reset()
