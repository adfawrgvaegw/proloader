"""Web 会话状态管理。

管理加载的建筑、运行中的导入会话、WebSocket 事件总线。

创新点3 的数据中枢：
    - LoadedStructure: 预渲染方块数据供 Three.js 首屏渲染
    - RunningSession: 后台导入会话 + pub/sub 事件广播（多 WebSocket 订阅者共享同一事件流）

设计要点：
    - 一个导入会话可被多个浏览器标签页同时订阅（pub/sub）
    - 每个 subscriber 拥有独立 asyncio.Queue，互不干扰
    - broadcast() 把事件扇出到所有订阅者队列
    - 事件序列化在前端/后端约定 JSON 格式（见 web/app.py 的 _serialize_event）
"""

from __future__ import annotations

import asyncio
from dataclasses import dataclass, field
from typing import Dict, List, Optional

from proloader.model.structure import Structure


@dataclass
class LoadedStructure:
    """已加载的建筑（供 Web 渲染）。

    Attributes:
        id: 结构 id（如 struct-0001）
        structure: 原始 Structure 对象
        blocks_payload: 预渲染方块数据（坐标+颜色+名称），给 Three.js InstancedMesh
    """

    id: str
    structure: Structure
    blocks_payload: dict


@dataclass
class RunningSession:
    """运行中的导入会话。

    持有后台 asyncio.Task 和 pub/sub 事件总线。
    多个 WebSocket 连接可同时 subscribe() 同一会话，各自收到完整事件流副本。

    Attributes:
        session_id: 会话 id
        structure_id: 关联的建筑 id
        origin: 放置原点 [x, y, z]
        placed/failed/total: 进度统计（由后台任务实时更新）
        done: 是否已完成
        task: 后台 asyncio.Task（run_import 协程）
        subscribers: 订阅者队列列表（pub/sub 扇出）
    """

    session_id: str
    structure_id: str
    origin: List[int]
    placed: int = 0
    failed: int = 0
    total: int = 0
    done: bool = False
    error: Optional[str] = None
    elapsed: float = 0.0
    per_backend: dict = field(default_factory=dict)
    task: Optional[asyncio.Task] = None
    subscribers: List["asyncio.Queue"] = field(default_factory=list)

    # ── pub/sub 事件总线 ─────────────────────────────────────────────────────
    def subscribe(self) -> "asyncio.Queue":
        """订阅会话事件流。返回独立队列，调用方异步读取。

        每个 WebSocket 连接调用一次，断开时调用 unsubscribe()。
        """
        q: asyncio.Queue = asyncio.Queue()
        self.subscribers.append(q)
        return q

    def unsubscribe(self, q: "asyncio.Queue") -> None:
        """取消订阅。"""
        if q in self.subscribers:
            self.subscribers.remove(q)

    def broadcast(self, event: dict) -> None:
        """向所有订阅者广播事件（非阻塞，put_nowait）。

        队列满时丢弃旧事件以避免阻塞导入主流程（实时可视化允许丢帧）。
        """
        for q in self.subscribers:
            try:
                q.put_nowait(event)
            except asyncio.QueueFull:
                # 队列满：丢弃最旧的事件再放新的（实时渲染允许丢帧）
                try:
                    q.get_nowait()
                    q.put_nowait(event)
                except asyncio.QueueEmpty:
                    pass

    def status_payload(self) -> dict:
        """返回当前会话状态快照（供 REST 轮询 / WebSocket 首帧）。"""
        return {
            "session_id": self.session_id,
            "structure_id": self.structure_id,
            "origin": self.origin,
            "placed": self.placed,
            "failed": self.failed,
            "total": self.total,
            "done": self.done,
            "error": self.error,
            "elapsed": self.elapsed,
            "per_backend": self.per_backend,
        }


class WebState:
    """Web 全局状态（单例）。

    持有所有已加载建筑和运行中会话。FastAPI app 启动时创建一次。
    """

    def __init__(self) -> None:
        self.structures: Dict[str, LoadedStructure] = {}
        self.sessions: Dict[str, RunningSession] = {}
        self._counter = 0

    def new_id(self, prefix: str = "id") -> str:
        self._counter += 1
        return f"{prefix}-{self._counter:04d}"

    def add_structure(self, structure: Structure) -> LoadedStructure:
        """加载建筑并预渲染方块数据。"""
        sid = self.new_id("struct")
        loaded = LoadedStructure(
            id=sid,
            structure=structure,
            blocks_payload=_build_blocks_payload(structure),
        )
        self.structures[sid] = loaded
        return loaded

    def get_structure(self, sid: str) -> Optional[LoadedStructure]:
        return self.structures.get(sid)

    def list_structures(self) -> List[LoadedStructure]:
        return list(self.structures.values())

    def add_session(self, session: RunningSession) -> None:
        self.sessions[session.session_id] = session

    def get_session(self, sid: str) -> Optional[RunningSession]:
        return self.sessions.get(sid)

    def list_sessions(self) -> List[RunningSession]:
        return list(self.sessions.values())


def _build_blocks_payload(structure: Structure) -> dict:
    """构建给 Three.js 渲染的方块数据。

    为减少传输体积与渲染开销，采用"调色板 + 索引数组"压缩：
        - palette: 去重的 [color, name] 列表
        - blocks: 每个非空气方块的 [x, y, z, palette_index]
    前端用 InstancedMesh 按调色板分组渲染，远比逐方块传色高效。

    返回:
        {size, block_count, palette: [{color, name}], blocks: [[x,y,z,idx], ...]}
    """
    palette_list: List[dict] = []
    palette_index: Dict[str, int] = {}
    blocks_list = []
    ox, oy, oz = structure.region.offset
    for lx, ly, lz, block in structure.region.iter_non_air():
        name = block.state.name
        if name not in palette_index:
            from proloader.parser.blockmap import get_block_color

            color = get_block_color(name)
            palette_index[name] = len(palette_list)
            palette_list.append(
                {"color": [color[0], color[1], color[2]], "name": name}
            )
        blocks_list.append([lx + ox, ly + oy, lz + oz, palette_index[name]])
    return {
        "size": list(structure.size),
        "block_count": len(blocks_list),
        "palette": palette_list,
        "blocks": blocks_list,
    }


# 全局单例
_state: Optional[WebState] = None


def get_state() -> WebState:
    global _state
    if _state is None:
        _state = WebState()
    return _state


def reset_state() -> None:
    """重置全局状态（测试用）。"""
    global _state
    _state = None
