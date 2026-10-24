# 版本升级定位

## 代码入口

| 问题阶段 | 首要位置 |
|---|---|
| 真实CLI、交互菜单、任务续传 | `proloader/cli/main.py`、`commands.py`、`wizard.py` |
| 身份锁/私有材料 | `proloader/bots.py`，私有根在 `~/.proloader/bots` |
| bootstrap、PE、租赁服查询、Link、AuthV2 | `proloader/net/netease/client.py`、`runtime.py`、`pe_profiles.py`、`link.py` |
| Cookie短信入口 | `proloader/net/netease/mobile_login.py`；不能靠重新发短信修复协议错误 |
| RakNet离线/在线、可靠性 | `proloader/net/raknet/connection.py`、`session.py`、`frames.py` |
| 登录、压缩、加密、spawn | `proloader/net/bedrock/session.py`、`login.py`、`batch.py`、`crypto.py` |
| 上游施工与解析/NBT源码 | `third_party/fatalder/` 及其modules，来源清单在上一级 |
| 原生规划桥、命令名兼容、计划/计时 | `proloader/fatalder.py`、`block_names.py`、`importer.py`、`pacing.py` |

## 更新时的证据顺序

1. 读官方版本源和已固定上游源码，记录协议、客户端/patch版本、文件摘要。当前版本常量只是最近验收值，不是永远正确的配置。
2. 对照真实失败阶段：版本清单下载失败不能写成游戏认证拒绝；服务端明确拒绝不能当临时网络无限重试。HTTP错误码与响应结构可以记录，Cookie/token/JWT正文不能记录。
3. Native PE profile绑定已分析客户端和资源hash。缺少新版本证据时明确阻塞点，不能换几个参数盲试，也不能跨系列拼engine与patch。不要用重新注册身份处理此问题。
4. 协议升级先对照Fatalder/Gophertunnel相应源码与本地脱敏线上样本；若只拿到发布二进制且源码不足，再使用环境可用的逆向工具。不要把旧686参考实现直接当当前服务器配置。
5. 先独立验证编码/签名/压缩/有序分片与丢包恢复，再顺序线上验证。日志应有NetworkSettings、服务端加密握手、StartGame、PLAYER_SPAWN、初始化和只读探针。

## 已证实的易错点

- Bedrock带符号VarInt是ZigZag；raw DEFLATE不是zlib包装；JOSE ECDSA是r||s而非DER。
- secure OpenConnectionRequest2的cookie段还有1字节clientSupportsSecurity；漏掉会错位地址、MTU、GUID。
- UDP收到乱序可靠帧不能按“最高编号”丢弃；分片重组后仍需按通道有序投递。
- 接收缺口要NACK；发送重传应参考RTT。非阻塞UDP可取消，关闭要等待后台任务结束。
- 加密CTR及压缩字典有状态。解码失败不能继续吞包假装已恢复，重传不能重新加密同一游戏数据。
- 必须收到PLAYER_SPAWN后发SetLocalPlayerAsInitialised。
- 新Java调色板的iron_chain可精确归一为旧chain，必须保留axis等属性，并用三轴对照验证；不能按名称相似度把其他链条或新材质也猜成铁链。
- 旧slab复合名称可能被映射为现代单独名称；states决定材质与上下/双半砖，不能只替换名称而丢掉几何语义。
- 导入命令坐标必须锚定计划原点；fill整个范围都要在施工窗口内；新区域传送后需有界位置确认。

## 运行入口

```sh
python3 -m proloader doctor --json
python3 -m proloader plan path/to/building.schem --to=0,80,0 --json
python3 -m pytest -q
```

线上使用真实服务器、机器人和覆盖范围前先核对当前用户授权。`tools/diagnostics/verify_rental_stability.py` 从stdin读取密码；`--rounds 10`、hold至少30秒、间隔至少5秒，没有重新登录重试。不要把密码复制进命令文件、报告或skill示例。


## 同引擎补丁更新的实用路径

`pe_profiles.py`把已验证engine与(patch,resources_hash)配对，不能直接删掉校验或接受任意hash。
官方清单更新时，旧PE元数据不匹配会触发刷新；先用旧PE做一次只读查询确认是否确实code10，
避免把版本检查失败误写成Cookie/账号失效。

本机曾出现3.9.41→3.9.42导致的PE失败：engine仍3.9.15，硬绑定旧补丁在发POST前就抛错，
旧_refresh吞掉原因后只显示“机器人身份验证失败”。解决方法是下载新manifest/MCP，验证字节MD5，
确认RN实际状态，离线比较相关认证方法，再新增已验证元数据配对。
`PEMetadataMismatchError`与`PEAuthenticationError`应保留可脱敏的分类/数字码；
HTTP/超时与服务端拒绝分开显示，不输出请求内容或任意响应正文。

旧凭据先私有备份，认证成功才原子更新本机PE；失败保留旧文件。不要靠重新发短信解决。
涉及运行状态的身份重新验证也要持有BotStore.lease，避免刷新凭据打断施工连接。
