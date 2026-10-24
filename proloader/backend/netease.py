"""NeteaseBackend —— 网易租赁服后端（Phase 10 实现）。

基于 BedrockProtocolBackend + 网易认证链，连接网易代理的基岩版租赁服。

实现路径（Phase 10）：
    1. auth/  网易认证（移植 g79client，github.com/UCKETX/g79client）
       - crypto.py: AES-CBC + Token 算法（移植 crypto.go）
       - release.py: 抓取 ReleaseJSON（版本/服务器地址）
       - pe_login.py: peUserLoginAfter 流程
       - domain_server.py: domain_server_get_detail 获取租赁服 IP:port
       - chain.py: 组装认证链（client chain）
       - credentials.py: --token-file / --cookie-file 入口
    2. backend/netease.py  BedrockProtocolBackend + 网易认证链 + 网易方块状态编码
       （UseNeteaseBlockStates，参考 bedrock-world-operator）

⚠️ 网易认证无法全自动（滑块验证码、设备指纹、风控）：
    - 账号密码登录：NotImplementedError，提示用 --token-file / --cookie-file
    - token 文件：用户抓包获取（手机端登录请求）
    - cookie 文件：浏览器登录网易 MC 官网后导出

当前为骨架。MVP 用 SimulatedBackend。
"""

from __future__ import annotations

from typing import AsyncIterator, Optional

from proloader.backend.events import (
    BackendCapabilities,
    BackendCheckpoint,
    PlacementEvent,
    PlacementOp,
)
from proloader.errors import CredentialsRequiredError, NotSupportedError
from proloader.model.block import Block
from proloader.model.region import BoundingBox, Region


class NeteaseBackend:
    """网易租赁服后端（Phase 10 实现）。

    一个实例 = 一个网易租赁服内的假人。

    Args:
        server_id: 租赁服编号（如 "123456"）
        password: 租赁服密码
        backend_id: 后端唯一标识
        credentials: 网易账号凭证（token 文件 / cookie 文件路径）

    Note:
        Phase 10 实现。需要用户提供凭证（token/cookie），无法全自动登录。
        详见 docs/09_netease_auth.md。
    """

    def __init__(
        self,
        server_id: str,
        password: str = "",
        backend_id: str = "netease-bot-0",
        token_file: Optional[str] = None,
        cookie_file: Optional[str] = None,
    ) -> None:
        self._server_id = server_id
        self._password = password
        self._bid = backend_id
        self._token_file = token_file
        self._cookie_file = cookie_file
        self._connected = False

    @property
    def backend_id(self) -> str:
        return self._bid

    @property
    def capabilities(self) -> BackendCapabilities:
        return BackendCapabilities(
            can_read_region=True,
            needs_teleport=True,
            needs_chunking=True,
            max_blocks_per_batch=256,
            load_radius=64,
        )

    async def connect(self) -> None:
        """网易认证 + RakNet 握手 + Login。

        Phase 10 实现流程：
            1. 加载凭证（token_file 或 cookie_file）
            2. auth/pe_login.py: peUserLoginAfter → 拿到 entity_id + seed
            3. auth/domain_server.py: domain_server_get_detail → 租赁服 IP:port
            4. auth/chain.py: 组装网易认证链（client chain JWT）
            5. BedrockProtocolBackend.connect() 走 RakNet 握手
            6. 发送含网易认证链的 LoginPacket

        ⚠️ 需要用户提供凭证，否则抛 CredentialsRequiredError。
        """
        if self._token_file is None and self._cookie_file is None:
            raise CredentialsRequiredError(
                "网易认证需要凭证。请通过 --token-file 或 --cookie-file 提供。\n"
                "  - token 文件：手机端抓包获取（详见 docs/09_netease_auth.md）\n"
                "  - cookie 文件：浏览器登录网易 MC 官网后导出\n"
                "账号密码自动登录涉及风控，不支持。"
            )
        raise NotImplementedError(
            "NeteaseBackend 尚未实现（Phase 10）。"
            "MVP 阶段请使用 SimulatedBackend。"
            "实现路径见 auth/ 和 docs/09_netease_auth.md。"
        )

    async def disconnect(self) -> None:
        raise NotImplementedError("Phase 10 实现")

    async def read_region(self, box: BoundingBox) -> Region:
        """读取网易租赁服区域（LevelChunkPacket + 网易方块状态解码）。

        Phase 10 实现：网易用 UseNeteaseBlockStates 编码 sub-chunk，
        需要 bedrock-world-operator 的方块 ID 表解码。
        """
        raise NotSupportedError("NeteaseBackend.read_region 尚未实现（Phase 10）")

    async def place_block(self, x: int, y: int, z: int, block: Block) -> None:
        raise NotImplementedError("Phase 10 实现")

    async def place_batch(self, ops: list[PlacementOp]) -> int:
        raise NotImplementedError("Phase 10 实现")

    async def teleport(self, x: float, y: float, z: float) -> None:
        raise NotImplementedError("Phase 10 实现")

    async def set_orientation(self, yaw: float, pitch: float) -> None:
        raise NotImplementedError("Phase 10 实现")

    async def events(self) -> AsyncIterator[PlacementEvent]:
        raise NotImplementedError("Phase 10 实现")

    async def checkpoint(self) -> BackendCheckpoint:
        raise NotImplementedError("Phase 10 实现")

    async def restore(self, cp: BackendCheckpoint) -> None:
        raise NotImplementedError("Phase 10 实现")
