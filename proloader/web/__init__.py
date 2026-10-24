"""proloader.web —— 创新点3：3D Web 可视化（FastAPI + Three.js）。

提供：
    - app.py: FastAPI 应用（REST API + WebSocket 实时事件流 + 后台导入编排）
    - state.py: Web 会话状态管理（pub/sub 事件总线）
    - static/: Three.js 3D 可视化前端

启动方式：
    proloader preview <file.mcstructure>          # CLI 预览
    python -m proloader.web                        # 直接启动空 Web（再上传文件）
"""

from __future__ import annotations

from proloader.web.state import (
    LoadedStructure,
    RunningSession,
    WebState,
    get_state,
    reset_state,
)

__all__ = [
    "LoadedStructure",
    "RunningSession",
    "WebState",
    "get_state",
    "reset_state",
]
