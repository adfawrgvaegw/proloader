"""Bedrock 登录会话：串联 RequestNetworkSettings → Login → 加密握手 → PlayStatus。

整条登录链路（vanilla BDS, online-mode=false 离线模式）：

    客户端 ── RequestNetworkSettings ──> 服务端      (协商压缩)
    客户端 <── NetworkSettings ──────── 服务端
    客户端 ── Login(含 ECDH 公钥) ────> 服务端       (自签名 JWT chain)
    客户端 <── ServerToClientHandshake  服务端       (JWT 含服务端公钥 + salt)
    客户端 ── 派生 keyBytes，启用 AES-CTR 加密
    客户端 ── ClientToServerHandshake ─> 服务端       (第一个加密包，空 payload)
    客户端 <── PlayStatus(LOGIN_SUCCESS) 服务端
    客户端 <── StartGame / ResourcePacksInfo ...      (后续游戏初始化)

⚠️ 关键点：
    1. RequestNetworkSettings 和 Login 前不压缩（algorithm=NONE）
    2. 收到 NetworkSettings 后按其 threshold/algorithm 压缩后续 batch
    3. ServerToClientHandshake 是明文收的（加密还没启用）
    4. ClientToServerHandshake 是第一个加密包
    5. PlayStatus LOGIN_SUCCESS 后才算登录成功
"""

from __future__ import annotations

import asyncio
import base64
from contextlib import suppress
import logging
import time
import uuid
from typing import Any, List, Optional

import numpy as np

from cryptography.hazmat.primitives.asymmetric import ec

from proloader.net.bedrock.batch import NeteaseDecompressor, decode_batch, encode_batch
from proloader.net.bedrock.crypto import BedrockCrypto, derive_key_bytes, generate_keypair
from proloader.net.bedrock.login import (
    ClientData,
    IdentityData,
    StartGameData,
    build_client_to_server_handshake,
    build_command_request,
    build_login_from_jwt_chain,
    build_login_packet,
    build_offline_login_chain,
    build_request_chunk_radius,
    build_request_network_settings,
    build_resource_pack_client_response,
    build_set_local_player_as_initialised,
    parse_disconnect,
    parse_network_settings,
    parse_play_status,
    parse_server_to_client_handshake,
    parse_start_game,
)
from proloader.net.bedrock.netease_packets import (
    build_all_netease_login_packets,
    parse_py_rpc,
)
from proloader.net.bedrock.packets import CompressionAlgorithm, PacketID, PlayStatus, ID_BATCH
from proloader.net.raknet.session import RakNetSession
from proloader.net.raknet.wire import ByteReader

logger = logging.getLogger(__name__)


def _msgpack_get(data: dict, key: str, default: Any = None) -> Any:
    """从 msgpack 解码的 dict 中取值。

    ⚠️ msgpack unpackb(raw=False) 解码后，dict key 可能是 str 或 bytes（取决于编码方式）。
    网易 PyRpc "c" 包的 dict key 实测是 bytes（如 b'uuid'），直接 data.get("uuid") 返回 None。
    此函数同时尝试 str 和 bytes 两种 key。
    """
    val = data.get(key)
    if val is None:
        val = data.get(key.encode("utf-8"))
    if val is None:
        return default
    return val


class BedrockLoginError(RuntimeError):
    """Bedrock 登录失败。"""


class BedrockDisconnectedError(BedrockLoginError):
    """服务端明确断开；原始显示文本仅保留在结构化属性，不写入诊断正文。"""

    def __init__(self, reason: int, message: str, hide_screen: bool) -> None:
        self.reason = reason
        self.server_message = message
        self.hide_screen = hide_screen
        super().__init__(f"服务端 Disconnect (reason={reason})")


class BedrockSession:
    """Bedrock 客户端会话。

    用法：
        conn = await handshake(host, port)           # RakNet 离线握手
        raknet = RakNetSession(conn)
        await raknet.start()
        await raknet.connect()                        # RakNet 在线握手
        bedrock = BedrockSession(raknet)
        await bedrock.login(server_addr=f"{host}:{port}")
        # 之后 bedrock.recv_packet() / send_packet() 收发游戏包
    """

    def __init__(
        self,
        raknet: RakNetSession,
        netease_mode: bool = False,
        netease_jwt_chain: Optional[str] = None,
        netease_uid: int = 0,
        netease_protocol: int = 860,
        priv_key: Optional[ec.EllipticCurvePrivateKey] = None,
        netease_bot_components: Optional[dict[str, Optional[int]]] = None,
    ) -> None:
        if netease_mode:
            if not netease_jwt_chain or priv_key is None:
                raise ValueError("网易模式必须提供 JWT chain 和获取 chain 时的私钥")
            if netease_protocol not in (686, 860):
                raise ValueError("网易协议仅支持 686 或 860")
        elif netease_jwt_chain is not None:
            raise ValueError("外部网易 JWT chain 需要 netease_mode=True")
        # g79 租赁服 LoginResult 没有填充 BotComponent，默认空；其他认证来源可显式传入。
        self._netease_bot_components = dict(netease_bot_components or {})
        # 提前校验组件，避免 spawn 后才因格式错误中断附加包序列。
        if netease_mode:
            build_all_netease_login_packets(0, 0, bot_components=self._netease_bot_components)
        self._raknet = raknet
        # 网易模式：Batch 包始终不写 compression byte（抓包确认）
        self._netease_mode: bool = netease_mode
        # 网易 JWT chain（从 SendAuthV2Request 获取，用于 Login 包）
        self._netease_jwt_chain: Optional[str] = netease_jwt_chain
        # 网易玩家 UID（IdentityData.Uid，用于 NeteaseJson LOGIN_UID）
        self._netease_uid: int = netease_uid
        # 网易 Bedrock 协议版本（686 = 1.21.2）
        self._netease_protocol: int = netease_protocol
        # 网易解压器（有状态，维护跨 batch 的 32KB 滑动窗口字典）
        # fatalder compression.go:neteaseCompression.Decompress 用 dict 实现跨 batch 解压
        self._netease_decompressor: Optional[NeteaseDecompressor] = (
            NeteaseDecompressor() if netease_mode else None
        )
        # 压缩协商（初始未启用，NetworkSettings 后启用）
        # ⚠️ compression_enabled=False 时 batch 无 compression byte（Gophertunnel: compression==nil）
        #    RequestNetworkSettings 阶段必须 False，否则 BDS 不认
        #    网易模式下协商后也不写 byte，但仍按 threshold 决定是否 DEFLATE 压缩 payload
        self._compression_enabled: bool = False
        self._compression_threshold: int = 0
        self._compression_algorithm: int = CompressionAlgorithm.NONE
        # 加密（Login 握手后启用）
        self._crypto: Optional[BedrockCrypto] = None
        # 客户端 ECDH 私钥（JWT 签名 + ECDH 共用）
        # ⚠️ 网易模式必须传入与 authv2 请求相同的私钥！JWT chain 的 identityPublicKey
        #    指向 authv2 请求时的公钥，Login 包的 RawToken 必须用同一私钥签名。
        #    vanilla 模式自动生成新密钥即可。
        self._priv: ec.EllipticCurvePrivateKey = priv_key if priv_key is not None else generate_keypair()
        # 玩家身份
        self._identity = IdentityData()
        self._client_data = ClientData()
        # 游戏初始化后填充
        self._start_game: Optional[StartGameData] = None
        self._entity_runtime_id: int = 0
        self._cmd_seq: int = 0  # CommandRequest 自增序号
        # aiCommand 通道状态（绕过 MCP 的魔法指令通道，对齐 fatalder SendSettingsCommand）
        self._ai_command_futures: dict[str, "asyncio.Future"] = {}  # uuid -> Future（等 ExecuteCommandOutputEvent / AfterExecuteCommandEvent）
        # 网易 PyRpc "r"/"c" 包事件注册表：event_id → event_name
        # "r" 包声明事件类型（event_id, "Package:Module:EventName"），"c" 包携带事件数据
        # 实测网易服务端用 "r"/"c" 而非 fatalder 定义的 "ModEventS2C"
        self._py_rpc_event_names: dict[int, str] = {}
        # 收包缓冲：从 batch 拆出的 packet 暂存
        self._inbox: asyncio.Queue[Optional[bytes]] = asyncio.Queue()
        self._terminal_error: Optional[BedrockLoginError] = None
        # 等待特定 packet_id 的 future 表（pid -> Future）
        self._waiting: dict = {}
        self._subchunk_waiters: dict = {}
        self._subchunk_lock = asyncio.Lock()
        # 收包循环任务
        self._recv_task: Optional[asyncio.Task] = None
        self._closed = False
        self._chat_subscribers = set()
        self._command_result_subscribers = set()
        self._ai_result_contexts = {}
        self._command_result_dropped = 0

    # ──────────────────────────────────────────────────────────────
    # 登录主流程
    # ──────────────────────────────────────────────────────────────
    def _build_connection_request(self) -> bytes:
        """在发送任何包前校验 chain/密钥，并按协议生成登录数据。"""
        if self._netease_mode:
            if self._netease_protocol == 860:
                # 网易模式：用外部 JWT chain（SendAuthV2Request 返回）
                # ⚠️ GameVersion 对应协议版本 860 = MC 1.21.120
                #    抓包确认网易客户端发 "1.21.120"（extract_game_version.py 提取）
                #    发 "1.21.70" 会被服务器拒为 gameVersionMismatch
                self._client_data.GameVersion = "1.21.120"
                # ⚠️ 抓包确认网易客户端的 ClientData 字段值（extract_game_version.py）：
                #    DeviceOS=2(iOS), UIProfile=1, CurrentInputMode=1, DefaultInputMode=2
                #    PremiumSkin=True, TrustedSkin=True, CompatibleWithClientSideChunkGen=True
                #    ArmSize="wide", SkinColor="#0", SkinImageWidth/Height=256
                #    SkinGeometryDataEngineVersion="MS4yLjU="(base64 "1.2.5")
                #    GrowthLevel=26, CapeId/SkinIID=非空, 新增 BloomData/MaxViewDistance 等
                self._client_data.DeviceOS = 2  # iOS
                self._client_data.UIProfile = 1
                self._client_data.CurrentInputMode = 1
                self._client_data.DefaultInputMode = 2
                self._client_data.PremiumSkin = True
                self._client_data.TrustedSkin = True
                self._client_data.CompatibleWithClientSideChunkGen = True
                self._client_data.ArmSize = "wide"
                self._client_data.SkinColor = "#0"
                self._client_data.SkinImageWidth = 256
                self._client_data.SkinImageHeight = 256
                self._client_data.SkinGeometryDataEngineVersion = base64.b64encode(b"1.2.5").decode("ascii")
                self._client_data.GrowthLevel = 26
                self._client_data.GraphicsMode = 1
                self._client_data.MaxViewDistance = 27
                self._client_data.MemoryTier = 4
                self._client_data.PlatformType = 1
                self._client_data.IsReconnect = False
                self._client_data.BloomData = ""
            else:
                # Fatalder protocol/info.go + dial.go 的 Android 默认资料。
                self._client_data.GameVersion = "1.21.2"
            return build_login_from_jwt_chain(
                self._netease_jwt_chain, self._client_data, self._priv,
                protocol=self._netease_protocol,
            )
        else:
            # vanilla 离线模式：自签名 JWT chain
            return build_offline_login_chain(
                self._priv, self._identity, self._client_data
            )

    async def login(self, server_addr: str, timeout: float = 15.0) -> None:
        """执行完整 Bedrock 登录流程。

        Args:
            server_addr: "host:port" 字符串（写入 ClientData.ServerAddress）
            timeout: 各阶段等待超时
        """
        self._raise_if_failed()
        self._client_data.ServerAddress = server_addr
        connection_request = self._build_connection_request()

        # 启动收包分发循环（解析 batch → 投递到事件处理器 / inbox）
        self._recv_task = asyncio.create_task(self._recv_loop())

        try:
            # ── 步骤 1：RequestNetworkSettings ───────────────────
            # 此时 compression_enabled=False，batch 无 compression byte（Gophertunnel: compression==nil）
            protocol = self._netease_protocol if self._netease_mode else 2168
            logger.info("Bedrock 登录 [1/4] 发 RequestNetworkSettings (protocol=%d)", protocol)
            await self._send_packet(build_request_network_settings(protocol=protocol))

            network_settings = await self._wait_packet(
                PacketID.NETWORK_SETTINGS, timeout, "NetworkSettings"
            )
            threshold, algorithm = parse_network_settings(network_settings)
            self._compression_threshold = threshold
            self._compression_algorithm = algorithm
            self._compression_enabled = True  # 协商后启用，后续 batch 带 compression byte
            logger.info(
                "Bedrock 登录 [1/4] 收到 NetworkSettings: threshold=%d algorithm=%d",
                threshold, algorithm,
            )

            # ── 步骤 2：Login（含 ECDH 公钥）─────────────────────
            logger.info("Bedrock 登录 [2/4] 发 Login")
            login_packet = build_login_packet(connection_request, protocol=protocol)
            logger.info("Login 包 [%d B]: %s", len(login_packet), login_packet[:64].hex())
            await self._send_packet(login_packet)

            # ── 步骤 3：ServerToClientHandshake（明文收）──────────
            handshake = await self._wait_packet(
                PacketID.SERVER_TO_CLIENT_HANDSHAKE, timeout, "ServerToClientHandshake"
            )
            parsed = parse_server_to_client_handshake(handshake)
            logger.info("Bedrock 登录 [3/4] 收到 ServerToClientHandshake，派生密钥")

            # ECDH 派生 keyBytes，启用加密
            key_bytes = derive_key_bytes(self._priv, parsed.server_public_key, parsed.salt)
            self._crypto = BedrockCrypto(key_bytes=key_bytes)
            logger.info("Bedrock 加密已启用 (keyBytes=%s...)", key_bytes[:4].hex())

            # 发 ClientToServerHandshake（第一个加密包，空 payload）
            await self._send_packet(build_client_to_server_handshake())

            # ── 步骤 4：PlayStatus(LOGIN_SUCCESS) ────────────────
            play_status = await self._wait_packet(
                PacketID.PLAY_STATUS, timeout, "PlayStatus"
            )
            status = parse_play_status(play_status)
            if status == PlayStatus.LOGIN_SUCCESS:
                logger.info("Bedrock 登录 [4/4] ✓ LOGIN_SUCCESS")
            elif status == PlayStatus.LOGIN_FAILED_CLIENT:
                raise BedrockLoginError("登录失败：客户端版本不匹配（LOGIN_FAILED_CLIENT）")
            elif status == PlayStatus.LOGIN_FAILED_SERVER:
                raise BedrockLoginError("登录失败：服务端拒绝（LOGIN_FAILED_SERVER）")
            else:
                raise BedrockLoginError(f"登录失败：非 LOGIN_SUCCESS 状态码 {status}")

            # ── 网易特有：LOGIN_SUCCESS 后立即发 ClientCacheStatus ──
            # 对齐 fatalder conn.go handlePlayStatus：在 LOGIN_SUCCESS 后发 ClientCacheStatus{false}
            # 之后再等 ResourcePacksInfo。这是网易服务端期望的顺序。
            if self._netease_mode:
                from proloader.net.bedrock.netease_packets import build_client_cache_status
                await self._send_packet(build_client_cache_status(enabled=False))
                logger.info("Bedrock 登录: 发 ClientCacheStatus{false}（网易 LOGIN_SUCCESS 后）")
        except Exception:
            await self.close()
            raise

    async def initialize_game(self, timeout: float = 20.0) -> StartGameData:
        """完成游戏初始化握手（登录成功后调用）。

        流程（vanilla BDS 离线模式，严格对齐 Gophertunnel v1.58.1 conn.go 客户端 spawn 序列）：
            1. 等 ResourcePacksInfo → 发 AllPacksDownloaded(2, 'downloadingfinished')
            2. 等 ResourcePackStack  → 发 Completed(3, 'resourcepackstackfinished')
            3. 等 StartGame           → 解析 EntityRuntimeID
            4. 发 RequestChunkRadius
            5. 等 ChunkRadiusUpdated   （BDS 回包）
            6. 等 PlayStatus(PLAYER_SPAWN)  ← 服务端通知「玩家可生成」
            7. 发 SetLocalPlayerAsInitialised(runtime_id)  ← 收到 PLAYER_SPAWN 后才发！

        ⚠️ 关键（之前 bug）：SetLocalPlayerAsInitialised 必须在收到 PlayStatus(PLAYER_SPAWN)
           **之后**发，不能发早了。Gophertunnel tryFinaliseClientConn() 要求
           waitingForSpawn && gameDataReceived 两个条件都满足才发。
           发早了 → 玩家未真正生成 → 后续 CommandRequest 会触发
           "Command exceeds maximum size / readNoHeader failed" 或 PacketViolationWarning。

        完成后才能发命令/放方块。返回 StartGameData。
        """
        # ── 步骤 1：ResourcePacksInfo → AllPacksDownloaded(2) ───
        await self._wait_packet(
            PacketID.RESOURCE_PACKS_INFO, timeout, "ResourcePacksInfo"
        )
        logger.info("游戏初始化 [1/7] 收到 ResourcePacksInfo → 发 AllPacksDownloaded(2)")
        await self._send_packet(build_resource_pack_client_response(status=2, netease_mode=self._netease_mode))

        # ── 步骤 2：ResourcePackStack → Completed(3)
        await self._wait_packet(
            PacketID.RESOURCE_PACK_STACK, timeout, "ResourcePackStack"
        )
        logger.info("游戏初始化 [2/7] 收到 ResourcePackStack → 发 Completed(3)")
        await self._send_packet(build_resource_pack_client_response(status=3, netease_mode=self._netease_mode))

        # ── 步骤 3：StartGame → 解析 EntityRuntimeID ─────────────
        # ⚠️ StartGame 前可能夹带一堆 CreativeContent / BiomeDefinitionList 等包，
        #    它们会进 inbox，不影响 _wait_packet（它按 pid 精确匹配）。
        start_game_pkt = await self._wait_packet(
            PacketID.START_GAME, timeout, "StartGame"
        )
        self._start_game = parse_start_game(start_game_pkt)
        self._entity_runtime_id = self._start_game.entity_runtime_id
        logger.info(
            "游戏初始化 [3/7] 收到 StartGame: runtime_id=%d game_type=%d",
            self._entity_runtime_id, self._start_game.player_game_type,
        )

        # ── 步骤 4：RequestChunkRadius ────────────────────────────
        # Gophertunnel 在收到 ItemRegistry 后才发 RequestChunkRadius；BDS 对此较宽容，
        # 直接在 StartGame 后发也能接受。关键是后续要等回包。
        legacy_netease = self._netease_mode and self._netease_protocol == 686
        radius = 16 if legacy_netease else 4
        await self._send_packet(build_request_chunk_radius(
            radius=radius, max_radius=0 if legacy_netease else 64,
            netease_mode=self._netease_mode,
            protocol=self._netease_protocol if self._netease_mode else 2168,
        ))
        logger.info("游戏初始化 [4/7] 发 RequestChunkRadius(%d)", radius)

        # ── 步骤 5：等 ChunkRadiusUpdated ──────────────────────────
        # fatalder conn.go: conn.expect(packet.IDChunkRadiusUpdated, packet.IDPlayStatus)
        # RequestChunkRadius 的响应是 ChunkRadiusUpdated(70)，不是 NetworkChunkPublisherUpdate(121)。
        # 121 是服务端定期发的区块发布更新（包含区块坐标），不是半径确认包。
        # 之前误把 121 当响应，实际上服务端发 121 是正常区块推送，同时因为 maxRadius 格式错误
        # 触发了 PacketViolationWarning。
        await self._wait_packet(
            PacketID.CHUNK_RADIUS_UPDATED, timeout, "ChunkRadiusUpdated"
        )
        logger.info("游戏初始化 [5/7] 收到 ChunkRadiusUpdated")

        # ── 步骤 6：等 PlayStatus(PLAYER_SPAWN) ──────────────────
        # Gophertunnel handlePlayStatus(PLAYER_SPAWN): 设 waitingForSpawn=true。
        # ⚠️ 这一步是关键：服务端通知「玩家可以生成了」，收到后才能发 SetLocalPlayerAsInitialised。
        # 顺序上 BDS 收到 RequestChunkRadius 后会先发 ChunkRadiusUpdated 再发 PlayStatus(PLAYER_SPAWN)，
        # 两者可能在同一个 batch 里到达（ChunkRadiusUpdated 先入 inbox）。
        spawn_pkt = await self._wait_packet(
            PacketID.PLAY_STATUS, timeout, "PlayStatus(PLAYER_SPAWN)"
        )
        spawn_status = parse_play_status(spawn_pkt)
        if spawn_status != PlayStatus.PLAYER_SPAWN:
            raise BedrockLoginError(
                f"期望 PlayStatus(PLAYER_SPAWN=3)，实际收到状态码 {spawn_status}"
            )
        logger.info("游戏初始化 [6/7] 收到 PlayStatus(PLAYER_SPAWN) → 玩家可生成")

        # ── 步骤 7：SetLocalPlayerAsInitialised ──────────────────
        # 收到 PLAYER_SPAWN 后才发（对齐 Gophertunnel tryFinaliseClientConn）。
        await self._send_packet(
            build_set_local_player_as_initialised(self._entity_runtime_id)
        )
        logger.info("游戏初始化 [7/7] 发 SetLocalPlayerAsInitialised → ✓ 初始化完成")

        # ── 网易特有：登录后发送 ClientCacheStatus + NeteaseJson + 7个PyRpc ──
        # 对齐 fatalder open_connection.go:67-123
        if self._netease_mode:
            await self._send_netease_login_packets()

        return self._start_game

    async def run_command(self, command: str, timeout: float = 5.0) -> Optional[str]:
        """发一条命令到服务器（如 setblock xxx 0 destroy）。

        command 不带 /（BDS 自动识别）。
        返回 CommandOutput 包的原始数据，超时返回 None。
        """
        self._cmd_seq += 1
        request_id = f"proloader_{self._cmd_seq}"
        pkt = build_command_request(
            command=command,
            request_id=request_id,
            player_runtime_id=self._entity_runtime_id,
            entity_unique_id=(self._start_game.entity_unique_id if self._start_game else 0),
            origin=0,  # Player
            netease_mode=self._netease_mode,
        )
        await self._send_packet(pkt)
        logger.info("→ 发命令: %s (request_id=%s)", command, request_id)
        # 等 CommandOutput（可选，有些命令无输出）
        try:
            output = await self._wait_packet(
                PacketID.COMMAND_OUTPUT, timeout, f"CommandOutput({command})"
            )
            return output.hex()
        except BedrockLoginError:
            logger.debug("命令 %s 无输出（超时），可能正常", command)
            return None

    async def run_command_ai(self, command: str, timeout: float = 5.0, await_response: bool = True, dimensional: bool = False, bot_name: str = "", result_context=None) -> Optional[str]:
        """通过 aiCommand 通道发命令（★ 绕过 MCP 反作弊）。

        对齐 fatalder commands.go:SendSettingsCommand + ai_command/execute_command_event.go。
        命令包在 PyRpc ModEventC2S aiCommand.ExecuteCommandEvent 里发，绕过 MCP 校验。

        Args:
            command: 命令行（不带 /，如 "setblock 10 64 10 minecraft:stone"）
            timeout: 等待 ExecuteCommandOutputEvent 响应的超时
            await_response: True=等响应；False=fire-and-forget（放方块批量用）
            dimensional: True=用 execute as @a[name=bot] at @s run 包裹命令（对齐 fatalder dimensional=true）
            bot_name: dimensional=True 时指定的假人游戏内名称
        Returns:
            命令输出文本，超时/不等返回 None
        """
        from proloader.net.bedrock.netease_packets import build_py_rpc_ai_execute_command

        request_uuid = str(uuid.uuid4())
        # dimensional 模式：execute as @a[name="bot"] at @s run <command>
        # 对齐 fatalder commands.go:72-77
        if dimensional and bot_name:
            command = f'execute as @a[name="{bot_name}"] at @s run {command}'
        full_cmd = "execute run " + command
        pkt = build_py_rpc_ai_execute_command(full_cmd, request_uuid)

        if result_context is not None and self._command_result_subscribers:
            if len(self._ai_result_contexts)>=4096:
                self._ai_result_contexts.pop(next(iter(self._ai_result_contexts)))
                self._command_result_dropped += 1
            self._ai_result_contexts[request_uuid] = result_context
        fut: Optional[asyncio.Future] = None
        if await_response:
            fut = asyncio.get_event_loop().create_future()
            self._ai_command_futures[request_uuid] = fut

        try:
            await self._send_packet(pkt)
            logger.info("→ aiCommand: %s (uuid=%s)", command, request_uuid)
            if not await_response or fut is None:
                return None
            return await asyncio.wait_for(fut, timeout=timeout)
        except asyncio.TimeoutError:
            logger.debug("aiCommand %s 无响应（超时 %ss）", command, timeout)
            return None
        except BedrockLoginError as e:
            self._raise_if_failed()  # 会话终止不能降级成单条命令无输出。
            # AfterExecuteCommandEvent(executeResult=False) 或 AvailableCheckFailed
            logger.warning("aiCommand %s 执行失败: %s", command, e)
            return None
        finally:
            self._ai_command_futures.pop(request_uuid, None)
            if fut is not None:
                if not fut.done():
                    fut.cancel()
                elif not fut.cancelled():
                    fut.exception()  # 发送阶段失败时也回收已投递的终止异常。

    async def set_block(self, x: int, y: int, z: int, name: str, states: Optional[dict] = None,
                        *, bot_name: str = "", bot_pos: Optional[tuple] = None) -> None:
        """用 aiCommand setblock 放方块（绕过 MCP）。

        对齐 fatalder setblock.go:SetBlockAsync。fire-and-forget，不等响应。
        批量放方块时调用方应自行控速（await asyncio.sleep）避免服务端限流。

        ⚠️ MCP 反作弊限制：aiCommand 通道的绝对坐标 setblock 会被拒绝（executeResult=False）。
        必须用相对坐标 ~dx ~dy ~dz + dimensional 模式（execute as @a[name=bot] at @s run）。
        当提供 bot_name + bot_pos 时，自动转为相对坐标模式。

        Args:
            x,y,z: 世界坐标
            name: 方块名（如 "minecraft:stone"）
            states: 方块状态 dict（如 {"stone_type": "granite"}），None/空=无状态
            bot_name: 假人游戏内名称（提供时启用相对坐标模式）
            bot_pos: 假人世界坐标 (x,y,z)，相对偏移基于此计算
        """
        states_str = self._states_to_str(states)
        if bot_name and bot_pos is not None:
            # 相对坐标模式：~dx ~dy ~dz（绕过 MCP 绝对坐标限制）
            dx, dy, dz = x - bot_pos[0], y - bot_pos[1], z - bot_pos[2]
            def _fmt_rel(d: int) -> str:
                if d == 0:
                    return "~"
                return f"~{d}"
            cmd = f"setblock {_fmt_rel(dx)} {_fmt_rel(dy)} {_fmt_rel(dz)} {name} {states_str}"
            await self.run_command_ai(cmd, timeout=3.0, await_response=False,
                                     dimensional=True, bot_name=bot_name)
        else:
            # 绝对坐标模式（会被 MCP 拒绝，仅用于兼容）
            cmd = f"setblock {x} {y} {z} {name} {states_str}"
            await self.run_command_ai(cmd, timeout=3.0, await_response=False)

    @staticmethod
    def _states_to_str(states: Optional[dict]) -> str:
        """方块状态 dict → setblock 命令的 states 字符串。

        基岩版 setblock states 格式（SNBT，对齐 fatalder utils.MarshalBlockStates）：
            空：[]
            有：["key"="value","key2"=123,"key3"=true]
        字符串值带引号，bool→true/false，int→数字。
        ⚠️ Python 里 bool 是 int 子类，必须先判 bool 再判 int。
        ⚠️ key 按字典序稳定排序（对齐 fatalder slices.SortStableFunc）。
        """
        if not states:
            return "[]"
        parts = []
        for k in sorted(states.keys()):  # 稳定排序，对齐 fatalder
            v = states[k]
            if isinstance(v, bool):
                parts.append(f'"{k}"={"true" if v else "false"}')
            elif isinstance(v, int):
                parts.append(f'"{k}"={v}')
            elif isinstance(v, str):
                parts.append(f'"{k}"="{v}"')
            else:
                parts.append(f'"{k}"="{v}"')
        return "[" + ",".join(parts) + "]"

    async def import_structure(
        self,
        structure,
        offset: tuple = (0, 0, 0),
        delay: float = 0.05,
        max_blocks: Optional[int] = None,
        progress_callback: Optional[Any] = None,
        bot_name: str = "",
    ) -> int:
        """导入建筑到服务器（逐方块 aiCommand setblock，绕过 MCP）。

        ⚠️ MCP 反作弊限制：绝对坐标 setblock 被拒绝，必须用相对坐标。
        流程：
        1. tp 假人到 offset 位置（建筑起始角）
        2. 等待区块加载（5 秒）
        3. 用相对坐标 ~dx ~dy ~dz 逐方块放置（dimensional 模式）
        4. 每隔一定数量方块重新 tp（保持区块加载）

        Args:
            structure: Structure 对象（proloader.parser.load_structure 解析得到）
            offset: 放置世界坐标偏移 (x, y, z)，也是假人 tp 的目标位置
            delay: 每方块间隔秒数（控速，避免服务端限流；0=最快）
            max_blocks: 最多放置方块数（测试用），None=不限
            progress_callback: 可选回调 (done, total, block_name)
            bot_name: 假人游戏内名称（相对坐标模式必需）
        Returns:
            实际放置的方块数（非空气且可放置）
        """
        # 不可放置的方块（创造模式专用/内部方块，租赁服会拒绝或无意义）
        UNPLACEABLE = {"minecraft:structure_block", "minecraft:structure_void", "minecraft:air"}
        sx, sy, sz = structure.size
        ox, oy, oz = offset
        total = sx * sy * sz
        done = 0
        placed = 0

        # ── 相对坐标模式：先 tp 假人到 offset，再用 ~dx ~dy ~dz 放方块 ──
        if bot_name:
            logger.info("建筑导入：tp 假人 %s 到 %s，等区块加载...", bot_name, offset)
            tp_cmd = f"tp {ox} {oy} {oz}"
            await self.run_command_ai(tp_cmd, timeout=8.0, await_response=True,
                                     dimensional=True, bot_name=bot_name)
            logger.info("tp 完成，等待 5 秒让区块加载...")
            await asyncio.sleep(5.0)
            # 设置 gamemode creative 确保能放方块
            await self.run_command_ai("gamemode creative", timeout=5.0, await_response=True,
                                     dimensional=True, bot_name=bot_name)

        for y in range(sy):
            for z in range(sz):
                for x in range(sx):
                    bid = int(structure.region.blocks[x, y, z])
                    blk = structure.palette.get(bid)
                    done += 1
                    if blk is None or blk.state.name in UNPLACEABLE:
                        continue
                    wx, wy, wz = x + ox, y + oy, z + oz
                    if bot_name:
                        await self.set_block(wx, wy, wz, blk.state.name, blk.state.states,
                                            bot_name=bot_name, bot_pos=offset)
                    else:
                        await self.set_block(wx, wy, wz, blk.state.name, blk.state.states)
                    placed += 1
                    if progress_callback is not None:
                        progress_callback(done, total, blk.state.name)
                    if delay > 0:
                        await asyncio.sleep(delay)
                    if max_blocks is not None and placed >= max_blocks:
                        logger.info("已达到最大方块数 %d，提前结束", max_blocks)
                        return placed
        logger.info("建筑导入完成: %d/%d 方块已放置", placed, total)
        return placed

    @staticmethod
    def _generate_merged_commands(structure, offset: tuple):
        """贪心合并同种方块的长方体，生成 fill/setblock 命令列表。

        移植 fatalder utils/chunk_fill/chunk_fill.go 的算法。
        遍历 3D 方块数组，从每个方块向 X→Z→Y 三方向贪心扩展，
        找到最大的同种方块长方体，合并成一条 fill（≤32k 体积）或 setblock。

        Returns:
            list of (cmd_type, wx1, wy1, wz1, wx2, wy2, wz2, name, states)
            cmd_type: "fill" 或 "setblock"
        """
        MAX_FILL_VOLUME = 32768
        UNPLACEABLE = {"minecraft:structure_block", "minecraft:structure_void", "minecraft:air"}
        sx, sy, sz = structure.size
        ox, oy, oz = offset
        blocks = structure.region.blocks  # numpy (sx, sy, sz)
        can_set = np.ones((sx, sy, sz), dtype=bool)

        # 标记空气/不可放置方块
        for x in range(sx):
            for y in range(sy):
                for z in range(sz):
                    bid = int(blocks[x, y, z])
                    blk = structure.palette.get(bid)
                    if blk is None or blk.state.name in UNPLACEABLE:
                        can_set[x, y, z] = False

        commands = []
        for x in range(sx):
            for y in range(sy):
                for z in range(sz):
                    if not can_set[x, y, z]:
                        continue
                    bid = int(blocks[x, y, z])
                    blk = structure.palette.get(bid)

                    # 向 X 扩展
                    ex = x + 1
                    while ex < sx and can_set[ex, y, z] and int(blocks[ex, y, z]) == bid:
                        ex += 1
                    # 向 Z 扩展
                    ez = z + 1
                    while ez < sz:
                        if not all(can_set[xx, y, ez] and int(blocks[xx, y, ez]) == bid
                                   for xx in range(x, ex)):
                            break
                        ez += 1
                    # 向 Y 扩展
                    ey = y + 1
                    while ey < sy:
                        ok = True
                        for xx in range(x, ex):
                            for zz in range(z, ez):
                                if not can_set[xx, ey, zz] or int(blocks[xx, ey, zz]) != bid:
                                    ok = False
                                    break
                            if not ok:
                                break
                        if not ok:
                            break
                        ey += 1

                    # 标记已合并
                    for xx in range(x, ex):
                        for yy in range(y, ey):
                            for zz in range(z, ez):
                                can_set[xx, yy, zz] = False
                    ex -= 1; ey -= 1; ez -= 1

                    wx1, wy1, wz1 = x + ox, y + oy, z + oz
                    wx2, wy2, wz2 = ex + ox, ey + oy, ez + oz
                    name = blk.state.name
                    states = blk.state.states

                    if x == ex and y == ey and z == ez:
                        commands.append(("setblock", wx1, wy1, wz1, wx2, wy2, wz2, name, states))
                    else:
                        vol = (ex - x + 1) * (ey - y + 1) * (ez - z + 1)
                        if vol <= MAX_FILL_VOLUME:
                            commands.append(("fill", wx1, wy1, wz1, wx2, wy2, wz2, name, states))
                        else:
                            # 沿最长边拆分
                            lx, ly, lz = ex - x + 1, ey - y + 1, ez - z + 1
                            per_vol = vol // max(lx, ly, lz)
                            step = max(1, MAX_FILL_VOLUME // (ly * lz)) if ly * lz > 0 else 1
                            for xs in range(x, ex + 1, step):
                                xe = min(xs + step - 1, ex)
                                commands.append(("fill", xs + ox, wy1, wz1, xe + ox, wy2, wz2, name, states))
        return commands

    async def import_structure_fast(
        self,
        structure,
        offset: tuple = (0, 0, 0),
        delay: float = 0.0,
        progress_callback: Optional[Any] = None,
        bot_name: str = "",
    ) -> int:
        """快速导入建筑（合并 fill 算法，对齐 fatalder chunk_fill.go）。

        将同种方块的连续区域合并成 fill 命令（每条最多 32768 块），
        不同种的方块各自合并，无法合并的退化为 setblock。
        比逐方块 setblock 快数十倍。

        流程：
        1. tp 假人到 offset 位置（建筑起始角）
        2. 等待区块加载
        3. 生成合并 fill 命令列表
        4. 逐条通过 aiCommand 通道发送（相对坐标模式）

        Args:
            structure: Structure 对象
            offset: 放置世界坐标偏移 (x, y, z)
            delay: 每条命令间隔秒数（控速；0=最快）
            progress_callback: 可选回调 (current, total, cmd_type, name)
            bot_name: 假人游戏内名称
        Returns:
            实际放置的方块数（估算，基于命令覆盖体积）
        """
        ox, oy, oz = offset
        # tp 假人到 offset
        if bot_name:
            logger.info("快速导入：tp 假人 %s 到 %s，等区块加载...", bot_name, offset)
            await self.run_command_ai(f"tp {ox} {oy} {oz}", timeout=8.0,
                                      await_response=True, dimensional=True, bot_name=bot_name)
            logger.info("tp 完成，等待 5 秒让区块加载...")
            await asyncio.sleep(5.0)
            await self.run_command_ai("gamemode creative", timeout=5.0,
                                      await_response=True, dimensional=True, bot_name=bot_name)

        # 生成合并命令
        logger.info("生成合并 fill 命令...")
        t0 = time.time()
        commands = self._generate_merged_commands(structure, offset)
        gen_time = time.time() - t0
        total_cmds = len(commands)
        fill_count = sum(1 for c in commands if c[0] == "fill")
        setblock_count = total_cmds - fill_count
        # 估算覆盖方块数
        blocks_covered = 0
        for c in commands:
            _, x1, y1, z1, x2, y2, z2, _, _ = c
            blocks_covered += (abs(x2 - x1) + 1) * (abs(y2 - y1) + 1) * (abs(z2 - z1) + 1)
        logger.info("合并完成: %d 条命令 (%d fill + %d setblock)，覆盖 %d 块，生成耗时 %.1fs",
                    total_cmds, fill_count, setblock_count, blocks_covered, gen_time)

        # 发送命令
        sent = 0
        for cmd_type, wx1, wy1, wz1, wx2, wy2, wz2, name, states in commands:
            states_str = self._states_to_str(states)
            if bot_name:
                # 相对坐标模式
                dx1, dy1, dz1 = wx1 - ox, wy1 - oy, wz1 - oz
                dx2, dy2, dz2 = wx2 - ox, wy2 - oy, wz2 - oz
                def _r(d): return "~" if d == 0 else f"~{d}"
                if cmd_type == "fill":
                    cmd = f"fill {_r(dx1)} {_r(dy1)} {_r(dz1)} {_r(dx2)} {_r(dy2)} {_r(dz2)} {name} {states_str}"
                else:
                    cmd = f"setblock {_r(dx1)} {_r(dy1)} {_r(dz1)} {name} {states_str}"
                await self.run_command_ai(cmd, timeout=3.0, await_response=False,
                                          dimensional=True, bot_name=bot_name)
            else:
                if cmd_type == "fill":
                    cmd = f"fill {wx1} {wy1} {wz1} {wx2} {wy2} {wz2} {name} {states_str}"
                else:
                    cmd = f"setblock {wx1} {wy1} {wz1} {name} {states_str}"
                await self.run_command_ai(cmd, timeout=3.0, await_response=False)

            sent += 1
            if progress_callback is not None:
                progress_callback(sent, total_cmds, cmd_type, name)
            if delay > 0:
                await asyncio.sleep(delay)

        logger.info("快速导入完成: 发送 %d/%d 条命令", sent, total_cmds)
        return blocks_covered

    async def _send_netease_login_packets(self) -> None:
        """发送网易特有登录包序列（对齐 fatalder open_connection.go:67-123）。

        在 SetLocalPlayerAsInitialised 之后发送：
            1. ClientCacheStatus{false}
            2. NeteaseJson{LOGIN_UID, uid}
            3-9. 7 个 PyRpc 包
        """
        if self._start_game is None:
            raise BedrockLoginError("网易附加包必须在收到 StartGame 并完成 spawn 后发送")
        skin_id = self._client_data.SkinId or ""
        packets = build_all_netease_login_packets(
            uid=self._netease_uid,
            entity_unique_id=self._start_game.entity_unique_id,
            skin_id=skin_id,
            bot_components=self._netease_bot_components,
        )
        for pkt in packets:
            await self._send_packet(pkt)
        logger.info("网易登录包已发送 (%d 个)", len(packets))

    # ──────────────────────────────────────────────────────────────
    # 收发 API
    # ──────────────────────────────────────────────────────────────
    async def send_packet(self, packet_bytes: bytes) -> None:
        """发送一个游戏包（按协商的压缩参数，加密后发）。"""
        await self._send_packet(packet_bytes)

    async def read_block_entity(self, position, dimension='overworld', timeout=8):
        """Read fresh NBT through this connection; timeout/malformed data never mean success."""
        from proloader.command_blocks import DIMENSIONS, decode_subchunk
        from .command_block import build_subchunk_request
        dimension_id = DIMENSIONS[dimension]
        position = tuple(position)
        key = (dimension_id, position[0]//16, 0, position[2]//16)
        async with self._subchunk_lock:
            self._raise_if_failed()
            future = asyncio.get_running_loop().create_future()
            self._subchunk_waiters[key] = future
            try:
                await self.send_packet(build_subchunk_request(position, dimension_id))
                response = await asyncio.wait_for(future, timeout)
                data = await asyncio.to_thread(decode_subchunk, response)
                expected_offset = [0, position[1]//16, 0]
                entries = [e for e in data['entries'] if e['offset'] == expected_offset]
                if data['dimension'] != dimension_id or data['position'] != list(key[1:]) or len(entries) != 1:
                    raise ValueError('子区块响应与请求位置不一致。')
                entry = entries[0]
                if entry['result'] not in (1, 6):
                    raise ValueError('服务器拒绝了子区块读取请求。')
                matches = [nbt for nbt in entry['nbt'] if tuple(nbt.get(k) for k in ('x','y','z')) == position]
                if len(matches) != 1:
                    raise ValueError('目标方块NBT缺失或重复，未确认内部数据。')
                self._raise_if_failed()
                return matches[0]
            finally:
                self._subchunk_waiters.pop(key, None)
                if not future.done():future.cancel()
                elif not future.cancelled():future.exception()

    async def recv_packet(self) -> bytes:
        """收一个游戏包（已解密解压）。"""
        self._raise_if_failed()
        packet = await self._inbox.get()
        if packet is None:
            self._inbox.put_nowait(None)  # 也唤醒同时等待 recv_packet 的其他调用者。
        self._raise_if_failed()
        assert packet is not None
        return packet

    def _raise_if_failed(self) -> None:
        if self._terminal_error is not None:
            raise self._terminal_error

    def _fail_session(self, error: BedrockLoginError) -> None:
        """首次终止原因优先，同时唤醒包、命令与公共收包 API 的等待者。"""
        if self._terminal_error is not None:
            return
        self._terminal_error = error
        for future in (list(self._waiting.values()) + list(self._ai_command_futures.values())
                       + list(getattr(self, '_subchunk_waiters', {}).values())):
            if not future.done():
                future.set_exception(error)
        self._inbox.put_nowait(None)

    async def close(self) -> None:
        if self._closed:
            return
        self._closed = True
        self._fail_session(BedrockLoginError("Bedrock 会话已关闭"))
        if self._recv_task and self._recv_task is not asyncio.current_task():
            self._recv_task.cancel()
            with suppress(asyncio.CancelledError):
                await self._recv_task
        if self._raknet is not None:
            await self._raknet.stop()

    # ──────────────────────────────────────────────────────────────
    # 内部
    # ──────────────────────────────────────────────────────────────
    async def _send_packet(self, packet_bytes: bytes) -> None:
        """打包成 batch（按协商状态决定有无 compression byte + 加密）发出去。"""
        self._raise_if_failed()
        algorithm = self._compression_algorithm if self._compression_enabled else CompressionAlgorithm.NONE
        threshold = self._compression_threshold if self._compression_enabled else 0
        batch = encode_batch(
            [packet_bytes],
            algorithm=algorithm,
            threshold=threshold,
            compression_enabled=self._compression_enabled,
            netease_mode=self._netease_mode,
        )
        if self._crypto is not None:
            batch = self._crypto.encrypt(batch)
        await self._raknet.send_reliable(batch)

    async def _recv_loop(self) -> None:
        """收包循环：从 RakNet 取帧 → 解密 → 解 batch → 拆 packet → 投递。"""
        while not self._closed and self._terminal_error is None:
            try:
                data = await self._raknet.recv()
            except Exception as exc:
                if not self._closed:
                    self._fail_session(BedrockLoginError(f"RakNet 收包失败 ({type(exc).__name__})"))
                return
            if not data or data[0] != ID_BATCH:
                logger.debug("非 Batch 帧 0x%02x，丢弃", data[0] if data else -1)
                continue
            try:
                # 解密（0xfe 头不加密，后面整体解密）
                if self._crypto is not None:
                    decrypted_tail = self._crypto.decrypt(data[1:])
                    data = bytes([ID_BATCH]) + decrypted_tail
                # 网易模式：协商后用滑动窗口解压器，协商前按明文处理
                # fatalder decoder.go: readCompressID=false 时总是用 NetEase 解压
                if self._netease_mode and self._compression_enabled and self._netease_decompressor:
                    packets = decode_batch(
                        data, netease_decompressor=self._netease_decompressor
                    )
                else:
                    packets = decode_batch(data, netease_mode=self._netease_mode)
            except Exception as exc:
                # 加密流与压缩字典有状态，失败后不能继续丢包并假装能恢复同步。
                self._fail_session(BedrockLoginError(f"Batch 解密/解压失败 ({type(exc).__name__})"))
                return
            for p in packets:
                if self._terminal_error is not None:
                    return
                try:
                    self._dispatch_packet(p)
                except Exception as exc:
                    self._fail_session(BedrockLoginError(f"Bedrock 分包处理失败 ({type(exc).__name__})"))
                    return

    def subscribe_command_results(self):
        queue = asyncio.Queue(maxsize=4096)
        self._command_result_subscribers.add(queue)
        return queue

    def unsubscribe_command_results(self, queue):
        self._command_result_subscribers.discard(queue)
        if not self._command_result_subscribers:self._ai_result_contexts.clear()

    def subscribe_chat(self):
        queue = asyncio.Queue(maxsize=128)
        self._chat_subscribers.add(queue)
        return queue

    def unsubscribe_chat(self, queue):
        self._chat_subscribers.discard(queue)

    def _dispatch_packet(self, packet: bytes) -> None:
        """把拆出的 packet 分发给等待者或投递到 inbox。"""
        if self._terminal_error is not None:
            return
        try:
            pid, _ = self._read_packet_id(packet)
        except Exception:
            logger.debug("无法解析 packet id，丢弃 %d bytes", len(packet))
            return
        if pid == PacketID.DISCONNECT:
            try:
                disconnect = parse_disconnect(packet)
                error = BedrockDisconnectedError(
                    disconnect.reason, disconnect.message, disconnect.hide_screen,
                )
            except (ValueError, EOFError, IndexError) as exc:
                error = BedrockLoginError(f"服务端 Disconnect 包格式无效 ({type(exc).__name__})")
            self._fail_session(error)
            return
        logger.info("← 收到 packet_id=%d [%d B]: %s", pid, len(packet), packet[:32].hex())

        if pid == PacketID.SUB_CHUNK and getattr(self, '_subchunk_waiters', None):
            from .command_block import subchunk_key
            try:
                key = subchunk_key(packet)
            except (ValueError, EOFError):
                return
            future = self._subchunk_waiters.get(key)
            if future is not None and not future.done():
                future.set_result(packet)
                return

        if pid == PacketID.TEXT and self._chat_subscribers:
            from .chat import parse_chat
            try:
                message = parse_chat(packet)
            except (ValueError, EOFError, IndexError):
                message = None
            if message is not None:
                for queue in tuple(self._chat_subscribers):
                    if queue.full():
                        queue.get_nowait()
                    queue.put_nowait(message)
            return

        # 网易 PyRpc 包拦截（MCP 反作弊等）
        if pid == PacketID.PYRPC and self._netease_mode:
            self._handle_netease_py_rpc(packet)
            return  # PyRpc 包不投递到 inbox

        # 等待中的特定包优先交给等待者
        fut = self._waiting.get(pid)
        if fut is not None and not fut.done():
            fut.set_result(packet)
            self._waiting.pop(pid, None)
            return
        # 其余投递到 inbox（给上层游戏逻辑）
        self._inbox.put_nowait(packet)

    def _handle_netease_py_rpc(self, packet: bytes) -> None:
        """处理网易 PyRpc 包（MCP 反作弊 / aiCommand 响应等）。

        网易服务端实际使用两种 PyRpc 包格式：
        1. fatalder 定义的具名包：name="GetStartType"/"GetMCPCheckNum"/"ModEventS2C" 等
        2. ★ 实测的 "r"/"c" 包（网易真正在用的格式）：
           - "r" 包：["r", [event_id, "Package:Module:EventName"], nil]
             声明即将发送的事件类型，event_id 用于和后续 "c" 包匹配
           - "c" 包：["c", [event_id, event_data_dict], nil]
             携带事件数据，通过 event_id 关联到 "r" 包声明的事件名

        实测日志确认：
            ← PyRpc: name=r payload=[25, b'Minecraft:aiCommand:AfterExecuteCommandEvent']
            ← PyRpc: name=c payload=[25, {b'uuid': b'...', b'executeResult': False}]

        对齐 fatalder mcp_challenge_solve.go 的 onPyRpc 逻辑：
        - GetStartType → TransferStartType（解密→拼uid→重加密）→ 回 SetStartType
        - GetMCPCheckNum → 记录日志（求解是闭源的，暂时跳过）
        """
        try:
            rpc = parse_py_rpc(packet)
        except Exception:
            logger.warning("PyRpc 包解析失败，丢弃")
            return

        name = rpc.get("name")
        # ⚠️ 服务端 PyRpc 的 name 是 bytes 类型（msgpack bin 格式），需转 str
        if isinstance(name, bytes):
            name = name.decode("utf-8")
        payload = rpc.get("payload")
        logger.info("← PyRpc: name=%s payload=%s", name, str(payload)[:200])

        # ── "r" 包：事件类型注册 ──
        # payload = [event_id:int, event_name:str]
        if name == "r":
            if isinstance(payload, list) and len(payload) >= 2:
                event_id = payload[0]
                event_name = payload[1]
                if isinstance(event_name, bytes):
                    event_name = event_name.decode("utf-8")
                self._py_rpc_event_names[int(event_id)] = event_name
                logger.debug("PyRpc 事件注册: id=%d → %s", event_id, event_name)
            return

        # ── "c" 包：事件数据 ──
        # payload = [event_id:int, event_data:dict]
        # 通过 event_id 查 "r" 包注册的事件名，分发到对应处理器
        if name == "c":
            if isinstance(payload, list) and len(payload) >= 2:
                event_id = int(payload[0])
                event_data = payload[1]
                event_name = self._py_rpc_event_names.get(event_id, "")
                logger.info("← PyRpc 事件 id=%d name=%s data=%s", event_id, event_name, str(event_data)[:200])
                if "AfterExecuteCommandEvent" in event_name:
                    self._handle_after_execute_command(event_data)
                elif "ExecuteCommandOutputEvent" in event_name:
                    self._handle_execute_command_output(event_data)
                elif "AvailableCheckFailed" in event_name:
                    self._handle_ai_command_available_check_failed(event_data)
                else:
                    logger.debug("PyRpc 事件 %s 未处理", event_name)
            return

        # ── fatalder 具名包格式（兼容）──
        if name == "GetStartType":
            # MCP StartType challenge：解密→拼uid→重加密→回 SetStartType
            try:
                self._handle_mcp_start_type(payload)
            except Exception as e:
                logger.warning("MCP StartType 处理失败: %s", e)
        elif name == "GetMCPCheckNum":
            # MCP CheckNum challenge：打印完整 payload + 发假回复试探服务端
            logger.info("← GetMCPCheckNum 完整 payload: %s", str(payload))
            self._handle_mcp_check_num(payload)
        elif name == "ModEventS2C":
            # aiCommand 响应（fatalder 格式，实测网易不用这个，但保留兼容）
            try:
                self._handle_ai_command_output(payload)
            except Exception as e:
                logger.warning("ModEventS2C 处理失败: %s", e)
        else:
            logger.debug("PyRpc 包 %s 未处理，忽略", name)

    def _handle_mcp_start_type(self, payload: Any) -> None:
        """处理 MCP StartType challenge（对齐 fatalder TransferStartType）。

        流程：解密 content → 拼 uid → 重加密 → 回 SetStartType PyRpc 包
        """
        from proloader.net.netease.crypto import g79_http_decrypt, g79_http_encrypt

        # payload 是 [content_hex] 列表（fatalder start_type.go: FromGo 期望 [string]）
        if isinstance(payload, list) and len(payload) >= 1:
            content_hex = payload[0]
        else:
            content_hex = payload

        # content_hex 可能是 bytes，转成 str
        if isinstance(content_hex, bytes):
            content_hex = content_hex.decode("utf-8")

        content_bytes = bytes.fromhex(content_hex)

        # 解密
        plain = g79_http_decrypt(content_bytes)
        plain_str = plain.decode("utf-8", errors="replace").rstrip("\x00")

        # 拼接 uid
        merged = str(self._netease_uid) + plain_str
        merged_bytes = merged.encode("utf-8")

        # 重加密
        enc = g79_http_encrypt(merged_bytes)
        enc_hex = enc.hex()

        # 回 SetStartType PyRpc 包
        from proloader.net.bedrock.netease_packets import build_py_rpc
        set_start_type_pkt = build_py_rpc(["SetStartType", [enc_hex], None])
        asyncio.ensure_future(self._send_packet(set_start_type_pkt))
        logger.info("→ MCP SetStartType 已回复")

    def _handle_mcp_check_num(self, payload: Any) -> None:
        """处理 MCP CheckNum challenge（算法已从网易客户端 vanilla.mcp 逆向得到）。

        ★ 算法来源：vanilla.mcp 解包 → mod/server/system/dynamicMCP.py 反编译：
            CalcCheckNum(data) = MD5('l00g<ks' + data + 'tsa').hexdigest()
            GetCheckNum(salt):
                valM = CalcCheckNum(salt)              # = MD5('l00g<ks' + salt + 'tsa')
                sign = MD5(valM[16:] + valM[:16])      # valM 旋转 16 字符再 MD5
                return [valM, sign]

        客户端流程（clientrpchandler.py GetMCPCheckNum，388-401 行）：
            data = binascii.unhexlify(mcp_hex)         # 服务端下发的 dynamicMCPLoader 模块
            dyMCP = McpImporter.read_module('dynamicMCPLoader', data)  # rotor+zlib+reverse 解密
            valM = dyMCP.GetCheckNum(salt)             # 上面算法
            rpc.CServerRpc().SetMCPCheckNum(valM)      # 回发 [valM, sign]

        服务端校验（dynamicMCP.py CheckMCPClientNum）：接受 len(params)>=2，
        验证 params[0]==valK 且 params[1]==MD5(valM[16:] + extra + valM[:16])。
        标准 2 元素响应 [valM, sign] 即可通过。

        GetMCPCheckNum payload 格式（fatalder get_mc_pc_check_num.go）：
            [mcp_hex, [salt, FirstExtraData, SecondExtraData]]
        SetMCPCheckNum payload 格式：
            ["SetMCPCheckNum", [valM, sign], None]

        ⚠️ 服务端下发的 dynamicMCPLoader 理论上可能和 bundled dynamicMCP.py 不同。
           若此算法被拒，用记录的 mcp_hex 离线解码验证（NlsCipher+zlib+reverse+marshal）。
        """
        import hashlib
        from proloader.net.bedrock.netease_packets import build_py_rpc

        # 解析 payload: [mcp_hex, [salt, FirstExtraData, SecondExtraData]]
        first_arg = None
        salt = None
        if isinstance(payload, list) and len(payload) >= 2:
            first_arg = payload[0]
            if isinstance(payload[1], list) and len(payload[1]) >= 1:
                salt = payload[1][0]

        # msgpack 解码后可能是 bytes，统一转 str
        if isinstance(salt, bytes):
            salt = salt.decode("utf-8", "replace")
        if isinstance(first_arg, bytes):
            first_arg_str = first_arg.decode("utf-8", "replace")
        else:
            first_arg_str = str(first_arg) if first_arg else ""

        logger.info(
            "MCP CheckNum: mcp_hex(%d chars) salt=%r EntityUID=%d",
            len(first_arg_str), salt, self._entity_runtime_id,
        )
        # 记录完整 mcp_hex 便于离线解码（NlsCipher 解密 → 反编译 dynamicMCPLoader 验证算法）
        logger.info("MCP CheckNum mcp_hex 全文: %s", first_arg_str)

        if not salt:
            logger.warning("MCP CheckNum: salt 为空，无法计算，跳过")
            return

        # ★ 真实算法（dynamicMCP.py 逆向）
        message = "l00g<ks" + salt + "tsa"
        valM = hashlib.md5(message.encode("utf-8")).hexdigest()
        rawS = valM[16:] + valM[:16]  # valM 旋转 16 字符
        sign = hashlib.md5(rawS.encode("utf-8")).hexdigest()
        response = [valM, sign]
        logger.info("MCP CheckNum 算法结果: valM=%s sign=%s", valM, sign)

        set_check_num_pkt = build_py_rpc(["SetMCPCheckNum", response, None])
        asyncio.ensure_future(self._send_packet(set_check_num_pkt))
        logger.info("→ MCP SetMCPCheckNum 已回复（真实算法 [valM, sign]）")

    def _handle_after_execute_command(self, data: Any) -> None:
        """处理 AfterExecuteCommandEvent（命令执行成功/失败）。

        对齐 fatalder after_execute_command_event.go。
        实测网易 "c" 包 data 格式：{b'uuid': b'...', b'executeResult': False}
        ⚠️ msgpack 解码后 dict key 是 bytes 类型，需用 _msgpack_get 取值。

        这是命令的终结信号：
        - executeResult=True：命令执行成功（之后可能还有 ExecuteCommandOutputEvent 输出）
        - executeResult=False：命令执行失败（不会有输出）

        用 uuid 匹配等待中的 future，set_result。
        对于 fire-and-forget 命令（无 future），只记录日志。
        """
        if not isinstance(data, dict):
            logger.debug("AfterExecuteCommandEvent data 非 dict: %s", str(data)[:200])
            return
        req_uuid = _msgpack_get(data, "uuid")
        if isinstance(req_uuid, bytes):
            req_uuid = req_uuid.decode("utf-8")
        success = _msgpack_get(data, "executeResult", False)
        context = self._ai_result_contexts.pop(str(req_uuid), None)
        if context is not None and not success:
            for queue in tuple(self._command_result_subscribers):
                if queue.full():
                    queue.get_nowait();self._command_result_dropped += 1
                queue.put_nowait(context)

        logger.info("← aiCommand 执行结果 (uuid=%s): %s", req_uuid, "✅ 成功" if success else "❌ 失败")
        fut = self._ai_command_futures.get(str(req_uuid))
        if fut is not None and not fut.done():
            if success:
                # 成功：设置结果为空字符串（如果 ExecuteCommandOutputEvent 先到并已 set_result，
                # 则 fut.done()=True，不会进入此分支，不覆盖已有输出）
                fut.set_result("")
            else:
                # 失败：设置异常让调用方知道
                fut.set_exception(BedrockLoginError(f"aiCommand 执行失败 (uuid={req_uuid})"))

    def _handle_execute_command_output(self, data: Any) -> None:
        """处理 ExecuteCommandOutputEvent（命令输出）。

        对齐 fatalder execute_command_output_event.go。
        实测网易 "c" 包 data 格式：{b'uuid': b'...', b'msg': b'...', b'is_last': bool, ...}
        ⚠️ msgpack 解码后 dict key 是 bytes 类型，需用 _msgpack_get 取值。

        ⚠️ 注释说"当且仅当魔法命令成功时收到"，所以收到此事件说明命令成功。
        用 uuid 匹配等待中的 future，set_result(msg)。
        """
        if not isinstance(data, dict):
            logger.debug("ExecuteCommandOutputEvent data 非 dict: %s", str(data)[:200])
            return
        req_uuid = _msgpack_get(data, "uuid")
        if isinstance(req_uuid, bytes):
            req_uuid = req_uuid.decode("utf-8")
        msg = _msgpack_get(data, "msg", "")
        if isinstance(msg, bytes):
            msg = msg.decode("utf-8", errors="replace")
        is_last = _msgpack_get(data, "is_last", True)
        logger.info("← aiCommand 输出 (uuid=%s, is_last=%s): %s", req_uuid, is_last, str(msg)[:200])
        fut = self._ai_command_futures.get(str(req_uuid))
        if fut is not None and not fut.done():
            fut.set_result(msg)

    def _handle_ai_command_available_check_failed(self, data: Any) -> None:
        """处理 AvailableCheckFailed（aiCommand 前置检查失败）。

        对齐 fatalder available_check_failed.go。
        收到此事件说明 aiCommand 通道本身不可用（可能是 MCP 未通过验证）。
        """
        reason = ""
        if isinstance(data, dict):
            reason = _msgpack_get(data, "reason", "")
            if isinstance(reason, bytes):
                reason = reason.decode("utf-8", errors="replace")
        logger.warning("← aiCommand AvailableCheckFailed: %s", reason)
        # 通知所有等待中的 future
        for req_uuid, fut in list(self._ai_command_futures.items()):
            if not fut.done():
                fut.set_exception(BedrockLoginError(f"aiCommand 不可用: {reason}"))

    def _handle_ai_command_output(self, payload: Any) -> None:
        """处理 fatalder 格式 ModEventS2C aiCommand 响应（兼容保留）。

        fatalder 格式 payload: ["Minecraft", "aiCommand", "ExecuteCommandOutputEvent", {msg, uuid, ...}]
        实测网易服务端用 "r"/"c" 包，不走这个格式，但保留兼容。
        """
        if not isinstance(payload, list) or len(payload) < 4:
            logger.debug("ModEventS2C payload 格式异常: %s", str(payload)[:200])
            return
        event_name = payload[2]
        if isinstance(event_name, bytes):
            event_name = event_name.decode("utf-8")
        data = payload[3]
        if event_name == "AfterExecuteCommandEvent":
            self._handle_after_execute_command(data)
        elif event_name == "ExecuteCommandOutputEvent":
            self._handle_execute_command_output(data)
        elif event_name == "AvailableCheckFailed":
            self._handle_ai_command_available_check_failed(data)
        else:
            logger.debug("ModEventS2C 事件 %s 未处理", event_name)

    def _read_packet_id(self, packet: bytes) -> tuple:
        """读 packet header，返回 (packet_id, rest_offset)。"""
        r = ByteReader(packet)
        value = r.read_unsigned_varint()
        pid = value & 0x3FF
        return pid, r.pos

    # ── 等待特定 packet ──────────────────────────────────────────
    async def _wait_packet(self, pid: int, timeout: float, name: str) -> bytes:
        """等一个特定 packet_id 的包。

        ⚠️ 先扫 inbox：有些包可能在 _wait_packet 调用前就到了（如 ResourcePacksInfo
        在 login() 期间就到，被 _dispatch_packet 投递到 inbox）。如果不先扫，
        就会一直等新包而超时。
        """
        self._raise_if_failed()
        # 1. 先扫 inbox，找已到达但未被消费的包
        temp: List[bytes] = []
        found: Optional[bytes] = None
        while True:
            try:
                pkt = self._inbox.get_nowait()
            except asyncio.QueueEmpty:
                break
            if found is None:
                try:
                    p, _ = self._read_packet_id(pkt)
                except Exception:
                    p = -1
                if p == pid:
                    found = pkt
                    continue  # 不放回，消费掉
            temp.append(pkt)  # 非目标包，暂存
        # 把非目标包放回 inbox（保序）
        for p in temp:
            self._inbox.put_nowait(p)
        if found is not None:
            logger.debug("_wait_packet(%s id=%d) 从 inbox 命中缓存", name, pid)
            return found

        # 2. inbox 没有，注册 future 等新包
        fut: asyncio.Future = asyncio.get_event_loop().create_future()
        self._waiting[pid] = fut
        try:
            result = await asyncio.wait_for(fut, timeout)
            self._raise_if_failed()
            return result
        except asyncio.TimeoutError:
            raise BedrockLoginError(f"等待 {name} (id={pid}) 超时（{timeout}s）")
        finally:
            self._waiting.pop(pid, None)
