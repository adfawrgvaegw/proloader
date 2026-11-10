# 故障分流与收尾

## 先读什么

读REPORT.json的status/reason，相关日志先tail 80行。`error`的异常类型不是根因；需要保留本地阶段码和栈位置。不要为诊断打印请求、完整异常响应或凭据。stdout截断不等于命令失败，PTY返回session_id不等于完成；必须读取最终退出状态。

| 最后成功阶段/症状 | 定位与处理 |
|---|---|
| 公共下载HTTP/超时 | 检查当前具体URL、状态、代理是否可用；使用现有代理，不改全局网络。当前Android manifest GET对网络、408/429/选定5xx及无效ZIP最多3次请求，成功时记录metadata_retry；永久HTTP错误/无效JSON停止。公开重取与游戏首次连接分开计数，禁止缓存旧元数据兜底。旧泛化ValueError可能丢失根因，后续只报已证明的分类，保留失败。不要将网络失败描述为认证拒绝。 |
| manifest/RN/hash | 清单与下载字节数/MD5逐项核；坏cache隔离重取。404、响应格式变化、CDN不支持Range分开记录。不能以零hash或旧patch兜底。 |
| 缺Python依赖 | 项目专用venv装pyproject dev依赖，使用同一解释器跑脚本及pytest；不要改系统Python。LibreSSL warning自身不等于HTTP请求失败。 |
| Go/解析器/构建失败 | 核go版本、固定源码摘要、build.log；自定义marshal按固定decoder转换，xdis版本6.1.8。不要执行目标pyc。超时后查子进程和锁，别另起同一工作。 |
| engine_changed | 按adaptation.md提取新原生库、证书、call site和算法；Python方法不变不能推断原生参数不变。 |
| 模块整体变、四方法相同 | 按函数名递归diff，检查顶层/新native调用；审阅前不自动更新baseline。 |
| PEMetadataMismatchError | HTTP发送前本地拒绝，记录engine/patch，补准确配方；不是账号失效。 |
| PEAuthenticationError/code | 数字码、阶段和元数据摘要可记录。查message/库hash/签名参数/资源拼接/证书；不得排列参数在线盲试。 |
| PE旧metadata/code10 | 在身份lease内用已有Cookie刷新一次，成功原子更新；保留私有before。不是无条件多次刷新。 |
| Cookie格式/确实过期 | 先核本机登录材料是否存在；不暴露内容、不删除旧身份。确实需要新登录材料且无可用授权时才告知用户具体缺口。 |
| 世界enter code0但IP/port空 | 读取state/启动进度，有界等候；现实现60秒。没有地址不能转UDP，也不当密码错误。非零拒绝/HTTP失败不混入启动重试。 |
| Link/GameStart/AuthV2 | 对照runtime/client/link阶段，核签名/版本字段和顺序；PE成功不代表此段成功。JWT只私有保存或仅记结构/摘要。 |
| RakNet超时/MTU/security | 核offline/online握手、secure cookie后clientSupportsSecurity字节；不要动无关PE参数。当前服务器拒绝与本机UDP超时分开。 |
| NetworkSettings/Login拒绝 | 读状态码及当前协议codec。protocol与engine不是同一版本轴，不把旧交接中的2168/686/860固定当作当前版本；核CURRENT_RENTAL_PROTOCOL。 |
| 协商阶段reason35/OUTDATED_CLIENT | 先区分尚未收到NetworkSettings与Login/ClientData拒绝。保存空message事实，不猜协议号。读对应公开实现的固定提交及GameVersion、逐项核wire；仅修改协议号而遗漏Certificate/GameVersion不算适配。本轮898依据见docs/MAINTENANCE_20261111.md。UDP pong若无响应只记未知。 |
| 加密/解压/有序包 | raw DEFLATE、JOSE raw签名、CTR状态、有序通道/分片/NACK/RTT重传；解码失败不能吞掉继续，重传不能重复加密。 |
| StartGame后spawn卡住 | 必须PLAYER_SPAWN后发送SetLocalPlayerAsInitialised；记录各stage有界超时，不能提前标初始化。 |
| 31–44秒后reason90等断开 | 核动态MCP实际下发模板/盐/八字段返回/实体unique ID；长连60秒覆盖延迟校验。静态解码模板必须白名单，未知则分析，不回旧算法。 |
| reason55/KICKED类通用断开 | 先保留本次阶段、清理和失败序列；数字码不是具体原因。若原日志无文本，下次仅私有保留server_message用于定位，不公开任意文本，也不能将参考协议枚举当网易特定根因。独立复验不拼接，重复拒绝先分析，不无限刷连接。 |
| 接收卡住/断线后还发包 | 查首个transport关闭原因、唤醒收包和发送拒绝；不能靠无限重连遮住。 |
| probe只收到响应 | 检查实际probe语义与响应内容是否支持结论；现验证器只验证响应存在，报告不得夸大成全世界读回。 |
| tests失败 | 先比较失败node ID是否本轮引入；保留完整日志。已有坏mcstructure失败不可静默排除；人工有条件验收写清失败项、证据及范围。新相关失败必须修复。 |
| ONLINE失败/源码变动 | 停于第一失败轮。保留报告，修复后另起固定源码10轮；不能拼接成功轮或覆盖失败。 |
| 回滚冲突 | 比较before/candidate/当前文件；不覆盖其他维护者改动，不reset整个仓库。人工适配涉及多文件，按本轮before有针对性恢复。 |
| 超时/中断/用户停止 | 收取子进程状态；只停止本轮拥有的进程/连接，确认lease释放。报告partial而非accepted。 |

## 自动化与人工适配的边界

自动入口只自动回滚其修改的pe_profiles.py，不会把已成功刷新的PE恢复旧版，也不会恢复agent额外修改的client/codec/baseline。操作前保存每个本轮变更文件的before及PE私有备份。回滚不是删除cache或凭据。

已有身份/任务繁忙就不新连、不删除lock、不停用户施工；共享常驻假人需要按当前状态决定复用能力或等待，不开第二连接。历史菜单/建筑/起床战争与版本维护无关，不能恢复或修改。

发现SDK/库更新不代表应同步Fatalder。只在当前协议需要或用户请求时审阅相应upstream，保持第三方许可证与来源。不把上游连接和Python同身份并连。

## 每轮收尾

- 等验证器进程结束，读最终JSON/退出码；不能用日志中单个PASS当全轮成功。
- 检查每轮cleanup socket_closed/tasks_finished、pending/queued、最后BotStore.is_busy；租约空闲才报告已下线。维护没有施工时不创建/删除加载区。
- 核验前后及当前网络源码digest相同。文档修改不算网络源码修改。
- 失败与超时记录不删；新run必须另命名，保留实际顺序。
- 配置成功、基线审阅、离线测试、长连、10轮、开源发布分别记录；只做了哪些就说哪些。
- 文件权限：含PE/身份/私有诊断文件0600，敏感目录0700。不输出任意server响应，不把本机cache放公开包。

## 日期化记录格式

在`docs/MAINTENANCE_YYYYMMDD.md`记录（同日追加小节而不覆盖历史）：

1. 用户目标与授权；开始时dirty状态、当前accepted版本、身份/租约、保留任务。
2. 官方来源URL、获取时间/Last-Modified、正式/灰度、engine/patch/hash/MCP摘要；APK成员及证书验证限制。
3. 自动入口每次report、status/reason，后续agent为什么继续、具体分析文件/工具版本/来源/执行边界。
4. Python模块变化、native函数/调用点/常量/参数和独立向量；已知事实、推断、未确认部分分开。
5. 修改清单及before路径，旧版本兼容与拒绝边界。
6. 每次失败、超时、错误尝试和修正，保留原始report路径；不只写最终成功。
7. 定向/全量测试计数、确切失败node ID、已知排除、原生test/vet；不写“全绿”掩盖遗留失败。
8. 长连/10轮结果、时长、探针、0重连、源码摘要、退出/租约；只读维护不承诺建筑/NBT能力。
9. baseline是否推进、当前profile和PE状态、后续待办、是否发布；HANDOFF顶部链接此文。

复用经验写进skill对应reference，具体版本证据放case/日期文档。不能声称穷尽未来所有未知更新；新增未覆盖分支时补上触发条件、分析入口、判据和恢复路径。
