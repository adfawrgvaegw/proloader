"""Bedrock 协议编解码 + 方块映射。

这里放 Bedrock 游戏层（RakNet 之上）的东西：方块名映射、包 ID、命令语法。
真实发包编解码（Login/UpdateBlockPacket 的二进制）较复杂，按需逐步实现；
而"通过命令放方块"（/setblock /fill）是更可靠的工程路径，先把它做扎实。
"""
