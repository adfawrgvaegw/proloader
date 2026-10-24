"""web/app.py —— 创新点3 服务端核心：FastAPI 应用。

提供：
    - REST API：加载建筑 / 启动导入 / 查询状态 / 回滚
    - WebSocket：实时推送方块放置事件 + 进度（3D 可视化的数据源）
    - 后台 asyncio.Task：运行 pipeline，事件流扇出到所有 WebSocket 订阅者

架构数据流：
    浏览器 ──HTTP──> FastAPI ──> WebState(建筑/会话)
    浏览器 <──WS──── FastAPI <── 事件转发器 <── Backend.events() <── Scheduler
                                          └──> pipeline.run_import (后台 Task)

事件转发器（_forward_backend_events）是关键创新：
    消费后端事件流，把细粒度的 BlockPlacedEvent 按批次聚合，
    既驱动 3D 高亮（每个方块），又驱动进度条（聚合统计），
    避免逐方块 WebSocket 消息淹没浏览器。

启动：
    proloader preview house.mcstructure
    python -m proloader.web
"""

from __future__ import annotations

import asyncio
import logging
import os
from pathlib import Path
from typing import List, Optional

# fastapi 类型需在模块级导入，因 `from __future__ import annotations` 使注解变字符串，
# FastAPI 用 get_type_hints() 解析时需在模块全局找到 WebSocket / UploadFile 等类型。
# app.py 仅在 Web 功能启用时被导入（CLI preview / python -m proloader.web），
# 未安装 fastapi 时此导入失败，由上层 try/except ImportError 捕获。
from fastapi import FastAPI, HTTPException, UploadFile, WebSocket, WebSocketDisconnect
from fastapi.responses import FileResponse
from fastapi.staticfiles import StaticFiles

from proloader.web.state import LoadedStructure, RunningSession, get_state

logger = logging.getLogger(__name__)

# 静态文件目录（Three.js 前端）
_STATIC_DIR = Path(__file__).parent / "static"

# 默认模拟世界尺寸
_DEFAULT_WORLD_SIZE = (256, 256, 256)


# ═══════════════════════════════════════════════════════════════════════════════
# FastAPI 应用工厂
# ═══════════════════════════════════════════════════════════════════════════════
def create_app(initial_structure=None) -> FastAPI:
    """创建 FastAPI 应用实例。

    Args:
        initial_structure: 可选，CLI preview 命令预加载的建筑
    """
    app = FastAPI(
        title="proloader",
        description="世界最好的网易我的世界租赁服建筑导入器 —— 3D 可视化控制台",
        version="0.1.0",
    )

    # 预加载建筑（CLI preview 用）
    if initial_structure is not None:
        get_state().add_structure(initial_structure)
        logger.info("预加载建筑: %s", initial_structure.name)

    # ── 静态文件（Three.js 前端）──────────────────────────────────────────────
    _STATIC_DIR.mkdir(parents=True, exist_ok=True)
    app.mount("/static", StaticFiles(directory=str(_STATIC_DIR)), name="static")

    # ── 页面路由 ─────────────────────────────────────────────────────────────
    @app.get("/")
    async def index():
        """3D 可视化主页面。"""
        return FileResponse(str(_STATIC_DIR / "index.html"))

    @app.get("/api/health")
    async def health():
        return {"status": "ok", "version": "0.1.0"}

    # ── 建筑 API ─────────────────────────────────────────────────────────────
    @app.get("/api/structures")
    async def list_structures():
        """列出所有已加载建筑。"""
        state = get_state()
        return [
            {
                "id": s.id,
                "name": s.structure.name,
                "size": list(s.structure.size),
                "block_count": s.structure.block_count,
                "summary": s.structure.summary(),
            }
            for s in state.list_structures()
        ]

    @app.get("/api/structures/{struct_id}")
    async def get_structure(struct_id: str):
        """获取建筑元数据 + 预渲染方块数据（供 Three.js 渲染）。"""
        state = get_state()
        loaded = state.get_structure(struct_id)
        if loaded is None:
            from fastapi import HTTPException
            raise HTTPException(status_code=404, detail=f"建筑 {struct_id} 不存在")
        return {
            "id": loaded.id,
            "name": loaded.structure.name,
            "summary": loaded.structure.summary(),
            "blocks_payload": loaded.blocks_payload,
        }

    @app.post("/api/structures/load")
    async def load_from_path(body: dict):
        """从服务器文件路径加载建筑（本地运行用）。

        Body: {"path": "/path/to/house.mcstructure"}
        """
        from fastapi import HTTPException
        from proloader.errors import UnsupportedFormatError
        from proloader.parser import load_structure

        path = body.get("path", "")
        if not path or not os.path.exists(path):
            raise HTTPException(status_code=400, detail=f"文件不存在: {path}")
        try:
            structure = load_structure(path)
        except UnsupportedFormatError as e:
            raise HTTPException(status_code=400, detail=str(e))
        loaded = get_state().add_structure(structure)
        logger.info("从路径加载建筑: %s -> %s", path, loaded.id)
        return {
            "id": loaded.id,
            "name": loaded.structure.name,
            "summary": loaded.structure.summary(),
            "block_count": loaded.structure.block_count,
        }

    @app.post("/api/structures/upload")
    async def upload_structure(file: UploadFile):
        """上传 .mcstructure 文件加载建筑。"""
        from proloader.errors import UnsupportedFormatError
        from proloader.parser import load_structure_from_bytes

        data = await file.read()
        name = Path(file.filename or "upload").stem
        ext = Path(file.filename or "").suffix
        try:
            structure = load_structure_from_bytes(data, ext, name)
        except UnsupportedFormatError as e:
            raise HTTPException(status_code=400, detail=str(e))
        loaded = get_state().add_structure(structure)
        return {
            "id": loaded.id,
            "name": loaded.structure.name,
            "summary": loaded.structure.summary(),
        }

    # ── 导入 API ─────────────────────────────────────────────────────────────
    @app.post("/api/structures/{struct_id}/import")
    async def start_import(struct_id: str, body: dict):
        """启动导入会话（后台异步执行）。

        Body:
            origin: [x, y, z]  放置原点
            num_bots: int       假人数量（默认 3）
            cell_size: int      切块大小（默认 16）
            use_diff: bool      差异增量（默认 true）
            place_delay: float  模拟延迟（默认 0）
        """
        from fastapi import HTTPException

        state = get_state()
        loaded = state.get_structure(struct_id)
        if loaded is None:
            raise HTTPException(status_code=404, detail=f"建筑 {struct_id} 不存在")

        origin = tuple(body.get("origin", [0, 0, 0]))
        num_bots = int(body.get("num_bots", 3))
        cell_size = int(body.get("cell_size", 16))
        use_diff = bool(body.get("use_diff", True))
        place_delay = float(body.get("place_delay", 0.0))

        # 创建 RunningSession
        session_id = state.new_id("sess")
        running = RunningSession(
            session_id=session_id,
            structure_id=struct_id,
            origin=list(origin),
            total=loaded.structure.block_count,
        )
        state.add_session(running)

        # 启动后台导入任务
        task = asyncio.create_task(
            _run_import_task(
                running,
                loaded.structure,
                origin,
                num_bots,
                cell_size,
                use_diff,
                place_delay,
            )
        )
        running.task = task
        logger.info("启动导入会话 %s（%d 假人）", session_id, num_bots)
        return running.status_payload()

    # ── 会话 API ─────────────────────────────────────────────────────────────
    @app.get("/api/sessions")
    async def list_sessions():
        """列出所有会话。"""
        state = get_state()
        return [s.status_payload() for s in state.list_sessions()]

    @app.get("/api/sessions/{sess_id}")
    async def get_session_status(sess_id: str):
        """获取会话状态。"""
        from fastapi import HTTPException

        state = get_state()
        session = state.get_session(sess_id)
        if session is None:
            raise HTTPException(status_code=404, detail=f"会话 {sess_id} 不存在")
        return session.status_payload()

    @app.post("/api/sessions/{sess_id}/cancel")
    async def cancel_session(sess_id: str):
        """取消运行中的会话。"""
        from fastapi import HTTPException

        state = get_state()
        session = state.get_session(sess_id)
        if session is None:
            raise HTTPException(status_code=404, detail=f"会话 {sess_id} 不存在")
        if session.task and not session.done:
            session.task.cancel()
            session.broadcast({"type": "cancelled", "session_id": sess_id})
        return {"session_id": sess_id, "cancelled": True}

    # ── WebSocket：实时事件流 ─────────────────────────────────────────────────
    @app.websocket("/ws/sessions/{sess_id}")
    async def session_ws(websocket: WebSocket, sess_id: str):
        """WebSocket 端点：实时推送导入进度 + 方块放置事件。

        客户端连接后：
            1. 服务端发送 status 首帧（当前快照）
            2. 持续推送 progress / blocks / error / done 事件
            3. 会话完成后发送 done 帧，客户端自行关闭
        """
        try:
            await websocket.accept()
        except RuntimeError as e:
            logger.warning("WebSocket accept 失败: %s", e)
            return

        state = get_state()
        session = state.get_session(sess_id)
        if session is None:
            await websocket.send_json({"type": "error", "error": f"会话 {sess_id} 不存在"})
            await websocket.close()
            return

        # 若会话已结束（导入极快时 WS 连接前就完成了），补发最终状态
        if session.done:
            await websocket.send_json({"type": "done", **session.status_payload()})
            await websocket.close()
            return

        queue = session.subscribe()

        # 首帧：当前状态快照
        await websocket.send_json({"type": "status", **session.status_payload()})

        try:
            while True:
                try:
                    event = await asyncio.wait_for(queue.get(), timeout=30.0)
                except asyncio.TimeoutError:
                    await websocket.send_json({"type": "heartbeat"})
                    continue
                await websocket.send_json(event)
                if event.get("type") in ("done", "cancelled") or (
                    event.get("type") == "error" and event.get("done")
                ):
                    await websocket.close()
                    break
        except WebSocketDisconnect:
            logger.info("WebSocket 客户端断开: %s", sess_id)
        except Exception as e:
            logger.exception("WebSocket 异常: %s", sess_id)
        finally:
            session.unsubscribe(queue)

    return app


# ═══════════════════════════════════════════════════════════════════════════════
# 后台导入任务 + 事件转发器
# ═══════════════════════════════════════════════════════════════════════════════
async def _run_import_task(
    session: RunningSession,
    structure,
    origin,
    num_bots: int,
    cell_size: int,
    use_diff: bool,
    place_delay: float,
) -> None:
    """后台导入任务：编排 pipeline + 事件转发。

    流程：
        1. 创建 SimulatedWorld + N 个 backend（共享世界）
        2. 启动事件转发器（消费 backend.events() → 广播到 WebSocket 订阅者）
        3. 运行 pipeline.run_import（差异 → 切块 → 调度 → journal）
        4. 等待转发器结束（backend emit_done 后终止）
        5. 广播最终 done 帧
    """
    from proloader.backend.simulated import SimulatedWorld
    from proloader.pipeline import make_simulated_backends, run_import
    from proloader.session import ImportOptions, ImportSession
    from proloader.snapshot.checkpoint import CheckpointManager

    try:
        world = SimulatedWorld(size=_DEFAULT_WORLD_SIZE)
        backends = make_simulated_backends(num_bots, world, place_delay=place_delay)

        for b in backends:
            await b.connect()

        session.total = structure.block_count
        session.broadcast({"type": "status", **session.status_payload()})

        # 每后端进度聚合（bid -> [placed, failed]）
        per_backend: dict = {}

        # 启动事件转发器（在 run_import 之前启动，确保不丢事件）
        forwarders = [
            asyncio.create_task(_forward_backend_events(b, session, per_backend))
            for b in backends
        ]

        # 构造导入会话并执行
        cm = CheckpointManager()
        options = ImportOptions(
            num_bots=num_bots,
            cell_size=cell_size,
            use_diff=use_diff,
            include_removes=True,
            place_delay=place_delay,
        )
        imp_session = ImportSession(
            structure=structure,
            origin=origin,
            backends=backends,
            options=options,
            checkpoint_manager=cm,
        )

        report = await run_import(imp_session)

        # 等待事件转发器排空（emit_done 后自然终止）
        await asyncio.gather(*forwarders, return_exceptions=True)

        # 最终统计
        session.placed = report.scheduler_report.placed_ops
        session.failed = report.scheduler_report.failed_ops
        session.elapsed = report.scheduler_report.elapsed
        session.per_backend = report.scheduler_report.per_backend
        session.done = True
        session.broadcast(
            {
                "type": "done",
                "session_id": session.session_id,
                "placed": session.placed,
                "failed": session.failed,
                "total": session.total,
                "elapsed": session.elapsed,
                "per_backend": session.per_backend,
            }
        )
        logger.info("导入会话 %s 完成: %s", session.session_id, report.scheduler_report.summary())

    except asyncio.CancelledError:
        session.done = True
        session.broadcast({"type": "cancelled", "session_id": session.session_id})
        raise
    except Exception as e:
        logger.exception("导入会话 %s 异常", session.session_id)
        session.error = str(e)
        session.done = True
        session.broadcast(
            {"type": "error", "error": str(e), "done": True, "session_id": session.session_id}
        )


async def _forward_backend_events(backend, session: RunningSession, per_backend: dict) -> None:
    """事件转发器：消费单个 backend 的事件流，聚合后广播。

    创新点：按批次聚合 BlockPlacedEvent，避免逐方块 WebSocket 消息。
    - BlockPlacedEvent → 缓冲到 block_buffer
    - ProgressEvent    → 连同缓冲的方块一起广播（一条消息含进度+方块列表）
    - ErrorEvent       → 立即广播（低频）
    - DoneEvent        → flush 残留 + 广播完成

    同时维护 per_backend 聚合计数，更新 session 的 placed/failed/total。
    """
    from proloader.backend.events import (
        BlockPlacedEvent,
        DoneEvent,
        ErrorEvent,
        ProgressEvent,
    )

    bid = backend.backend_id
    block_buffer: List[list] = []

    async for event in backend.events():
        if isinstance(event, BlockPlacedEvent):
            # 缓冲方块放置事件（[x, y, z, op_type]）
            block_buffer.append([event.x, event.y, event.z, event.op_type.value])

        elif isinstance(event, ErrorEvent):
            # 错误立即广播（通常低频）
            session.broadcast(
                {
                    "type": "block_error",
                    "backend_id": bid,
                    "error": event.error,
                    "coord": list(event.coord) if event.coord else None,
                }
            )

        elif isinstance(event, ProgressEvent):
            # 更新聚合进度
            per_backend[bid] = [event.placed, event.failed]
            session.placed = sum(v[0] for v in per_backend.values())
            session.failed = sum(v[1] for v in per_backend.values())

            # 连同缓冲方块一起广播（前端用于 3D 高亮 + 进度条）
            session.broadcast(
                {
                    "type": "progress",
                    "backend_id": bid,
                    "placed": session.placed,
                    "failed": session.failed,
                    "total": session.total,
                    "blocks": block_buffer,
                }
            )
            block_buffer = []

        elif isinstance(event, DoneEvent):
            # flush 残留方块
            per_backend[bid] = [event.placed, event.failed]
            session.placed = sum(v[0] for v in per_backend.values())
            session.failed = sum(v[1] for v in per_backend.values())
            if block_buffer:
                session.broadcast(
                    {
                        "type": "progress",
                        "backend_id": bid,
                        "placed": session.placed,
                        "failed": session.failed,
                        "total": session.total,
                        "blocks": block_buffer,
                    }
                )
                block_buffer = []
            session.broadcast(
                {
                    "type": "backend_done",
                    "backend_id": bid,
                    "placed": event.placed,
                    "failed": event.failed,
                }
            )


# ═══════════════════════════════════════════════════════════════════════════════
# 启动入口
# ═══════════════════════════════════════════════════════════════════════════════
async def run_web(
    structure=None, host: str = "127.0.0.1", port: int = 8000
) -> None:
    """启动 Web 服务（CLI preview 命令调用）。

    Args:
        structure: 可选，预加载的建筑 Structure 对象
        host: 监听地址
        port: 监听端口
    """
    import uvicorn

    app = create_app(initial_structure=structure)
    # ws='wsproto' 对各类 WebSocket 客户端（浏览器 / websockets v15 / wsproto）兼容性最佳
    config = uvicorn.Config(app, host=host, port=port, log_level="info", ws="wsproto")
    server = uvicorn.Server(config)
    logger.info("proloader Web 可视化启动: http://%s:%d", host, port)
    await server.serve()


def main() -> None:
    """python -m proloader.web 入口。"""
    import argparse

    parser = argparse.ArgumentParser(prog="proloader.web", description="3D 可视化控制台")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=8000)
    parser.add_argument("--file", default=None, help="预加载建筑文件路径")
    args = parser.parse_args()

    structure = None
    if args.file:
        from proloader.parser import load_structure

        structure = load_structure(args.file)
        print(f"预加载建筑: {structure}")

    asyncio.run(run_web(structure=structure, host=args.host, port=args.port))


if __name__ == "__main__":
    main()
