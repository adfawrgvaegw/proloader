"""PlacementJournal —— 创新点4 核心：WAL 式放置日志。

原理（Write-Ahead Logging）：
    每个 SubTask 完成后追加一条记录到 JSONL 文件。
    重启时读 journal 知道哪些 SubTask 已完成，跳过它们 = 断点续传。

    每条记录 fsync 后才认为该 SubTask 完成（durability）。
    这样进程崩溃时，已确认的 SubTask 不会丢失，未确认的会重做。

格式（JSONL，每行一条）：
    {"ts":..., "subtask_id":7, "bbox":[...], "backend":"bot-1",
     "ops_count":1234, "placed":1234, "failed":0, "status":"done"}

学习要点：
    - WAL 是数据库/文件系统保证持久性的经典技术
    - "先写日志再执行"或"执行完写日志"：这里用后者（执行完确认）
    - resume 时用 journal 的 completed_subtask_ids 跳过已完成任务
"""

from __future__ import annotations

import json
import os
import time
from pathlib import Path
from typing import List, Set

from proloader.sched.task import SubTask, SubTaskStatus


class PlacementJournal:
    """放置日志（WAL）。

    用法：
        journal = PlacementJournal(path)
        journal.append(subtask)  # SubTask 完成后调用
        done = journal.completed_subtask_ids()  # resume 时调用
    """

    def __init__(self, path: str | Path) -> None:
        self.path = Path(path)
        self.path.parent.mkdir(parents=True, exist_ok=True)
        # 缓存已完成的 subtask id（避免每次读全文件）
        self._completed: Set[int] = set()
        self._loaded = False

    def _ensure_loaded(self) -> None:
        """惰性加载已完成的 subtask id。"""
        if self._loaded:
            return
        self._completed = self._read_completed()
        self._loaded = True

    def _read_completed(self) -> Set[int]:
        """读取所有已完成的 subtask id。"""
        if not self.path.exists():
            return set()
        result: Set[int] = set()
        with self.path.open("r", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                try:
                    record = json.loads(line)
                    if record.get("status") == SubTaskStatus.DONE.value:
                        result.add(int(record["subtask_id"]))
                except (json.JSONDecodeError, KeyError, ValueError):
                    continue
        return result

    def append(self, subtask: SubTask) -> None:
        """追加一条 SubTask 完成记录。

        fsync 保证持久性（进程崩溃不丢失已确认记录）。
        """
        self._ensure_loaded()
        record = {
            "ts": time.time(),
            "subtask_id": subtask.id,
            "bbox": list(subtask.bbox),
            "backend": subtask.assigned_backend,
            "ops_count": subtask.ops_count,
            "placed": subtask.placed,
            "failed": subtask.failed,
            "status": subtask.status.value,
        }
        with self.path.open("a", encoding="utf-8") as f:
            f.write(json.dumps(record, ensure_ascii=False) + "\n")
            f.flush()
            os.fsync(f.fileno())
        if subtask.status == SubTaskStatus.DONE:
            self._completed.add(subtask.id)

    def completed_subtask_ids(self) -> Set[int]:
        """返回已完成的 subtask id 集合（resume 时跳过这些）。"""
        self._ensure_loaded()
        return set(self._completed)

    def filter_pending(self, subtasks: List[SubTask]) -> List[SubTask]:
        """过滤掉已完成的 SubTask（断点续传用）。"""
        done = self.completed_subtask_ids()
        return [st for st in subtasks if st.id not in done]

    def records(self) -> List[dict]:
        """读取所有记录（用于报告/调试）。"""
        if not self.path.exists():
            return []
        result = []
        with self.path.open("r", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if line:
                    try:
                        result.append(json.loads(line))
                    except json.JSONDecodeError:
                        continue
        return result

    def reset(self) -> None:
        """清空日志（新会话或回滚后）。"""
        self._completed = set()
        self._loaded = True
        if self.path.exists():
            self.path.unlink()
