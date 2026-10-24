"""网易我的世界 G79 客户端库（Python 移植自 g79client Go 版）。

完整登录链路：
    cookie (sauth_json) → PE 认证 → 搜索租赁服 → 进入世界(IP:port)
    → Link+GameStart → JWT chain → Bedrock 连接
"""
