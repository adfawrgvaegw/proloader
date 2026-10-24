"""proloader.backend.neomega —— 网易租赁服真实连接后端（Remote 模式）。

这是 proloader 连接【真实网易我的世界租赁服】的路径。

## 架构（修正后，Remote 模式，免 token）

```
用户                              proloader                    网易租赁服
 │                                  │                            │
 │  下载运行 fastbuilder            │                            │
 │  (omega_launcher)                │                            │
 │  ──────────────────>             │                            │
 │  登录网易账号 + 输入服务器编号密码  │                            │
 │  假人入服                         │                            │
 │  开接入点 tcp://localhost:24013  │                            │
 │                                  │                            │
 │  运行 proloader                  │                            │
 │  ──────────────────>             │                            │
 │                                  │  ctypes 加载 neomega.dylib │
 │                                  │  ConnectOmega(addr)        │
 │                                  │  ──────────────────────────>│
 │                                  │  send /setblock 命令        │
 │                                  │  假人放方块                  │
```

## 为什么走 NeOmega 而不是从零逆向

网易账号登录 + 网易魔改 RakNet 握手 + 协议加密 是封闭的，
全生态没人开源这部分（封号风险 + 商业价值）。
NeOmega 把这些封闭部分封装在 omega_launcher（Go 二进制）里，
proloader 通过原生库连接入点，对外暴露命令 API。

## 为什么免 token

接入点 TCP 连接本身无认证（ConnectOmega 只接收地址字符串）。
真正的网易登录 + fbtoken 由 omega_launcher 进程在启动时完成，
proloader 只是把命令转发给已登录的接入点。

## 前置条件（用户必须准备）

1. omega_launcher（fastbuilder 可执行文件）—— 从 neomega.top 下载
2. NeOmega 原生库（neomega_<os>_<arch>.dylib/.so/.dll）—— 从
   ToolDelta-Basic/DependencyLibrary 仓库下载
3. 网易账号 + 租赁服编号 + 密码（在 omega_launcher 里输入）
4. 假人在服内被授予 OP 权限（用于 /setblock）

## 风险说明

NeOmega 官方已宣布"停止自动更新服务"（ToolDelta neo_conn.py:1198 注释）。
ToolDelta 现主推 FateArk 接入点（gRPC，纯 Python）。
proloader 当前走 NeOmega 路径（与现有代码方向一致），
FateArk 作为备选记录在 netease-ecosystem-research.md。

## 参考实现

ToolDelta/neo_libs/neo_conn.py（ThreadOmega 类，本模块镜像其 ctypes 接口）
"""
