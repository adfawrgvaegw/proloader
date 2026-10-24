"""创新点3 测试：3D Web 可视化。

测试 WebState (pub/sub 事件总线) / FastAPI REST API / WebSocket 实时事件流。
"""

from __future__ import annotations

import asyncio

import pytest
from starlette.websockets import WebSocketDisconnect

from proloader.web.app import create_app, _run_import_task
from proloader.web.state import (
    LoadedStructure,
    RunningSession,
    WebState,
    get_state,
    reset_state,
)


# ── WebState pub/sub ──────────────────────────────────────────────────────────
class TestWebState:
    def setup_method(self):
        reset_state()

    def test_add_structure(self, simple_structure):
        state = get_state()
        loaded = state.add_structure(simple_structure)
        assert loaded.id.startswith("struct-")
        assert loaded.blocks_payload["block_count"] == simple_structure.block_count
        # 调色板压缩
        assert len(loaded.blocks_payload["palette"]) > 0
        # 方块数据格式 [x,y,z,palette_idx]
        first = loaded.blocks_payload["blocks"][0]
        assert len(first) == 4

    def test_subscribe_unsubscribe(self):
        """pub/sub：多订阅者各自收到事件。"""
        session = RunningSession(
            session_id="test-sess", structure_id="s1", origin=[0, 0, 0]
        )
        q1 = session.subscribe()
        q2 = session.subscribe()
        assert len(session.subscribers) == 2

        session.broadcast({"type": "progress", "placed": 10})
        assert q1.get_nowait() == {"type": "progress", "placed": 10}
        assert q2.get_nowait() == {"type": "progress", "placed": 10}

        session.unsubscribe(q1)
        assert len(session.subscribers) == 1

    def test_status_payload(self):
        session = RunningSession(
            session_id="s1", structure_id="struct-1", origin=[1, 2, 3], total=100
        )
        session.placed = 50
        session.done = True
        payload = session.status_payload()
        assert payload["placed"] == 50
        assert payload["total"] == 100
        assert payload["done"] is True
        assert payload["origin"] == [1, 2, 3]

    def test_blocks_payload_palette_compression(self, simple_structure):
        """方块数据使用调色板压缩（相同方块共享索引）。"""
        state = get_state()
        loaded = state.add_structure(simple_structure)
        palette = loaded.blocks_payload["palette"]
        blocks = loaded.blocks_payload["blocks"]
        # 所有 palette_idx 都在合法范围
        for b in blocks:
            assert 0 <= b[3] < len(palette)


# ── FastAPI REST API ──────────────────────────────────────────────────────────
class TestRestAPI:
    def setup_method(self):
        reset_state()

    def test_health(self, simple_structure):
        from fastapi.testclient import TestClient

        app = create_app(initial_structure=simple_structure)
        client = TestClient(app)
        r = client.get("/api/health")
        assert r.status_code == 200
        assert r.json()["status"] == "ok"

    def test_list_structures(self, simple_structure):
        from fastapi.testclient import TestClient

        app = create_app(initial_structure=simple_structure)
        client = TestClient(app)
        r = client.get("/api/structures")
        assert r.status_code == 200
        data = r.json()
        assert len(data) == 1
        assert data[0]["name"] == "simple"
        assert data[0]["block_count"] == simple_structure.block_count

    def test_get_structure_detail(self, simple_structure):
        from fastapi.testclient import TestClient

        app = create_app(initial_structure=simple_structure)
        client = TestClient(app)
        sid = client.get("/api/structures").json()[0]["id"]
        r = client.get(f"/api/structures/{sid}")
        assert r.status_code == 200
        detail = r.json()
        assert "blocks_payload" in detail
        assert detail["blocks_payload"]["block_count"] == simple_structure.block_count

    def test_get_nonexistent_structure(self, simple_structure):
        from fastapi.testclient import TestClient

        app = create_app(initial_structure=simple_structure)
        client = TestClient(app)
        r = client.get("/api/structures/nonexistent")
        assert r.status_code == 404

    def test_start_import(self, simple_structure):
        from fastapi.testclient import TestClient

        app = create_app(initial_structure=simple_structure)
        client = TestClient(app)
        sid = client.get("/api/structures").json()[0]["id"]
        r = client.post(f"/api/structures/{sid}/import", json={
            "origin": [0, 0, 0], "num_bots": 2, "use_diff": True, "place_delay": 0.0,
        })
        assert r.status_code == 200
        sess = r.json()
        assert sess["total"] == simple_structure.block_count
        assert "session_id" in sess

    def test_list_sessions(self, simple_structure):
        from fastapi.testclient import TestClient

        app = create_app(initial_structure=simple_structure)
        client = TestClient(app)
        sid = client.get("/api/structures").json()[0]["id"]
        client.post(f"/api/structures/{sid}/import", json={"origin": [0, 0, 0], "place_delay": 0.0})
        r = client.get("/api/sessions")
        assert r.status_code == 200
        assert len(r.json()) >= 1


# ── WebSocket 实时事件流 ──────────────────────────────────────────────────────
class TestWebSocket:
    def setup_method(self):
        reset_state()

    def test_ws_done_for_completed_session(self, simple_structure):
        """会话已完成时 WS 连接补发 done 帧。"""
        from fastapi.testclient import TestClient

        app = create_app(initial_structure=simple_structure)
        client = TestClient(app)
        sid = client.get("/api/structures").json()[0]["id"]
        # 启动极快导入
        r = client.post(f"/api/structures/{sid}/import", json={
            "origin": [0, 0, 0], "num_bots": 1, "place_delay": 0.0,
        })
        sess_id = r.json()["session_id"]

        # 等待完成
        import time
        time.sleep(0.1)

        # WS 连接时会话已完成 → 补发 done
        with client.websocket_connect(f"/ws/sessions/{sess_id}") as ws:
            msg = ws.receive_json()
            assert msg["type"] in ("done", "status")


# ── 后台导入任务 ──────────────────────────────────────────────────────────────
class TestImportTask:
    def setup_method(self):
        reset_state()

    async def test_run_import_task(self, simple_structure):
        """直接调用 _run_import_task 验证事件转发。"""
        state = get_state()
        loaded = state.add_structure(simple_structure)
        session = RunningSession(
            session_id="test-task",
            structure_id=loaded.id,
            origin=[0, 0, 0],
            total=simple_structure.block_count,
        )
        state.add_session(session)

        # 订阅事件
        q = session.subscribe()

        await _run_import_task(
            session, loaded.structure, (0, 0, 0), 3, 16, True, 0.001
        )

        assert session.done is True
        assert session.placed == simple_structure.block_count
        assert session.error is None
        assert session.elapsed > 0

        # 验证收到了事件
        events = []
        while not q.empty():
            events.append(q.get_nowait())
        types = [e["type"] for e in events]
        assert "done" in types
        # 应该有 progress 事件
        assert "progress" in types or "backend_done" in types
