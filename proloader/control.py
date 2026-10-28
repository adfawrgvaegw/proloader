"""proloader 控制器（对齐 fatalder control.Control + control/task/build_task.go）。

职责：
1. Control：聚合完整的租赁服登录 + Bedrock 连接流程
   - Cookie → PE 认证 → 搜索租赁服 → 进入世界 → Link+GameStart → JWT chain
   - RakNet 握手 → Bedrock 登录 → 游戏初始化 → MCP 处理
2. BuildTask：建筑导入任务
   - 解析 .mcstructure/.schem/.litematic 建筑文件
   - 合并 fill 算法（贪心长方体合并）
   - aiCommand 通道执行（fire-and-forget）
   - 完整性校验（fatalder 四级：None/Air/Similar/Complete）

用法：
    from proloader.config import ControlConfig, auto_refresh_cookie
    from proloader.control import Control, BuildTask

    cfg = ControlConfig(rental_server_code="12345678")
    ctl = Control(cfg)
    cookie = auto_refresh_cookie()
    bot_name = await ctl.enter_rental_server(cookie)
    bedrock = ctl.bedrock_session

    task = BuildTask(file_path="house.mcstructure", start_pos=(200,100,200))
    report = await task.run(bedrock, bot_name=bot_name)
"""

from __future__ import annotations

import asyncio
import gzip
import json
import logging
import time
from dataclasses import asdict, dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Any, Callable, Dict, List, Optional, Tuple

import numpy as np

from proloader.block_names import command_state
from proloader.config import STORAGE_ROOT, ControlConfig, CookieData, save_last_server
from proloader.model.block import Block, BlockState
from proloader.model.palette import BlockPalette
from proloader.model.region import Region
from proloader.model.structure import Structure
from proloader.net.bedrock.crypto import generate_keypair, serialize_public_key_b64
from proloader.net.bedrock.session import BedrockSession
from proloader.net.raknet.connection import handshake
from proloader.net.raknet.session import RakNetSession
from proloader.net.netease.client import NeteaseClient, RentalServerEntity
from proloader.parser.nbt import parse_nbt_auto

# 网易协议常量（与 test_netease_bedrock.py 一致）
NETEASE_RAKNET_PROTOCOL: int = 8          # RakNet 协议版本（网易租赁服特定）
NETEASE_BEDROCK_PROTOCOL: int = 860       # Bedrock 协议版本（网易魔改）

logger = logging.getLogger(__name__)


# ═══════════════════════════════════════════════════════════════════════════════
# 完整性校验等级（对齐 fatalder VerifyChunkLevel）
# ═══════════════════════════════════════════════════════════════════════════════
VERIFY_LEVEL_NONE: int = 1      # 不校验
VERIFY_LEVEL_AIR: int = 2       # 只校验空气方块（漏放检测，最轻量）
VERIFY_LEVEL_SIMILAR: int = 3   # 校验前 10% 随机方块
VERIFY_LEVEL_COMPLETE: int = 4  # 全量校验（逐方块 testforblock，很慢但最可靠）


# ═══════════════════════════════════════════════════════════════════════════════
# 断点续传：导入会话 + 进度持久化
# ═══════════════════════════════════════════════════════════════════════════════
SESSIONS_DIR = STORAGE_ROOT / "sessions"
_SAVE_INTERVAL: int = 50  # 每发送 50 条命令保存一次进度


@dataclass
class ImportSession:
    """导入会话（断点续传用）。

    进度文件存在 ~/.proloader/sessions/{session_id}.json。
    恢复时重新解析建筑 + 重新生成命令列表（算法确定性），从 sent_commands 继续。
    """
    session_id: str
    file_path: str
    start_pos: Tuple[int, int, int]
    speed: int
    verify_level: int
    bot_name: str
    total_commands: int
    sent_commands: int = 0
    status: str = "running"  # running / interrupted / completed
    created_at: str = ""
    updated_at: str = ""

    def to_dict(self) -> dict:
        d = asdict(self)
        d["start_pos"] = list(self.start_pos)
        return d

    @classmethod
    def from_dict(cls, d: dict) -> "ImportSession":
        d = dict(d)
        d["start_pos"] = tuple(d.get("start_pos", [0, 0, 0]))
        return cls(**{k: d.get(k) for k in cls.__dataclass_fields__})


def _ensure_sessions_dir() -> None:
    SESSIONS_DIR.mkdir(parents=True, exist_ok=True)


def save_session(session: ImportSession) -> Path:
    """保存导入会话到 JSON 文件。"""
    _ensure_sessions_dir()
    session.updated_at = datetime.now().isoformat()
    path = SESSIONS_DIR / f"{session.session_id}.json"
    path.write_text(json.dumps(session.to_dict(), ensure_ascii=False, indent=2))
    return path


def load_session(session_id: str) -> Optional[ImportSession]:
    """加载导入会话。"""
    path = SESSIONS_DIR / f"{session_id}.json"
    if not path.exists():
        return None
    try:
        return ImportSession.from_dict(json.loads(path.read_text()))
    except (json.JSONDecodeError, KeyError, TypeError):
        return None


def list_interrupted_sessions() -> List[ImportSession]:
    """列出所有中断未完成的导入会话。"""
    _ensure_sessions_dir()
    sessions = []
    for p in SESSIONS_DIR.glob("*.json"):
        try:
            s = ImportSession.from_dict(json.loads(p.read_text()))
            if s.status == "interrupted":
                sessions.append(s)
        except Exception:
            continue
    sessions.sort(key=lambda s: s.updated_at, reverse=True)
    return sessions


def list_all_sessions() -> List[ImportSession]:
    """列出所有导入会话（按更新时间倒序）。"""
    _ensure_sessions_dir()
    sessions = []
    for p in SESSIONS_DIR.glob("*.json"):
        try:
            sessions.append(ImportSession.from_dict(json.loads(p.read_text())))
        except Exception:
            continue
    sessions.sort(key=lambda s: s.updated_at, reverse=True)
    return sessions


def delete_session(session_id: str) -> bool:
    """删除导入会话文件。"""
    path = SESSIONS_DIR / f"{session_id}.json"
    if path.exists():
        path.unlink()
        return True
    return False


# ═══════════════════════════════════════════════════════════════════════════════
# 方块状态修复（对齐 fatalder nbt_parser/block/block_states_fixer.go）
# ═══════════════════════════════════════════════════════════════════════════════
# 特殊方块类型（对齐 fatalder mapping.SupportNBTBlockType*）
BLOCK_TYPE_DEFAULT: int = 0
BLOCK_TYPE_CONTAINER: int = 1    # 桶/箱子/漏斗/投掷器等
BLOCK_TYPE_FRAME: int = 2        # 物品展示框
BLOCK_TYPE_LECTERN: int = 3      # 讲台
BLOCK_TYPE_CRAFTER: int = 4      # 合成器
BLOCK_TYPE_COMMAND: int = 5      # 命令方块
BLOCK_TYPE_STRUCTURE: int = 6    # 结构方块
BLOCK_TYPE_BREWING: int = 7      # 酿造台
BLOCK_TYPE_JUKEBOX: int = 8      # 唱片机
BLOCK_TYPE_BANNER: int = 9       # 旗帜

# 方块名 → 特殊类型映射（对齐 fatalder mapping.blockNameToType）
_BLOCK_TYPE_MAP: Dict[str, int] = {
    # Container
    "minecraft:barrel": BLOCK_TYPE_CONTAINER, "minecraft:hopper": BLOCK_TYPE_CONTAINER,
    "minecraft:dispenser": BLOCK_TYPE_CONTAINER, "minecraft:dropper": BLOCK_TYPE_CONTAINER,
    "minecraft:chest": BLOCK_TYPE_CONTAINER, "minecraft:trapped_chest": BLOCK_TYPE_CONTAINER,
    "minecraft:shulker_box": BLOCK_TYPE_CONTAINER, "minecraft:undyed_shulker_box": BLOCK_TYPE_CONTAINER,
    "minecraft:brewing_stand": BLOCK_TYPE_BREWING,
    # Frame
    "minecraft:frame": BLOCK_TYPE_FRAME, "minecraft:glow_frame": BLOCK_TYPE_FRAME,
    # Lectern
    "minecraft:lectern": BLOCK_TYPE_LECTERN,
    # Crafter
    "minecraft:crafter": BLOCK_TYPE_CRAFTER,
    # Command
    "minecraft:command_block": BLOCK_TYPE_COMMAND,
    "minecraft:chain_command_block": BLOCK_TYPE_COMMAND,
    "minecraft:repeating_command_block": BLOCK_TYPE_COMMAND,
    # Structure
    "minecraft:structure_block": BLOCK_TYPE_STRUCTURE,
    # Jukebox
    "minecraft:jukebox": BLOCK_TYPE_JUKEBOX,
    # Banner
    "minecraft:standing_banner": BLOCK_TYPE_BANNER, "minecraft:wall_banner": BLOCK_TYPE_BANNER,
}


def get_block_type(block_name: str) -> int:
    """获取方块的特殊类型（对齐 fatalder mapping.GetBlockType）。"""
    return _BLOCK_TYPE_MAP.get(block_name, BLOCK_TYPE_DEFAULT)


def deepcopy_and_fix_states(block_name: str, states: Dict[str, Any]) -> Dict[str, Any]:
    """深拷贝并修复方块状态（对齐 fatalder DeepCopyAndFixStates）。

    修复不可能的方块状态（如红石激活、容器打开等），因为这些状态在
    导入时不应该保留（实际环境中没有红石信号）。
    """
    result = dict(states)  # 浅拷贝即可（值都是标量）
    block_type = get_block_type(block_name)

    if block_type == BLOCK_TYPE_CONTAINER:
        if block_name == "minecraft:barrel":
            result["open_bit"] = False
        elif block_name == "minecraft:hopper":
            result["toggle_bit"] = False
        elif block_name in ("minecraft:dispenser", "minecraft:dropper"):
            result["triggered_bit"] = False
    elif block_type == BLOCK_TYPE_FRAME:
        result["item_frame_map_bit"] = False
        result["item_frame_photo_bit"] = False
    elif block_type == BLOCK_TYPE_LECTERN:
        result["powered_bit"] = False
    elif block_type == BLOCK_TYPE_CRAFTER:
        result["crafting"] = False
        result["triggered_bit"] = False

    return result


def need_check_completely(block_name: str) -> bool:
    """该方块是否需要完整性校验（对齐 fatalder NeedCheckCompletely）。

    返回 True 的方块类型：command_block, structure_block, crafter,
    brewing_stand, jukebox, container（箱子等含 NBT 数据的方块）。
    DefaultBlock（普通方块）返回 False。
    """
    block_type = get_block_type(block_name)
    return block_type in (
        BLOCK_TYPE_COMMAND, BLOCK_TYPE_STRUCTURE, BLOCK_TYPE_CRAFTER,
        BLOCK_TYPE_BREWING, BLOCK_TYPE_JUKEBOX, BLOCK_TYPE_CONTAINER,
    )


# ═══════════════════════════════════════════════════════════════════════════════
# Control：完整连接流程
# ═══════════════════════════════════════════════════════════════════════════════
@dataclass
class ConnectionReport:
    """连接报告。"""
    host: str = ""
    port: int = 0
    user_id: str = ""
    user_name: str = ""
    server_id: str = ""
    runtime_id: int = 0
    elapsed_sec: float = 0.0


class Control:
    """控制器（对齐 fatalder control.Control）。

    管理从 Cookie 到 Bedrock 连接的完整生命周期。
    提供机器人上线/下线的脚本控制 API。
    """

    def __init__(self, config: ControlConfig) -> None:
        self.config: ControlConfig = config
        self._netease_client: Optional[NeteaseClient] = None
        self._raknet_conn: Optional[Any] = None
        self._raknet_session: Optional[RakNetSession] = None
        self.bedrock_session: Optional[BedrockSession] = None
        self._priv_key: Optional[Any] = None
        self._server_entity: Optional[RentalServerEntity] = None
        self.connected: bool = False
        self.connection_report: Optional[ConnectionReport] = None

    async def enter_rental_server(
        self,
        cookie: CookieData,
        bot_platform: str = "ios",
        login_timeout: float = 20.0,
        mcp_wait: float = 8.0,
    ) -> ConnectionReport:
        """执行完整的登录链路（= bot_online）。

        Returns:
            ConnectionReport（包含假人昵称 user_name + 连接元数据）
        """
        t0 = time.time()
        if self.connected:
            raise RuntimeError("Control.enter_rental_server：已连接，请先 close()")

        # 1. PE 认证
        nc = NeteaseClient(platform=bot_platform)
        nc.bootstrap()
        nc.authenticate_with_cookie(cookie.to_cookie_str())
        user_id = nc.user_id
        user_name = nc._get_display_name()
        self._netease_client = nc

        # 2. 搜索租赁服 + 进入世界 + Link + GameStart
        server_code = self.config.rental_server_code
        server_passcode = self.config.rental_server_passcode
        server_entity = nc.search_rental_server(server_code)
        self._server_entity = server_entity
        host, port = nc.enter_rental_server_world(server_entity.entity_id, server_passcode)
        nc.connect_link_and_send_game_start(server_code, server_entity)

        # 3. ECDH + authv2 → JWT chain
        priv = generate_keypair()
        self._priv_key = priv
        client_pub = serialize_public_key_b64(priv)
        authv2_data = nc.generate_rental_game_auth_v2(server_entity.entity_id, client_pub)
        jwt_chain = nc.send_auth_v2_request(authv2_data)

        # 4. RakNet 握手 + Bedrock 登录 + 初始化
        conn = await handshake(
            host=host, port=port, timeout=15.0,
            protocol=NETEASE_RAKNET_PROTOCOL,
        )
        raknet = RakNetSession(conn)
        await raknet.start()
        await raknet.connect()
        self._raknet_conn = conn
        self._raknet_session = raknet

        bedrock = BedrockSession(
            raknet,
            netease_mode=True,
            netease_jwt_chain=jwt_chain,
            netease_uid=int(user_id),
            netease_protocol=NETEASE_BEDROCK_PROTOCOL,
            priv_key=priv,
        )
        await bedrock.login(server_addr=f"{host}:{port}", timeout=login_timeout)
        start_game = await bedrock.initialize_game(timeout=30.0)
        self.bedrock_session = bedrock

        # 5. 等 MCP challenge 被 session 的 handler 处理
        await asyncio.sleep(mcp_wait)

        # 记住上次使用的服务器
        save_last_server(self.config)
        self.connected = True

        report = ConnectionReport(
            host=host, port=port,
            user_id=str(user_id), user_name=user_name,
            server_id=server_entity.entity_id,
            runtime_id=start_game.entity_runtime_id,
            elapsed_sec=time.time() - t0,
        )
        self.connection_report = report
        return report

    # ── 机器人上下线控制 API（用户要求：脚本里控制上线/下线）──────────────
    async def bot_online(
        self,
        cookie: Optional[CookieData] = None,
        bot_platform: str = "ios",
        login_timeout: float = 20.0,
        mcp_wait: float = 8.0,
    ) -> ConnectionReport:
        """机器人上线（enter_rental_server 的语义别名，自动读取 Cookie）。

        Args:
            cookie: CookieData；为 None 时自动调用 auto_refresh_cookie()
        Returns:
            ConnectionReport
        """
        if cookie is None:
            cookie = auto_refresh_cookie()
        return await self.enter_rental_server(
            cookie, bot_platform=bot_platform,
            login_timeout=login_timeout, mcp_wait=mcp_wait,
        )

    async def bot_offline(self) -> None:
        """机器人下线（close 的语义别名）。"""
        await self.close()

    async def close(self) -> None:
        """关闭所有连接。"""
        if self.bedrock_session is not None:
            try:
                await self.bedrock_session.close()
            except Exception:
                pass
        if self._raknet_session is not None:
            try:
                await self._raknet_session.close()
            except Exception:
                pass
        self.connected = False

    # ── 便捷命令接口（代理到 bedrock_session）──────────────────────────────
    async def run_command(self, command: str, **kwargs) -> Optional[str]:
        """执行命令（标准 CommandRequest 通道，可能被 MCP 拦截）。"""
        if not self.bedrock_session:
            raise RuntimeError("未连接")
        return await self.bedrock_session.run_command(command, **kwargs)

    async def run_command_ai(self, command: str, **kwargs) -> Optional[str]:
        """执行命令（aiCommand 通道，绕过 MCP）。

        如需使用相对坐标（dimensional），需传 bot_name=self.connection_report.user_name。
        """
        if not self.bedrock_session:
            raise RuntimeError("未连接")
        if "bot_name" not in kwargs and self.connection_report:
            kwargs["bot_name"] = self.connection_report.user_name
            kwargs.setdefault("dimensional", True)
        return await self.bedrock_session.run_command_ai(command, **kwargs)

    async def import_building(
        self,
        file_path: str,
        start_pos: Tuple[int, int, int],
        speed: int = 5000,
        verify_level: int = VERIFY_LEVEL_AIR,
        progress_cb: Optional[Callable] = None,
    ) -> "BuildReport":
        """便捷建筑导入（一步到位）。

        流程：
          1. tp 假人到 start_pos（确保相对坐标正确）
          2. 等待服务端处理 tp
          3. 执行合并 fill 导入
          4. 完整性校验
        """
        if not self.bedrock_session:
            raise RuntimeError("未连接")
        bot_name = self.connection_report.user_name if self.connection_report else ""

        # ★ 关键：先 tp 假人到起始坐标，否则相对坐标 ~dx ~dy ~dz 会放错位置
        ox, oy, oz = start_pos
        logger.info("tp 假人 %s 到起始坐标 (%d, %d, %d)", bot_name, ox, oy, oz)
        try:
            await self.bedrock_session.run_command_ai(
                f"tp {ox} {oy} {oz}",
                timeout=8.0, dimensional=True, bot_name=bot_name,
            )
        except Exception as e:
            logger.warning("tp 命令异常（继续尝试导入）: %s", e)
        await asyncio.sleep(2.0)  # 等服务端处理 tp

        task = BuildTask(
            file_path=file_path, start_pos=start_pos,
            speed=speed, verify_level=verify_level,
            progress_cb=progress_cb,
        )
        return await task.run(self.bedrock_session, bot_name=bot_name)

    async def resume_import(self, session_id: str) -> "BuildReport":
        """恢复中断的导入（断点续传）。

        Args:
            session_id: 导入会话 ID（从 list_interrupted_sessions 获取）
        """
        if not self.bedrock_session:
            raise RuntimeError("未连接")
        bot_name = self.connection_report.user_name if self.connection_report else ""

        # tp 假人到起始坐标（恢复前必须确保假人在正确位置）
        session = load_session(session_id)
        if session is None:
            raise ValueError(f"找不到会话: {session_id}")
        ox, oy, oz = session.start_pos
        logger.info("tp 假人 %s 到起始坐标 (%d, %d, %d)", bot_name, ox, oy, oz)
        try:
            await self.bedrock_session.run_command_ai(
                f"tp {ox} {oy} {oz}",
                timeout=8.0, dimensional=True, bot_name=bot_name,
            )
        except Exception as e:
            logger.warning("tp 命令异常（继续恢复）: %s", e)
        await asyncio.sleep(2.0)

        return await BuildTask.resume(session_id, self.bedrock_session, bot_name=bot_name)


# ═══════════════════════════════════════════════════════════════════════════════
# 建筑文件解析（.mcstructure / .schem 通吃）
# ═══════════════════════════════════════════════════════════════════════════════
def load_any_structure(file_path: str) -> Structure:
    """根据后缀自动选择解析器。

    支持：.bdx（BDX）、.mcstructure（基岩原生 NBT）、.schem（Sponge v2/v3）、.schematic（MCEdit Alpha / Sponge）
    """
    p = Path(file_path)
    if not p.exists():
        raise FileNotFoundError(f"建筑文件不存在: {file_path}")

    suffix = p.suffix.lower()
    raw = p.read_bytes()

    if suffix == ".mcstructure":
        from proloader.parser.mcstructure import parse_mcstructure_bytes
        return parse_mcstructure_bytes(raw, name=p.stem)

    if suffix in ('.bdx', '.schem', '.schematic'):
        from proloader.parser import load_structure
        return load_structure(p)

    # 未知后缀：尝试 gzip → NBT（Sponge 格式），否则试 NBT 直读
    try:
        return _parse_sponge_schem(raw, name=p.stem)
    except Exception:
        from proloader.parser.mcstructure import parse_mcstructure_bytes
        return parse_mcstructure_bytes(raw, name=p.stem)


def _parse_sponge_schem(raw: bytes, name: str) -> Structure:
    """Compatibility entry point using the strict registered Sponge reader."""
    from proloader.parser.sponge import parse_sponge
    return parse_sponge(raw, name)


# ═══════════════════════════════════════════════════════════════════════════════
# BuildTask：建筑导入任务（对齐 fatalder control/task/build_task.go）
# ═══════════════════════════════════════════════════════════════════════════════
@dataclass
class BuildTask:
    """建筑导入任务配置。"""
    file_path: str
    start_pos: Tuple[int, int, int]
    speed: int = 5000                       # 目标 BPS（实际决定 delay）
    verify_level: int = VERIFY_LEVEL_AIR    # 完整性校验等级
    show_progress: bool = True
    progress_cb: Optional[Callable[[int, int, str, str], None]] = None
    # 预解析的 Structure（可选，如已解析好就不再读文件）
    _structure: Optional[Structure] = field(default=None, init=False, repr=False)
    _report: Optional["BuildReport"] = field(default=None, init=False, repr=False)
    _session: Optional[ImportSession] = field(default=None, init=False, repr=False)

    @property
    def delay_sec(self) -> float:
        """根据目标 speed（块/秒）换算命令间隔。"""
        if self.speed <= 0: return 0.0
        blocks_per_cmd = 4.0
        return max(0.0, 1.0 / (self.speed / blocks_per_cmd))

    # ── 主入口（新建导入，带断点续传保护） ───────────────────────────────
    async def run(
        self,
        bedrock: BedrockSession,
        bot_name: str = "",
    ) -> "BuildReport":
        """执行导入 + 校验，返回报告。自动创建 ImportSession 支持断点续传。"""
        t0 = time.time()
        self._structure = load_any_structure(self.file_path)
        st = self._structure

        # 生成命令列表（确定性算法，恢复时可重新生成）
        commands = bedrock._generate_merged_commands(st, self.start_pos)
        total = len(commands)
        logger.info("生成 %d 条命令（合并 fill），开始导入...", total)

        # 创建导入会话
        session = ImportSession(
            session_id=datetime.now().strftime("%Y%m%d_%H%M%S"),
            file_path=self.file_path,
            start_pos=self.start_pos,
            speed=self.speed,
            verify_level=self.verify_level,
            bot_name=bot_name,
            total_commands=total,
            sent_commands=0,
            status="running",
            created_at=datetime.now().isoformat(),
        )
        save_session(session)
        self._session = session

        # 发送命令（带进度保存 + 中断捕获）
        try:
            covered = await self._send_commands(
                bedrock, commands, st, bot_name, session, start_index=0
            )
        except (KeyboardInterrupt, asyncio.CancelledError):
            session.status = "interrupted"
            save_session(session)
            logger.warning("⚠️ 导入中断！已发送 %d/%d 条命令。"
                          "可用 resume('%s') 恢复。",
                          session.sent_commands, total, session.session_id)
            raise
        except Exception as e:
            session.status = "interrupted"
            save_session(session)
            logger.error("导入异常中断: %s（可用 resume('%s') 恢复）",
                        e, session.session_id)
            raise

        session.status = "completed"
        save_session(session)
        import_sec = time.time() - t0

        # 完整性校验
        verify_result: Dict[str, Any] = {
            "level": self.verify_level,
            "checked": 0, "passed": 0, "failed": 0,
            "details": [],
        }
        if self.verify_level > VERIFY_LEVEL_NONE:
            verify_result = await self._verify(bedrock, bot_name, st, covered)

        report = BuildReport(
            file_path=self.file_path,
            size=st.size, block_count=st.block_count,
            palette_size=len(st.palette),
            commands_executed=total,
            covered_blocks=covered,
            import_sec=import_sec,
            verify=verify_result,
        )
        self._report = report
        return report

    # ── 断点恢复入口 ───────────────────────────────────────────────────
    @classmethod
    async def resume(
        cls,
        session_id: str,
        bedrock: BedrockSession,
        bot_name: str = "",
    ) -> "BuildReport":
        """从断点恢复导入。

        流程：
          1. 读取 ImportSession
          2. 重新解析建筑 + 重新生成命令列表（算法确定性）
          3. 用 testforblock 验证断点位置（往前检查 5 条）
          4. 从确认的断点继续发送
          5. 完成后校验
        """
        session = load_session(session_id)
        if session is None:
            raise ValueError(f"找不到会话: {session_id}")
        if session.status == "completed":
            raise ValueError(f"会话已完成: {session_id}")

        logger.info("恢复会话 %s: %d/%d 条命令已发送",
                    session_id, session.sent_commands, session.total_commands)

        # 重新解析建筑
        structure = load_any_structure(session.file_path)

        # 重新生成命令列表（确定性算法）
        commands = bedrock._generate_merged_commands(structure, session.start_pos)
        if len(commands) != session.total_commands:
            logger.warning("命令数不一致: %d vs %d（建筑文件可能已修改）",
                          len(commands), session.total_commands)

        # 验证断点位置
        actual_start = await cls._verify_checkpoint(
            bedrock, commands, session, bot_name
        )
        if actual_start < session.sent_commands:
            logger.info("断点回退: %d → %d（部分命令未执行）",
                       session.sent_commands, actual_start)
            session.sent_commands = actual_start
            save_session(session)

        # 从断点继续
        task = cls(
            file_path=session.file_path,
            start_pos=session.start_pos,
            speed=session.speed,
            verify_level=session.verify_level,
        )
        task._structure = structure
        task._session = session
        session.status = "running"
        save_session(session)

        t0 = time.time()
        try:
            covered = await task._send_commands(
                bedrock, commands, structure, bot_name, session,
                start_index=session.sent_commands,
            )
        except (KeyboardInterrupt, asyncio.CancelledError):
            session.status = "interrupted"
            save_session(session)
            logger.warning("⚠️ 恢复后再次中断！已发送 %d/%d 条命令。",
                          session.sent_commands, session.total_commands)
            raise

        session.status = "completed"
        save_session(session)
        import_sec = time.time() - t0

        # 校验
        verify_result: Dict[str, Any] = {
            "level": session.verify_level,
            "checked": 0, "passed": 0, "failed": 0,
        }
        if session.verify_level > VERIFY_LEVEL_NONE:
            verify_result = await task._verify(bedrock, bot_name, structure, covered)

        report = BuildReport(
            file_path=session.file_path,
            size=structure.size, block_count=structure.block_count,
            palette_size=len(structure.palette),
            commands_executed=len(commands),
            covered_blocks=covered,
            import_sec=import_sec,
            verify=verify_result,
        )
        task._report = report
        return report

    @staticmethod
    async def _verify_checkpoint(
        bedrock: BedrockSession,
        commands: list,
        session: ImportSession,
        bot_name: str,
    ) -> int:
        """验证断点位置：从 sent_commands 往前检查 5 条命令是否已执行。

        返回确认的断点索引（从该索引继续发送）。
        """
        if session.sent_commands == 0:
            return 0

        check_count = min(5, session.sent_commands)
        ox, oy, oz = session.start_pos

        for i in range(session.sent_commands - 1, session.sent_commands - 1 - check_count, -1):
            if i < 0:
                break
            cmd = commands[i]
            _, wx1, wy1, wz1, _, _, _, name, name_states = cmd

            dx, dy, dz = wx1 - ox, wy1 - oy, wz1 - oz
            def _r(d): return "~" if d == 0 else f"~{d}"
            rel = f"{_r(dx)} {_r(dy)} {_r(dz)}"

            try:
                probe_name, _ = command_state(name, name_states, probe=True)
                r = await bedrock.run_command_ai(
                    f"testforblock {rel} {probe_name} []",
                    timeout=6.0, dimensional=bool(bot_name), bot_name=bot_name,
                )
                if r is not None:
                    # 方块已放置 → 这条命令已执行 → 从下一条继续
                    logger.info("断点验证: 命令 #%d 已执行 ✓", i)
                    return i + 1
                else:
                    logger.info("断点验证: 命令 #%d 未执行 ✗", i)
            except Exception:
                logger.info("断点验证: 命令 #%d 验证失败 ✗", i)
                return i

        return session.sent_commands

    async def _send_commands(
        self,
        bedrock: BedrockSession,
        commands: list,
        st: Structure,
        bot_name: str,
        session: ImportSession,
        start_index: int = 0,
    ) -> int:
        """发送命令列表（带进度保存 + 中断捕获）。

        Args:
            start_index: 从第几条命令开始发送（断点续传用）
        Returns:
            covered: 覆盖的方块数
        """
        ox, oy, oz = self.start_pos
        total = len(commands)
        covered = 0
        sent_this_round = 0

        for i in range(start_index, total):
            cmd_type, wx1, wy1, wz1, wx2, wy2, wz2, name, states = commands[i]
            states_str = bedrock._states_to_str(states)

            if bot_name:
                dx1, dy1, dz1 = wx1 - ox, wy1 - oy, wz1 - oz
                dx2, dy2, dz2 = wx2 - ox, wy2 - oy, wz2 - oz
                def _r(d): return "~" if d == 0 else f"~{d}"
                if cmd_type == "fill":
                    cmd_str = f"fill {_r(dx1)} {_r(dy1)} {_r(dz1)} {_r(dx2)} {_r(dy2)} {_r(dz2)} {name} {states_str}"
                else:
                    cmd_str = f"setblock {_r(dx1)} {_r(dy1)} {_r(dz1)} {name} {states_str}"
                await bedrock.run_command_ai(cmd_str, timeout=3.0,
                    await_response=False, dimensional=True, bot_name=bot_name)
            else:
                if cmd_type == "fill":
                    cmd_str = f"fill {wx1} {wy1} {wz1} {wx2} {wy2} {wz2} {name} {states_str}"
                else:
                    cmd_str = f"setblock {wx1} {wy1} {wz1} {name} {states_str}"
                await bedrock.run_command_ai(cmd_str, timeout=3.0, await_response=False)

            vol = (abs(wx2 - wx1) + 1) * (abs(wy2 - wy1) + 1) * (abs(wz2 - wz1) + 1)
            covered += vol
            session.sent_commands = i + 1
            sent_this_round += 1

            # 进度回调
            self._on_progress(i + 1, total, cmd_type, name)

            # 每 _SAVE_INTERVAL 条保存一次进度
            if sent_this_round % _SAVE_INTERVAL == 0:
                save_session(session)
                logger.debug("进度已保存: %d/%d (%.0f%%)",
                           session.sent_commands, total,
                           100.0 * session.sent_commands / total)

            if self.delay_sec > 0:
                await asyncio.sleep(self.delay_sec)

        # 最终保存
        save_session(session)
        logger.info("命令发送完成: %d/%d, 覆盖 %d 块",
                   session.sent_commands, total, covered)
        return covered

    # ── 进度回调适配 ──────────────────────────────────────────────────────
    def _on_progress(self, current: int, total: int, cmd_type: str, name: str) -> None:
        if self.progress_cb is not None:
            self.progress_cb(current, total, cmd_type, name)

    # ── 完整性校验（四种等级） ───────────────────────────────────────────
    async def _verify(
        self,
        bedrock: BedrockSession,
        bot_name: str,
        st: Structure,
        covered: int,
    ) -> Dict[str, Any]:
        """按 verify_level 执行 testforblock 校验。"""
        import random
        ox, oy, oz = self.start_pos
        SX, SY, SZ = st.size
        air_id = st.palette.air_id

        # 收集 (dx,dy,dz,name) 列表
        non_air: List[Tuple[int, int, int, str]] = []
        air_only: List[Tuple[int, int, int, str]] = []

        for y in range(SY):
            for z in range(SZ):
                for x in range(SX):
                    bid = int(st.region.blocks[x, y, z])
                    if bid == air_id:
                        # 只抽样少量空气，避免太多
                        if random.random() < 0.01:
                            air_only.append((x, y, z, "minecraft:air"))
                        continue
                    blk = st.palette.get(bid)
                    if blk is not None:
                        non_air.append((x, y, z, blk.state.name))

        # 按等级决定校验集
        if self.verify_level == VERIFY_LEVEL_AIR:
            # 取前 100 + 最后 100 块 + 中间抽样
            samples = non_air[:100] + non_air[-100:]
            if len(non_air) > 400:
                step = len(non_air) // 200
                samples.extend(non_air[100:-100:step])
        elif self.verify_level == VERIFY_LEVEL_SIMILAR:
            count = max(50, len(non_air) // 10)
            samples = random.sample(non_air, min(count, len(non_air)))
        elif self.verify_level == VERIFY_LEVEL_COMPLETE:
            samples = non_air
        else:
            samples = non_air[:10]
        # 去重（同位置多次加入则保留）并限制最大数量（避免请求过多）
        max_check = 1000 if self.verify_level < VERIFY_LEVEL_COMPLETE else 999999
        if len(samples) > max_check:
            samples = random.sample(samples, max_check)

        checked = 0; passed = 0; failed = 0
        failures: List[str] = []

        for dx, dy, dz, blk_name in samples:
            rel = f"~{dx} ~{dy} ~{dz}"
            try:
                r = await bedrock.run_command_ai(
                    f"testforblock {rel} {blk_name} []",
                    timeout=6.0, dimensional=bool(bot_name), bot_name=bot_name,
                )
                checked += 1
                if r is not None:
                    passed += 1
                else:
                    failed += 1
                    if len(failures) < 10:
                        failures.append(f"({ox+dx},{oy+dy},{oz+dz}) {blk_name}")
            except Exception:
                checked += 1
                failed += 1
            # 控速
            if self.delay_sec > 0:
                await asyncio.sleep(self.delay_sec * 0.5)

        return {
            "level": self.verify_level,
            "checked": checked, "passed": passed, "failed": failed,
            "failures": failures,
            "total_non_air": len(non_air),
        }


# ═══════════════════════════════════════════════════════════════════════════════
# 导入报告
# ═══════════════════════════════════════════════════════════════════════════════
@dataclass
class BuildReport:
    """BuildTask.run 的结果。"""
    file_path: str
    size: Tuple[int, int, int]
    block_count: int
    palette_size: int
    commands_executed: int
    covered_blocks: int
    import_sec: float
    verify: Dict[str, Any]

    def summary(self) -> str:
        lines = [
            f"文件: {self.file_path}",
            f"尺寸: {self.size[0]}×{self.size[1]}×{self.size[2]}",
            f"方块: {self.block_count} ({self.palette_size} 种)",
            f"覆盖块数: {self.covered_blocks}",
            f"耗时: {self.import_sec:.1f}s ({self.covered_blocks / max(1, self.import_sec):.0f} 块/s)",
            f"校验: 等级 {self.verify['level']}, "
                  f"{self.verify['passed']}/{self.verify['checked']} 通过, "
                  f"失败 {self.verify['failed']}",
        ]
        if self.verify.get("failures"):
            lines.append(f"  失败样例: {self.verify['failures'][:5]}")
        return "\n".join(lines)
