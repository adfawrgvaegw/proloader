"""RakNet 协议层 —— 基于 UDP 的基岩版传输协议。

RakNet 是基岩版的底层传输协议（不是 Bedrock 游戏协议本身）。
它跑在 UDP 之上，提供：可靠传输、有序交付、分片重组、连接管理。
Bedrock 游戏包（Login、UpdateBlock 等）被 RakNet 帧封装后通过 UDP 收发。

学习要点：
    - 基岩版默认端口 19132（IPv4）/ 19133（IPv6）
    - 连接分两阶段：离线握手（Unconnected）→ 在线握手（Connected）
    - 离线握手用 MAGIC 常量识别，不加密
    - 在线阶段用 Frame Set Packet (0x80-0x8D) 封装游戏包
    - 网易在标准 RakNet 之上加了私有握手/加密（这部分封闭，见 neomega_backend）

本模块实现的是【标准、开放】的 RakNet，能连原版 BDS 服，
也是理解网易私有层的基础（网易层只是在此基础上加料）。
"""
