"""NeOmega 接入点连接配置（Remote 模式）。

重要：proloader 走 Remote 模式——用户先运行 omega_launcher（fastbuilder 可执行文件）
登录网易租赁服，启动器在本地开接入点（默认 tcp://localhost:24013），
proloader 只连这个接入点发命令，**不处理登录、不需要 token**。

为什么免 token：
    接入点 TCP 连接本身无认证（ConnectOmega 只接收地址字符串）。
    真正的网易登录 + fbtoken 由 omega_launcher 进程在启动时完成，
    proloader 只是把命令转发给已登录的接入点。
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path


@dataclass
class NeOmegaConfig:
    """NeOmega 接入点连接配置（Remote 模式，免 token）。

    Attributes:
        lib_path: NeOmega 原生库路径（neomega_<os>_<arch>.dylib/.so/.dll）。
                  从 ToolDelta 的 DependencyLibrary 仓库下载，或随 omega_launcher 提供。
        access_point_addr: 接入点本地地址（默认 tcp://localhost:24013）。
                           用户运行 fastbuilder 后默认开这个地址。
        command_interval: 命令发送间隔秒数（防网易风控限频，默认 0.05 = 20cps）。
        place_via: 放方块用哪种命令通道：
            - "websocket": /setblock 走 websocket 命令（需 OP，可拿回执，推荐）
            - "player": 走玩家命令（聊天栏，受限，有副作用）
            - "settings": 走 settings/WO 命令（最高权限，无回执，最快）
        need_response: 放方块是否要回执（True 更可靠但更慢，批量导入建议 False）。
    """

    lib_path: str
    access_point_addr: str = "tcp://localhost:24013"
    command_interval: float = 0.05
    place_via: str = "websocket"
    need_response: bool = False

    def validate(self) -> None:
        """校验配置完整性，缺项给清晰报错。"""
        if not self.lib_path:
            raise ValueError(
                "NeOmegaConfig.lib_path 未设置：需要 NeOmega 原生库路径\n"
                "从 https://github.com/ToolDelta-Basic/DependencyLibrary 下载 "
                "neomega_<os>_<arch>.dylib/.so/.dll"
            )
        if not Path(self.lib_path).exists():
            raise ValueError(
                f"NeOmega 原生库不存在: {self.lib_path}\n"
                "从 https://github.com/ToolDelta-Basic/DependencyLibrary 下载对应平台的库\n"
                "  macOS arm64 (M1/M2): neomega_macos_arm64.dylib\n"
                "  macOS amd64 (Intel): neomega_macos_amd64.dylib\n"
                "  Linux: neomega_linux_amd64.so\n"
                "  Windows: neomega_windows_amd64.dll"
            )
        if self.place_via not in ("websocket", "player", "settings"):
            raise ValueError(f"place_via 必须是 websocket/player/settings，得到 {self.place_via}")
