"""proloader.net —— 真实网络协议层。

这是把 proloader 从"模拟玩具"变成"真实可用导入器"的关键一层。

三层结构（自底向上）：
    raknet/   RakNet over UDP（开放协议，wiki.bedrock.dev 有完整文档）
              负责：不可靠/可靠 UDP 传输、握手、分片重组、ACK
    bedrock/  Bedrock 协议编解码（Login / RequestNetworkSettings / GamePacket 等）
    （backend/bedrock_backend.py / neomega_backend.py 在上层消费这些）

为什么单独抽一层：
    - 学习价值：RakNet 是基岩版整个网络栈的地基，理解它就理解了"假人怎么连进去"
    - 可验证：Unconnected Ping 能直接 ping 通真实公共基岩服，证明协议实现正确
    - 解耦：上层后端只依赖这一层，换协议实现不影响业务

参考：
    - RakNet 协议规范: https://wiki.bedrock.dev/servers/raknet
    - Gophertunnel (Go, MIT): https://github.com/Sandertv/gophertunnel
    - SkinGetBE (C++ 从零实现): https://github.com/androidprod/SkinGetBE
"""
