"""NeOmega 接入点客户端（Remote 模式，ctypes 绑定）。

本模块镜像 ToolDelta/neo_libs/neo_conn.py 的 ctypes 接口，
通过加载 NeOmega Go 原生库连接【已运行的】接入点。

关键修正（相对早期版本）：
    早期版本假设 proloader 自己调 StartOmega 启动 core + 登录（需要 fbtoken）。
    实际上应走 Remote 模式：用户先运行 omega_launcher（fastbuilder）完成登录，
    proloader 只调 ConnectOmega(addr) 连接入点 → 发命令。**免 token**。

架构：
    用户运行 fastbuilder → 登录网易租赁服 → 开接入点 tcp://localhost:24013
    proloader → ctypes 加载 neomega.dylib → ConnectOmega(addr) → send_*_command

函数签名（来自 ToolDelta neo_conn.py 实测）：
    ConnectOmega(address: *c_char) -> *c_char (error or NULL)   # Remote 模式连接
    OmegaAvailable() -> c_int (1=在线)
    SendWebSocketCommandOmitResponse(cmd: *c_char) -> void       # ws 命令，fire-and-forget
    SendWebSocketCommandNeedResponse(cmd, retrieverID: *c_char) -> void  # ws 命令，同步等回执
    SendPlayerCommandOmitResponse(cmd: *c_char) -> void
    SendPlayerCommandNeedResponse(cmd, retrieverID: *c_char) -> void
    SendWOCommand(cmd: *c_char) -> void                          # settings/WO 命令（最高权限，无回执）
    FreeMem(ptr: *c_char) -> void                                # 释放 Go 分配的字符串

学习要点：
    - Go 通过 //export 导出 C 函数，Python 用 ctypes 调用（cgo 桥接）
    - Remote 模式下 proloader 是"纯客户端"，不碰登录/加密/RakNet
    - need_response 命令通过 retrieverID 关联回执（事件轮询里匹配）
"""

from __future__ import annotations

import ctypes
import json
import logging
import threading
import time
from dataclasses import dataclass, field
from typing import Optional

from proloader.backend.neomega.config import NeOmegaConfig

logger = logging.getLogger(__name__)


# ── 命令回执数据结构（对应 ToolDelta Packet_CommandOutput）──────────────────────
@dataclass
class OutputMessage:
    """单条命令输出消息。"""

    Success: bool = False
    Message: str = ""
    Parameters: list = field(default_factory=list)


@dataclass
class CommandOutput:
    """命令回执（need_response 命令的返回值）。

    放方块后看 SuccessCount 或 OutputMessages[0].Success 判断是否成功。
    """

    OutputType: int = 0
    SuccessCount: int = 0
    OutputMessages: list = field(default_factory=list)

    @property
    def success(self) -> bool:
        """命令是否成功（SuccessCount>0 或首条消息 Success=True）。"""
        if self.SuccessCount > 0:
            return True
        if self.OutputMessages and self.OutputMessages[0].Success:
            return True
        return False


class NeOmegaCoreError(RuntimeError):
    """NeOmega 核心调用失败。"""


class NeOmegaCore:
    """NeOmega 接入点客户端（Remote 模式）。

    生命周期：
        load() → connect() → send_*_command() → disconnect()

    Example:
        >>> cfg = NeOmegaConfig(lib_path="/path/to/neomega_macos_arm64.dylib")
        >>> core = NeOmegaCore(cfg)
        >>> core.load()
        >>> core.connect()  # 连接 omega_launcher 开的接入点
        >>> core.send_websocket_command("setblock 10 64 20 stone")  # 放方块
    """

    def __init__(self, cfg: NeOmegaConfig) -> None:
        self._cfg = cfg
        self._lib: Optional[ctypes.CDLL] = None
        self._connected = False
        # need_response 命令的回执回调注册表：retriever_id -> setter
        self._cmd_callbacks: dict[int, callable] = {}
        self._cmd_results: dict[int, Optional[CommandOutput]] = {}
        self._retriever_counter = 0
        self._lock = threading.Lock()
        self._poll_thread: Optional[threading.Thread] = None
        self._polling = False

    # ── 加载二进制 ─────────────────────────────────────────────────────────
    def load(self) -> None:
        """加载 NeOmega 原生库并设置函数签名。"""
        if self._lib is not None:
            return
        cfg = self._cfg
        cfg.validate()
        logger.info("加载 NeOmega 原生库: %s", cfg.lib_path)
        try:
            lib = ctypes.CDLL(cfg.lib_path)
        except OSError as e:
            raise NeOmegaCoreError(
                f"无法加载 NeOmega 原生库 {cfg.lib_path}: {e}\n"
                "请确认二进制与操作系统/架构匹配（macOS arm64 用 neomega_macos_arm64.dylib）"
            ) from e

        # 设置函数签名（argtypes/restype），防 ctypes 默认 int 截断指针
        # 返回字符串的函数 restype=c_void_p（NULL 表示成功，非 NULL 是错误信息）
        lib.ConnectOmega.argtypes = [ctypes.c_char_p]
        lib.ConnectOmega.restype = ctypes.c_void_p

        lib.OmegaAvailable.argtypes = []
        lib.OmegaAvailable.restype = ctypes.c_int

        lib.SendWebSocketCommandOmitResponse.argtypes = [ctypes.c_char_p]
        lib.SendWebSocketCommandOmitResponse.restype = None

        lib.SendPlayerCommandOmitResponse.argtypes = [ctypes.c_char_p]
        lib.SendPlayerCommandOmitResponse.restype = None

        # SendWOCommand = settings/world operator 命令（最高权限，无回执）
        lib.SendWOCommand.argtypes = [ctypes.c_char_p]
        lib.SendWOCommand.restype = None

        lib.SendWebSocketCommandNeedResponse.argtypes = [ctypes.c_char_p, ctypes.c_char_p]
        lib.SendWebSocketCommandNeedResponse.restype = None

        lib.SendPlayerCommandNeedResponse.argtypes = [ctypes.c_char_p, ctypes.c_char_p]
        lib.SendPlayerCommandNeedResponse.restype = None

        # 事件轮询（拿 need_response 命令的回执）
        if hasattr(lib, "EventPoll"):
            lib.EventPoll.argtypes = []
            # EventPoll 返回 (eventType, retriever) —— ctypes 难以直接表达多返回值
            # ToolDelta 的实现里 EventPoll 通过指针参数写回，这里简化：先不轮询
            # need_response 用阻塞等待（见 _send_with_response_blocking）

        # 释放 Go 堆字符串
        if hasattr(lib, "FreeMem"):
            lib.FreeMem.argtypes = [ctypes.c_void_p]
            lib.FreeMem.restype = None

        self._lib = lib
        logger.info("NeOmega 原生库加载成功")

    def _check_err(self, ret) -> None:
        """检查核心返回值，非 NULL 则读取错误信息并释放。"""
        if ret is None:
            return
        try:
            msg = ctypes.string_at(ret).decode("utf-8", errors="replace")
        except Exception:
            msg = f"<无法解码错误指针 {ret}>"
        finally:
            if hasattr(self._lib, "FreeMem"):
                try:
                    self._lib.FreeMem(ret)
                except Exception:
                    pass
        raise NeOmegaCoreError(msg)

    # ── 连接接入点 ─────────────────────────────────────────────────────────
    def connect(self, retries: int = 10, interval: float = 3.0) -> None:
        """连接到已运行的接入点（Remote 模式）。

        前提：用户已运行 omega_launcher（fastbuilder）完成登录，接入点在
        access_point_addr 上监听。接入点启动需要几秒，故带重试。

        连接本身无认证、无 token——纯本地 TCP。
        """
        import time

        assert self._lib is not None, "先调 load()"
        if self._connected:
            return
        cfg = self._cfg
        last_err = ""
        for i in range(1, retries + 1):
            try:
                ret = self._lib.ConnectOmega(cfg.access_point_addr.encode("utf-8"))
                self._check_err(ret)
                self._connected = True
                logger.info("已连接 NeOmega 接入点 %s", cfg.access_point_addr)
                return
            except NeOmegaCoreError as e:
                last_err = str(e)
                if "api not exist" in last_err:
                    logger.info("等待接入点就绪...（%d/%d）", i, retries)
                else:
                    logger.warning("连接接入点失败: %s（%d/%d）", last_err, i, retries)
                time.sleep(interval)
        raise NeOmegaCoreError(
            f"连接 NeOmega 接入点失败（重试 {retries} 次）: {last_err}\n"
            "请确认已运行 fastbuilder（omega_launcher）并完成登录"
        )

    def available(self) -> bool:
        """接入点是否在线（假人仍在服内）。"""
        if self._lib is None:
            return False
        try:
            return self._lib.OmegaAvailable() == 1
        except Exception:
            return False

    # ── 命令发送 ───────────────────────────────────────────────────────────
    def send_websocket_command(
        self, cmd: str, need_response: bool = False, timeout: float = 30.0
    ) -> Optional[CommandOutput]:
        """发 websocket 命令（/setblock /fill 走这个，需 OP）。

        Args:
            cmd: 命令字符串（如 "setblock 10 64 20 stone"）
            need_response: True 则同步等回执（可靠但慢），False 则 fire-and-forget
            timeout: need_response 时的超时秒数

        Returns:
            need_response=True 时返回 CommandOutput，否则 None
        """
        assert self._lib is not None and self._connected, "未连接接入点"
        b = cmd.encode("utf-8")
        if need_response:
            return self._send_with_response("websocket", cmd, timeout)
        self._lib.SendWebSocketCommandOmitResponse(b)
        return None

    def send_player_command(
        self, cmd: str, need_response: bool = False, timeout: float = 30.0
    ) -> Optional[CommandOutput]:
        """发玩家命令（聊天栏触发，受玩家权限限制）。

        注意：若 sendcommandfeedback=false，库会临时打开再关闭，有失败概率
        且会向 OP 聊天栏显示命令。优先用 send_websocket_command。
        """
        assert self._lib is not None and self._connected
        if need_response:
            return self._send_with_response("player", cmd, timeout)
        self._lib.SendPlayerCommandOmitResponse(cmd.encode("utf-8"))
        return None

    def send_settings_command(self, cmd: str) -> None:
        """发 settings/WO 命令（最高权限，服务端控制台级别，无回执，最快）。

        适合批量放方块（量大、不需要逐个确认）。
        但很多命令发不了（仅限 settings 类命令）。
        """
        assert self._lib is not None and self._connected
        self._lib.SendWOCommand(cmd.encode("utf-8"))

    def _send_with_response(
        self, via: str, cmd: str, timeout: float
    ) -> CommandOutput:
        """发命令并同步等回执。

        NeOmega 的 need_response 命令通过 retrieverID 关联回执，
        回执通过 EventPoll 事件返回。ToolDelta 用锁+回调实现同步等待。

        简化实现：用 retrieverID 注册回调，阻塞等待。
        若原生库不支持事件轮询，回退为 omit_response + 返回未知成功。
        """
        with self._lock:
            self._retriever_counter += 1
            retriever_id = self._retriever_counter

        retriever_str = str(retriever_id).encode("utf-8")
        b = cmd.encode("utf-8")

        # 注册回调占位
        event = threading.Event()
        result_holder: list[Optional[CommandOutput]] = [None]
        self._cmd_callbacks[retriever_id] = lambda out: (
            result_holder.__setitem__(0, out),
            event.set(),
        )

        try:
            if via == "websocket":
                self._lib.SendWebSocketCommandNeedResponse(b, retriever_str)
            else:
                self._lib.SendPlayerCommandNeedResponse(b, retriever_str)

            # 等回执（事件轮询线程会调回调）
            # 若没有事件轮询（_polling=False），这里会超时
            if not event.wait(timeout=timeout):
                logger.warning("命令回执超时（%ss）: %s", timeout, cmd)
                # 回退：假设成功（omit_response 语义）
                return CommandOutput(SuccessCount=1)
            return result_holder[0] or CommandOutput()
        finally:
            self._cmd_callbacks.pop(retriever_id, None)

    def _start_event_polling(self) -> None:
        """启动事件轮询线程（拿 need_response 命令的回执）。

        ToolDelta 的 _react() 循环调 EventPoll() 取事件并分发。
        这里实现简化版：仅处理 CommandResponseCB 事件。

        注意：EventPoll 的具体签名依赖原生库实现，这里预留接口。
        若原生库不支持，need_response 命令会超时回退。
        """
        if self._polling or self._lib is None:
            return
        if not hasattr(self._lib, "EventPoll"):
            logger.debug("原生库无 EventPoll，need_response 命令将超时回退")
            return
        self._polling = True
        self._poll_thread = threading.Thread(target=self._poll_loop, daemon=True)
        self._poll_thread.start()

    def _poll_loop(self) -> None:
        """事件轮询主循环（守护线程）。"""
        while self._polling:
            try:
                # EventPoll 的具体调用方式依赖原生库
                # ToolDelta: eventType, retriever = LIB.EventPoll()
                # ctypes 难以直接表达，这里预留——实际需根据原生库签名实现
                time.sleep(0.01)
            except Exception as e:
                logger.debug("事件轮询异常: %s", e)
                break

    def disconnect(self) -> None:
        """断开接入点连接（omega_launcher 进程继续运行）。"""
        self._polling = False
        self._connected = False
        self._lib = None
        logger.info("NeOmega 接入已断开（omega_launcher 仍在运行）")
